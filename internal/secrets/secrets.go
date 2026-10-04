// Package secrets scans stash content for likely credentials so fcheap can warn
// before a stash containing live secrets is shared, restored elsewhere, or sealed.
//
// It records only the file, rule, and line of each match -- never the secret
// value itself.
//
// Files are scanned as streams (no per-file size cap) under a per-scan byte
// budget, and ZIP archives (including Playwright trace zips) have their
// text-like members scanned in memory without extracting anything to disk.
// Anything not inspected is counted in the Report with a reason, so an empty
// findings list is never mistaken for "everything was checked".
package secrets

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const (
	// DefaultBudgetBytes is the default total number of content bytes one scan
	// inspects. Files that would exceed it are counted as skipped (SkipBudget).
	DefaultBudgetBytes int64 = 256 << 20

	// MaxZipMemberBytes caps how many decompressed bytes of one archive member
	// are scanned.
	MaxZipMemberBytes int64 = 64 << 20

	// maxZipRatio is the largest decompressed:compressed ratio accepted for an
	// archive member once it is big enough for the ratio to matter.
	maxZipRatio       = 200
	zipRatioFloorSize = 1 << 20

	// chunkSize is the read granularity of the streaming scanner.
	chunkSize = 128 << 10
	// maxPendingLine bounds how much of one unterminated line is held in memory.
	// A longer line (minified JSON) is scanned in fragments that overlap by
	// overlapBytes, so a match straddling a fragment boundary is still found.
	maxPendingLine = 1 << 20
	overlapBytes   = 4 << 10
)

// Finding is a single likely-secret match. The secret value is never stored.
type Finding struct {
	File string `json:"file"`
	Rule string `json:"rule"`
	Line int    `json:"line"`
}

type rule struct {
	name string
	re   *regexp.Regexp
	// check, when set, must also accept the matched text. It lets a rule stay
	// strict where RE2 has no lookahead.
	check func(match []byte) bool
	// hints are lowercase literals; a line is only run through re when it contains
	// at least one (case-insensitively). This keeps the scan fast on large files.
	hints []string
}

const (
	tokenChars   = `[A-Za-z0-9._~+/=-]`
	cookieValue  = `[A-Za-z0-9%._~+/=-]{16,}`
	cookieBound  = `(?:^|[\s;:,"'])`
	sessionNames = `(?:[A-Za-z0-9_.-]*(?:session|sessid|token|auth)[A-Za-z0-9_.-]*|(?:[A-Za-z0-9]+[_.-])*sid(?:[_.-][A-Za-z0-9]+)*)`
)

var rules = []rule{
	{name: "aws-access-key", hints: []string{"akia"}, re: regexp.MustCompile(`AKIA[0-9A-Z]{16}`)},
	{name: "github-token", hints: []string{"ghp_", "gho_", "ghu_", "ghs_", "ghr_"}, re: regexp.MustCompile(`gh[pousr]_[A-Za-z0-9]{20,}`)},
	{name: "slack-token", hints: []string{"xox"}, re: regexp.MustCompile(`xox[baprs]-[A-Za-z0-9-]{10,}`)},
	{name: "google-api-key", hints: []string{"aiza"}, re: regexp.MustCompile(`AIza[0-9A-Za-z\-_]{35}`)},
	{name: "private-key", hints: []string{"private key"}, re: regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`)},
	{name: "jwt", hints: []string{"eyj"}, re: regexp.MustCompile(`eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}`)},
	{name: "generic-secret", hints: []string{"key", "secret", "token", "password", "passwd"}, re: regexp.MustCompile(`(?i)(api[_-]?key|secret|token|password|passwd|access[_-]?key)["'\s]*[:=]["'\s]*[A-Za-z0-9_\-/+]{16,}`)},

	// Credentials that appear in captured network evidence (HAR, Playwright
	// trace.network). The header rules cover the plain "Name: value" form and
	// the JSON {"name":...,"value":...} shape those formats use.
	{name: "bearer-token", hints: []string{"bearer"}, re: regexp.MustCompile(
		`(?i)authorization["']?\s*[:=]\s*["']?bearer\s+` + tokenChars + `{20,}` +
			`|["']value["']\s*:\s*["']bearer\s+` + tokenChars + `{20,}`)},
	{name: "cookie-session", hints: []string{"cookie"}, re: regexp.MustCompile(
		`(?i)\b(?:set-)?cookie["']?\s*[:=]\s*["']?[^\n]*?` + cookieBound + sessionNames + `=` + cookieValue +
			`|["']name["']\s*:\s*["'](?:set-)?cookie["']\s*,\s*["']value["']\s*:\s*["'](?:[^\n"']*?[\s;])?` + sessionNames + `=` + cookieValue)},
	{name: "sk-api-key", hints: []string{"sk-"}, re: regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{20,}`), check: hasLetterAndDigit},
	{name: "stripe-secret-key", hints: []string{"sk_"}, re: regexp.MustCompile(`\bsk_(?:live|test)_[A-Za-z0-9]{16,}`)},
	{name: "digitalocean-token", hints: []string{"dop_v1_"}, re: regexp.MustCompile(`\bdop_v1_[a-f0-9]{64}`)},
	{name: "npm-token", hints: []string{"npm_"}, re: regexp.MustCompile(`\bnpm_[A-Za-z0-9]{36}`)},
}

// hasLetterAndDigit rejects hyphenated prose such as CSS class names that
// happen to start with "sk-": real keys are random and contain both.
func hasLetterAndDigit(match []byte) bool {
	var letter, digit bool
	for _, c := range match {
		switch {
		case c >= '0' && c <= '9':
			digit = true
		case (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z'):
			letter = true
		}
	}
	return letter && digit
}

func (r rule) matches(line, lower []byte) bool {
	if !hasAnyHint(lower, r.hints) {
		return false
	}
	if r.check == nil {
		return r.re.Match(line)
	}
	for _, m := range r.re.FindAll(line, -1) {
		if r.check(m) {
			return true
		}
	}
	return false
}

func hasAnyHint(lower []byte, hints []string) bool {
	if len(hints) == 0 {
		return true
	}
	for _, h := range hints {
		if bytes.Contains(lower, []byte(h)) {
			return true
		}
	}
	return false
}

// asciiLower lowercases ASCII letters of src into dst (reused across lines).
func asciiLower(dst, src []byte) []byte {
	dst = append(dst[:0], src...)
	for i, c := range dst {
		if c >= 'A' && c <= 'Z' {
			dst[i] = c + 32
		}
	}
	return dst
}

// Reasons a file is counted as skipped rather than scanned.
const (
	SkipNotRegular = "not_regular" // symlink, FIFO, socket, or device
	SkipUnreadable = "unreadable"  // could not be opened, or changed while opening
	SkipBudget     = "budget"      // the scan's total byte budget ran out first
	SkipZipGuard   = "zip_guard"   // archive member over the size or ratio guard
)

// Options tunes one scan. The zero value uses the defaults.
type Options struct {
	// BudgetBytes is the total content bytes the scan may inspect. Zero uses
	// DefaultBudgetBytes; a negative value disables the budget.
	BudgetBytes int64
}

// Report is the outcome of one scan: the findings plus how much of the tree was
// actually inspected, so an empty Findings slice is never mistaken for
// "everything was checked". It carries counts only, never content.
type Report struct {
	Findings       []Finding
	FilesScanned   int
	FilesSkipped   int
	SkippedReasons map[string]int
	// BytesScanned is the content bytes inspected (decompressed bytes for
	// archive members).
	BytesScanned int64
}

func (r *Report) skip(reason string) {
	r.FilesSkipped++
	if r.SkippedReasons == nil {
		r.SkippedReasons = map[string]int{}
	}
	r.SkippedReasons[reason]++
}

// Scan walks dir and returns likely-secret findings across its files.
func Scan(dir string) []Finding {
	findings, _ := ScanContext(context.Background(), dir)
	return findings
}

// ScanContext is Scan with cancellation support. Filesystem/read failures stay
// best-effort, while cancellation is returned so a save can abort before
// committing its manifest.
func ScanContext(ctx context.Context, dir string) ([]Finding, error) {
	report, err := ScanReportContext(ctx, dir)
	return report.Findings, err
}

// ScanReportContext is ScanContext that also accounts for every file it did not
// fully scan, using the default options.
func ScanReportContext(ctx context.Context, dir string) (Report, error) {
	return ScanReportOptions(ctx, dir, Options{})
}

type candidate struct {
	rel  string
	info os.FileInfo
}

// budget tracks the bytes the scan may still inspect. A nil budget is unlimited.
type budget struct{ remaining int64 }

func newBudget(limit int64) *budget {
	if limit < 0 {
		return nil
	}
	if limit == 0 {
		limit = DefaultBudgetBytes
	}
	return &budget{remaining: limit}
}

// left returns the remaining bytes, or -1 when unlimited.
func (b *budget) left() int64 {
	if b == nil {
		return -1
	}
	return b.remaining
}

func (b *budget) exhausted() bool { return b != nil && b.remaining <= 0 }

func (b *budget) spend(n int64) {
	if b != nil {
		b.remaining -= n
		if b.remaining < 0 {
			b.remaining = 0
		}
	}
}

// ScanReportOptions is ScanReportContext with explicit options.
//
// Files are scanned smallest first so a large trace or video cannot exhaust
// the budget before small, high-value files such as .env are inspected.
func ScanReportOptions(ctx context.Context, dir string, opts Options) (Report, error) {
	var report Report
	if err := ctx.Err(); err != nil {
		return report, err
	}
	rootInfo, err := os.Lstat(dir)
	if err != nil || !rootInfo.IsDir() {
		return report, nil
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return report, nil
	}
	defer root.Close() //nolint:errcheck
	openedRootInfo, err := root.Stat(".")
	if err != nil || !openedRootInfo.IsDir() || !os.SameFile(rootInfo, openedRootInfo) {
		return report, nil
	}

	var candidates []candidate
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
		if err != nil {
			report.skip(SkipUnreadable)
			return nil
		}
		if !info.Mode().IsRegular() {
			report.skip(SkipNotRegular)
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			report.skip(SkipUnreadable)
			return nil
		}
		candidates = append(candidates, candidate{rel: rel, info: info})
		return nil
	})
	if ctx.Err() != nil {
		return report, ctx.Err()
	}
	_ = walkErr // non-cancellation walk errors remain best-effort

	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].info.Size() < candidates[j].info.Size()
	})

	b := newBudget(opts.BudgetBytes)
	for _, c := range candidates {
		if err := ctx.Err(); err != nil {
			return report, err
		}
		if err := scanCandidate(ctx, root, c, b, &report); err != nil {
			return report, err
		}
	}
	// Smallest-first order is an implementation detail; report in path order.
	sort.SliceStable(report.Findings, func(i, j int) bool {
		return report.Findings[i].File < report.Findings[j].File
	})
	return report, nil
}

func scanCandidate(ctx context.Context, root *os.Root, c candidate, b *budget, report *Report) error {
	f, err := root.Open(c.rel)
	if err != nil {
		report.skip(SkipUnreadable)
		return nil
	}
	defer f.Close() //nolint:errcheck
	openedInfo, statErr := f.Stat()
	if statErr != nil || !openedInfo.Mode().IsRegular() || !os.SameFile(c.info, openedInfo) {
		report.skip(SkipUnreadable)
		return nil
	}

	if strings.HasSuffix(strings.ToLower(c.rel), ".zip") {
		handled, err := scanZip(ctx, c.rel, f, openedInfo.Size(), b, report)
		if err != nil || handled {
			return err
		}
		// Not a ZIP container: fall through and scan the raw bytes.
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			report.skip(SkipUnreadable)
			return nil
		}
	}

	if b.exhausted() {
		report.skip(SkipBudget)
		return nil
	}
	res, err := scanStream(ctx, c.rel, f, b.left())
	report.Findings = append(report.Findings, res.findings...)
	report.BytesScanned += res.read
	b.spend(res.read)
	switch {
	case err != nil && ctx.Err() != nil:
		return ctx.Err()
	case err != nil:
		report.skip(SkipUnreadable)
	case res.hitLimit:
		report.skip(SkipBudget)
	default:
		report.FilesScanned++
	}
	return nil
}

// zipMemberScannable reports whether an archive member is text-like evidence
// worth scanning (network captures, logs, JSON) rather than media.
func zipMemberScannable(name string) bool {
	base := strings.ToLower(name)
	if i := strings.LastIndexAny(base, "/\\"); i >= 0 {
		base = base[i+1:]
	}
	if strings.HasSuffix(base, "trace.network") || strings.HasSuffix(base, "trace.trace") {
		return true
	}
	for _, ext := range []string{".har", ".ndjson", ".json", ".txt", ".log"} {
		if strings.HasSuffix(base, ext) {
			return true
		}
	}
	return false
}

// scanZip scans the text-like members of one archive in memory (streamed, never
// extracted to disk). It returns handled=false only when the file is not a ZIP
// container, so the caller can scan it as a plain file instead.
//
// The archive counts as scanned only when every matching member was fully
// inspected. A member over the size/ratio guard counts the archive as skipped
// (zip_guard), and so does running out of budget (budget); findings from the
// members that were inspected are kept either way.
func scanZip(ctx context.Context, rel string, f *os.File, size int64, b *budget, report *Report) (bool, error) {
	zr, err := zip.NewReader(f, size)
	if err != nil {
		if errors.Is(err, zip.ErrFormat) {
			return false, nil
		}
		report.skip(SkipUnreadable)
		return true, nil
	}
	var guard, overBudget, unreadable bool
	for _, member := range zr.File {
		if err := ctx.Err(); err != nil {
			return true, err
		}
		if member.FileInfo().IsDir() || !zipMemberScannable(member.Name) {
			continue
		}
		if b.exhausted() {
			overBudget = true
			break
		}
		limit := MaxZipMemberBytes
		if ratioLimit := max(int64(member.CompressedSize64)*maxZipRatio, zipRatioFloorSize); ratioLimit < limit {
			limit = ratioLimit
		}
		budgetBound := false
		if left := b.left(); left >= 0 && left < limit {
			limit, budgetBound = left, true
		}
		// A declared size over the limit is rejected before decompressing. The
		// declared size can lie, so the read below is bounded as well.
		if member.UncompressedSize64 > uint64(limit) {
			if budgetBound {
				overBudget = true
			} else {
				guard = true
			}
			continue
		}
		rc, err := member.Open()
		if err != nil {
			unreadable = true
			continue
		}
		res, err := scanStream(ctx, rel+"!"+sanitizeName(member.Name), rc, limit)
		_ = rc.Close()
		report.Findings = append(report.Findings, res.findings...)
		report.BytesScanned += res.read
		b.spend(res.read)
		switch {
		case err != nil && ctx.Err() != nil:
			return true, ctx.Err()
		case err != nil:
			unreadable = true
		case res.hitLimit && budgetBound:
			overBudget = true
		case res.hitLimit:
			guard = true
		}
	}
	switch {
	case guard:
		report.skip(SkipZipGuard)
	case overBudget:
		report.skip(SkipBudget)
	case unreadable:
		report.skip(SkipUnreadable)
	default:
		report.FilesScanned++
	}
	return true, nil
}

func sanitizeName(name string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return '?'
		}
		return r
	}, name)
}

// streamResult is the outcome of scanning one reader.
type streamResult struct {
	findings []Finding
	read     int64
	hitLimit bool // more data remained after limit bytes were read
}

// limitedReader reads at most limit bytes (limit < 0 = unlimited) and records
// whether the underlying reader had more.
type limitedReader struct {
	r        io.Reader
	limit    int64
	read     int64
	exceeded bool
}

func (l *limitedReader) Read(p []byte) (int, error) {
	if l.limit >= 0 && l.read >= l.limit {
		var probe [1]byte
		n, err := l.r.Read(probe[:])
		if n > 0 {
			l.exceeded = true
			return 0, io.EOF
		}
		return 0, err
	}
	if l.limit >= 0 && int64(len(p)) > l.limit-l.read {
		p = p[:l.limit-l.read]
	}
	n, err := l.r.Read(p)
	l.read += int64(n)
	return n, err
}

// scanStream scans r line by line in chunks, with no whole-file buffering and
// no line-length cap. An unterminated line longer than maxPendingLine is
// scanned in fragments that overlap by overlapBytes so a match straddling a
// fragment boundary is still found; each rule is reported at most once per line.
func scanStream(ctx context.Context, name string, r io.Reader, limit int64) (streamResult, error) {
	lr := &limitedReader{r: r, limit: limit}
	var (
		res      streamResult
		line     = 1
		reported = map[string]struct{}{}
		store    = make([]byte, 0, chunkSize+maxPendingLine)
		chunk    = make([]byte, chunkSize)
	)
	var lower []byte
	scanLine := func(text []byte) {
		lower = asciiLower(lower, text)
		for _, rl := range rules {
			if _, done := reported[rl.name]; done {
				continue
			}
			if rl.matches(text, lower) {
				reported[rl.name] = struct{}{}
				res.findings = append(res.findings, Finding{File: name, Rule: rl.name, Line: line})
			}
		}
	}
	for {
		if err := ctx.Err(); err != nil {
			return res, err
		}
		n, rerr := lr.Read(chunk)
		store = append(store, chunk[:n]...)
		data := store
		for {
			i := bytes.IndexByte(data, '\n')
			if i < 0 {
				break
			}
			scanLine(data[:i])
			line++
			clear(reported)
			data = data[i+1:]
		}
		if len(data) > maxPendingLine {
			scanLine(data)
			data = data[len(data)-overlapBytes:]
		}
		// Move the unterminated remainder to the front of the backing array.
		store = store[:copy(store[:cap(store)], data)]
		if rerr != nil {
			if len(store) > 0 {
				scanLine(store)
			}
			res.read = lr.read
			res.hitLimit = lr.exceeded
			if errors.Is(rerr, io.EOF) {
				return res, nil
			}
			return res, rerr
		}
	}
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
