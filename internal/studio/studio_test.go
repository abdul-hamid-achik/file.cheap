package studio

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/file.cheap/internal/detect"
	"github.com/abdul-hamid-achik/file.cheap/internal/diff"
	"github.com/abdul-hamid-achik/file.cheap/internal/manifest"
	"github.com/abdul-hamid-achik/file.cheap/internal/stash"
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

// TestViewFillsTerminalHeight verifies the rendered TUI occupies the full
// terminal height (no large dead space below the content) — the layout fix.
func TestViewFillsTerminalHeight(t *testing.T) {
	const w, h = 120, 40
	for _, tc := range []struct {
		name string
		view viewName
	}{
		{"empty-list", viewList},
		{"help", viewHelp},
		{"detail-none", viewDetail},
	} {
		m := Model{width: w, height: h, activeView: tc.view, searchMode: "auto"}
		out := clean(m.render())
		if got := strings.Count(out, "\n") + 1; got != h {
			t.Errorf("%s: rendered %d lines, want %d (should fill the terminal)", tc.name, got, h)
		}
		for i, ln := range strings.Split(out, "\n") {
			if cells := len([]rune(ln)); cells > w {
				t.Errorf("%s: line %d is %d cells wide, want <= %d", tc.name, i, cells, w)
				break
			}
		}
	}
}

// TestStashSort verifies the list sort modes cycled by the "o" key.
func TestStashSort(t *testing.T) {
	mk := func(name, tool string, size int64, created string) *stash.Stash {
		return &stash.Stash{Manifest: &manifest.Manifest{
			ID: name, Name: name, Tool: tool, TotalSize: size, CreatedAt: created,
		}}
	}
	m := &Model{stashes: []*stash.Stash{
		mk("c-old-small", "generic", 100, "2026-06-20T00:00:00Z"),
		mk("a-new-big", "vidtrace", 9000, "2026-06-23T00:00:00Z"),
		mk("b-mid", "vecgrep", 500, "2026-06-21T00:00:00Z"),
	}}

	m.sortIdx = 0 // AGE desc (newest first)
	m.sortStashes()
	if got := m.stashes[0].Manifest.Name; got != "a-new-big" {
		t.Errorf("AGE sort first = %q, want a-new-big", got)
	}

	m.sortIdx = 1 // NAME asc
	m.sortStashes()
	if got := m.stashes[0].Manifest.Name; got != "a-new-big" {
		t.Errorf("NAME sort first = %q, want a-new-big", got)
	}
	if got := m.stashes[2].Manifest.Name; got != "c-old-small" {
		t.Errorf("NAME sort last = %q, want c-old-small", got)
	}

	m.sortIdx = 4 // SIZE desc
	m.sortRev = false
	m.sortStashes()
	if got := m.stashes[0].Manifest.TotalSize; got != 9000 {
		t.Errorf("SIZE sort first = %d, want 9000", got)
	}

	m.sortRev = true // reverse -> smallest first
	m.sortStashes()
	if got := m.stashes[0].Manifest.TotalSize; got != 100 {
		t.Errorf("reversed SIZE sort first = %d, want 100", got)
	}
	if m.effectiveSortDesc() {
		t.Errorf("effectiveSortDesc = true, want false (a reversed descending sort is ascending)")
	}
}

// TestStashFilter verifies the live list filter (case-insensitive substring over
// name / id / tool / tags).
func TestStashFilter(t *testing.T) {
	mk := func(name, tool string, tags ...string) *stash.Stash {
		return &stash.Stash{Manifest: &manifest.Manifest{ID: name, Name: name, Tool: tool, Tags: tags}}
	}
	m := &Model{stashes: []*stash.Stash{
		mk("alpha-bug", "vidtrace", "urgent"),
		mk("beta-notes", "generic"),
		mk("gamma-auth", "vidtrace", "auth"),
	}}

	if got := len(m.visible()); got != 3 {
		t.Errorf("no filter: %d visible, want 3", got)
	}
	m.filter = "vidtrace" // by tool
	if got := len(m.visible()); got != 2 {
		t.Errorf("tool filter: %d visible, want 2", got)
	}
	m.filter = "beta" // by name substring
	if got := m.visible(); len(got) != 1 || got[0].Manifest.Name != "beta-notes" {
		t.Errorf("name filter: %+v, want only beta-notes", got)
	}
	m.filter = "ALPHA" // case-insensitive
	if got := len(m.visible()); got != 1 {
		t.Errorf("case-insensitive filter: %d visible, want 1", got)
	}
	m.filter = "auth" // by tag
	if got := len(m.visible()); got != 1 {
		t.Errorf("tag filter: %d visible, want 1", got)
	}
	m.filter = "zzz" // no match
	if got := len(m.visible()); got != 0 {
		t.Errorf("no-match filter: %d visible, want 0", got)
	}
}

// TestListLayoutBounds verifies the list panel keeps its bottom border and no
// line exceeds the terminal width when the list overflows — across widths and
// down to a tiny terminal. Regression test for the TUI-review layout findings.
func TestListLayoutBounds(t *testing.T) {
	var ss []*stash.Stash
	for i := 0; i < 50; i++ {
		ss = append(ss, &stash.Stash{Manifest: &manifest.Manifest{
			ID: fmt.Sprintf("stash-%02d", i), Name: fmt.Sprintf("stash-%02d", i),
			Tool: "vidtrace", FileCount: i, TotalSize: int64(i) * 1000,
			CreatedAt: "2026-06-23T06:00:00Z", Compression: "zstd",
			Custom: map[string]string{"secrets_found": "2"},
		}})
	}
	for _, w := range []int{70, 90, 120, 200} {
		m := Model{width: w, height: 24, activeView: viewList, searchMode: "auto", stashes: ss}
		out := clean(m.render())
		lines := strings.Split(out, "\n")
		bottom := false
		for _, ln := range lines {
			if strings.ContainsAny(ln, "╰└") {
				bottom = true
			}
			if cells := len([]rune(ln)); cells > w {
				t.Errorf("w=%d: a line is %d cells wide, want <= %d", w, cells, w)
				break
			}
		}
		if !bottom {
			t.Errorf("w=%d: list panel lost its bottom border on overflow", w)
		}
		if got := len(lines); got != 24 {
			t.Errorf("w=%d: rendered %d lines, want 24", w, got)
		}
	}
	// A terminal too short for the chrome must not overflow.
	m := Model{width: 60, height: 3, activeView: viewList, searchMode: "auto", stashes: ss}
	if got := len(strings.Split(clean(m.render()), "\n")); got > 3 {
		t.Errorf("tiny terminal rendered %d lines, want <= 3", got)
	}
}
