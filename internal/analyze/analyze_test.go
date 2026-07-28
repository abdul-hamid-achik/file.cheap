package analyze

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/abdul-hamid-achik/file.cheap/internal/manifest"
)

func makeContent(t *testing.T, root, id string, files map[string]string) string {
	t.Helper()
	stashDir := filepath.Join(root, id)
	content := filepath.Join(stashDir, "content")
	if err := os.MkdirAll(content, 0755); err != nil {
		t.Fatal(err)
	}
	for rel, c := range files {
		p := filepath.Join(content, rel)
		_ = os.MkdirAll(filepath.Dir(p), 0755)
		if err := os.WriteFile(p, []byte(c), 0644); err != nil {
			t.Fatal(err)
		}
	}
	man := manifest.New(id, content)
	if err := man.Save(stashDir); err != nil {
		t.Fatal(err)
	}
	return stashDir
}

type testEmbedder struct{}

func (testEmbedder) Embed(text string) ([]float32, error) {
	if strings.Contains(text, "weather") {
		return []float32{0, 1}, nil
	}
	return []float32{1, 0}, nil
}

func (e testEmbedder) EmbedBatch(texts []string) ([][]float32, error) {
	result := make([][]float32, 0, len(texts))
	for _, text := range texts {
		vector, err := e.Embed(text)
		if err != nil {
			return nil, err
		}
		result = append(result, vector)
	}
	return result, nil
}

func (testEmbedder) Dimension() int { return 2 }

func analyzerWithTestEmbedder(root string) *Analyzer {
	return &Analyzer{
		stashRoot: root,
		emb: EmbedderSettings{
			Provider: "test",
			Model:    "deterministic",
		},
		embCache: testEmbedder{},
		embTried: true,
	}
}

func TestIndexAndSearchPerFile(t *testing.T) {
	root := t.TempDir()
	an := NewAnalyzer(root, "")
	stashDir := makeContent(t, root, "s1", map[string]string{
		"logs/app.log": "login failure on retry when token expired",
		"notes.md":     "checkout crash report and weather notes",
	})

	idx, err := an.IndexStash(context.Background(), stashDir)
	if err != nil {
		t.Fatalf("IndexStash: %v", err)
	}
	if idx.FilesIndex < 2 {
		t.Errorf("FilesIndex = %d, want >= 2", idx.FilesIndex)
	}

	res, err := an.Search(context.Background(), "login", 0, "")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(res) == 0 {
		t.Fatal("no results for 'login'")
	}
	// The hit must name the exact file, not a blob.
	if res[0].File != "logs/app.log" {
		t.Errorf("File = %q, want logs/app.log", res[0].File)
	}
	if res[0].StashID != "s1" {
		t.Errorf("StashID = %q, want s1", res[0].StashID)
	}
}

func TestIndexAndSearchMonitorIncident(t *testing.T) {
	root := t.TempDir()
	an := NewAnalyzer(root, "")
	stashDir := makeContent(t, root, "monitor-incident", map[string]string{
		"manifest.json": `{
  "kind":"monitor.incident",
  "schema_version":"1",
  "trigger":"node memory leak",
  "alert":{"rule":"heap-growth"},
  "diagnosis":{"summary":"request objects remain reachable"},
  "context":{"component":"example-api"}
}`,
		"correlations.json": `{"matches":[{"fqn":"runtime.HandleRequest","func":"handleRequest","file":"src/server.ts","line":42}]}`,
		"semantic.json":     `{"hits":[{"file":"src/server.ts","symbol":"handleRequest","snippet":"retained request context"}]}`,
		"process.json":      `{"runtime":"nodejs","main_script":"server.mjs","codebase_root":"/workspace/example","name":"example-api"}`,
		"snapshot.json":     `{"payload":"not-indexed-monitor-snapshot-token"}`,
	})

	indexed, err := an.IndexStash(context.Background(), stashDir)
	if err != nil {
		t.Fatalf("IndexStash: %v", err)
	}
	if indexed.BundleType != "monitor.incident" {
		t.Fatalf("BundleType = %q, want monitor.incident", indexed.BundleType)
	}
	if indexed.FilesIndex != 4 {
		t.Fatalf("FilesIndex = %d, want four bounded Monitor projections", indexed.FilesIndex)
	}

	for query, wantFile := range map[string]string{
		"node memory leak":         "manifest.json",
		"runtime.HandleRequest":    "correlations.json",
		"retained request context": "semantic.json",
		"server.mjs":               "process.json",
	} {
		results, searchErr := an.Search(context.Background(), query, 10, "keyword")
		if searchErr != nil {
			t.Fatalf("Search(%q): %v", query, searchErr)
		}
		found := false
		for _, result := range results {
			if result.StashID == "monitor-incident" && result.File == wantFile {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Search(%q) did not find %s: %+v", query, wantFile, results)
		}
	}

	results, err := an.Search(context.Background(), "not-indexed-monitor-snapshot-token", 10, "keyword")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Fatalf("snapshot payload unexpectedly indexed: %+v", results)
	}
}

func TestSearchStashFilter(t *testing.T) {
	root := t.TempDir()
	an := NewAnalyzer(root, "")
	s1 := makeContent(t, root, "s1", map[string]string{"a.txt": "unique-token-alpha"})
	s2 := makeContent(t, root, "s2", map[string]string{"b.txt": "unique-token-alpha"})
	if _, err := an.IndexStash(context.Background(), s1); err != nil {
		t.Fatal(err)
	}
	if _, err := an.IndexStash(context.Background(), s2); err != nil {
		t.Fatal(err)
	}

	all, _ := an.Search(context.Background(), "unique-token-alpha", 0, "")
	if len(all) < 2 {
		t.Errorf("global search = %d, want >= 2", len(all))
	}

	limited, _ := an.Search(context.Background(), "unique-token-alpha", 1, "")
	if len(limited) != 1 {
		t.Errorf("limited search = %d, want 1", len(limited))
	}

	scoped, _ := an.SearchStash(context.Background(), s1, "unique-token-alpha", 0, "")
	for _, r := range scoped {
		if r.StashID != "s1" {
			t.Errorf("scoped result from %q, want only s1", r.StashID)
		}
	}
}

// TestHybridSemanticRecall verifies that hybrid search surfaces a semantically
// related document that keyword (BM25) search misses entirely. Skips when no
// ollama embedder is reachable (so CI without an embedder stays green).
func TestHybridSemanticRecall(t *testing.T) {
	root := t.TempDir()
	an := NewAnalyzer(root, "").WithEmbedder(EmbedderSettings{
		Provider: "ollama", Model: "nomic-embed-text", URL: "http://localhost:11434",
	})
	if _, err := an.CheckEmbedder(); err != nil {
		t.Skipf("ollama embedder not available: %v", err)
	}

	stashDir := makeContent(t, root, "s1", map[string]string{
		"auth.go":    "renews the session credential before it expires and retries on failure",
		"weather.go": "sunny skies with a gentle breeze across the coastal town",
	})
	if _, err := an.IndexStash(context.Background(), stashDir); err != nil {
		t.Fatal(err)
	}

	// Keyword-disjoint query ("refresh login token" shares no tokens with the docs).
	if kw, _ := an.Search(context.Background(), "refresh login token", 5, "keyword"); len(kw) != 0 {
		t.Logf("keyword search returned %d hits (expected 0 for a disjoint query)", len(kw))
	}

	hy, err := an.Search(context.Background(), "refresh login token", 5, "hybrid")
	if err != nil {
		t.Fatalf("hybrid search: %v", err)
	}
	if len(hy) == 0 || hy[0].File != "auth.go" {
		t.Errorf("hybrid top hit = %+v, want auth.go ranked first", hy)
	}
}

// TestEmbedderProfileDrift verifies that re-indexing with a different embedding
// model is rejected with a clear error (rather than a cryptic dimension error).
// Skips when no ollama embedder is reachable.
func TestEmbedderProfileDrift(t *testing.T) {
	root := t.TempDir()
	a1 := NewAnalyzer(root, "").WithEmbedder(EmbedderSettings{
		Provider: "ollama", Model: "nomic-embed-text", URL: "http://localhost:11434",
	})
	if _, err := a1.CheckEmbedder(); err != nil {
		t.Skipf("ollama embedder not available: %v", err)
	}
	stashDir := makeContent(t, root, "s1", map[string]string{"a.txt": "the auth module refreshes credentials"})
	if _, err := a1.IndexStash(context.Background(), stashDir); err != nil {
		t.Fatal(err)
	}

	// A different model against the same index must be rejected.
	a2 := NewAnalyzer(root, "").WithEmbedder(EmbedderSettings{
		Provider: "ollama", Model: "some-other-model", URL: "http://localhost:11434",
	})
	_, err := a2.IndexStash(context.Background(), stashDir)
	if err == nil || !strings.Contains(err.Error(), "embedding model changed") {
		t.Errorf("expected an 'embedding model changed' error, got: %v", err)
	}
}

// TestSearchFallbackToBM25WhenEmbedderEnabledAfterIndexing verifies that
// enabling an embedder after stashes were already indexed (into the BM25 "files"
// collection) does not make search silently return nothing.
func TestSearchFallbackToBM25WhenEmbedderEnabledAfterIndexing(t *testing.T) {
	root := t.TempDir()

	// Index WITHOUT an embedder -> docs land in the "files" collection.
	plain := NewAnalyzer(root, "")
	stashDir := makeContent(t, root, "s1", map[string]string{"a.txt": "unique-fallback-keyword here"})
	if _, err := plain.IndexStash(context.Background(), stashDir); err != nil {
		t.Fatal(err)
	}

	// Now search WITH an embedder configured (skip if unavailable). The vector
	// collection is empty, so it must fall back to BM25 rather than return nothing.
	withEmb := NewAnalyzer(root, "").WithEmbedder(EmbedderSettings{
		Provider: "ollama", Model: "nomic-embed-text", URL: "http://localhost:11434",
	})
	if _, err := withEmb.CheckEmbedder(); err != nil {
		t.Skipf("ollama embedder not available: %v", err)
	}
	res, err := withEmb.Search(context.Background(), "unique-fallback-keyword", 5, "hybrid")
	if err != nil {
		t.Fatal(err)
	}
	if len(res) == 0 || res[0].File != "a.txt" {
		t.Errorf("expected BM25 fallback to find a.txt, got %+v", res)
	}
	if res[0].Source != "keyword" {
		t.Errorf("fallback Source = %q, want keyword", res[0].Source)
	}
}

func TestMixedCollectionsBothContributeResults(t *testing.T) {
	root := t.TempDir()
	plain := NewAnalyzer(root, "")
	plainStash := makeContent(t, root, "plain", map[string]string{
		"plain.txt": "shared-token from the plain index",
	})
	if _, err := plain.IndexStash(context.Background(), plainStash); err != nil {
		t.Fatal(err)
	}

	withEmb := analyzerWithTestEmbedder(root)
	vectorStash := makeContent(t, root, "vector", map[string]string{
		"vector.txt": "shared-token from the vector index",
	})
	if _, err := withEmb.IndexStash(context.Background(), vectorStash); err != nil {
		t.Fatal(err)
	}

	results, err := withEmb.Search(context.Background(), "shared-token", 10, "hybrid")
	if err != nil {
		t.Fatal(err)
	}
	seen := make(map[string]bool)
	for _, result := range results {
		seen[result.StashID] = true
	}
	if !seen["plain"] || !seen["vector"] {
		t.Fatalf("mixed search omitted a collection: results=%+v", results)
	}
}

func TestKeywordSearchFindsVectorOnlyStashWithoutEmbedder(t *testing.T) {
	root := t.TempDir()
	withEmb := analyzerWithTestEmbedder(root)
	stashDir := makeContent(t, root, "vector", map[string]string{
		"vector.txt": "vector-only-keyword",
	})
	if _, err := withEmb.IndexStash(context.Background(), stashDir); err != nil {
		t.Fatal(err)
	}

	results, err := NewAnalyzer(root, "").Search(context.Background(), "vector-only-keyword", 5, "keyword")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].StashID != "vector" {
		t.Fatalf("keyword search did not inspect vector collection: %+v", results)
	}
}

func TestShortTextFileIsIndexed(t *testing.T) {
	root := t.TempDir()
	stashDir := makeContent(t, root, "tiny", map[string]string{"tiny.txt": "tiny"})
	result, err := NewAnalyzer(root, "").IndexStash(context.Background(), stashDir)
	if err != nil {
		t.Fatal(err)
	}
	if result.FilesIndex != 1 {
		t.Fatalf("FilesIndex = %d, want 1 for a printable 4-byte file", result.FilesIndex)
	}
}

func TestRemoteEmbeddingBlockedForSecretBearingStash(t *testing.T) {
	root := t.TempDir()
	stashDir := makeContent(t, root, "secret", map[string]string{
		"config.txt": "api_key = example",
	})
	man, err := manifest.Load(stashDir)
	if err != nil {
		t.Fatal(err)
	}
	if man.Custom == nil {
		man.Custom = make(map[string]string)
	}
	man.Custom["secrets_found"] = "1"
	if err := man.Save(stashDir); err != nil {
		t.Fatal(err)
	}

	an := NewAnalyzer(root, "").WithEmbedder(EmbedderSettings{
		Provider: "openai",
		Model:    "text-embedding-3-small",
	})
	_, err = an.IndexStash(context.Background(), stashDir)
	if err == nil || !strings.Contains(err.Error(), "remote embedding blocked") {
		t.Fatalf("IndexStash error = %v, want remote embedding policy error", err)
	}

	an.emb.AllowSecretContent = true
	if err := an.checkOutboundPolicy(stashDir); err != nil {
		t.Fatalf("explicit override was rejected: %v", err)
	}

	an.emb = EmbedderSettings{Provider: "ollama", URL: "http://127.0.0.1:11434"}
	if err := an.checkOutboundPolicy(stashDir); err != nil {
		t.Fatalf("loopback Ollama was treated as remote: %v", err)
	}
	an.emb.URL = "http://ollama.example.test:11434"
	if err := an.checkOutboundPolicy(stashDir); err == nil || !strings.Contains(err.Error(), "remote embedding blocked") {
		t.Fatalf("remote Ollama policy error = %v, want block", err)
	}
}

func TestSearchHidesExpiredAndCorruptStashes(t *testing.T) {
	root := t.TempDir()
	an := NewAnalyzer(root, "")
	expired := makeContent(t, root, "expired", map[string]string{"a.txt": "ghost-token"})
	corrupt := makeContent(t, root, "corrupt", map[string]string{"b.txt": "ghost-token"})
	if _, err := an.IndexStash(context.Background(), expired); err != nil {
		t.Fatal(err)
	}
	if _, err := an.IndexStash(context.Background(), corrupt); err != nil {
		t.Fatal(err)
	}

	man, err := manifest.Load(expired)
	if err != nil {
		t.Fatal(err)
	}
	man.ExpiresAt = time.Now().Add(-time.Minute).UTC().Format(time.RFC3339)
	if err := man.Save(expired); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(corrupt, "manifest.json"), []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}

	results, err := an.Search(context.Background(), "ghost-token", 10, "keyword")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Fatalf("search returned expired/corrupt ghost hits: %+v", results)
	}
}

func TestDropIndex(t *testing.T) {
	root := t.TempDir()
	an := NewAnalyzer(root, "")
	stashDir := makeContent(t, root, "s1", map[string]string{"a.txt": "droptest-keyword"})
	if _, err := an.IndexStash(context.Background(), stashDir); err != nil {
		t.Fatal(err)
	}
	if res, _ := an.Search(context.Background(), "droptest-keyword", 0, ""); len(res) == 0 {
		t.Fatal("expected a result before drop")
	}
	if err := an.DropIndex("s1"); err != nil {
		t.Fatalf("DropIndex: %v", err)
	}
	if res, _ := an.Search(context.Background(), "droptest-keyword", 0, ""); len(res) != 0 {
		t.Errorf("expected no results after drop, got %d", len(res))
	}
}

// TestSearchNotIndexedReturnsErrNotIndexed verifies that searching a stash root
// with no built index returns ErrNotIndexed (not a generic error), so callers
// can treat "not indexed" as empty data (exit 0) rather than a tool failure.
func TestSearchNotIndexedReturnsErrNotIndexed(t *testing.T) {
	root := t.TempDir()
	an := NewAnalyzer(root, "")
	_, err := an.Search(context.Background(), "anything", 0, "")
	if !errors.Is(err, ErrNotIndexed) {
		t.Fatalf("Search on unindexed root: err = %v, want ErrNotIndexed", err)
	}
}

// TestSearchStashNotIndexedReturnsErrNotIndexed verifies the stash-scoped path
// also surfaces ErrNotIndexed when the stash was never analyzed.
func TestSearchStashNotIndexedReturnsErrNotIndexed(t *testing.T) {
	root := t.TempDir()
	an := NewAnalyzer(root, "")
	stashDir := makeContent(t, root, "s1", map[string]string{"a.txt": "hello world"})
	_, err := an.SearchStash(context.Background(), stashDir, "hello", 0, "")
	if !errors.Is(err, ErrNotIndexed) {
		t.Fatalf("SearchStash on unindexed stash: err = %v, want ErrNotIndexed", err)
	}
}

// TestParseVecgrepJSONLineField verifies that connect matches carry a clean
// file path (no ":line" suffix) plus a separate integer line field, so callers
// can build a Location{File, StartLine} without splitting a string.
func TestParseVecgrepJSONLineField(t *testing.T) {
	in := []byte(`[{"file_path":"internal/auth.go","content":"func refreshToken","score":0.81,"start_line":42}]`)
	results := parseVecgrepJSON(in)
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	r := results[0]
	if r.File != "internal/auth.go" {
		t.Errorf("File = %q, want internal/auth.go (no :line suffix)", r.File)
	}
	if r.Line != 42 {
		t.Errorf("Line = %d, want 42", r.Line)
	}
	if r.StashID != "vecgrep" || r.Source != "vecgrep" {
		t.Errorf("StashID/Source = %q/%q, want vecgrep/vecgrep", r.StashID, r.Source)
	}
}

// TestParseVecgrepJSONNoLine verifies a match without a start_line yields a
// clean file and a zero (omittable) line.
func TestParseVecgrepJSONNoLine(t *testing.T) {
	in := []byte(`[{"file_path":"README.md","content":"docs","score":0.1,"start_line":0}]`)
	results := parseVecgrepJSON(in)
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0].File != "README.md" {
		t.Errorf("File = %q, want README.md", results[0].File)
	}
	if results[0].Line != 0 {
		t.Errorf("Line = %d, want 0", results[0].Line)
	}
}
