package stash

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseMetadataAcceptsKeyValuePairs(t *testing.T) {
	meta, err := ParseMetadata([]string{"sha256=abc", "dataset.version=v3", "note=a=b"})
	if err != nil {
		t.Fatalf("ParseMetadata: %v", err)
	}
	if meta["sha256"] != "abc" || meta["dataset.version"] != "v3" || meta["note"] != "a=b" {
		t.Fatalf("meta = %v", meta)
	}
	if got, err := ParseMetadata(nil); err != nil || got != nil {
		t.Fatalf("empty ParseMetadata = %v, %v", got, err)
	}
}

func TestParseMetadataRejectsUnsafeInput(t *testing.T) {
	cases := map[string][]string{
		"missing equals":  {"novalue"},
		"empty key":       {"=v"},
		"duplicate key":   {"a=1", "a=2"},
		"uppercase key":   {"Key=v"},
		"reserved source": {"source=/tmp/x"},
		"reserved scan":   {"secrets_found=0"},
		"reserved index":  {"indexed=true"},
		"control char":    {"k=line\nbreak"},
		"long value":      {"k=" + strings.Repeat("x", MaxMetadataValueBytes+1)},
	}
	for name, pairs := range cases {
		if _, err := ParseMetadata(pairs); err == nil {
			t.Errorf("%s: ParseMetadata(%q) succeeded, want error", name, pairs)
		}
	}
	tooMany := make([]string, 0, MaxMetadataEntries+1)
	for i := 0; i <= MaxMetadataEntries; i++ {
		tooMany = append(tooMany, "k"+strings.Repeat("x", i)+"=v")
	}
	if _, err := ParseMetadata(tooMany); err == nil {
		t.Error("ParseMetadata accepted more than MaxMetadataEntries entries")
	}
}

func TestMergeMetadataNeverOverwrites(t *testing.T) {
	merged, err := MergeMetadata(map[string]string{"run": "1"}, map[string]string{"sha": "abc"})
	if err != nil || merged["run"] != "1" || merged["sha"] != "abc" {
		t.Fatalf("MergeMetadata = %v, %v", merged, err)
	}
	if _, err := MergeMetadata(map[string]string{"sha": "1"}, map[string]string{"sha": "2"}); err == nil {
		t.Fatal("MergeMetadata overwrote an existing key")
	}
	base := map[string]string{"x": "1"}
	if got, err := MergeMetadata(base, nil); err != nil || got["x"] != "1" {
		t.Fatalf("MergeMetadata(base, nil) = %v, %v", got, err)
	}
}

func TestSavedMetadataIsListedBack(t *testing.T) {
	tmp := t.TempDir()
	mgr, err := NewManager(filepath.Join(tmp, "vault"))
	if err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(tmp, "data.bin")
	if err := os.WriteFile(src, []byte("payload"), 0o600); err != nil {
		t.Fatal(err)
	}
	meta, err := ParseMetadata([]string{"sha256=deadbeef", "version=v1"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	st, err := mgr.Save(ctx, &SaveOptions{SourcePath: src, Tags: []string{"kind=dataset"}, Custom: meta, NoScan: true})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if st.Manifest.Custom["sha256"] != "deadbeef" {
		t.Fatalf("saved custom = %v", st.Manifest.Custom)
	}
	listed, err := mgr.ListFiltered(ctx, ListOptions{Tags: []string{"kind=dataset"}})
	if err != nil || len(listed) != 1 {
		t.Fatalf("ListFiltered = %d, %v", len(listed), err)
	}
	if listed[0].Manifest.Custom["version"] != "v1" {
		t.Fatalf("listed custom = %v", listed[0].Manifest.Custom)
	}
}
