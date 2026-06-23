package studio

import (
	"regexp"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/file.cheap/internal/detect"
	"github.com/abdul-hamid-achik/file.cheap/internal/diff"
)

// ansi strips terminal escape sequences so assertions test the text content.
var ansi = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func clean(s string) string { return ansi.ReplaceAllString(s, "") }

func TestFormatSize(t *testing.T) {
	cases := map[int64]string{
		0:               "0 B",
		512:             "512 B",
		1024:            "1.0 KiB",
		1536:            "1.5 KiB",
		1024 * 1024:     "1.0 MiB",
		3 * 1024 * 1024: "3.0 MiB",
	}
	for in, want := range cases {
		if got := formatSize(in); got != want {
			t.Errorf("formatSize(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestCompLabel(t *testing.T) {
	for in, want := range map[string]string{"zstd": "zst", "zst": "zst", "gzip": "gz", "gz": "gz"} {
		if got := compLabel(in); got != want {
			t.Errorf("compLabel(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRelTime(t *testing.T) {
	if got := relTime(""); got != "—" {
		t.Errorf("relTime(empty) = %q, want —", got)
	}
	// Unparseable falls back to the first 10 chars (a date prefix).
	if got := relTime("2026-06-22T11:52:54Z-bogus"); got != "2026-06-22" {
		t.Errorf("relTime(bad) = %q, want 2026-06-22", got)
	}
}

func TestTruncate(t *testing.T) {
	if got := clean(truncate("hello world", 100)); got != "hello world" {
		t.Errorf("truncate(short) = %q, want unchanged", got)
	}
	got := clean(truncate("hello world", 5))
	if !strings.HasSuffix(got, "…") || len([]rune(got)) > 5 {
		t.Errorf("truncate(narrow) = %q, want <=5 runes ending in …", got)
	}
}

func TestPlural(t *testing.T) {
	if plural(1, "entry", "entries") != "entry" {
		t.Error("plural(1) should be singular")
	}
	if plural(2, "entry", "entries") != "entries" {
		t.Error("plural(2) should be plural")
	}
}

func TestFormatDiff(t *testing.T) {
	r := &diff.DiffResult{
		OnlyInStash:  []string{"gone.txt"},
		OnlyInTarget: []string{"added.txt"},
		Changed:      []diff.ChangedFile{{Path: "edited.txt"}},
		Unchanged:    3,
	}
	out := clean(formatDiff(r, "/some/path"))
	for _, want := range []string{"vs /some/path", "+ gone.txt", "- added.txt", "~ edited.txt", "Unchanged: 3"} {
		if !strings.Contains(out, want) {
			t.Errorf("formatDiff missing %q in:\n%s", want, out)
		}
	}

	identical := clean(formatDiff(&diff.DiffResult{Unchanged: 5}, "/p"))
	if !strings.Contains(identical, "identical") {
		t.Errorf("formatDiff(no changes) should say identical, got:\n%s", identical)
	}
}

func TestFormatTimeline(t *testing.T) {
	entries := []detect.TimelineEntry{
		{TimeSeconds: 0, Frame: "frames/f1.png", OCR: "NullPointer at checkout", Transcript: "it crashed"},
		{TimeSeconds: 12, Frame: "frames/f2.png", OCR: "Retry shown"},
	}
	out := clean(formatTimeline(entries))
	for _, want := range []string{"0s", "frames/f1.png", "NullPointer at checkout", "it crashed", "12s", "frames/f2.png"} {
		if !strings.Contains(out, want) {
			t.Errorf("formatTimeline missing %q in:\n%s", want, out)
		}
	}
}
