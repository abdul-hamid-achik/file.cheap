// Package detect identifies bundle types from directory structures.
package detect

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// BundleType represents the detected type of a directory.
type BundleType string

const (
	TypeVidtrace BundleType = "vidtrace"
	TypeGeneric  BundleType = "generic"

	// maxSearchableTextBytes caps the synthesized SearchableText so detecting a
	// large stash can't accumulate an unbounded string (OOM / O(n^2) churn).
	maxSearchableTextBytes = 4 << 20 // 4 MiB
	// maxBundleJSONBytes caps reads of bundle JSON (metadata.json / timeline.json)
	// so a malformed or oversized bundle can't exhaust memory.
	maxBundleJSONBytes = 32 << 20 // 32 MiB
)

// readCappedFile reads path but refuses files larger than max bytes.
func readCappedFile(path string, max int64) ([]byte, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if fi.Size() > max {
		return nil, fmt.Errorf("%s too large (%d bytes > %d cap)", filepath.Base(path), fi.Size(), max)
	}
	return os.ReadFile(path)
}

// Result holds detection metadata about a directory.
type Result struct {
	Type            BundleType
	SearchableText  string         // concatenated text content for indexing
	SearchableFiles []string       // paths to text-readable files
	Units           []TextUnit     // structured indexable units (e.g. per-frame evidence)
	Metadata        map[string]any // bundle-specific metadata
}

// TextUnit is a structured, individually-searchable chunk of text that does not
// correspond to a single file on disk -- e.g. one vidtrace timeline entry
// (a frame at a timestamp with its OCR + transcript).
type TextUnit struct {
	Label string // human-readable locator, e.g. "frames/f1.png @ 12s"
	Text  string
}

// TimelineEntry is a flattened vidtrace timeline entry: a frame at a timestamp
// with its OCR text and joined transcript.
type TimelineEntry struct {
	TimeSeconds float64 `json:"time_seconds"`
	Frame       string  `json:"frame"`
	OCR         string  `json:"ocr"`
	Transcript  string  `json:"transcript"`
}

// BundleTypeOf cheaply classifies a directory by structure alone (no content
// read), suitable for the save path where a full Detect would be wasteful.
func BundleTypeOf(dir string) BundleType {
	if fileExists(filepath.Join(dir, "metadata.json")) && fileExists(filepath.Join(dir, "timeline.json")) {
		return TypeVidtrace
	}
	return TypeGeneric
}

// VidtraceMetadata reads and parses contentDir/metadata.json (a vidtrace bundle's
// metadata object). Returns false if it is missing or malformed.
func VidtraceMetadata(dir string) (map[string]any, bool) {
	data, err := readCappedFile(filepath.Join(dir, "metadata.json"), maxBundleJSONBytes)
	if err != nil {
		return nil, false
	}
	var meta map[string]any
	if json.Unmarshal(data, &meta) != nil {
		return nil, false
	}
	return meta, true
}

// ParseVidtraceTimeline reads and flattens contentDir/timeline.json. It returns
// nil if the file is missing or malformed.
func ParseVidtraceTimeline(contentDir string) []TimelineEntry {
	data, err := readCappedFile(filepath.Join(contentDir, "timeline.json"), maxBundleJSONBytes)
	if err != nil {
		return nil
	}
	var tl struct {
		Entries []struct {
			TimeSeconds float64 `json:"time_seconds"`
			Frame       string  `json:"frame"`
			OCR         struct {
				Text string `json:"text"`
			} `json:"ocr"`
			Transcript []struct {
				Text string `json:"text"`
			} `json:"transcript"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(data, &tl); err != nil {
		return nil
	}
	out := make([]TimelineEntry, 0, len(tl.Entries))
	for _, e := range tl.Entries {
		var tr []string
		for _, seg := range e.Transcript {
			if seg.Text != "" {
				tr = append(tr, seg.Text)
			}
		}
		out = append(out, TimelineEntry{
			TimeSeconds: e.TimeSeconds,
			Frame:       e.Frame,
			OCR:         e.OCR.Text,
			Transcript:  strings.Join(tr, " "),
		})
	}
	return out
}

// Detector identifies a specific bundle type.
type Detector interface {
	Detect(dir string) (Result, bool)
}

var detectors = []Detector{
	&vidtraceDetector{},
	&genericDetector{},
}

// Detect runs all detectors and returns the first match.
func Detect(dir string) Result {
	for _, d := range detectors {
		if r, ok := d.Detect(dir); ok {
			return r
		}
	}
	return Result{Type: TypeGeneric}
}

// --- vidtrace detector ---

type vidtraceDetector struct{}

func (d *vidtraceDetector) Detect(dir string) (Result, bool) {
	metaPath := filepath.Join(dir, "metadata.json")
	timelinePath := filepath.Join(dir, "timeline.json")
	if !fileExists(metaPath) || !fileExists(timelinePath) {
		return Result{}, false
	}

	r := Result{
		Type:     TypeVidtrace,
		Metadata: map[string]any{},
	}

	// Parse metadata.json for searchable text. SearchableText is built in a
	// bounded Builder to avoid unbounded accumulation / O(n^2) churn.
	var st strings.Builder
	if data, err := readCappedFile(metaPath, maxBundleJSONBytes); err == nil {
		var meta map[string]any
		if json.Unmarshal(data, &meta) == nil {
			r.Metadata["vidtrace_metadata"] = meta
			if source, ok := meta["source_video"].(string); ok {
				st.WriteString(source)
				st.WriteByte('\n')
			}
		}
	}

	// Flatten the vidtrace timeline into per-entry searchable text + units so a
	// hit can point at the exact frame and timestamp, not a concatenated blob.
	for _, e := range ParseVidtraceTimeline(dir) {
		var parts []string
		if e.OCR != "" {
			if st.Len() < maxSearchableTextBytes {
				st.WriteString(e.OCR)
				st.WriteByte('\n')
			}
			parts = append(parts, e.OCR)
		}
		if e.Transcript != "" {
			if st.Len() < maxSearchableTextBytes {
				st.WriteString(e.Transcript)
				st.WriteByte('\n')
			}
			parts = append(parts, e.Transcript)
		}
		if len(parts) > 0 {
			label := e.Frame
			if label == "" {
				label = "timeline"
			}
			r.Units = append(r.Units, TextUnit{
				Label: fmt.Sprintf("%s @ %.0fs", label, e.TimeSeconds),
				Text:  strings.Join(parts, " "),
			})
		}
	}
	r.SearchableText = st.String()

	// Only index the raw ocr/ and transcript/ text files when the timeline
	// produced no per-frame units. When units exist, that same OCR/transcript
	// text is already indexed per frame, so re-indexing the files (and the
	// combined ocr_all_frames.txt, which lives under ocr/) would double-count it
	// and skew BM25 scores.
	if len(r.Units) == 0 {
		r.SearchableFiles = collectTextFiles(dir, []string{"ocr", "transcript"})
	}

	return r, true
}

// --- generic detector ---

type genericDetector struct{}

func (d *genericDetector) Detect(dir string) (Result, bool) {
	r := Result{
		Type:     TypeGeneric,
		Metadata: map[string]any{},
	}

	// Collect all text-readable files
	r.SearchableFiles = collectTextFiles(dir, nil)

	// Read small text files into searchable text, bounded total (Builder avoids
	// the O(n^2) churn of repeated string concatenation on a large stash).
	var sb strings.Builder
	for _, f := range r.SearchableFiles {
		if sb.Len() >= maxSearchableTextBytes {
			break
		}
		fullPath := filepath.Join(dir, f)
		info, err := os.Stat(fullPath)
		if err != nil || info.Size() > 100*1024 { // skip files > 100KB
			continue
		}
		if data, err := os.ReadFile(fullPath); err == nil && isPrintable(data) {
			sb.Write(data)
			sb.WriteByte('\n')
		}
	}
	r.SearchableText = sb.String()

	return r, true
}

func collectTextFiles(root string, subdirs []string) []string {
	var files []string
	walkFn := func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		switch ext {
		case ".txt", ".md", ".json", ".yaml", ".yml", ".csv", ".tsv",
			".log", ".vtt", ".srt", ".go", ".js", ".ts", ".py", ".rs", ".rb",
			".java", ".c", ".cpp", ".h", ".sh", ".sql", ".html", ".css", ".xml":
			rel, _ := filepath.Rel(root, path)
			files = append(files, rel)
		}
		return nil
	}

	if len(subdirs) > 0 {
		for _, sub := range subdirs {
			subPath := filepath.Join(root, sub)
			if dirExists(subPath) {
				filepath.Walk(subPath, walkFn) //nolint:errcheck
			}
		}
	} else {
		filepath.Walk(root, walkFn) //nolint:errcheck
	}
	return files
}

func isPrintable(data []byte) bool {
	if len(data) == 0 {
		return false
	}
	nonPrintable := 0
	for _, b := range data {
		if b == 0 {
			return false // binary
		}
		if b < 32 && b != '\n' && b != '\r' && b != '\t' {
			nonPrintable++
		}
	}
	return nonPrintable < len(data)/10 // allow up to 10% non-printable
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
