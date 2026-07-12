// Package cleanup provides heuristic analysis of stashes for automated cleanup.
//
// It scores each stash 0-100 on "droppability" using weighted signals:
//   - source path gone (+35)
//   - cache tool like codemap/vecgrep (+25)
//   - evidence tool like vidtrace/cairntrace (-30, protective)
//   - age > 90 days (+15)
//   - large size > 100MB (+10)
//   - keep tags (-50, protective)
//   - expired TTL (+40)
//   - content-hash dedup with newer stash (+20)
//
// Verdicts: drop (>=60), review (>=30), keep (<30).
// The Run function is a dry-run by default; Apply=true drops only high-confidence
// candidates (verdict=drop) via the normal Drop path.
package cleanup

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/abdul-hamid-achik/file.cheap/internal/stash"
)

// Options controls a cleanup run.
type Options struct {
	Apply    bool   // actually drop stashes scored as drop (default: dry-run)
	KeepTag  string // tag that exempts a stash from cleanup (default: keep)
	Tool     string // only analyze stashes from this tool (empty = all)
	Tag      string // only analyze stashes with this tag (empty = all)
	DropOnly bool   // only show stashes scored as drop (default: show all)
	Expired  bool   // include stashes with an expired TTL even if not yet swept
}

// Candidate is a single stash's cleanup analysis.
type Candidate struct {
	StashID   string   `json:"stash_id"`
	Name      string   `json:"name,omitempty"`
	Tool      string   `json:"tool,omitempty"`
	Tags      []string `json:"tags,omitempty"`
	Score     int      `json:"score"`
	Verdict   string   `json:"verdict"` // drop, review, keep
	Reasons   []string `json:"reasons"`
	Size      int64    `json:"size"`
	CreatedAt string   `json:"created_at"`
}

// Failure records a cleanup operation that could not be completed. Stage is
// "cancel", "drop", or "index".
type Failure struct {
	ID    string `json:"id"`
	Stage string `json:"stage"`
	Error string `json:"error"`
}

// Result is the outcome of a cleanup run.
type Result struct {
	Candidates []Candidate `json:"candidates"`
	Dropped    []string    `json:"dropped"`
	Skipped    []string    `json:"skipped"`
	Failed     []Failure   `json:"failed"`
	Reclaimed  int64       `json:"reclaimed"`
	Applied    bool        `json:"applied"`
}

// cacheTools are tools whose stashes are regenerable cache — safe to drop.
var cacheTools = map[string]bool{
	"codemap": true,
	"vecgrep": true,
}

// evidenceTools are tools whose stashes contain valuable evidence — protect.
var evidenceTools = map[string]bool{
	"vidtrace":   true,
	"cairntrace": true,
}

// Score thresholds for verdicts.
const (
	thresholdDrop   = 60
	thresholdReview = 30

	// Weights
	wSourceGone   = 35
	wCacheTool    = 25
	wEvidenceTool = -30
	wOldAge       = 15
	wLargeSize    = 10
	wKeepTag      = -50
	wExpiredTTL   = 40
	wContentDedup = 20

	oldAgeThreshold    = 90 * 24 * time.Hour // 90 days
	largeSizeThreshold = 100 * 1024 * 1024   // 100MB
)

// Run analyzes stashes and optionally drops high-confidence candidates.
// dropIndex (may be nil) removes search-index documents for each dropped stash.
func Run(ctx context.Context, mgr *stash.Manager, dropIndex func(id string) error, opts Options) (*Result, error) {
	if opts.KeepTag == "" {
		opts.KeepTag = "keep"
	}

	// List all stashes including expired ones (so we can report on them).
	stashes, err := mgr.ListFiltered(ctx, stash.ListOptions{
		Tool:           opts.Tool,
		Tag:            opts.Tag,
		IncludeExpired: true,
	})
	if err != nil {
		return nil, fmt.Errorf("list stashes: %w", err)
	}

	// Build a content-hash index for dedup detection: if two stashes have the
	// same content hash, the older one is a dedup candidate.
	hashIndex := make(map[string][]*stash.Stash) // hash -> stashes (any order)
	for _, st := range stashes {
		if st.Manifest.ContentHash != "" {
			hashIndex[st.Manifest.ContentHash] = append(hashIndex[st.Manifest.ContentHash], st)
		}
	}

	res := &Result{
		Applied:    opts.Apply,
		Candidates: []Candidate{},
		Dropped:    []string{},
		Skipped:    []string{},
		Failed:     []Failure{},
	}
	stashesByID := make(map[string]*stash.Stash, len(stashes))

	for _, st := range stashes {
		c := analyze(st, hashIndex, opts.KeepTag)

		// Filter: drop-only mode hides review/keep verdicts.
		if opts.DropOnly && c.Verdict != "drop" {
			continue
		}

		// Filter: expired-only mode (when not Expired, skip expired stashes unless
		// the caller wants them — but we already listed them; the Expired flag
		// means "include expired" so we don't filter them out here).
		// Actually: when Expired=false and the stash IS expired, we skip it
		// (expired stashes are handled by sweep, not cleanup, unless the caller
		// opts in).
		if !opts.Expired && stash.IsExpired(st.Manifest) {
			continue
		}

		res.Candidates = append(res.Candidates, c)
		stashesByID[c.StashID] = st
	}

	// Sort before applying so both the plan and operation order are stable.
	sort.Slice(res.Candidates, func(i, j int) bool {
		if res.Candidates[i].Score == res.Candidates[j].Score {
			return res.Candidates[i].StashID < res.Candidates[j].StashID
		}
		return res.Candidates[i].Score > res.Candidates[j].Score
	})

	// Apply only high-confidence candidates, after the complete plan has been
	// built. A canceled context stops every remaining destructive operation but
	// leaves an explicit machine-readable failure for each unattempted candidate.
	if opts.Apply {
		for _, c := range res.Candidates {
			if c.Verdict != "drop" {
				continue
			}
			st := stashesByID[c.StashID]
			if !safeToAutoDrop(st) {
				res.Skipped = append(res.Skipped, c.StashID)
				continue
			}
			if err := ctx.Err(); err != nil {
				res.Failed = append(res.Failed, Failure{ID: c.StashID, Stage: "cancel", Error: err.Error()})
				continue
			}
			if err := mgr.Drop(ctx, c.StashID); err != nil {
				slog.Warn("cleanup: failed to drop stash", "id", c.StashID, "err", err)
				res.Failed = append(res.Failed, Failure{ID: c.StashID, Stage: "drop", Error: err.Error()})
				continue
			}

			// The stash bytes are already gone at this point, so report the deletion
			// even when derived search-index cleanup fails.
			res.Dropped = append(res.Dropped, c.StashID)
			res.Reclaimed += c.Size
			if dropIndex != nil {
				if err := dropIndex(c.StashID); err != nil {
					res.Failed = append(res.Failed, Failure{ID: c.StashID, Stage: "index", Error: err.Error()})
				}
			}
		}
	}

	sort.Strings(res.Dropped)
	sort.Strings(res.Skipped)
	sort.Slice(res.Failed, func(i, j int) bool {
		if res.Failed[i].ID == res.Failed[j].ID {
			return res.Failed[i].Stage < res.Failed[j].Stage
		}
		return res.Failed[i].ID < res.Failed[j].ID
	})

	return res, nil
}

// safeToAutoDrop requires explicit lifecycle intent (an expired TTL) or a
// documented regenerable cache producer. A high heuristic score alone is not
// sufficient consent to delete agent evidence.
func safeToAutoDrop(st *stash.Stash) bool {
	if st == nil || st.Manifest == nil {
		return false
	}
	return stash.IsExpired(st.Manifest) || cacheTools[st.Manifest.Tool]
}

// analyze scores a single stash and returns its candidate.
func analyze(st *stash.Stash, hashIndex map[string][]*stash.Stash, keepTag string) Candidate {
	m := st.Manifest
	score := 0
	var reasons []string

	// Source path gone: +35 (strong signal the stash is orphaned).
	if m.SourcePath != "" {
		if _, err := os.Stat(m.SourcePath); os.IsNotExist(err) {
			score += wSourceGone
			reasons = append(reasons, fmt.Sprintf("source path gone (%s)", m.SourcePath))
		}
	}

	// Cache tool: +25 (regenerable, safe to drop).
	if cacheTools[m.Tool] {
		score += wCacheTool
		reasons = append(reasons, fmt.Sprintf("cache tool (%s)", m.Tool))
	}

	// Evidence tool: -30 (protective — these stashes are valuable).
	if evidenceTools[m.Tool] {
		score += wEvidenceTool
		reasons = append(reasons, fmt.Sprintf("evidence tool (%s, protected)", m.Tool))
	}

	// Old age: +15 (stale, likely irrelevant).
	if t, err := time.Parse(time.RFC3339, m.CreatedAt); err == nil {
		if time.Since(t) > oldAgeThreshold {
			score += wOldAge
			reasons = append(reasons, fmt.Sprintf("old (%.0fd)", time.Since(t).Hours()/24))
		}
	}

	// Large size: +10 (reclaiming space matters more for big stashes).
	if m.TotalSize >= largeSizeThreshold {
		score += wLargeSize
		reasons = append(reasons, fmt.Sprintf("large (%.1f MiB)", float64(m.TotalSize)/(1024*1024)))
	}

	// Keep tag: -50 (protective — user pinned this stash).
	if keepTag != "" && m.HasTag(keepTag) {
		score += wKeepTag
		reasons = append(reasons, fmt.Sprintf("has keep tag (%s)", keepTag))
	}

	// Expired TTL: +40 (already past its intended lifetime).
	if stash.IsExpired(m) {
		score += wExpiredTTL
		reasons = append(reasons, "TTL expired")
	}

	// Content-hash dedup: +20 (an older stash with the same content as a newer one).
	if m.ContentHash != "" {
		if dups := hashIndex[m.ContentHash]; len(dups) > 1 {
			// Check if this is NOT the newest one with this hash.
			newest := true
			for _, other := range dups {
				if other.Manifest.ID != m.ID && other.Manifest.CreatedAt > m.CreatedAt {
					newest = false
					break
				}
			}
			if !newest {
				score += wContentDedup
				reasons = append(reasons, "content-hash dup (newer copy exists)")
			}
		}
	}

	// Clamp to 0-100.
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}

	verdict := "keep"
	// Keep-tag is a hard floor: a stash bearing it is always "keep"
	// regardless of other signals. This is a safety net for pinned stashes.
	if keepTag != "" && m.HasTag(keepTag) {
		return Candidate{
			StashID:   m.ID,
			Name:      m.Name,
			Tool:      m.Tool,
			Tags:      m.Tags,
			Score:     score,
			Verdict:   "keep",
			Reasons:   reasons,
			Size:      m.TotalSize,
			CreatedAt: m.CreatedAt,
		}
	}
	if score >= thresholdDrop {
		verdict = "drop"
	} else if score >= thresholdReview {
		verdict = "review"
	}

	return Candidate{
		StashID:   m.ID,
		Name:      m.Name,
		Tool:      m.Tool,
		Tags:      m.Tags,
		Score:     score,
		Verdict:   verdict,
		Reasons:   reasons,
		Size:      m.TotalSize,
		CreatedAt: m.CreatedAt,
	}
}

// FormatReasons joins reasons into a human-readable string for CLI display.
func FormatReasons(reasons []string) string {
	return strings.Join(reasons, "; ")
}
