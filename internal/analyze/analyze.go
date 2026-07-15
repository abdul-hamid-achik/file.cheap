// Package analyze provides per-file search over stash content via veclite.
//
// Each readable file in a stash is indexed as a separate veclite document tagged
// with its stash ID and relative path, so search results point at the exact file
// that matched rather than a concatenated blob. Search is BM25 keyword by default;
// when an embedder (ollama/openai) is configured, documents also carry a vector,
// enabling semantic (vector) and hybrid (vector+BM25) search with graceful
// fallback to keyword. An optional vecgrep subprocess provides semantic code
// search over a separate codebase.
package analyze

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/abdul-hamid-achik/file.cheap/internal/compress"
	"github.com/abdul-hamid-achik/file.cheap/internal/detect"
	"github.com/abdul-hamid-achik/file.cheap/internal/fslock"
	"github.com/abdul-hamid-achik/file.cheap/internal/manifest"
	"github.com/abdul-hamid-achik/veclite"
)

// errEmbeddingDrift signals that the stored vector index was built with a
// different embedding model than the one now configured. Indexing treats this as
// fatal (you must rebuild), but search degrades to BM25 instead of failing.
var errEmbeddingDrift = errors.New("embedding model changed")

// ErrNotIndexed signals that no search index exists yet — the caller should
// treat this as an empty result (exit 0), not a tool failure. Search returns it
// when the veclite database has no indexed collection, so callers can distinguish
// "not indexed" from a real error.
var ErrNotIndexed = errors.New("no search index — run analyze first")

// veclite collection holding one document per indexed file.
const (
	// filesCollection holds one text-only (BM25) document per file.
	filesCollection = "files"
	// filesVecCollection holds dimensioned documents (vector + BM25) when an
	// embedder is configured, enabling semantic/hybrid search.
	filesVecCollection = "files_vec"
)

// maxIndexFileBytes caps how much of a single file is indexed for search.
const maxIndexFileBytes = 512 * 1024

// defaultSearchLimit bounds how many results a search returns.
const defaultSearchLimit = 20

// SearchResult is a single match from search or analysis.
type SearchResult struct {
	StashID string  `json:"stash_id"`
	Score   float64 `json:"score"`
	Text    string  `json:"text"`
	File    string  `json:"file,omitempty"`
	Line    int     `json:"line,omitempty"`
	Source  string  `json:"source,omitempty"`
}

// IndexResult summarizes an indexing run.
type IndexResult struct {
	StashID    string `json:"stash_id"`
	BundleType string `json:"bundle_type"`
	FilesIndex int    `json:"files_indexed"`
}

// EmbedderSettings configures an optional embedding model for semantic/hybrid
// search. A zero/empty Provider means BM25-only (no embedder).
type EmbedderSettings struct {
	Provider           string // "ollama" | "openai" | "" (none)
	Model              string
	URL                string // base URL (ollama)
	AllowSecretContent bool   // explicit opt-in for remote embedding of flagged stashes
}

// Enabled reports whether an embedder is configured.
func (e EmbedderSettings) Enabled() bool {
	return e.Provider != "" && e.Provider != "none"
}

// Analyzer provides indexing and search across stashes.
type Analyzer struct {
	stashRoot   string
	vecgrepPath string

	emb      EmbedderSettings
	embCache veclite.Embedder // lazily built; nil until first use
	embTried bool
}

// NewAnalyzer creates an Analyzer for the given stash root (BM25-only).
func NewAnalyzer(stashRoot, vecgrepPath string) *Analyzer {
	return &Analyzer{stashRoot: stashRoot, vecgrepPath: vecgrepPath}
}

// WithEmbedder configures an optional embedder for semantic/hybrid search and
// returns the analyzer for chaining. Callers that only need BM25 (drop/vacuum)
// can skip it.
func (a *Analyzer) WithEmbedder(e EmbedderSettings) *Analyzer {
	a.emb = e
	return a
}

// embedder lazily builds the configured embedder, caching the result. Returns
// nil when no embedder is configured or it cannot be constructed (BM25 fallback).
func (a *Analyzer) embedder() veclite.Embedder {
	if !a.emb.Enabled() {
		return nil
	}
	if a.embTried {
		return a.embCache
	}
	a.embTried = true

	cfg := veclite.EmbedderConfig{Provider: a.emb.Provider}
	switch a.emb.Provider {
	case "ollama":
		cfg.Ollama = veclite.OllamaConfig{BaseURL: a.emb.URL, Model: a.emb.Model}
	case "openai":
		cfg.OpenAI = veclite.OpenAIConfig{Model: a.emb.Model} // API key via env
	}
	e, err := veclite.NewEmbedderFromConfig(cfg)
	if err != nil {
		slog.Warn("embedder unavailable; falling back to BM25", "provider", a.emb.Provider, "err", err)
		return nil
	}
	a.embCache = e
	return e
}

// CheckEmbedder verifies the configured embedder is reachable, returning its
// embedding dimension. Returns (0, nil) when no embedder is configured.
func (a *Analyzer) CheckEmbedder() (int, error) {
	if !a.emb.Enabled() {
		return 0, nil
	}
	emb := a.embedder()
	if emb == nil {
		return 0, fmt.Errorf("embedder %q could not be constructed", a.emb.Provider)
	}
	v, err := emb.Embed("ping")
	if err != nil {
		return 0, err
	}
	return len(v), nil
}

// embProfile describes the active embedding pipeline, used to detect when the
// configured model no longer matches what an index was built with.
func (a *Analyzer) embProfile(emb veclite.Embedder) veclite.EmbeddingProfile {
	return veclite.EmbeddingProfile{
		Provider:  a.emb.Provider,
		Model:     a.emb.Model,
		Dimension: emb.Dimension(),
	}
}

// collFor returns the collection appropriate for the current embedder state:
// "files_vec" (dimensioned) when an embedder is active, else "files" (text-only).
// Both are always created with a text index so BM25 works regardless. For an
// existing vector collection it verifies the embedding model still matches.
func (a *Analyzer) collFor(db *veclite.DB, emb veclite.Embedder) (*veclite.Collection, error) {
	name := filesCollection
	var opts []veclite.CollectionOption
	var profile veclite.EmbeddingProfile
	if emb != nil {
		name = filesVecCollection
		profile = a.embProfile(emb)
		opts = []veclite.CollectionOption{veclite.WithDimension(emb.Dimension()), veclite.WithTextIndex("path"), veclite.WithEmbeddingProfile(profile)}
	} else {
		opts = []veclite.CollectionOption{veclite.WithTextIndex("path")}
	}

	if coll, err := db.GetCollection(name); err == nil && coll != nil {
		// Drift detection: a changed embedding model invalidates stored vectors.
		if emb != nil {
			if stored, ok := coll.EmbeddingProfile(); ok {
				if cerr := stored.Compatible(profile); cerr != nil {
					return nil, fmt.Errorf("%w (index built with %s/%s, now %s/%s): delete %s and re-run analyze — %v",
						errEmbeddingDrift, stored.Provider, stored.Model, profile.Provider, profile.Model, a.vecliteDBPath(), cerr)
				}
			}
		}
		return coll, nil
	}

	coll, err := db.CreateCollection(name, opts...)
	if err != nil {
		if coll, gerr := db.GetCollection(name); gerr == nil && coll != nil {
			return coll, nil
		}
		return nil, fmt.Errorf("create %s collection: %w", name, err)
	}
	return coll, nil
}

// collForReadOnly is like collFor but never creates a collection — it only
// returns an existing one. Used by the read-only search path so the DB can be
// opened with WithSharedRead.
func (a *Analyzer) collForReadOnly(db *veclite.DB, emb veclite.Embedder) (*veclite.Collection, error) {
	name := filesCollection
	var profile veclite.EmbeddingProfile
	if emb != nil {
		name = filesVecCollection
		profile = a.embProfile(emb)
	}

	coll, err := db.GetCollection(name)
	if err != nil || coll == nil {
		// When an embedder is configured but the vector collection doesn't
		// exist, the index was built without an embedder. Treat this as drift
		// so the caller falls back to keyword search on the text collection.
		if emb != nil {
			return nil, fmt.Errorf("%w: vector collection %q not found — run analyze with an embedder to build it",
				errEmbeddingDrift, filesVecCollection)
		}
		return nil, ErrNotIndexed
	}

	// Drift detection: a changed embedding model invalidates stored vectors.
	if emb != nil {
		if stored, ok := coll.EmbeddingProfile(); ok {
			if cerr := stored.Compatible(profile); cerr != nil {
				return nil, fmt.Errorf("%w (index built with %s/%s, now %s/%s): delete %s and re-run analyze — %v",
					errEmbeddingDrift, stored.Provider, stored.Model, profile.Provider, profile.Model, a.vecliteDBPath(), cerr)
			}
		}
	}
	return coll, nil
}

// vecliteDBPath returns the path to the shared veclite database.
func (a *Analyzer) vecliteDBPath() string {
	return filepath.Join(a.stashRoot, "fcheap.veclite")
}

// openDB opens (or creates) the veclite database with an exclusive lock.
func (a *Analyzer) openDB() (*veclite.DB, error) {
	return veclite.Open(a.vecliteDBPath())
}

// openDBReadOnly opens the veclite database lock-free (read-only, no flock),
// so multiple processes (or parallel MCP tool calls within the same process)
// can read the same database simultaneously without blocking each other or a
// concurrent writer. Readers see a point-in-time snapshot.
func (a *Analyzer) openDBReadOnly() (*veclite.DB, error) {
	return veclite.Open(a.vecliteDBPath(), veclite.WithReadOnly(true), veclite.WithSharedRead(true))
}

// dbLocks serializes veclite access per stash root within this process.
var dbLocks sync.Map // stashRoot -> *sync.Mutex

// lockDB acquires the per-root lock and returns the release func. veclite takes
// an exclusive, non-blocking file lock, so concurrent opens (e.g. parallel MCP
// tool calls, which each build a fresh Analyzer) would otherwise fail with a
// lock error; this makes the second caller wait instead. Callers must hold it
// for the whole open→use→close window: `unlock := a.lockDB(); defer unlock()`.
func (a *Analyzer) lockDB() func() {
	mu, _ := dbLocks.LoadOrStore(a.stashRoot, &sync.Mutex{})
	m := mu.(*sync.Mutex)
	m.Lock()
	return m.Unlock
}

// insertDoc inserts a document, attaching an embedding vector when an embedder
// is active (enabling semantic search); on embed failure it falls back to a
// text-only insert so the document stays BM25-searchable.
func (a *Analyzer) insertDoc(coll *veclite.Collection, emb veclite.Embedder, text string, payload map[string]any) error {
	if emb != nil {
		vec, err := emb.Embed(text)
		if err == nil {
			_, ierr := coll.InsertDocument(vec, text, payload)
			return ierr
		}
		slog.Warn("embed failed; indexing text-only", "path", payload["path"], "err", err)
	}
	_, err := coll.InsertTextDocument(text, payload)
	return err
}

// clearStash removes a stash's documents from both the text-only and vector
// collections, so re-indexing (including after toggling the embedder) never
// leaves stale documents behind.
func (a *Analyzer) clearStash(db *veclite.DB, stashID string) error {
	for _, name := range []string{filesCollection, filesVecCollection} {
		if coll, err := db.GetCollection(name); err == nil && coll != nil {
			if _, derr := coll.DeleteWhere(veclite.Equal("stash_id", stashID)); derr != nil {
				return derr
			}
		}
	}
	return nil
}

// ProgressFunc reports indexing progress (done out of total documents).
type ProgressFunc func(done, total int)

// IndexStash indexes every readable file in a stash into veclite, one document
// per file, replacing any previous index for that stash.
func (a *Analyzer) IndexStash(ctx context.Context, stashDir string) (*IndexResult, error) {
	return a.IndexStashWithProgress(ctx, stashDir, nil)
}

// IndexStashWithProgress is IndexStash with an optional progress callback,
// invoked after each document is indexed and once more at completion.
func (a *Analyzer) IndexStashWithProgress(ctx context.Context, stashDir string, progress ProgressFunc) (*IndexResult, error) {
	stashID := filepath.Base(stashDir)
	stashInfo, err := os.Lstat(stashDir)
	if err != nil {
		return nil, fmt.Errorf("lstat stash: %w", err)
	}
	if stashInfo.Mode()&os.ModeSymlink != 0 || !stashInfo.IsDir() {
		return nil, fmt.Errorf("stash path %q is not a real directory", stashDir)
	}
	man, err := manifest.Load(stashDir)
	if err != nil {
		return nil, fmt.Errorf("load stash manifest: %w", err)
	}
	if man.ID != stashID {
		return nil, fmt.Errorf("stash manifest ID %q does not match directory %q", man.ID, stashID)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	operationLock, err := fslock.Acquire(ctx, filepath.Join(stashDir, ".fcheap.lock"))
	if err != nil {
		return nil, err
	}
	defer operationLock.Release() //nolint:errcheck
	man, err = manifest.Load(stashDir)
	if err != nil {
		return nil, fmt.Errorf("reload stash manifest: %w", err)
	}
	if man.ID != stashID {
		return nil, fmt.Errorf("stash manifest ID %q does not match directory %q", man.ID, stashID)
	}
	if err := a.checkOutboundPolicy(stashDir); err != nil {
		return nil, err
	}

	contentDir, cleanup, err := readableContentDir(ctx, stashDir)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	result := detect.Detect(contentDir)

	unlock := a.lockDB()
	defer unlock()

	db, err := a.openDB()
	if err != nil {
		return nil, fmt.Errorf("open index: %w", err)
	}
	closed := false
	defer func() {
		if !closed {
			_ = db.Close()
		}
	}()

	emb := a.embedder()
	coll, err := a.collFor(db, emb)
	if err != nil {
		return nil, err
	}

	// Clear any previous documents for this stash (from either collection) so
	// re-indexing — including after toggling the embedder — is idempotent.
	if err := a.clearStash(db, stashID); err != nil {
		return nil, fmt.Errorf("clear old index: %w", err)
	}

	// Estimate the total documents for progress reporting.
	total := len(result.SearchableFiles)
	if len(result.Units) > 0 {
		total += len(result.Units)
	} else if result.Type != detect.TypeGeneric && strings.TrimSpace(result.SearchableText) != "" {
		total++
	}
	report := func(done int) {
		if progress != nil {
			progress(done, total)
		}
	}
	report(0)

	indexed := 0
	for _, rel := range result.SearchableFiles {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		text, ok := readTextFile(filepath.Join(contentDir, rel))
		if !ok {
			report(indexed)
			continue
		}
		payload := map[string]any{
			"stash_id":  stashID,
			"path":      rel,
			"file_type": strings.TrimPrefix(filepath.Ext(rel), "."),
		}
		if err := a.insertDoc(coll, emb, text, payload); err != nil {
			return nil, fmt.Errorf("index %s: %w", rel, err)
		}
		indexed++
		report(indexed)
	}

	// Structured units (e.g. one vidtrace timeline entry per frame+timestamp)
	// are indexed individually so a hit names the exact frame, not a blob.
	if len(result.Units) > 0 {
		for _, u := range result.Units {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			payload := map[string]any{
				"stash_id":  stashID,
				"path":      u.Label,
				"file_type": "entry",
			}
			if err := a.insertDoc(coll, emb, u.Text, payload); err != nil {
				return nil, fmt.Errorf("index unit %q: %w", u.Label, err)
			}
			indexed++
			report(indexed)
		}
	} else if result.Type != detect.TypeGeneric && strings.TrimSpace(result.SearchableText) != "" {
		// Fallback: index the synthesized text as a single derived document.
		payload := map[string]any{
			"stash_id":  stashID,
			"path":      string(result.Type) + ":derived",
			"file_type": "derived",
		}
		if err := a.insertDoc(coll, emb, result.SearchableText, payload); err != nil {
			return nil, fmt.Errorf("index derived text: %w", err)
		}
		indexed++
		report(indexed)
	}

	report(total) // snap to complete
	if err := db.Close(); err != nil {
		return nil, fmt.Errorf("persist index: %w", err)
	}
	closed = true

	// Only record indexing status after veclite has successfully persisted the
	// new generation. A close/sync failure must never leave a manifest claiming
	// that the stash is indexed when the durable index is incomplete.
	man, err = manifest.Load(stashDir)
	if err != nil {
		return nil, fmt.Errorf("reload manifest after indexing: %w", err)
	}
	if man.ID != stashID {
		return nil, fmt.Errorf("stash manifest ID %q does not match directory %q", man.ID, stashID)
	}
	if man.Custom == nil {
		man.Custom = make(map[string]string)
	}
	man.Custom["indexed"] = "true"
	man.Custom["indexed_files"] = fmt.Sprintf("%d", indexed)
	if err := man.Save(stashDir); err != nil {
		return nil, fmt.Errorf("save indexing status: %w", err)
	}

	slog.Debug("stash indexed", "id", stashID, "docs", indexed, "bundle", result.Type)
	return &IndexResult{
		StashID:    stashID,
		BundleType: string(result.Type),
		FilesIndex: indexed,
	}, nil
}

// checkOutboundPolicy prevents a configured remote embedder from receiving the
// contents of a stash that the save-time scanner flagged. Local loopback
// Ollama indexing is unaffected. Users must make a deliberate config choice to
// override this guard after reviewing the findings.
func (a *Analyzer) checkOutboundPolicy(stashDir string) error {
	if !embedderMayLeaveHost(a.emb) || a.emb.AllowSecretContent {
		return nil
	}
	man, err := manifest.Load(stashDir)
	if err != nil {
		return fmt.Errorf("verify stash manifest before remote embedding: %w", err)
	}
	if man.Custom == nil {
		return nil
	}
	count := man.Custom["secrets_found"]
	if count == "" || count == "0" {
		return nil
	}
	return fmt.Errorf("remote embedding blocked: stash %q contains %s potential secret(s); review them, use a loopback ollama_url, or explicitly enable allow_remote_secrets", man.ID, count)
}

func embedderMayLeaveHost(settings EmbedderSettings) bool {
	switch settings.Provider {
	case "", "none":
		return false
	case "openai":
		return true
	case "ollama":
		if strings.TrimSpace(settings.URL) == "" {
			return false // veclite's default Ollama endpoint is localhost
		}
		parsed, err := url.Parse(settings.URL)
		if err != nil || parsed.Hostname() == "" {
			return true // fail closed for malformed/non-URL endpoints
		}
		host := strings.ToLower(parsed.Hostname())
		if host == "localhost" {
			return false
		}
		ip := net.ParseIP(host)
		return ip == nil || !ip.IsLoopback()
	default:
		return true // unknown providers are conservatively treated as remote
	}
}

// Search runs a search across all indexed stashes. mode is "keyword",
// "semantic", "hybrid", or "" (auto: hybrid when an embedder is configured, else
// keyword). A limit <= 0 falls back to the default.
func (a *Analyzer) Search(ctx context.Context, query string, limit int, mode string) ([]SearchResult, error) {
	return a.search(ctx, query, nil, limit, mode)
}

// SearchStash searches within a single stash. See Search for mode/limit.
func (a *Analyzer) SearchStash(ctx context.Context, stashDir, query string, limit int, mode string) ([]SearchResult, error) {
	stashID := filepath.Base(stashDir)
	return a.search(ctx, query, veclite.Equal("stash_id", stashID), limit, mode)
}

// search runs a query with an optional payload filter. With an embedder it can
// do semantic (vector) or hybrid (vector+BM25) search, falling back to BM25
// keyword search when no embedder is available or the query embed fails.
func (a *Analyzer) search(ctx context.Context, query string, filter veclite.Filter, limit int, mode string) ([]SearchResult, error) {
	if strings.TrimSpace(query) == "" {
		return nil, nil
	}
	switch mode {
	case "", "keyword", "semantic", "hybrid":
		// ok — "" auto-resolves below based on whether an embedder is configured
	default:
		return nil, fmt.Errorf("unknown search mode %q (valid: keyword, semantic, hybrid)", mode)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = defaultSearchLimit
	}

	// Search is read-only: use a shared lock so parallel MCP tool calls and
	// concurrent processes can search without blocking each other or a writer.
	db, err := a.openDBReadOnly()
	if err != nil {
		return nil, fmt.Errorf("open index: %w", err)
	}
	defer db.Close() //nolint:errcheck

	emb := a.embedder()
	// Resolve mode: no embedder forces keyword; empty defaults to hybrid.
	if emb == nil {
		mode = "keyword"
	} else if mode == "" {
		mode = "hybrid"
	}

	opts := []veclite.SearchOption{veclite.TopK(limit)}
	if filter != nil {
		opts = append(opts, veclite.WithFilter(filter))
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	var hits []veclite.Result
	if mode == "keyword" {
		hits, err = textSearchCollections(db, query, limit, opts)
	} else {
		var coll *veclite.Collection
		coll, err = a.collForReadOnly(db, emb)
		if err != nil {
			if !errors.Is(err, errEmbeddingDrift) {
				return nil, err
			}
			// A missing or stale vector collection must not hide text-indexed
			// stashes. Keyword search spans both collections, including vector
			// collections left behind after embeddings are disabled.
			slog.Warn("vector index unavailable; using keyword search — re-run analyze to rebuild vectors", "err", err)
			mode = "keyword"
			hits, err = textSearchCollections(db, query, limit, opts)
		} else {
			qvec, eerr := emb.Embed(query)
			if eerr != nil {
				slog.Warn("query embed failed; falling back to keyword", "err", eerr)
				mode = "keyword"
				hits, err = textSearchCollections(db, query, limit, opts)
			} else {
				var vectorHits []veclite.Result
				if mode == "semantic" {
					vectorHits, err = coll.Search(qvec, opts...)
				} else {
					vectorHits, err = coll.HybridSearch(qvec, query, append(opts, veclite.WithVectorWeight(0.6), veclite.WithTextWeight(0.4))...)
				}
				if err == nil {
					// Stashes indexed without embeddings live in the plain collection.
					// Merge them on every query, not only when the vector side happens
					// to return zero hits, otherwise mixed vaults silently lose recall.
					var plainHits []veclite.Result
					if fc, ferr := db.GetCollection(filesCollection); ferr == nil && fc != nil {
						plainHits, err = fc.TextSearch(query, opts...)
					} else if ferr != nil && !errors.Is(ferr, veclite.ErrNotFound) {
						err = ferr
					}
					if err == nil {
						hits = fuseSearchResults(limit, vectorHits, plainHits)
						if len(vectorHits) == 0 && len(plainHits) > 0 {
							mode = "keyword"
						}
					}
				}
			}
		}
	}
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}

	results := make([]SearchResult, 0, len(hits))
	visibility := make(map[string]bool)
	now := time.Now()
	for _, h := range hits {
		if h.Record == nil {
			continue
		}
		stashID := payloadString(h.Record.Payload, "stash_id")
		visible, checked := visibility[stashID]
		if !checked {
			visible = a.stashSearchable(stashID, now)
			visibility[stashID] = visible
		}
		if !visible {
			continue
		}
		results = append(results, SearchResult{
			StashID: stashID,
			File:    payloadString(h.Record.Payload, "path"),
			Score:   float64(h.Score),
			Text:    extractSnippet(h.Record.Content, query, 200),
			Source:  mode,
		})
	}
	slog.Debug("search", "query", query, "mode", mode, "hits", len(results))
	return results, nil
}

// textSearchCollections searches both BM25-capable collections. A stash lives
// in exactly one collection after indexing, but a vault may contain a mix of
// plain and vector-backed stashes as embedding configuration changes over time.
func textSearchCollections(db *veclite.DB, query string, limit int, opts []veclite.SearchOption) ([]veclite.Result, error) {
	foundCollection := false
	sets := make([][]veclite.Result, 0, 2)
	for _, name := range []string{filesCollection, filesVecCollection} {
		coll, err := db.GetCollection(name)
		if errors.Is(err, veclite.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		foundCollection = true
		hits, err := coll.TextSearch(query, opts...)
		if err != nil {
			return nil, err
		}
		sets = append(sets, hits)
	}
	if !foundCollection {
		return nil, ErrNotIndexed
	}
	return fuseSearchResults(limit, sets...), nil
}

func fuseSearchResults(limit int, sets ...[]veclite.Result) []veclite.Result {
	nonEmpty := sets[:0]
	for _, set := range sets {
		if len(set) > 0 {
			nonEmpty = append(nonEmpty, set)
		}
	}
	switch len(nonEmpty) {
	case 0:
		return nil
	case 1:
		if len(nonEmpty[0]) > limit {
			return nonEmpty[0][:limit]
		}
		return nonEmpty[0]
	default:
		// Record IDs are collection-local, so veclite's ID-based fusion can
		// collapse unrelated documents whose IDs happen to match. Fuse by the
		// stable stash/path identity instead.
		type fusedResult struct {
			result veclite.Result
			score  float64
		}
		fused := make(map[string]*fusedResult)
		for _, set := range nonEmpty {
			for rank, result := range set {
				if result.Record == nil {
					continue
				}
				key := payloadString(result.Record.Payload, "stash_id") + "\x00" + payloadString(result.Record.Payload, "path")
				entry, ok := fused[key]
				if !ok {
					entry = &fusedResult{result: result}
					fused[key] = entry
				}
				entry.score += 1.0 / float64(60+rank+1)
			}
		}
		ranked := make([]*fusedResult, 0, len(fused))
		for _, entry := range fused {
			entry.result.Score = float32(entry.score)
			ranked = append(ranked, entry)
		}
		sort.SliceStable(ranked, func(i, j int) bool {
			return ranked[i].score > ranked[j].score
		})
		if len(ranked) > limit {
			ranked = ranked[:limit]
		}
		results := make([]veclite.Result, 0, len(ranked))
		for _, entry := range ranked {
			results = append(results, entry.result)
		}
		return results
	}
}

// stashSearchable keeps the search view consistent with the manifest catalog:
// deleted, corrupt, or expired stashes must not survive as ghost index hits.
func (a *Analyzer) stashSearchable(stashID string, now time.Time) bool {
	if stashID == "" || stashID == "." || stashID == ".." || filepath.Base(stashID) != stashID {
		return false
	}
	stashDir := filepath.Join(a.stashRoot, stashID)
	manifestPath := filepath.Join(stashDir, "manifest.json")
	if _, err := os.Stat(manifestPath); err != nil {
		return false
	}
	man, err := manifest.Load(stashDir)
	if err != nil || man.ID != stashID {
		return false
	}
	if man.ExpiresAt == "" {
		return true
	}
	expiresAt, err := time.Parse(time.RFC3339, man.ExpiresAt)
	return err == nil && now.Before(expiresAt)
}

// vecgrepBin resolves the vecgrep binary, preferring the configured path.
// Returns "" if vecgrep is not available.
func (a *Analyzer) vecgrepBin() string {
	if a.vecgrepPath != "" {
		return a.vecgrepPath
	}
	if path, err := exec.LookPath("vecgrep"); err == nil {
		return path
	}
	return ""
}

// parseVecgrepJSON parses vecgrep's `-f json` search output (a bare array or an
// object wrapping a `results` array) into SearchResults.
func parseVecgrepJSON(output []byte) []SearchResult {
	type vgRes struct {
		FilePath  string  `json:"file_path"`
		Content   string  `json:"content"`
		Score     float64 `json:"score"`
		StartLine int     `json:"start_line"`
	}
	var hits []vgRes
	if err := json.Unmarshal(output, &hits); err != nil {
		var wrapper struct {
			Results []vgRes `json:"results"`
		}
		if err := json.Unmarshal(output, &wrapper); err != nil {
			return nil
		}
		hits = wrapper.Results
	}

	results := make([]SearchResult, 0, len(hits))
	for _, r := range hits {
		results = append(results, SearchResult{
			StashID: "vecgrep",
			Score:   r.Score,
			Text:    r.Content,
			File:    r.FilePath,
			Line:    r.StartLine,
			Source:  "vecgrep",
		})
	}
	return results
}

// ConnectResult is the outcome of connecting a stash to a codebase.
type ConnectResult struct {
	StashID     string         `json:"stash_id"`
	Codebase    string         `json:"codebase"`
	Query       string         `json:"query"`
	Matches     []SearchResult `json:"matches"`
	IndexStatus string         `json:"index_status,omitempty"` // "indexed" or "missing"
}

// VecgrepResult is the raw outcome of a vecgrep codebase search: the matches
// and whether the codebase had a usable index. IndexStatus is "indexed" when
// vecgrep had a built index to search, or "missing" when the codebase was not
// initialized/indexed (so the caller can report an empty result, not an error).
type VecgrepResult struct {
	Matches     []SearchResult `json:"matches"`
	IndexStatus string         `json:"index_status"` // "indexed" | "missing"
}

// VecgrepSearchIn runs vecgrep search within a codebase directory, optionally
// (re)building the index first. This is the engine behind `fcheap connect`:
// point the stashed bug report's text at a codebase and rank related candidates.
//
// When doIndex is false and the codebase has no vecgrep index, it returns a
// VecgrepResult with IndexStatus "missing" and no matches (rather than an
// error), so callers can distinguish "not indexed" from a real failure.
func (a *Analyzer) VecgrepSearchIn(ctx context.Context, codebaseDir, query string, limit int, doIndex bool, mode string) (*VecgrepResult, error) {
	bin := a.vecgrepBin()
	if bin == "" {
		return nil, fmt.Errorf("vecgrep not found; install it or set vecgrep_path in config")
	}
	if info, err := os.Stat(codebaseDir); err != nil || !info.IsDir() {
		return nil, fmt.Errorf("codebase directory not found: %s", codebaseDir)
	}
	if limit <= 0 {
		limit = 10
	}

	if doIndex {
		// vecgrep is project-based: `init` creates the project (idempotent — a
		// re-init on an existing project is a harmless no-op we ignore), then
		// `index .` builds the searchable index (semantic embeddings require an
		// embedder, e.g. ollama + nomic-embed-text — vecgrep's defaults).
		initCmd := exec.CommandContext(ctx, bin, "init")
		initCmd.Dir = codebaseDir
		_, _ = initCmd.CombinedOutput()

		idx := exec.CommandContext(ctx, bin, "index", ".")
		idx.Dir = codebaseDir
		if out, err := idx.CombinedOutput(); err != nil {
			return nil, fmt.Errorf("vecgrep index failed: %v: %s", err, strings.TrimSpace(string(out)))
		}
	} else if !a.vecgrepIndexed(ctx, bin, codebaseDir) {
		// Fresh workspace: no index yet. Report an empty "missing" result rather
		// than erroring so callers treat it as data (exit 0), not a tool failure.
		return &VecgrepResult{Matches: []SearchResult{}, IndexStatus: "missing"}, nil
	}

	// vecgrep's default mode is "hybrid" (semantic + BM25). Pass -m only when the
	// caller specifies one.
	args := []string{"search", "-f", "json", "-n", fmt.Sprintf("%d", limit)}
	if mode != "" {
		args = append(args, "-m", mode)
	}
	args = append(args, query)

	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = codebaseDir
	var stderr strings.Builder
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		// Defensive: if the pre-flight check somehow missed (e.g. the project was
		// removed between the check and the search), vecgrep reports "not in a
		// vecgrep project". Treat that as a missing index, not a hard failure.
		if strings.Contains(stderr.String(), "not in a vecgrep project") {
			return &VecgrepResult{Matches: []SearchResult{}, IndexStatus: "missing"}, nil
		}
		return nil, fmt.Errorf("vecgrep search failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return &VecgrepResult{Matches: parseVecgrepJSON(output), IndexStatus: "indexed"}, nil
}

// vecgrepIndexed reports whether the codebase has a built vecgrep index with at
// least one indexed file. It runs `vecgrep status -f json`; an error or zero
// indexed files means the codebase is not searchable yet.
func (a *Analyzer) vecgrepIndexed(ctx context.Context, bin, codebaseDir string) bool {
	cmd := exec.CommandContext(ctx, bin, "status", "-f", "json")
	cmd.Dir = codebaseDir
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	var st struct {
		Stats struct {
			Files int `json:"files"`
		} `json:"stats"`
	}
	if err := json.Unmarshal(out, &st); err != nil {
		return false
	}
	return st.Stats.Files > 0
}

// StashQuery returns representative searchable text for a stash, suitable as a
// semantic query against a codebase. It transparently handles compressed
// stashes and truncates to maxLen runes.
func (a *Analyzer) StashQuery(stashDir string, maxLen int) (string, error) {
	return a.StashQueryContext(context.Background(), stashDir, maxLen)
}

// StashQueryContext is StashQuery with cancellation and per-stash mutation
// serialization.
func (a *Analyzer) StashQueryContext(ctx context.Context, stashDir string, maxLen int) (string, error) {
	stashID := filepath.Base(stashDir)
	info, err := os.Lstat(stashDir)
	if err != nil {
		return "", fmt.Errorf("lstat stash: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return "", fmt.Errorf("stash path %q is not a real directory", stashDir)
	}
	man, err := manifest.Load(stashDir)
	if err != nil {
		return "", fmt.Errorf("load stash manifest: %w", err)
	}
	if man.ID != stashID {
		return "", fmt.Errorf("stash manifest ID %q does not match directory %q", man.ID, stashID)
	}
	operationLock, err := fslock.Acquire(ctx, filepath.Join(stashDir, ".fcheap.lock"))
	if err != nil {
		return "", err
	}
	defer operationLock.Release() //nolint:errcheck

	contentDir, cleanup, err := readableContentDir(ctx, stashDir)
	if err != nil {
		return "", err
	}
	defer cleanup()

	result := detect.Detect(contentDir)
	text := strings.TrimSpace(result.SearchableText)
	if text == "" {
		var b strings.Builder
		for _, u := range result.Units {
			b.WriteString(u.Text)
			b.WriteString(" ")
		}
		text = strings.TrimSpace(b.String())
	}
	text = strings.Join(strings.Fields(text), " ") // collapse whitespace
	if maxLen > 0 {
		if r := []rune(text); len(r) > maxLen {
			text = string(r[:maxLen]) // truncate on a rune boundary, not mid-UTF-8
		}
	}
	return text, nil
}

// DropIndex removes all indexed documents for a stash from veclite.
func (a *Analyzer) DropIndex(stashID string) error {
	// Nothing was ever indexed — don't create an empty DB just to clear it.
	if _, err := os.Stat(a.vecliteDBPath()); errors.Is(err, os.ErrNotExist) {
		return nil
	}

	unlock := a.lockDB()
	defer unlock()

	db, err := a.openDB()
	if err != nil {
		return err
	}
	defer db.Close() //nolint:errcheck
	return a.clearStash(db, stashID)
}

// --- helpers ---

// readableContentDir returns a directory containing the stash's extracted files.
// For compressed stashes it extracts the archive to a temp directory and returns
// a cleanup function to remove it.
func readableContentDir(ctx context.Context, stashDir string) (dir string, cleanup func(), err error) {
	contentDir := filepath.Join(stashDir, "content")
	if info, statErr := os.Lstat(contentDir); statErr == nil && info.Mode()&os.ModeSymlink == 0 && info.IsDir() {
		return contentDir, func() {}, nil
	}
	for _, name := range []string{"content.tar.zst", "content.tar.gz", "content.tar"} {
		archive := filepath.Join(stashDir, name)
		if info, statErr := os.Lstat(archive); statErr == nil && info.Mode().IsRegular() {
			tmp, mkErr := os.MkdirTemp("", "fcheap-index-")
			if mkErr != nil {
				return "", func() {}, mkErr
			}
			if exErr := compress.ExtractContext(ctx, archive, tmp); exErr != nil {
				_ = os.RemoveAll(tmp)
				return "", func() {}, fmt.Errorf("extract for indexing: %w", exErr)
			}
			return tmp, func() { _ = os.RemoveAll(tmp) }, nil
		}
	}
	return "", func() {}, fmt.Errorf("stash has no content to index")
}

// readTextFile reads a file's content for indexing, skipping binary files and
// capping the amount read.
func readTextFile(path string) (string, bool) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return "", false
	}
	f, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer f.Close() //nolint:errcheck

	data, err := io.ReadAll(io.LimitReader(f, maxIndexFileBytes))
	if err != nil {
		return "", false
	}
	if !isPrintable(data) {
		return "", false
	}
	return string(data), true
}

// isPrintable reports whether data looks like text (no NUL bytes, mostly printable).
func isPrintable(data []byte) bool {
	if len(data) == 0 {
		return false
	}
	nonPrintable := 0
	for _, b := range data {
		if b == 0 {
			return false
		}
		if b < 32 && b != '\n' && b != '\r' && b != '\t' {
			nonPrintable++
		}
	}
	return nonPrintable*10 <= len(data)
}

func payloadString(payload map[string]any, key string) string {
	if payload == nil {
		return ""
	}
	if v, ok := payload[key].(string); ok {
		return v
	}
	return ""
}

// extractSnippet finds the query in text and returns surrounding context.
func extractSnippet(text, query string, maxLen int) string {
	lower := strings.ToLower(text)
	idx := strings.Index(lower, strings.ToLower(query))
	if idx < 0 {
		if terms := strings.Fields(query); len(terms) > 0 {
			idx = strings.Index(lower, strings.ToLower(terms[0]))
		}
	}
	if idx < 0 {
		if len(text) > maxLen {
			return strings.TrimSpace(text[:maxLen]) + "..."
		}
		return strings.TrimSpace(text)
	}

	start := idx - maxLen/2
	if start < 0 {
		start = 0
	}
	end := start + maxLen
	if end > len(text) {
		end = len(text)
	}
	snippet := strings.TrimSpace(text[start:end])
	if start > 0 {
		snippet = "..." + snippet
	}
	if end < len(text) {
		snippet += "..."
	}
	return snippet
}
