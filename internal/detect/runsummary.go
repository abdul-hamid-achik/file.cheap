package detect

import (
	"encoding/json"
)

const (
	maxReportJSONBytes = 4 << 20
	// MaxRunSummaryOutcomes bounds how many outcome rows ReadRunSummary returns.
	MaxRunSummaryOutcomes = 50
)

// RunOutcome is one outcome row from a run's run.json or report.json.
type RunOutcome struct {
	ID     string
	Status string // passed, failed, skipped, errored, or unknown
}

// RunSummary is the bounded, human-oriented projection of a cairntrace or
// glyphrun run directory used by the Studio run view. Every string passed
// through safeRunMetadataString, so it is free of control characters and
// length-capped. Counts cover every outcome; Outcomes holds only the first
// MaxRunSummaryOutcomes rows.
type RunSummary struct {
	Type              BundleType
	RunID             string
	SpecName          string
	Status            string
	Environment       string
	Backend           string
	StartedAt         string
	EndedAt           string
	DurationMS        *int64
	ExitCode          *int64
	StepCount         int
	OutcomeCount      int
	PassedOutcomes    int
	FailedOutcomes    int
	OtherOutcomes     int
	Outcomes          []RunOutcome
	OutcomesTruncated bool
	HasReportHTML     bool
	HasReportJSON     bool
}

// ReadRunSummary summarizes dir when it is a cairntrace-run or glyphrun-run
// bundle. It reads only run.json and report.json (size-capped, confined to
// dir, symlinks rejected) and stats report.html; it never reads evidence
// files. report.json fills any field run.json did not provide.
func ReadRunSummary(dir string) (RunSummary, bool) {
	res := Detect(dir)
	if (res.Type != TypeCairntraceRun && res.Type != TypeGlyphrunRun) || res.Run == nil {
		return RunSummary{}, false
	}
	r := res.Run
	s := RunSummary{
		Type:          res.Type,
		RunID:         r.RunID,
		SpecName:      r.SpecName,
		Status:        r.Status,
		Environment:   r.Environment,
		Backend:       r.Backend,
		StartedAt:     r.StartedAt,
		EndedAt:       r.EndedAt,
		DurationMS:    r.DurationMS,
		ExitCode:      r.ExitCode,
		StepCount:     r.StepCount,
		OutcomeCount:  r.OutcomeCount,
		HasReportHTML: regularFileExists(dir, "report.html"),
		HasReportJSON: regularFileExists(dir, "report.json"),
	}

	var outcomes []json.RawMessage
	if run, ok := readNativeRun(dir); ok {
		outcomes, _ = rawArray(run.raw["outcomes"])
	}
	if s.HasReportJSON {
		if report, ok := readReportJSON(dir); ok {
			s.fillFromReport(report, &outcomes)
		}
	}
	s.tallyOutcomes(outcomes)
	return s, true
}

func (s *RunSummary) fillFromReport(report map[string]json.RawMessage, outcomes *[]json.RawMessage) {
	var run map[string]json.RawMessage
	if json.Unmarshal(report["run"], &run) == nil {
		fill := func(dst *string, key string) {
			if *dst == "" {
				*dst = jsonString(run[key])
			}
		}
		fill(&s.RunID, "runId")
		fill(&s.SpecName, "specName")
		fill(&s.Environment, "environment")
		fill(&s.Backend, "backend")
		fill(&s.StartedAt, "startedAt")
		fill(&s.EndedAt, "endedAt")
		if s.Status == "" {
			s.Status = normalizedRunStatus(jsonString(run["status"]))
		}
		if s.DurationMS == nil {
			s.DurationMS = jsonInt64(run["durationMs"])
		}
		if s.ExitCode == nil {
			s.ExitCode = jsonInt64(run["exitCode"])
		}
	}
	if s.StepCount == 0 {
		if n, ok := jsonArrayCount(report["steps"]); ok {
			s.StepCount = n
		}
	}
	if len(*outcomes) == 0 {
		if rows, ok := rawArray(report["outcomes"]); ok {
			*outcomes = rows
		}
	}
}

func (s *RunSummary) tallyOutcomes(rows []json.RawMessage) {
	if len(rows) > 0 || s.OutcomeCount == 0 {
		s.OutcomeCount = len(rows)
	}
	for _, row := range rows {
		var o map[string]json.RawMessage
		if json.Unmarshal(row, &o) != nil {
			s.OtherOutcomes++
			continue
		}
		status := normalizedOutcomeStatus(jsonString(o["status"]))
		switch status {
		case "passed":
			s.PassedOutcomes++
		case "failed":
			s.FailedOutcomes++
		default:
			s.OtherOutcomes++
		}
		if len(s.Outcomes) >= MaxRunSummaryOutcomes {
			s.OutcomesTruncated = true
			continue
		}
		id := jsonString(o["id"])
		if id == "" {
			id = "(unnamed)"
		}
		s.Outcomes = append(s.Outcomes, RunOutcome{ID: id, Status: status})
	}
}

func readReportJSON(dir string) (map[string]json.RawMessage, bool) {
	data, err := readCappedFile(dir, "report.json", maxReportJSONBytes)
	if err != nil {
		return nil, false
	}
	var report map[string]json.RawMessage
	if json.Unmarshal(data, &report) != nil {
		return nil, false
	}
	return report, true
}

func rawArray(raw json.RawMessage) ([]json.RawMessage, bool) {
	if len(raw) == 0 {
		return nil, false
	}
	var rows []json.RawMessage
	if json.Unmarshal(raw, &rows) != nil {
		return nil, false
	}
	return rows, true
}

func normalizedOutcomeStatus(value string) string {
	switch value {
	case "passed", "failed", "skipped", "errored":
		return value
	default:
		return "unknown"
	}
}
