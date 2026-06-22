// Package detect identifies bundle types from directory structures.
package detect

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// BundleType represents the detected type of a directory.
type BundleType string

const (
	TypeVidtrace BundleType = "vidtrace"
	TypeGeneric  BundleType = "generic"
)

// Result holds detection metadata about a directory.
type Result struct {
	Type             BundleType
	SearchableText   string // concatenated text content for indexing
	SearchableFiles  []string // paths to text-readable files
	Metadata         map[string]any // bundle-specific metadata
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
		Type:    TypeVidtrace,
		Metadata: map[string]any{},
	}

	// Parse metadata.json for searchable text
	if data, err := os.ReadFile(metaPath); err == nil {
		var meta map[string]any
		if json.Unmarshal(data, &meta) == nil {
			r.Metadata["vidtrace_metadata"] = meta
			if source, ok := meta["source_video"].(string); ok {
				r.SearchableText += source + "\n"
			}
		}
	}

	// Parse timeline.json for searchable text (OCR + transcripts)
	if data, err := os.ReadFile(timelinePath); err == nil {
		var tl []map[string]any
		if json.Unmarshal(data, &tl) == nil {
			for _, entry := range tl {
				if ocr, ok := entry["ocr_text"].(string); ok && ocr != "" {
					r.SearchableText += ocr + "\n"
				}
				if transcript, ok := entry["transcript"].(string); ok && transcript != "" {
					r.SearchableText += transcript + "\n"
				}
			}
		}
	}

	// Collect text files from ocr/ and transcript/ directories
	r.SearchableFiles = collectTextFiles(dir, []string{"ocr", "transcript"})

	// Also read ocr_all_frames.txt
	ocrAll := filepath.Join(dir, "ocr", "ocr_all_frames.txt")
	if data, err := os.ReadFile(ocrAll); err == nil {
		r.SearchableText += string(data) + "\n"
	}

	return r, true
}

// --- generic detector ---

type genericDetector struct{}

func (d *genericDetector) Detect(dir string) (Result, bool) {
	r := Result{
		Type:    TypeGeneric,
		Metadata: map[string]any{},
	}

	// Collect all text-readable files
	r.SearchableFiles = collectTextFiles(dir, nil)

	// Read small text files into searchable text
	for _, f := range r.SearchableFiles {
		fullPath := filepath.Join(dir, f)
		info, err := os.Stat(fullPath)
		if err != nil || info.Size() > 100*1024 { // skip files > 100KB
			continue
		}
		if data, err := os.ReadFile(fullPath); err == nil {
			if isPrintable(data) {
				r.SearchableText += string(data) + "\n"
			}
		}
	}

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