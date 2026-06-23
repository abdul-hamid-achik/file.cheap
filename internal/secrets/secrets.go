// Package secrets scans stash content for likely credentials so fcheap can warn
// before a stash containing live secrets is shared, restored elsewhere, or sealed.
//
// It records only the file, rule, and line of each match -- never the secret
// value itself.
package secrets

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
)

// maxScanFileBytes skips files larger than this (likely data, not config).
const maxScanFileBytes = 1 << 20 // 1 MiB

// Finding is a single likely-secret match. The secret value is never stored.
type Finding struct {
	File string `json:"file"`
	Rule string `json:"rule"`
	Line int    `json:"line"`
}

var rules = []struct {
	name string
	re   *regexp.Regexp
}{
	{"aws-access-key", regexp.MustCompile(`AKIA[0-9A-Z]{16}`)},
	{"github-token", regexp.MustCompile(`gh[pousr]_[A-Za-z0-9]{20,}`)},
	{"slack-token", regexp.MustCompile(`xox[baprs]-[A-Za-z0-9-]{10,}`)},
	{"google-api-key", regexp.MustCompile(`AIza[0-9A-Za-z\-_]{35}`)},
	{"private-key", regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`)},
	{"jwt", regexp.MustCompile(`eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}`)},
	{"generic-secret", regexp.MustCompile(`(?i)(api[_-]?key|secret|token|password|passwd|access[_-]?key)["'\s]*[:=]["'\s]*[A-Za-z0-9_\-/+]{16,}`)},
}

// Scan walks dir and returns likely-secret findings across its text files.
func Scan(dir string) []Finding {
	var findings []Finding
	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if info, ierr := d.Info(); ierr == nil && info.Size() > maxScanFileBytes {
			return nil
		}
		findings = append(findings, scanFile(dir, path)...)
		return nil
	})
	return findings
}

func scanFile(root, path string) []Finding {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close() //nolint:errcheck

	rel, err := filepath.Rel(root, path)
	if err != nil {
		rel = path
	}

	var findings []Finding
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), maxScanFileBytes)
	line := 0
	for sc.Scan() {
		line++
		text := sc.Text()
		for _, r := range rules {
			if r.re.MatchString(text) {
				findings = append(findings, Finding{File: rel, Rule: r.name, Line: line})
			}
		}
	}
	return findings
}

// Rules returns the distinct rule names that matched in a set of findings.
func Rules(findings []Finding) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, fnd := range findings {
		if _, ok := seen[fnd.Rule]; !ok {
			seen[fnd.Rule] = struct{}{}
			out = append(out, fnd.Rule)
		}
	}
	return out
}
