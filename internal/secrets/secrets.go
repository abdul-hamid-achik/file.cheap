// Package secrets scans stash content for likely credentials so fcheap can warn
// before a stash containing live secrets is shared, restored elsewhere, or sealed.
//
// It records only the file, rule, and line of each match -- never the secret
// value itself.
package secrets

import (
	"bufio"
	"bytes"
	"context"
	"io"
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
	findings, _ := ScanContext(context.Background(), dir)
	return findings
}

// ScanContext is Scan with cancellation support. Filesystem/read failures stay
// best-effort, while cancellation is returned so a save can abort before
// committing its manifest.
func ScanContext(ctx context.Context, dir string) ([]Finding, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	rootInfo, err := os.Lstat(dir)
	if err != nil || !rootInfo.IsDir() {
		return nil, nil
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, nil
	}
	defer root.Close() //nolint:errcheck
	openedRootInfo, err := root.Stat(".")
	if err != nil || !openedRootInfo.IsDir() || !os.SameFile(rootInfo, openedRootInfo) {
		return nil, nil
	}

	var findings []Finding
	walkErr := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if err != nil || d.IsDir() {
			return nil
		}
		// WalkDir does not follow symlinked directories. Requiring an Lstat-style
		// regular entry here also skips leaf symlinks, FIFOs, sockets, and devices
		// before any potentially blocking open.
		info, err := d.Info()
		if err != nil || !info.Mode().IsRegular() || info.Size() > maxScanFileBytes {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return nil
		}
		f, err := root.Open(rel)
		if err != nil {
			return nil
		}
		openedInfo, statErr := f.Stat()
		if statErr != nil || !openedInfo.Mode().IsRegular() || !os.SameFile(info, openedInfo) || openedInfo.Size() > maxScanFileBytes {
			_ = f.Close()
			return nil
		}
		fileFindings, scanErr := scanFile(ctx, rel, f)
		findings = append(findings, fileFindings...)
		_ = f.Close()
		return scanErr
	})
	if ctx.Err() != nil {
		return findings, ctx.Err()
	}
	if walkErr != nil {
		return findings, nil // non-cancellation scan errors remain best-effort
	}
	return findings, nil
}

func scanFile(ctx context.Context, rel string, f *os.File) ([]Finding, error) {
	var findings []Finding
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), maxScanFileBytes)
	line := 0
	for sc.Scan() {
		if err := ctx.Err(); err != nil {
			return findings, err
		}
		line++
		text := sc.Text()
		for _, r := range rules {
			if r.re.MatchString(text) {
				findings = append(findings, Finding{File: rel, Rule: r.name, Line: line})
			}
		}
	}
	if sc.Err() != nil {
		// A line exceeded the scan buffer (e.g. a minified single-line file).
		// Fall back to a whole-file regex scan so secrets are not silently
		// missed; line numbers are approximate in this case. The file size was
		// already capped at maxScanFileBytes above. Rewind and read from the same
		// verified descriptor rather than reopening a path that could have changed.
		if err := ctx.Err(); err != nil {
			return findings, err
		}
		if _, seekErr := f.Seek(0, io.SeekStart); seekErr == nil {
			data, rerr := io.ReadAll(io.LimitReader(f, maxScanFileBytes+1))
			if rerr != nil || len(data) > maxScanFileBytes {
				return findings, nil
			}
			for _, r := range rules {
				if loc := r.re.FindIndex(data); loc != nil {
					findings = append(findings, Finding{
						File: rel,
						Rule: r.name,
						Line: 1 + bytes.Count(data[:loc[0]], []byte{'\n'}),
					})
				}
			}
		}
	}
	return findings, ctx.Err()
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
