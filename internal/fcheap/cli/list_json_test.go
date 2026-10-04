package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/file.cheap/internal/fcheap/config"
	"github.com/abdul-hamid-achik/file.cheap/internal/fcheap/output"
	"github.com/abdul-hamid-achik/file.cheap/internal/stash"
)

func TestListJSONCarriesBundleTypeAndCompressedSize(t *testing.T) {
	oldCfg, oldPrinter, oldRootCtx := cfg, printer, rootCtx
	oldTags, oldTool, oldSince, oldExpired := listTags, listTool, listSince, listIncludeExpired
	t.Cleanup(func() {
		cfg, printer, rootCtx = oldCfg, oldPrinter, oldRootCtx
		listTags, listTool, listSince, listIncludeExpired = oldTags, oldTool, oldSince, oldExpired
	})
	vault := filepath.Join(t.TempDir(), "vault")
	mgr, err := stash.NewManager(vault)
	if err != nil {
		t.Fatal(err)
	}
	plain := t.TempDir()
	if err := os.WriteFile(filepath.Join(plain, "a.txt"), []byte(strings.Repeat("compressible ", 4096)), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	raw, err := mgr.Save(ctx, &stash.SaveOptions{SourcePath: plain, Name: "raw"})
	if err != nil {
		t.Fatal(err)
	}
	packed, err := mgr.Save(ctx, &stash.SaveOptions{SourcePath: plain, Name: "packed"})
	if err != nil {
		t.Fatal(err)
	}
	cres, err := mgr.Compress(ctx, packed.Manifest.ID, "zstd")
	if err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	cfg = &config.Config{StashDir: vault}
	printer = output.New(output.WithJSON(true), output.WithOutput(&stdout), output.WithNoColor(true))
	rootCtx = ctx
	listTags, listTool, listSince, listIncludeExpired = nil, "", "", false
	if err := listCmd.RunE(listCmd, nil); err != nil {
		t.Fatal(err)
	}
	var items []map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &items); err != nil {
		t.Fatalf("decode list %q: %v", stdout.String(), err)
	}
	byID := map[string]map[string]any{}
	for _, item := range items {
		byID[item["id"].(string)] = item
	}
	if len(byID) != 2 {
		t.Fatalf("items = %v", items)
	}
	rawItem, packedItem := byID[raw.Manifest.ID], byID[packed.Manifest.ID]
	if rawItem["bundle_type"] != "generic" || packedItem["bundle_type"] != "generic" {
		t.Fatalf("bundle_type = %v / %v, want generic", rawItem["bundle_type"], packedItem["bundle_type"])
	}
	if got := int64(packedItem["compressed_size"].(float64)); got != cres.CompressedSize || got <= 0 {
		t.Fatalf("compressed_size = %v, want %d", packedItem["compressed_size"], cres.CompressedSize)
	}
	if _, present := rawItem["compressed_size"]; present {
		t.Fatalf("uncompressed stash must omit compressed_size: %v", rawItem)
	}
	if packedItem["compression"] != "zstd" {
		t.Fatalf("compression = %v", packedItem["compression"])
	}
}
