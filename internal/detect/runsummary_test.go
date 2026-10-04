package detect

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadRunSummaryCairntrace(t *testing.T) {
	s, ok := ReadRunSummary(filepath.Join("testdata", "run-summary", "cairntrace-full"))
	if !ok {
		t.Fatal("ReadRunSummary reported not a run")
	}
	if s.Type != TypeCairntraceRun || s.SpecName != "checkout_alpha" || s.Status != "failed" ||
		s.Environment != "staging" || s.Backend != "agent-browser" {
		t.Fatalf("unexpected identity: %+v", s)
	}
	if s.StepCount != 3 || s.OutcomeCount != 5 {
		t.Fatalf("StepCount/OutcomeCount = %d/%d, want 3/5", s.StepCount, s.OutcomeCount)
	}
	// "weird\a id" has a control character, so its id is dropped to (unnamed)
	// but it is still counted; the status-less outcome is "unknown".
	if s.PassedOutcomes != 2 || s.FailedOutcomes != 1 || s.OtherOutcomes != 2 {
		t.Fatalf("passed/failed/other = %d/%d/%d, want 2/1/2", s.PassedOutcomes, s.FailedOutcomes, s.OtherOutcomes)
	}
	want := []RunOutcome{
		{"cart-total", "passed"}, {"order-confirmed", "failed"}, {"receipt-emailed", "skipped"},
		{"(unnamed)", "passed"}, {"no-status", "unknown"},
	}
	if len(s.Outcomes) != len(want) {
		t.Fatalf("Outcomes = %+v", s.Outcomes)
	}
	for i := range want {
		if s.Outcomes[i] != want[i] {
			t.Errorf("Outcomes[%d] = %+v, want %+v", i, s.Outcomes[i], want[i])
		}
	}
	if !s.HasReportHTML || !s.HasReportJSON {
		t.Fatalf("report flags = html %v json %v, want both true", s.HasReportHTML, s.HasReportJSON)
	}
	if s.DurationMS == nil || *s.DurationMS != 2000 || s.ExitCode == nil || *s.ExitCode != 1 {
		t.Fatalf("duration/exit = %v/%v", s.DurationMS, s.ExitCode)
	}
}

func TestReadRunSummaryFallsBackToReportJSON(t *testing.T) {
	s, ok := ReadRunSummary(filepath.Join("testdata", "run-summary", "cairntrace-report-only"))
	if !ok {
		t.Fatal("ReadRunSummary reported not a run")
	}
	if s.RunID != "run-from-report" || s.Backend != "playwright" {
		t.Fatalf("run.json fields must win: %+v", s)
	}
	if s.SpecName != "login_flow" || s.Environment != "local" || s.Status != "passed" || s.StepCount != 2 || s.OutcomeCount != 1 {
		t.Fatalf("report.json fallback not applied: %+v", s)
	}
	if s.HasReportHTML || !s.HasReportJSON {
		t.Fatalf("report flags = html %v json %v", s.HasReportHTML, s.HasReportJSON)
	}
}

func TestReadRunSummaryGlyphrun(t *testing.T) {
	s, ok := ReadRunSummary(filepath.Join("testdata", "glyphrun-run-current"))
	if !ok || s.Type != TypeGlyphrunRun || s.Status != "errored" || s.SpecName != "project_open" {
		t.Fatalf("glyphrun summary = %+v ok=%v", s, ok)
	}
}

func TestReadRunSummaryRejectsOtherBundles(t *testing.T) {
	if _, ok := ReadRunSummary(t.TempDir()); ok {
		t.Fatal("empty directory must not be a run")
	}
	if _, ok := ReadRunSummary(filepath.Join("testdata", "does-not-exist")); ok {
		t.Fatal("missing directory must not be a run")
	}
}

func TestReadRunSummaryCapsOutcomes(t *testing.T) {
	dir := t.TempDir()
	var rows []string
	for i := 0; i < MaxRunSummaryOutcomes+30; i++ {
		status := "passed"
		if i%10 == 0 {
			status = "failed"
		}
		rows = append(rows, fmt.Sprintf(`{"id":"o%d","status":%q}`, i, status))
	}
	run := `{"$schema":"urn:cairntrace.dev:run:v1","version":"1","spec":{"name":"big"},"status":"passed","outcomes":[` + strings.Join(rows, ",") + `]}`
	if err := os.WriteFile(filepath.Join(dir, "run.json"), []byte(run), 0o644); err != nil {
		t.Fatal(err)
	}
	s, ok := ReadRunSummary(dir)
	if !ok {
		t.Fatal("not a run")
	}
	if len(s.Outcomes) != MaxRunSummaryOutcomes || !s.OutcomesTruncated {
		t.Fatalf("len=%d truncated=%v", len(s.Outcomes), s.OutcomesTruncated)
	}
	if s.OutcomeCount != 80 || s.FailedOutcomes != 8 || s.PassedOutcomes != 72 {
		t.Fatalf("counts must cover every outcome: total %d failed %d passed %d", s.OutcomeCount, s.FailedOutcomes, s.PassedOutcomes)
	}
	if s.HasReportHTML || s.HasReportJSON {
		t.Fatal("no report files were written")
	}
}

func TestReadRunSummaryIgnoresSymlinkedReport(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "secret.html")
	if err := os.WriteFile(outside, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	run := `{"$schema":"urn:cairntrace.dev:run:v1","version":"1","spec":{"name":"x"},"status":"passed"}`
	if err := os.WriteFile(filepath.Join(dir, "run.json"), []byte(run), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "report.html")); err != nil {
		t.Skip("symlinks unavailable")
	}
	s, ok := ReadRunSummary(dir)
	if !ok || s.HasReportHTML {
		t.Fatalf("symlinked report.html must not count: %+v ok=%v", s, ok)
	}
}
