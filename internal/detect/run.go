package detect

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	maxRunJSONBytes      = 1 << 20 // Run metadata should stay small; evidence lives in separate files.
	maxManifestJSONBytes = 4 << 20
	maxRunMetadataString = 512
)

var runEvidencePathPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+(?:/[A-Za-z0-9._-]+)*$`)

type cairntraceRunDetector struct{}

func (d *cairntraceRunDetector) Detect(dir string) (Result, bool) {
	hasManifest := regularFileExists(dir, "artifact-manifest.json")
	hasRun := regularFileExists(dir, "run.json")
	if !hasManifest && !hasRun {
		return Result{}, false
	}

	run, parsed := readNativeRun(dir)
	if parsed && !isCairntraceRun(run) {
		return Result{}, false
	}
	if !parsed && !hasManifest {
		return Result{}, false
	}

	return nativeRunResult(dir, TypeCairntraceRun, run, "artifact-manifest.json", hasManifest), true
}

type glyphrunRunDetector struct{}

func (d *glyphrunRunDetector) Detect(dir string) (Result, bool) {
	hasRun := regularFileExists(dir, "run.json")
	hasManifest := regularFileExists(dir, "manifest.json")
	if !hasRun {
		return Result{}, false
	}

	run, parsed := readNativeRun(dir)
	if parsed && !isGlyphrunRun(run) {
		return Result{}, false
	}
	// run.json + manifest.json is Glyphrun's durable directory shape. Treat an
	// unreadable run with that shape as Glyphrun so generic detection cannot
	// accidentally index raw diagnostics or terminal evidence from the bundle.
	if !parsed && !hasManifest {
		return Result{}, false
	}

	return nativeRunResult(dir, TypeGlyphrunRun, run, "manifest.json", hasManifest), true
}

type nativeRun struct {
	raw          map[string]json.RawMessage
	schema       string
	runID        string
	specName     string
	status       string
	startedAt    string
	endedAt      string
	environment  string
	backend      string
	errorKind    string
	durationMs   *int64
	exitCode     *int64
	stepCount    int
	outcomeCount int
}

func readNativeRun(dir string) (nativeRun, bool) {
	data, err := readCappedFile(dir, "run.json", maxRunJSONBytes)
	if err != nil {
		return nativeRun{}, false
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nativeRun{}, false
	}
	run := nativeRun{
		raw:          raw,
		schema:       jsonString(raw["$schema"]),
		runID:        jsonString(raw["runId"]),
		specName:     jsonString(raw["specName"]),
		status:       normalizedRunStatus(jsonString(raw["status"])),
		startedAt:    jsonString(raw["startedAt"]),
		endedAt:      jsonString(raw["endedAt"]),
		environment:  jsonString(raw["environment"]),
		backend:      jsonString(raw["backend"]),
		errorKind:    jsonString(raw["errorKind"]),
		durationMs:   jsonInt64(raw["durationMs"]),
		exitCode:     jsonInt64(raw["exitCode"]),
		stepCount:    jsonArrayLength(raw["steps"]),
		outcomeCount: jsonArrayLength(raw["outcomes"]),
	}
	if run.specName == "" {
		var spec map[string]json.RawMessage
		if json.Unmarshal(raw["spec"], &spec) == nil {
			run.specName = jsonString(spec["name"])
		}
	}
	return run, true
}

func isCairntraceRun(run nativeRun) bool {
	if strings.HasPrefix(run.schema, "urn:cairntrace.dev:run:") {
		return true
	}
	if run.specName == "" {
		return false
	}
	_, hasVersion := run.raw["version"]
	_, hasSummary := run.raw["summary"]
	_, hasBackend := run.raw["backend"]
	return hasVersion && (hasSummary || hasBackend)
}

func isGlyphrunRun(run nativeRun) bool {
	if strings.HasPrefix(run.schema, "urn:glyphrun.dev:run:") {
		return true
	}
	if run.specName == "" {
		return false
	}
	_, hasSchemaVersion := run.raw["schemaVersion"]
	_, hasTarget := run.raw["target"]
	_, hasTerminal := run.raw["terminal"]
	return hasSchemaVersion && (hasTarget || hasTerminal)
}

func nativeRunResult(dir string, bundleType BundleType, run nativeRun, manifestName string, hasManifest bool) Result {
	metadata := map[string]any{}
	searchFields := []string{string(bundleType)}
	addSafeRunString(metadata, &searchFields, "schema", run.schema)
	addSafeRunString(metadata, &searchFields, "run_id", run.runID)
	addSafeRunString(metadata, &searchFields, "spec_name", run.specName)
	addSafeRunString(metadata, &searchFields, "status", run.status)
	addSafeRunString(metadata, &searchFields, "started_at", run.startedAt)
	addSafeRunString(metadata, &searchFields, "ended_at", run.endedAt)
	addSafeRunString(metadata, &searchFields, "environment", run.environment)
	addSafeRunString(metadata, &searchFields, "backend", run.backend)
	addSafeRunString(metadata, &searchFields, "error_kind", run.errorKind)
	if run.durationMs != nil && *run.durationMs >= 0 {
		metadata["duration_ms"] = *run.durationMs
	}
	if run.exitCode != nil {
		metadata["exit_code"] = *run.exitCode
	}
	metadata["step_count"] = run.stepCount
	metadata["outcome_count"] = run.outcomeCount

	stats := nativeManifestStats{}
	manifestAvailable := false
	if hasManifest {
		metadata["manifest_file"] = manifestName
		if got, ok := manifestArtifactStats(dir, manifestName); ok {
			stats = got
			manifestAvailable = true
			metadata["artifact_count"] = stats.declared
		}
	} else if got, ok := inlineManifestArtifactStats(dir, run); ok {
		stats = got
		manifestAvailable = true
		metadata["artifact_count"] = stats.declared
	}

	return Result{
		Type:           bundleType,
		SearchableText: strings.Join(searchFields, "\n") + "\n",
		Metadata:       metadata,
		Run: &NativeRunMetadata{
			Schema:            run.schema,
			RunID:             run.runID,
			SpecName:          run.specName,
			Status:            run.status,
			StartedAt:         run.startedAt,
			EndedAt:           run.endedAt,
			Environment:       run.environment,
			Backend:           run.backend,
			ErrorKind:         run.errorKind,
			DurationMS:        run.durationMs,
			ExitCode:          run.exitCode,
			StepCount:         run.stepCount,
			OutcomeCount:      run.outcomeCount,
			ArtifactCount:     stats.declared,
			ManifestAvailable: manifestAvailable,
			SchemaDrift:       stats.schemaDrift,
			PresentCount:      stats.present,
			EmptyCount:        stats.empty,
			MissingCount:      stats.missing,
			ChangedCount:      stats.changed,
		},
	}
}

type nativeManifestStats struct {
	declared    int
	present     int
	empty       int
	missing     int
	changed     int
	schemaDrift bool
}

func manifestArtifactStats(dir, name string) (nativeManifestStats, bool) {
	data, err := readCappedFile(dir, name, maxManifestJSONBytes)
	if err != nil {
		return nativeManifestStats{}, false
	}
	entries, ok := manifestEntries(data)
	if !ok {
		return nativeManifestStats{schemaDrift: true}, false
	}
	return inspectManifestEntries(dir, entries), true
}

func inlineManifestArtifactStats(dir string, run nativeRun) (nativeManifestStats, bool) {
	entries, ok := manifestEntryArray(run.raw["manifest"])
	if !ok {
		return nativeManifestStats{}, false
	}
	return inspectManifestEntries(dir, entries), true
}

func manifestEntries(data []byte) ([]map[string]json.RawMessage, bool) {
	if entries, ok := manifestEntryArray(data); ok {
		return entries, true
	}
	var envelope map[string]json.RawMessage
	if json.Unmarshal(data, &envelope) != nil {
		return nil, false
	}
	return manifestEntryArray(envelope["artifacts"])
}

func manifestEntryArray(raw []byte) ([]map[string]json.RawMessage, bool) {
	if len(raw) == 0 {
		return nil, false
	}
	var entries []map[string]json.RawMessage
	if json.Unmarshal(raw, &entries) != nil {
		return nil, false
	}
	return entries, true
}

func inspectManifestEntries(dir string, entries []map[string]json.RawMessage) nativeManifestStats {
	stats := nativeManifestStats{declared: len(entries)}
	root, err := openStableRoot(dir)
	if err != nil {
		stats.schemaDrift = true
		return stats
	}
	defer root.Close() //nolint:errcheck

	for _, entry := range entries {
		name := jsonString(entry["path"])
		if name == "" {
			name = jsonString(entry["relativePath"])
		}
		if !safeRunEvidencePath(name) {
			stats.schemaDrift = true
			continue
		}
		info, err := root.Lstat(filepath.FromSlash(name))
		if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			stats.missing++
			continue
		}
		stats.present++
		if info.Size() == 0 {
			stats.empty++
		}
		if declaredBytes := jsonInt64(entry["bytes"]); declaredBytes != nil && *declaredBytes >= 0 && info.Size() != *declaredBytes {
			stats.changed++
		}
	}
	return stats
}

func safeRunEvidencePath(name string) bool {
	if name == "" || len(name) > 512 || !runEvidencePathPattern.MatchString(name) {
		return false
	}
	for _, part := range strings.Split(name, "/") {
		if part == "." || part == ".." {
			return false
		}
	}
	return true
}

func jsonString(raw json.RawMessage) string {
	var value string
	if json.Unmarshal(raw, &value) != nil {
		return ""
	}
	return safeRunMetadataString(value)
}

func jsonInt64(raw json.RawMessage) *int64 {
	var value int64
	if json.Unmarshal(raw, &value) != nil {
		return nil
	}
	return &value
}

func jsonArrayLength(raw json.RawMessage) int {
	count, ok := jsonArrayCount(raw)
	if !ok {
		return 0
	}
	return count
}

func jsonArrayCount(raw json.RawMessage) (int, bool) {
	if len(raw) == 0 {
		return 0, false
	}
	var values []json.RawMessage
	if json.Unmarshal(raw, &values) != nil {
		return 0, false
	}
	return len(values), true
}

func normalizedRunStatus(value string) string {
	switch value {
	case "queued", "running", "passed", "failed", "errored", "cancelled", "incomplete", "unknown":
		return value
	case "":
		return ""
	default:
		return "unknown"
	}
}

func addSafeRunString(metadata map[string]any, searchFields *[]string, key, value string) {
	value = safeRunMetadataString(value)
	if value == "" {
		return
	}
	metadata[key] = value
	*searchFields = append(*searchFields, value)
}

func safeRunMetadataString(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > maxRunMetadataString || !utf8.ValidString(value) {
		return ""
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return ""
		}
	}
	return value
}
