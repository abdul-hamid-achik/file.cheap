package studio

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/abdul-hamid-achik/file.cheap/internal/detect"
	"github.com/abdul-hamid-achik/file.cheap/internal/stash"
)

// runLoadedMsg carries the rendered run summary for a cairntrace/glyphrun
// bundle into the run view.
type runLoadedMsg struct {
	content string
	status  string
	err     error
}

// isRunBundle reports whether a manifest bundle type is a native run
// directory the run view understands.
func isRunBundle(bundleType string) bool {
	return bundleType == string(detect.TypeCairntraceRun) || bundleType == string(detect.TypeGlyphrunRun)
}

func (m Model) selectedIsRun() bool {
	return m.selected != nil && m.selected.Manifest != nil && isRunBundle(m.selected.Manifest.BundleType)
}

// loadRunViewCmd summarizes an uncompressed run stash in place (bounded reads
// of run.json/report.json only). A compressed stash has no on-disk content, so
// it points the user at the restore key instead.
func (m *Model) loadRunViewCmd() tea.Cmd {
	st := m.selected
	if st == nil || st.Manifest == nil {
		return nil
	}
	if st.Manifest.Compression != "" {
		m.statusMessage = "stash is compressed — press V to restore it and view the run"
		return nil
	}
	dir := filepath.Join(st.Dir, "content")
	return func() tea.Msg {
		s, ok := detect.ReadRunSummary(dir)
		if !ok {
			return runLoadedMsg{err: fmt.Errorf("run.json is missing or not a cairntrace/glyphrun run")}
		}
		return runLoadedMsg{content: formatRunSummary(s, ""), status: runStatusLine(s)}
	}
}

// restoreRunCmd restores the stash into a fresh temp directory (works for
// compressed stashes too), summarizes the restored run, and reports where
// report.html is. The studio has no platform opener, so it prints the path.
func (m *Model) restoreRunCmd() tea.Cmd {
	st := m.selected
	if st == nil || st.Manifest == nil {
		return nil
	}
	id := st.Manifest.ID
	stashDir := m.stashDir
	ctx := m.ctx
	m.working = true
	m.statusMessage = "restoring run..."
	m.errMessage = ""
	return func() tea.Msg {
		return restoreRunReport(ctx, stashDir, id)
	}
}

func restoreRunReport(ctx context.Context, stashDir, id string) runLoadedMsg {
	mgr, err := stash.NewManager(stashDir)
	if err != nil {
		return runLoadedMsg{err: err}
	}
	res, err := mgr.Restore(ctx, id, "")
	if err != nil {
		return runLoadedMsg{err: err}
	}
	if !res.Verified {
		return runLoadedMsg{err: fmt.Errorf("restored to %s with %d verification mismatch(es)", res.Target, len(res.Mismatches))}
	}
	s, ok := detect.ReadRunSummary(res.Target)
	if !ok {
		return runLoadedMsg{err: fmt.Errorf("restored to %s, but it is not a cairntrace/glyphrun run", res.Target)}
	}
	status := runStatusLine(s) + " · restored to " + res.Target
	if s.HasReportHTML {
		status = "report: " + filepath.Join(res.Target, "report.html") + " · " + runStatusLine(s)
	}
	return runLoadedMsg{content: formatRunSummary(s, res.Target), status: status}
}

func runStatusLine(s detect.RunSummary) string {
	name := s.SpecName
	if name == "" {
		name = s.RunID
	}
	return strings.TrimSpace(fmt.Sprintf("%s %s", name, s.Status))
}

func runStatusStyle(status string) func(...string) string {
	switch status {
	case "passed":
		return goodStyle.Render
	case "failed", "errored":
		return errorStyle.Render
	case "":
		return mutedStyle.Render
	default:
		return warnStyle.Render
	}
}

// formatRunSummary renders a run summary for the preview pane. restoredTo is
// the temp directory of a restore ("" when read in place); it adds the exact
// report.html path.
func formatRunSummary(s detect.RunSummary, restoredTo string) string {
	var b strings.Builder
	detailSection(&b, "RUN")
	status := s.Status
	if status == "" {
		status = "unknown"
	}
	kvLine(&b, "Status", runStatusStyle(s.Status)(status))
	if s.SpecName != "" {
		kvLine(&b, "Spec", inkStyle.Render(s.SpecName))
	}
	if s.RunID != "" {
		kvLine(&b, "Run ID", dimStyle.Render(s.RunID))
	}
	if env := runEnvLine(s); env != "" {
		kvLine(&b, "Env", inkStyle.Render(env))
	}
	if s.StartedAt != "" {
		started := inkStyle.Render(relTime(s.StartedAt))
		if s.DurationMS != nil && *s.DurationMS >= 0 {
			started += dimStyle.Render("  ·  " + (time.Duration(*s.DurationMS) * time.Millisecond).String())
		}
		kvLine(&b, "Started", started)
	}
	if s.ExitCode != nil {
		kvLine(&b, "Exit", inkStyle.Render(fmt.Sprintf("%d", *s.ExitCode)))
	}
	kvLine(&b, "Steps", inkStyle.Render(fmt.Sprintf("%d", s.StepCount)))
	kvLine(&b, "Outcomes", outcomeTotals(s))
	switch {
	case s.HasReportHTML && restoredTo != "":
		kvLine(&b, "Report", goodStyle.Render("report.html")+dimStyle.Render("  "+filepath.Join(restoredTo, "report.html")))
	case s.HasReportHTML:
		kvLine(&b, "Report", goodStyle.Render("report.html present")+dimStyle.Render("  (press V to restore and get its path)"))
	default:
		kvLine(&b, "Report", mutedStyle.Render("no report.html in this bundle"))
	}
	if restoredTo != "" {
		kvLine(&b, "Restored", dimStyle.Render(restoredTo))
	}

	if len(s.Outcomes) > 0 {
		title := fmt.Sprintf("OUTCOMES (%d)", len(s.Outcomes))
		if s.OutcomesTruncated {
			title = fmt.Sprintf("OUTCOMES (first %d of %d)", len(s.Outcomes), s.OutcomeCount)
		}
		detailSection(&b, title)
		for _, o := range s.Outcomes {
			mark, style := "·", mutedStyle.Render
			switch o.Status {
			case "passed":
				mark, style = "✓", goodStyle.Render
			case "failed", "errored":
				mark, style = "✗", errorStyle.Render
			case "skipped":
				mark, style = "–", warnStyle.Render
			}
			b.WriteString("  " + style(mark) + " " + inkStyle.Render(o.ID) + dimStyle.Render("  "+o.Status) + "\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func runEnvLine(s detect.RunSummary) string {
	parts := make([]string, 0, 2)
	if s.Environment != "" {
		parts = append(parts, s.Environment)
	}
	if s.Backend != "" {
		parts = append(parts, s.Backend)
	}
	return strings.Join(parts, " · ")
}

func outcomeTotals(s detect.RunSummary) string {
	out := inkStyle.Render(fmt.Sprintf("%d", s.OutcomeCount))
	if s.OutcomeCount == 0 {
		return out
	}
	out += dimStyle.Render("  ·  ") + goodStyle.Render(fmt.Sprintf("%d passed", s.PassedOutcomes))
	out += dimStyle.Render("  ·  ")
	if s.FailedOutcomes > 0 {
		out += errorStyle.Render(fmt.Sprintf("%d failed", s.FailedOutcomes))
	} else {
		out += mutedStyle.Render("0 failed")
	}
	if s.OtherOutcomes > 0 {
		out += dimStyle.Render(fmt.Sprintf("  ·  %d other", s.OtherOutcomes))
	}
	return out
}
