package stash

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/abdul-hamid-achik/file.cheap/internal/manifest"
)

// IsExpired reports whether a manifest has passed its expires_at timestamp.
// A manifest with no expires_at (empty string) never expires. A bad timestamp
// is treated as "not expired" — the safe default so a corrupt manifest never
// causes an accidental deletion.
func IsExpired(m *manifest.Manifest) bool {
	if m.ExpiresAt == "" {
		return false
	}
	t, err := time.Parse(time.RFC3339, m.ExpiresAt)
	if err != nil {
		return false
	}
	return time.Now().After(t)
}

// SweepResult reports what a sweep operation found and (when applied) dropped.
type SweepResult struct {
	Expired   []string `json:"expired"`             // IDs that are expired (would be / were dropped)
	Skipped   []string `json:"skipped,omitempty"`   // IDs with unreadable manifests (not dropped)
	Applied   bool     `json:"applied"`             // whether the expired stashes were actually dropped
	Reclaimed int64    `json:"reclaimed,omitempty"` // bytes reclaimed (only when Applied)
}

// SweepExpired finds stashes whose TTL has elapsed and, when apply is true,
// drops them via the normal Drop path (cleaning DB rows + search index). When
// apply is false it is a dry-run: it reports which stashes would be dropped
// without touching them. dropIndex (may be nil) removes search-index documents
// for each dropped stash, matching the vacuum pattern.
//
// keepTag exempts any stash bearing that tag from sweeping — it's a safety net
// so a user can pin a stash even if its TTL has passed. Empty keepTag means
// no exemption.
func (m *Manager) SweepExpired(ctx context.Context, apply bool, keepTag string, dropIndex func(id string) error) (*SweepResult, error) {
	entries, err := os.ReadDir(m.rootDir)
	if err != nil {
		return nil, fmt.Errorf("read stash root: %w", err)
	}

	res := &SweepResult{Applied: apply}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		stashDir := filepath.Join(m.rootDir, entry.Name())
		man, err := manifest.Load(stashDir)
		if err != nil {
			// Unreadable manifest: skip, don't drop — we can't confirm it's expired.
			res.Skipped = append(res.Skipped, entry.Name())
			continue
		}
		if !IsExpired(man) {
			continue
		}
		// Safety net: a stash with the keep tag is never swept, even if expired.
		if keepTag != "" && man.HasTag(keepTag) {
			continue
		}
		res.Expired = append(res.Expired, man.ID)

		if !apply {
			continue // dry-run: just report
		}

		// Drop via the normal path so DB row + search index are cleaned.
		if err := m.Drop(ctx, man.ID); err != nil {
			slog.Warn("sweep: failed to drop expired stash", "id", man.ID, "err", err)
			continue
		}
		if dropIndex != nil {
			_ = dropIndex(man.ID)
		}
		res.Reclaimed += man.TotalSize
	}

	sort.Strings(res.Expired)
	sort.Strings(res.Skipped)
	return res, nil
}

// SetExpiry sets or clears the TTL on an existing stash. A ttl of "" clears
// the expiry (makes the stash permanent). The expiry is computed from the
// manifest's CreatedAt + ttl, so it's deterministic regardless of when the
// TTL is set. The manifest is saved atomically and the DB row updated.
func (m *Manager) SetExpiry(ctx context.Context, id, ttl string) error {
	if !validStashID(id) {
		return fmt.Errorf("invalid stash id %q", id)
	}
	stashDir := m.StashDir(id)
	man, err := manifest.Load(stashDir)
	if err != nil {
		return fmt.Errorf("load manifest: %w", err)
	}

	if ttl == "" {
		man.ExpiresAt = "" // clear expiry
	} else {
		d, err := ParseTTL(ttl)
		if err != nil {
			return fmt.Errorf("invalid ttl: %w", err)
		}
		created, err := time.Parse(time.RFC3339, man.CreatedAt)
		if err != nil {
			return fmt.Errorf("parse created_at: %w", err)
		}
		man.ExpiresAt = created.Add(d).UTC().Format(time.RFC3339)
	}

	if err := man.Save(stashDir); err != nil {
		return fmt.Errorf("save manifest: %w", err)
	}
	m.syncToDB(ctx, man)
	slog.Debug("stash expiry set", "id", id, "ttl", ttl, "expires_at", man.ExpiresAt)
	return nil
}
