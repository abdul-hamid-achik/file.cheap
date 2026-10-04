package studio

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/file.cheap/internal/analyze"
	"github.com/abdul-hamid-achik/file.cheap/internal/detect"
	"github.com/abdul-hamid-achik/file.cheap/internal/stash"
)

// runFixture is a synthetic cairntrace run directory (no real stashes).
var runFixture = filepath.Join("..", "detect", "testdata", "run-summary", "cairntrace-full")

func TestIsRunBundle(t *testing.T) {
	for bt, want := range map[string]bool{
		"cairntrace-run": true, "glyphrun-run": true, "vidtrace": false, "generic": false, "": false,
	} {
		if got := isRunBundle(bt); got != want {
			t.Errorf("isRunBundle(%q) = %v, want %v", bt, got, want)
		}
	}
}

func TestFormatRunSummaryShowsStatusSpecOutcomesAndReport(t *testing.T) {
	s, ok := detect.ReadRunSummary(runFixture)
	if !ok {
		t.Fatal("fixture is not a run")
	}
	out := clean(formatRunSummary(s, ""))
	for _, want := range []string{
		"RUN", "failed", "checkout_alpha", "staging · agent-browser",
		"Steps", "3", "5  ·  2 passed  ·  1 failed  ·  2 other",
		"report.html present", "OUTCOMES (5)",
		"✓ cart-total", "✗ order-confirmed", "– receipt-emailed", "(unnamed)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("run view missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "\a") {
		t.Error("control characters must never reach the terminal")
	}
}

func TestFormatRunSummaryRestoredPathAndTruncation(t *testing.T) {
	s := detect.RunSummary{Status: "passed", SpecName: "big", HasReportHTML: true, OutcomeCount: 80, PassedOutcomes: 80}
	for i := 0; i < detect.MaxRunSummaryOutcomes; i++ {
		s.Outcomes = append(s.Outcomes, detect.RunOutcome{ID: fmt.Sprintf("o%d", i), Status: "passed"})
	}
	s.OutcomesTruncated = true
	out := clean(formatRunSummary(s, "/tmp/restored-run"))
	if !strings.Contains(out, "OUTCOMES (first 50 of 80)") {
		t.Errorf("missing truncation title:\n%s", out)
	}
	if got := strings.Count(out, "✓ o"); got != detect.MaxRunSummaryOutcomes {
		t.Errorf("rendered %d outcome rows, want %d", got, detect.MaxRunSummaryOutcomes)
	}
	if !strings.Contains(out, filepath.Join("/tmp/restored-run", "report.html")) {
		t.Errorf("restored view must print the report.html path:\n%s", out)
	}

	none := clean(formatRunSummary(detect.RunSummary{Status: "failed"}, ""))
	if !strings.Contains(none, "no report.html in this bundle") {
		t.Errorf("missing no-report message:\n%s", none)
	}
}

// saveRunStash saves the fixture run into a temp vault and returns the vault
// dir and stash.
func saveRunStash(t *testing.T) (string, *stash.Stash) {
	t.Helper()
	vault := t.TempDir()
	mgr, err := stash.NewManager(vault)
	if err != nil {
		t.Fatal(err)
	}
	st, err := mgr.Save(context.Background(), &stash.SaveOptions{SourcePath: runFixture, Tool: "cairntrace", NoScan: true})
	if err != nil {
		t.Fatal(err)
	}
	if st.Manifest.BundleType != "cairntrace-run" {
		t.Fatalf("fixture saved as %q, want cairntrace-run", st.Manifest.BundleType)
	}
	return vault, st
}

func TestRunViewKeyLoadsSummaryInPlace(t *testing.T) {
	vault, st := saveRunStash(t)
	m := NewModel(context.Background(), vault, "", analyze.EmbedderSettings{})
	m.width, m.height = 120, 40
	m.resize()
	m.activeView = viewDetail
	m.selected = st

	cmd, handled := m.handleDetailKey("v")
	if !handled || cmd == nil {
		t.Fatalf("v on a run stash: handled=%v cmd=%v", handled, cmd)
	}
	msg, ok := cmd().(runLoadedMsg)
	if !ok || msg.err != nil {
		t.Fatalf("run load = %#v", msg)
	}
	updated, _ := m.Update(msg)
	got := updated.(Model)
	if got.activeView != viewRun {
		t.Fatalf("activeView = %v, want viewRun", got.activeView)
	}
	view := clean(got.View().Content)
	for _, want := range []string{"Run", "checkout_alpha", "order-confirmed"} {
		if !strings.Contains(view, want) {
			t.Errorf("rendered run view missing %q", want)
		}
	}
	if _, handled := got.handleTimelineKey("v"); !handled {
		t.Fatal("v must leave the run view")
	}
}

func TestRunViewKeysRejectNonRunBundles(t *testing.T) {
	m := NewModel(context.Background(), t.TempDir(), "", analyze.EmbedderSettings{})
	m.activeView = viewDetail
	m.selected = manyFileStash(1)
	for _, key := range []string{"v", "V"} {
		cmd, handled := m.handleDetailKey(key)
		if !handled || cmd != nil || !strings.Contains(m.statusMessage, "cairntrace-run and glyphrun-run") {
			t.Errorf("key %q on a generic stash: handled=%v cmd=%v status=%q", key, handled, cmd, m.statusMessage)
		}
	}
}

func TestRestoreRunReportRestoresToTempAndReportsPath(t *testing.T) {
	vault, st := saveRunStash(t)
	msg := restoreRunReport(context.Background(), vault, st.Manifest.ID)
	if msg.err != nil {
		t.Fatal(msg.err)
	}
	path := strings.TrimPrefix(strings.SplitN(msg.status, " · ", 2)[0], "report: ")
	t.Cleanup(func() { _ = os.RemoveAll(filepath.Dir(path)) })
	if filepath.Base(path) != "report.html" {
		t.Fatalf("status %q does not lead with the report.html path", msg.status)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("report.html was not restored: %v", err)
	}
	if strings.HasPrefix(filepath.Dir(path), vault) {
		t.Fatalf("restore target %s is inside the vault", path)
	}
	if !strings.Contains(clean(msg.content), path) {
		t.Errorf("run view does not print the restored report path:\n%s", clean(msg.content))
	}
}
