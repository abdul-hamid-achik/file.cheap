package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/abdul-hamid-achik/file.cheap/internal/analyze"
	cleanupdomain "github.com/abdul-hamid-achik/file.cheap/internal/cleanup"
	"github.com/abdul-hamid-achik/file.cheap/internal/fcheap/config"
	"github.com/abdul-hamid-achik/file.cheap/internal/fcheap/output"
	"github.com/abdul-hamid-achik/file.cheap/internal/manifest"
	"github.com/abdul-hamid-achik/file.cheap/internal/stash"
)

func TestSmartCleanupJSONAppliesAndReportsActualResult(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	vault := filepath.Join(root, "vault")
	mgr, err := stash.NewManager(vault)
	if err != nil {
		t.Fatal(err)
	}

	dropSource := filepath.Join(root, "drop.txt")
	keepSource := filepath.Join(root, "keep.txt")
	if err := os.WriteFile(dropSource, []byte("drop me"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keepSource, []byte("keep me"), 0600); err != nil {
		t.Fatal(err)
	}
	dropStash, err := mgr.Save(ctx, &stash.SaveOptions{SourcePath: dropSource, Name: "drop-candidate"})
	if err != nil {
		t.Fatal(err)
	}
	keepStash, err := mgr.Save(ctx, &stash.SaveOptions{
		SourcePath: keepSource,
		Name:       "keep-candidate",
		Tags:       []string{"keep"},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{dropStash.Manifest.ID, keepStash.Manifest.ID} {
		if err := mgr.SetExpiry(ctx, id, "-1h"); err != nil {
			t.Fatalf("expire %s: %v", id, err)
		}
	}

	oldCfg, oldPrinter := cfg, printer
	oldApply, oldKeepTag := cleanupApply, cleanupKeepTag
	oldCategories := cleanupCategories
	t.Cleanup(func() {
		cfg, printer = oldCfg, oldPrinter
		cleanupApply, cleanupKeepTag = oldApply, oldKeepTag
		cleanupCategories = oldCategories
	})

	var stdout bytes.Buffer
	cfg = &config.Config{StashDir: vault}
	printer = output.New(output.WithJSON(true), output.WithOutput(&stdout), output.WithNoColor(true))
	cleanupApply = false
	cleanupKeepTag = "keep"
	cleanupCategories = []string{"expired"}

	if err := runSmartCleanup(mgr); err != nil {
		t.Fatalf("runSmartCleanup dry-run: %v", err)
	}
	var got smartCleanupOutput
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("decode JSON %q: %v", stdout.String(), err)
	}
	if got.Applied || len(got.Dropped) != 0 || !mgr.Exists(dropStash.Manifest.ID) || !mgr.Exists(keepStash.Manifest.ID) {
		t.Fatalf("dry-run changed state: output=%+v drop_exists=%t keep_exists=%t",
			got, mgr.Exists(dropStash.Manifest.ID), mgr.Exists(keepStash.Manifest.ID))
	}

	stdout.Reset()
	printer = output.New(output.WithJSON(true), output.WithOutput(&stdout), output.WithNoColor(true))
	cleanupApply = true
	if err := runSmartCleanup(mgr); err != nil {
		t.Fatalf("runSmartCleanup apply: %v", err)
	}
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("decode applied JSON %q: %v", stdout.String(), err)
	}
	if !got.Applied {
		t.Fatal("applied = false, want true")
	}
	if len(got.Dropped) != 1 || got.Dropped[0] != dropStash.Manifest.ID {
		t.Fatalf("dropped = %v, want [%s]", got.Dropped, dropStash.Manifest.ID)
	}
	if got.Reclaimed != dropStash.Manifest.TotalSize {
		t.Fatalf("reclaimed = %d, want %d", got.Reclaimed, dropStash.Manifest.TotalSize)
	}
	if len(got.Skipped) != 1 || got.Skipped[0].ID != keepStash.Manifest.ID {
		t.Fatalf("skipped = %+v, want protected %s", got.Skipped, keepStash.Manifest.ID)
	}
	if len(got.Failed) != 0 {
		t.Fatalf("failed = %+v, want none", got.Failed)
	}
	if mgr.Exists(dropStash.Manifest.ID) {
		t.Fatalf("dropped stash %s still exists", dropStash.Manifest.ID)
	}
	if !mgr.Exists(keepStash.Manifest.ID) {
		t.Fatalf("keep-tagged stash %s was dropped", keepStash.Manifest.ID)
	}
}

func TestSmartCleanupAutoApplyProtectsEvidenceWithMissingSource(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	mgr, err := stash.NewManager(filepath.Join(root, "vault"))
	if err != nil {
		t.Fatal(err)
	}

	evidenceSource := filepath.Join(root, "evidence")
	cacheSource := filepath.Join(root, "cache")
	for _, source := range []string{evidenceSource, cacheSource} {
		if err := os.MkdirAll(source, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(source, "result.txt"), []byte("payload"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	evidence, err := mgr.Save(ctx, &stash.SaveOptions{SourcePath: evidenceSource, Tool: "vidtrace"})
	if err != nil {
		t.Fatal(err)
	}
	cache, err := mgr.Save(ctx, &stash.SaveOptions{SourcePath: cacheSource, Tool: "codemap"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(evidenceSource); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(cacheSource); err != nil {
		t.Fatal(err)
	}

	plan, err := mgr.AnalyzeCleanup(ctx, stash.CleanupOptions{Categories: []string{"orphaned"}})
	if err != nil {
		t.Fatal(err)
	}
	result := applySmartCleanup(ctx, mgr, plan, "keep", nil)
	if !mgr.Exists(evidence.Manifest.ID) {
		t.Fatal("evidence stash with a missing source was auto-deleted")
	}
	if mgr.Exists(cache.Manifest.ID) {
		t.Fatal("regenerable cache stash was not deleted")
	}
	if len(result.Dropped) != 1 || result.Dropped[0] != cache.Manifest.ID {
		t.Fatalf("dropped = %+v, want cache %s", result.Dropped, cache.Manifest.ID)
	}
	if len(result.Skipped) != 1 || result.Skipped[0].ID != evidence.Manifest.ID {
		t.Fatalf("skipped = %+v, want evidence %s", result.Skipped, evidence.Manifest.ID)
	}
}

func TestRestoreMismatchPrintsJSONThenFailsUnlessAllowed(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	vault := filepath.Join(root, "vault")
	mgr, err := stash.NewManager(vault)
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "source.txt")
	if err := os.WriteFile(source, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	st, err := mgr.Save(ctx, &stash.SaveOptions{SourcePath: source, Name: "restore-contract"})
	if err != nil {
		t.Fatal(err)
	}
	stored := filepath.Join(st.Dir, "content", filepath.Base(source))
	if err := os.WriteFile(stored, []byte("tampered"), 0600); err != nil {
		t.Fatal(err)
	}

	oldCfg, oldPrinter := cfg, printer
	oldTarget, oldAllow := restoreTarget, restoreAllowMismatch
	t.Cleanup(func() {
		cfg, printer = oldCfg, oldPrinter
		restoreTarget, restoreAllowMismatch = oldTarget, oldAllow
	})

	var stdout bytes.Buffer
	cfg = &config.Config{StashDir: vault}
	printer = output.New(output.WithJSON(true), output.WithOutput(&stdout), output.WithNoColor(true))
	restoreTarget = filepath.Join(root, "restore-default")
	restoreAllowMismatch = false

	err = restoreCmd.RunE(restoreCmd, []string{st.Manifest.ID})
	if err == nil || !strings.Contains(err.Error(), "verification failed") {
		t.Fatalf("restore error = %v, want verification failure", err)
	}
	var got restoreOutput
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("decode JSON %q: %v", stdout.String(), err)
	}
	if got.Verified || len(got.Mismatches) == 0 {
		t.Fatalf("restore output = %+v, want mismatch details", got)
	}
	if got.Status != "restored_with_mismatches" {
		t.Fatalf("restore status = %q, want restored_with_mismatches", got.Status)
	}

	stdout.Reset()
	printer = output.New(output.WithJSON(true), output.WithOutput(&stdout), output.WithNoColor(true))
	restoreTarget = filepath.Join(root, "restore-allowed")
	restoreAllowMismatch = true
	if err := restoreCmd.RunE(restoreCmd, []string{st.Manifest.ID}); err != nil {
		t.Fatalf("restore --allow-mismatch: %v", err)
	}
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("decode allowed JSON %q: %v", stdout.String(), err)
	}
	if got.Verified {
		t.Fatal("allowed mismatch unexpectedly reported verified=true")
	}
	if got.Status != "restored_with_mismatches" {
		t.Fatalf("allowed restore status = %q, want restored_with_mismatches", got.Status)
	}
}

func TestConfigInitAndSetJSONContracts(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, "data"))

	oldCfg, oldPrinter := cfg, printer
	oldForce := configInitForce
	t.Cleanup(func() {
		cfg, printer = oldCfg, oldPrinter
		configInitForce = oldForce
	})

	var stdout bytes.Buffer
	printer = output.New(output.WithJSON(true), output.WithOutput(&stdout), output.WithNoColor(true))
	configInitForce = false
	if err := configInitCmd.RunE(configInitCmd, nil); err != nil {
		t.Fatalf("config init: %v", err)
	}
	var initOut configInitOutput
	if err := json.Unmarshal(stdout.Bytes(), &initOut); err != nil {
		t.Fatalf("decode init JSON %q: %v", stdout.String(), err)
	}
	if initOut.Status != "created" || !initOut.Changed {
		t.Fatalf("init output = %+v", initOut)
	}
	data, err := os.ReadFile(initOut.Path)
	if err != nil {
		t.Fatal(err)
	}
	yamlText := string(data)
	if !strings.Contains(yamlText, "allow_remote_secrets: false") ||
		!strings.Contains(yamlText, "default_ttl:") || !strings.Contains(yamlText, "ttl_rules: {}") {
		t.Fatalf("initial config omits TTL policy fields:\n%s", yamlText)
	}

	stdout.Reset()
	printer = output.New(output.WithJSON(true), output.WithOutput(&stdout), output.WithNoColor(true))
	err = configInitCmd.RunE(configInitCmd, nil)
	if err == nil {
		t.Fatal("second config init without --force succeeded")
	}
	if err := json.Unmarshal(stdout.Bytes(), &initOut); err != nil {
		t.Fatalf("decode exists JSON %q: %v", stdout.String(), err)
	}
	if initOut.Status != "exists" || initOut.Changed {
		t.Fatalf("existing init output = %+v", initOut)
	}

	stdout.Reset()
	printer = output.New(output.WithJSON(true), output.WithOutput(&stdout), output.WithNoColor(true))
	configInitForce = true
	if err := configInitCmd.RunE(configInitCmd, nil); err != nil {
		t.Fatalf("config init --force: %v", err)
	}
	if err := json.Unmarshal(stdout.Bytes(), &initOut); err != nil {
		t.Fatalf("decode overwrite JSON %q: %v", stdout.String(), err)
	}
	if initOut.Status != "overwritten" || !initOut.Changed {
		t.Fatalf("overwrite init output = %+v", initOut)
	}
	configInitForce = false

	stdout.Reset()
	printer = output.New(output.WithJSON(true), output.WithOutput(&stdout), output.WithNoColor(true))
	if err := configSetCmd.RunE(configSetCmd, []string{"default_ttl", "14d"}); err != nil {
		t.Fatalf("config set: %v", err)
	}
	var setOut configSetOutput
	if err := json.Unmarshal(stdout.Bytes(), &setOut); err != nil {
		t.Fatalf("decode set JSON %q: %v", stdout.String(), err)
	}
	if setOut.Status != "updated" || setOut.Key != "default_ttl" || setOut.Value != "14d" {
		t.Fatalf("set output = %+v", setOut)
	}
	diskCfg, err := config.LoadFromDisk()
	if err != nil {
		t.Fatal(err)
	}
	if diskCfg.DefaultTTL != "14d" {
		t.Fatalf("default_ttl = %q, want 14d", diskCfg.DefaultTTL)
	}

	stdout.Reset()
	printer = output.New(output.WithJSON(true), output.WithOutput(&stdout), output.WithNoColor(true))
	if err := configSetCmd.RunE(configSetCmd, []string{"allow_remote_secrets", "true"}); err != nil {
		t.Fatalf("config set allow_remote_secrets: %v", err)
	}
	if err := json.Unmarshal(stdout.Bytes(), &setOut); err != nil {
		t.Fatalf("decode privacy set JSON %q: %v", stdout.String(), err)
	}
	if !setOut.Config.AllowRemoteSecrets {
		t.Fatalf("set output config = %+v, want allow_remote_secrets=true", setOut.Config)
	}
	diskCfg, err = config.LoadFromDisk()
	if err != nil {
		t.Fatal(err)
	}
	if !diskCfg.AllowRemoteSecrets {
		t.Fatal("allow_remote_secrets was not persisted")
	}

	stdout.Reset()
	cfg = diskCfg
	printer = output.New(output.WithJSON(true), output.WithOutput(&stdout), output.WithNoColor(true))
	if err := configGetCmd.RunE(configGetCmd, []string{"allow_remote_secrets"}); err != nil {
		t.Fatalf("config get allow_remote_secrets: %v", err)
	}
	var getOut map[string]string
	if err := json.Unmarshal(stdout.Bytes(), &getOut); err != nil {
		t.Fatalf("decode privacy get JSON %q: %v", stdout.String(), err)
	}
	if getOut["allow_remote_secrets"] != "true" {
		t.Fatalf("allow_remote_secrets get = %q, want true", getOut["allow_remote_secrets"])
	}
}

func TestBuildEcosystemStatusUsesStableJSONFieldsAndSeconds(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	stashes := []*stash.Stash{
		{Manifest: &manifest.Manifest{
			ID:             "one",
			Tool:           "vidtrace",
			TotalSize:      10,
			Compression:    "zstd",
			CompressedSize: 4,
			CreatedAt:      now.Add(-48 * time.Hour).Format(time.RFC3339),
			ExpiresAt:      now.Add(-time.Hour).Format(time.RFC3339),
		}},
		{Manifest: &manifest.Manifest{
			ID:        "two",
			TotalSize: 20,
			CreatedAt: now.Add(-24 * time.Hour).Format(time.RFC3339),
		}},
	}
	cleanupResult := &stash.CleanupResult{
		Recommendations: []stash.CleanupRecommendation{{
			ID: "one", Tool: "vidtrace", Category: stash.CatOrphaned,
		}},
		ByCategory:  map[stash.CleanupCategory]int{stash.CatOrphaned: 1},
		Reclaimable: 10,
	}

	got := buildEcosystemStatus(stashes, cleanupResult, now)
	if got.Tools["vidtrace"].OldestAgeSeconds != int64((48*time.Hour)/time.Second) {
		t.Fatalf("oldest_age_seconds = %d", got.Tools["vidtrace"].OldestAgeSeconds)
	}
	if got.Tools["vidtrace"].TotalSize != 10 || got.Tools["vidtrace"].Orphaned != 1 {
		t.Fatalf("vidtrace stats = %+v", got.Tools["vidtrace"])
	}
	if got.Overall.TotalSize != 30 || got.Overall.ReclaimableSize != 10 {
		t.Fatalf("overall = %+v", got.Overall)
	}
	if got.Overall.LogicalSize != 30 || got.Overall.StoredSize != 24 || got.Overall.DiskUsage != "24 B" {
		t.Fatalf("logical/stored size split = %+v", got.Overall)
	}

	data, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	jsonText := string(data)
	for _, key := range []string{"\"total_size\"", "\"oldest_age_seconds\"", "\"reclaimable_size\""} {
		if !strings.Contains(jsonText, key) {
			t.Fatalf("JSON %s missing %s", jsonText, key)
		}
	}
	if strings.Contains(jsonText, "OldestAge") {
		t.Fatalf("JSON leaked Go field names/raw Duration: %s", jsonText)
	}
}

func TestScoringCleanupJSONReportsIndexFailureThenReturnsError(t *testing.T) {
	root := t.TempDir()
	vault := filepath.Join(root, "vault")
	source := filepath.Join(root, "cache")
	if err := os.MkdirAll(source, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "cache.txt"), []byte("cache"), 0600); err != nil {
		t.Fatal(err)
	}
	mgr, err := stash.NewManager(vault)
	if err != nil {
		t.Fatal(err)
	}
	st, err := mgr.Save(context.Background(), &stash.SaveOptions{SourcePath: source, Tool: "codemap"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(source); err != nil {
		t.Fatal(err)
	}

	oldCfg, oldPrinter, oldRootCtx := cfg, printer, rootCtx
	oldApply, oldKeepTag := cleanupApply, cleanupKeepTag
	oldTool, oldTag, oldDropOnly, oldExpired := cleanupTool, cleanupTag, cleanupDropOnly, cleanupExpired
	oldDropper := cleanupIndexDropper
	t.Cleanup(func() {
		cfg, printer, rootCtx = oldCfg, oldPrinter, oldRootCtx
		cleanupApply, cleanupKeepTag = oldApply, oldKeepTag
		cleanupTool, cleanupTag, cleanupDropOnly, cleanupExpired = oldTool, oldTag, oldDropOnly, oldExpired
		cleanupIndexDropper = oldDropper
	})

	var stdout bytes.Buffer
	cfg = &config.Config{StashDir: vault}
	printer = output.New(output.WithJSON(true), output.WithOutput(&stdout), output.WithNoColor(true))
	rootCtx = context.Background()
	cleanupApply, cleanupKeepTag = true, "keep"
	cleanupTool, cleanupTag, cleanupDropOnly, cleanupExpired = "", "", false, false
	cleanupIndexDropper = func(string) error { return errors.New("index unavailable") }

	err = runScoringCleanup(mgr)
	if err == nil || !strings.Contains(err.Error(), "cleanup failed") {
		t.Fatalf("runScoringCleanup error = %v, want partial-failure error", err)
	}
	var got cleanupdomain.Result
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("decode JSON %q: %v", stdout.String(), err)
	}
	if len(got.Dropped) != 1 || got.Dropped[0] != st.Manifest.ID {
		t.Fatalf("dropped = %v, want [%s]", got.Dropped, st.Manifest.ID)
	}
	if len(got.Failed) != 1 || got.Failed[0].Stage != "index" {
		t.Fatalf("failed = %+v, want index failure", got.Failed)
	}
	if mgr.Exists(st.Manifest.ID) {
		t.Fatal("stash still exists after successful drop with index failure")
	}
}

func TestSweepAutoJSONReportsIndexFailureThenReturnsError(t *testing.T) {
	root := t.TempDir()
	vault := filepath.Join(root, "vault")
	source := filepath.Join(root, "cache")
	if err := os.MkdirAll(source, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "cache.txt"), []byte("cache"), 0600); err != nil {
		t.Fatal(err)
	}
	mgr, err := stash.NewManager(vault)
	if err != nil {
		t.Fatal(err)
	}
	st, err := mgr.Save(context.Background(), &stash.SaveOptions{SourcePath: source, Tool: "codemap"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(source); err != nil {
		t.Fatal(err)
	}

	oldCfg, oldPrinter, oldRootCtx := cfg, printer, rootCtx
	oldApply, oldKeep, oldInclude, oldAuto, oldStale := sweepApply, sweepKeepTag, sweepIncludeTag, sweepAuto, sweepIncludeStale
	oldDropper := sweepIndexDropper
	t.Cleanup(func() {
		cfg, printer, rootCtx = oldCfg, oldPrinter, oldRootCtx
		sweepApply, sweepKeepTag, sweepIncludeTag, sweepAuto, sweepIncludeStale = oldApply, oldKeep, oldInclude, oldAuto, oldStale
		sweepIndexDropper = oldDropper
	})

	var stdout bytes.Buffer
	cfg = &config.Config{StashDir: vault}
	printer = output.New(output.WithJSON(true), output.WithOutput(&stdout), output.WithNoColor(true))
	rootCtx = context.Background()
	sweepApply, sweepKeepTag, sweepIncludeTag, sweepAuto, sweepIncludeStale = true, "keep", "", true, false
	sweepIndexDropper = func(string) error { return errors.New("index unavailable") }

	err = sweepCmd.RunE(sweepCmd, nil)
	if err == nil || !strings.Contains(err.Error(), "sweep failed") {
		t.Fatalf("sweep error = %v, want partial-failure error", err)
	}
	var got sweepOutput
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("decode JSON %q: %v", stdout.String(), err)
	}
	if len(got.AutoCandidates) != 1 || len(got.AutoDropped) != 1 || got.AutoDropped[0].ID != st.Manifest.ID {
		t.Fatalf("auto plan/result = candidates:%+v dropped:%+v", got.AutoCandidates, got.AutoDropped)
	}
	if len(got.AutoFailed) != 1 || got.AutoFailed[0].Stage != "index" {
		t.Fatalf("auto_failed = %+v, want index failure", got.AutoFailed)
	}
	if got.AutoSkipped == nil || got.AutoFailed == nil {
		t.Fatal("auto result arrays must be non-nil")
	}
}

func TestRunAutoSweepCancellationLeavesRemainingCandidate(t *testing.T) {
	root := t.TempDir()
	vault := filepath.Join(root, "vault")
	source := filepath.Join(root, "cache")
	if err := os.MkdirAll(source, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "cache.txt"), []byte("cache"), 0600); err != nil {
		t.Fatal(err)
	}
	mgr, err := stash.NewManager(vault)
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]string, 0, 2)
	for range 2 {
		st, err := mgr.Save(context.Background(), &stash.SaveOptions{SourcePath: source, Tool: "codemap"})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, st.Manifest.ID)
	}
	if err := os.RemoveAll(source); err != nil {
		t.Fatal(err)
	}
	analysis, err := mgr.AnalyzeCleanup(context.Background(), stash.CleanupOptions{})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	indexCalls := 0
	got := runAutoSweep(ctx, mgr, analysis, true, "keep", "", false, func(string) error {
		indexCalls++
		cancel()
		return nil
	})
	if len(got.Candidates) != 2 || len(got.Dropped) != 1 || indexCalls != 1 {
		t.Fatalf("auto sweep after cancellation = %+v, index calls %d", got, indexCalls)
	}
	if len(got.Failed) != 1 || got.Failed[0].Stage != "cancel" {
		t.Fatalf("failed = %+v, want one cancellation", got.Failed)
	}
	existing := 0
	for _, id := range ids {
		if mgr.Exists(id) {
			existing++
		}
	}
	if existing != 1 {
		t.Fatalf("existing stashes = %d, want one after cancellation", existing)
	}
}

func TestSaveJSONReportsPostSaveFailuresThenReturnsError(t *testing.T) {
	root := t.TempDir()
	vault := filepath.Join(root, "vault")
	source := filepath.Join(root, "source.txt")
	if err := os.WriteFile(source, []byte("payload"), 0600); err != nil {
		t.Fatal(err)
	}

	oldCfg, oldPrinter, oldRootCtx := cfg, printer, rootCtx
	oldName, oldTags, oldTool, oldSource, oldTTL := saveName, saveTags, saveTool, saveSource, saveTTL
	oldNoScan, oldNoCompress, oldIndex := saveNoScan, saveNoCompress, saveIndex
	oldIndexOperation, oldCompressOperation := saveIndexOperation, saveCompressOperation
	t.Cleanup(func() {
		cfg, printer, rootCtx = oldCfg, oldPrinter, oldRootCtx
		saveName, saveTags, saveTool, saveSource, saveTTL = oldName, oldTags, oldTool, oldSource, oldTTL
		saveNoScan, saveNoCompress, saveIndex = oldNoScan, oldNoCompress, oldIndex
		saveIndexOperation, saveCompressOperation = oldIndexOperation, oldCompressOperation
	})

	var stdout bytes.Buffer
	cfg = &config.Config{StashDir: vault, Compression: "zstd", CompressThreshold: 1}
	printer = output.New(output.WithJSON(true), output.WithOutput(&stdout), output.WithNoColor(true))
	rootCtx = context.Background()
	saveName, saveTags, saveTool, saveSource, saveTTL = "", nil, "", "", ""
	saveNoScan, saveNoCompress, saveIndex = true, false, true
	saveIndexOperation = func(context.Context, *stash.Manager, string) (*analyze.IndexResult, error) {
		return nil, errors.New("index unavailable")
	}
	saveCompressOperation = func(context.Context, *stash.Manager, string, string) (*stash.CompressResult, error) {
		return nil, errors.New("compression unavailable")
	}

	err := saveCmd.RunE(saveCmd, []string{source})
	if err == nil || !strings.Contains(err.Error(), "saved with 2 failed") {
		t.Fatalf("save error = %v, want two partial failures", err)
	}
	var got saveOutput
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("decode JSON %q: %v", stdout.String(), err)
	}
	if got.Status != "saved_with_failures" || !got.IndexRequested || !got.AutoCompressionRequested {
		t.Fatalf("save output = %+v", got)
	}
	if got.Indexed || got.AutoCompressed || len(got.Failed) != 2 {
		t.Fatalf("save post-operations = indexed:%t compressed:%t failed:%+v", got.Indexed, got.AutoCompressed, got.Failed)
	}
	mgr, err := stash.NewManager(vault)
	if err != nil {
		t.Fatal(err)
	}
	if got.Manifest == nil || !mgr.Exists(got.ID) {
		t.Fatalf("saved manifest missing or stash does not exist: %+v", got.Manifest)
	}
}
