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
)

func TestSaveMetaIsStoredAndListedWithContentHash(t *testing.T) {
	root := t.TempDir()
	vault := filepath.Join(root, "vault")
	source := filepath.Join(root, "dataset.archive.gz")
	if err := os.WriteFile(source, []byte("archive bytes"), 0o600); err != nil {
		t.Fatal(err)
	}

	oldCfg, oldPrinter, oldRootCtx := cfg, printer, rootCtx
	oldTags, oldTool, oldSource, oldMeta := saveTags, saveTool, saveSource, saveMeta
	oldNoScan, oldNoCompress, oldIndex := saveNoScan, saveNoCompress, saveIndex
	oldListTags, oldListTool := listTags, listTool
	t.Cleanup(func() {
		cfg, printer, rootCtx = oldCfg, oldPrinter, oldRootCtx
		saveTags, saveTool, saveSource, saveMeta = oldTags, oldTool, oldSource, oldMeta
		saveNoScan, saveNoCompress, saveIndex = oldNoScan, oldNoCompress, oldIndex
		listTags, listTool = oldListTags, oldListTool
	})

	var stdout bytes.Buffer
	cfg = &config.Config{StashDir: vault, Compression: "zstd"}
	printer = output.New(output.WithJSON(true), output.WithOutput(&stdout), output.WithNoColor(true))
	rootCtx = context.Background()
	saveTags, saveTool, saveSource = []string{"kind=dataset", "dataset=demo"}, "bench", "/data/origin"
	saveMeta = []string{"sha256=abc123", "version=v2"}
	saveNoScan, saveNoCompress, saveIndex = true, true, false

	if err := saveCmd.RunE(saveCmd, []string{source}); err != nil {
		t.Fatalf("save: %v", err)
	}
	var saved saveOutput
	if err := json.Unmarshal(stdout.Bytes(), &saved); err != nil {
		t.Fatalf("decode save %q: %v", stdout.String(), err)
	}
	if saved.Custom["sha256"] != "abc123" || saved.Custom["version"] != "v2" || saved.Custom["source"] != "/data/origin" {
		t.Fatalf("saved custom = %v", saved.Custom)
	}

	stdout.Reset()
	listTags, listTool = []string{"kind=dataset", "dataset=demo"}, "bench"
	if err := listCmd.RunE(listCmd, nil); err != nil {
		t.Fatalf("list: %v", err)
	}
	var items []struct {
		ID          string            `json:"id"`
		ContentHash string            `json:"content_hash"`
		Custom      map[string]string `json:"custom"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &items); err != nil {
		t.Fatalf("decode list %q: %v", stdout.String(), err)
	}
	if len(items) != 1 || items[0].ID != saved.ID {
		t.Fatalf("list items = %+v", items)
	}
	if items[0].ContentHash == "" || items[0].ContentHash != saved.ContentHash {
		t.Fatalf("list content_hash = %q, save = %q", items[0].ContentHash, saved.ContentHash)
	}
	if items[0].Custom["version"] != "v2" {
		t.Fatalf("list custom = %v", items[0].Custom)
	}
}

func TestSaveMetaRejectsReservedKeysBeforeSaving(t *testing.T) {
	root := t.TempDir()
	vault := filepath.Join(root, "vault")
	source := filepath.Join(root, "f.txt")
	if err := os.WriteFile(source, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	oldCfg, oldPrinter, oldRootCtx, oldMeta, oldNoScan := cfg, printer, rootCtx, saveMeta, saveNoScan
	t.Cleanup(func() { cfg, printer, rootCtx, saveMeta, saveNoScan = oldCfg, oldPrinter, oldRootCtx, oldMeta, oldNoScan })

	var stdout bytes.Buffer
	cfg = &config.Config{StashDir: vault}
	printer = output.New(output.WithJSON(true), output.WithOutput(&stdout), output.WithNoColor(true))
	rootCtx = context.Background()
	saveMeta, saveNoScan = []string{"secrets_found=0"}, true

	err := saveCmd.RunE(saveCmd, []string{source})
	if err == nil || !strings.Contains(err.Error(), "reserved") {
		t.Fatalf("save err = %v, want reserved-key refusal", err)
	}
	if entries, _ := os.ReadDir(vault); len(entries) > 0 {
		for _, e := range entries {
			if e.IsDir() {
				t.Fatalf("a stash directory was created despite invalid metadata: %s", e.Name())
			}
		}
	}
}
