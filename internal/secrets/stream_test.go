package secrets

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// scanText runs the streaming scanner over text and returns the findings.
func scanText(t *testing.T, text string) []Finding {
	t.Helper()
	res, err := scanStream(context.Background(), "f", strings.NewReader(text), -1)
	if err != nil {
		t.Fatal(err)
	}
	return res.findings
}

func ruleNames(findings []Finding) []string {
	var out []string
	for _, f := range findings {
		out = append(out, f.Rule)
	}
	return out
}

func TestScanFindsSecretPastOneMiBWithCorrectLine(t *testing.T) {
	var sb strings.Builder
	for i := 0; i < 30000; i++ {
		fmt.Fprintf(&sb, "line %05d padding padding padding padding\n", i) // ~1.3 MiB
	}
	sb.WriteString("token=abcdefghijklmnop1234\n")
	sb.WriteString("tail\n")
	findings := scanText(t, sb.String())
	if len(findings) != 1 || findings[0].Rule != "generic-secret" || findings[0].Line != 30001 {
		t.Fatalf("findings = %+v, want one generic-secret at line 30001", findings)
	}
}

func TestScanFindsMatchStraddlingFragmentBoundaries(t *testing.T) {
	const secret = "AKIAIOSFODNN7EXAMPLE"
	// A single unterminated line is scanned in fragments; place the secret so it
	// straddles the fragment boundary and the read-chunk boundaries.
	boundaries := []int{chunkSize, 2 * chunkSize, maxPendingLine, maxPendingLine + chunkSize, 2 * maxPendingLine}
	for _, boundary := range boundaries {
		for _, back := range []int{1, 7, 19} {
			prefix := strings.Repeat("x", boundary-back)
			text := "first\n" + prefix + secret + strings.Repeat("y", 100) + "\nlast\n"
			findings := scanText(t, text)
			if len(findings) != 1 || findings[0].Rule != "aws-access-key" || findings[0].Line != 2 {
				t.Fatalf("boundary %d back %d: findings = %+v, want one aws-access-key at line 2", boundary, back, findings)
			}
		}
	}
}

func TestScanReportsEachRuleOncePerLineEvenForOverlappingFragments(t *testing.T) {
	text := strings.Repeat("AKIAIOSFODNN7EXAMPLE ", 200000) // one ~4 MiB line, secret repeated
	findings := scanText(t, text)
	if len(findings) != 1 || findings[0].Line != 1 {
		t.Fatalf("findings = %d, want 1 (rule once per line): %+v", len(findings), findings[:min(3, len(findings))])
	}
}

func TestScanBudgetSkipsLargestFilesFirstAndCountsThem(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, dir, "a.env", "aws_id = AKIAIOSFODNN7EXAMPLE\n") // 31 bytes
	mustWrite(t, dir, "b.txt", strings.Repeat("b", 60)+"\n")
	mustWrite(t, dir, "c.log", strings.Repeat("c", 500)+"\n")

	report, err := ScanReportOptions(context.Background(), dir, Options{BudgetBytes: 80})
	if err != nil {
		t.Fatal(err)
	}
	// The small high-value file is inspected before the budget runs out.
	if len(report.Findings) != 1 || report.Findings[0].File != "a.env" {
		t.Fatalf("findings = %+v", report.Findings)
	}
	if report.FilesScanned != 1 || report.FilesSkipped != 2 || report.SkippedReasons[SkipBudget] != 2 {
		t.Fatalf("scanned/skipped = %d/%d reasons %v, want 1/2 budget (b is cut off by the budget, c never starts)", report.FilesScanned, report.FilesSkipped, report.SkippedReasons)
	}
	if report.BytesScanned != 80 {
		t.Fatalf("bytes scanned = %d, want the 80-byte budget", report.BytesScanned)
	}

	unlimited, err := ScanReportOptions(context.Background(), dir, Options{BudgetBytes: -1})
	if err != nil {
		t.Fatal(err)
	}
	if unlimited.FilesScanned != 3 || unlimited.FilesSkipped != 0 {
		t.Fatalf("unlimited budget scanned/skipped = %d/%d", unlimited.FilesScanned, unlimited.FilesSkipped)
	}
}

func TestScanDefaultBudgetIsDocumented(t *testing.T) {
	if DefaultBudgetBytes != 256<<20 {
		t.Fatalf("DefaultBudgetBytes = %d, want 256 MiB", DefaultBudgetBytes)
	}
}

func writeZip(t *testing.T, path string, members map[string]string) {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	names := make([]string, 0, len(members))
	for name := range members {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(members[name])); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestScanZipMembersReportsArchiveBangMember(t *testing.T) {
	dir := t.TempDir()
	writeZip(t, filepath.Join(dir, "trace.zip"), map[string]string{
		"trace.trace":                 `{"type":"before"}` + "\n",
		"0-trace.network":             `{"a":1}` + "\n" + `{"request":{"headers":[{"name":"Authorization","value":"Bearer abcdefghijklmnopqrstuvwx"}]}}` + "\n",
		"resources/body.json":         `{"key":"AKIAIOSFODNN7EXAMPLE"}`,
		"resources/shot.png":          "AKIAIOSFODNN7EXAMPLE", // not a scanned member type
		"notes/README.md":             "AKIAIOSFODNN7EXAMPLE", // not a scanned member type
		"network/capture.har":         `{"log":{}}` + "\n" + `"value": "Bearer ABCDEFGHIJKLMNOPQRSTUVWX"` + "\n",
		"logs/run.log":                "clean\n",
		"nested/dir/events.ndjson":    "{}\n{}\n",
		"nested/dir/notes.txt":        "password: hunter2hunter2hunter2\n",
		"resources/not-a-dir/":        "",
		"resources/other.unknownext2": "AKIAIOSFODNN7EXAMPLE",
	})
	report, err := ScanReportContext(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]int{}
	for _, f := range report.Findings {
		got[f.File+"|"+f.Rule] = f.Line
	}
	want := map[string]int{
		"trace.zip!0-trace.network|bearer-token":        2,
		"trace.zip!resources/body.json|aws-access-key":  1,
		"trace.zip!network/capture.har|bearer-token":    2,
		"trace.zip!nested/dir/notes.txt|generic-secret": 1,
	}
	if len(got) != len(want) {
		t.Fatalf("findings = %v, want %v", got, want)
	}
	for k, line := range want {
		if got[k] != line {
			t.Fatalf("finding %q line = %d, want %d (all: %v)", k, got[k], line, got)
		}
	}
	if report.FilesScanned != 1 || report.FilesSkipped != 0 {
		t.Fatalf("scanned/skipped = %d/%d, want 1/0", report.FilesScanned, report.FilesSkipped)
	}
	// Nothing was extracted next to the archive.
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("scan wrote to disk: %v", entries)
	}
}

func TestScanZipGuardSkipsHighRatioMemberButKeepsOtherFindings(t *testing.T) {
	dir := t.TempDir()
	writeZip(t, filepath.Join(dir, "bomb.zip"), map[string]string{
		"a-small.json": `{"k":"AKIAIOSFODNN7EXAMPLE"}`,
		"b-bomb.json":  strings.Repeat("\x00", 8<<20), // 8 MiB of zeros: ratio far above the guard
	})
	report, err := ScanReportContext(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if report.FilesScanned != 0 || report.FilesSkipped != 1 || report.SkippedReasons[SkipZipGuard] != 1 {
		t.Fatalf("scanned/skipped = %d/%d reasons %v, want the archive skipped as zip_guard", report.FilesScanned, report.FilesSkipped, report.SkippedReasons)
	}
	if len(report.Findings) != 1 || report.Findings[0].File != "bomb.zip!a-small.json" {
		t.Fatalf("findings = %+v, want the small member's finding kept", report.Findings)
	}
	if report.BytesScanned > 1<<20 {
		t.Fatalf("bytes scanned = %d: the bomb member must not be decompressed past the guard", report.BytesScanned)
	}
}

func TestScanZipBudgetCountsDecompressedBytes(t *testing.T) {
	dir := t.TempDir()
	body := strings.Repeat("0123456789abcdef", 64) + "\n" // 1 KiB, compresses well
	writeZip(t, filepath.Join(dir, "t.zip"), map[string]string{
		"a.json": body, "b.json": body, "c.json": body,
	})
	report, err := ScanReportOptions(context.Background(), dir, Options{BudgetBytes: 2 * int64(len(body))})
	if err != nil {
		t.Fatal(err)
	}
	if report.SkippedReasons[SkipBudget] != 1 || report.FilesScanned != 0 {
		t.Fatalf("report = %+v, want the archive skipped as budget", report)
	}
}

func TestScanTreatsNonZipNamedZipAsPlainFile(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, dir, "fake.zip", "aws_id = AKIAIOSFODNN7EXAMPLE\n")
	report, err := ScanReportContext(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Findings) != 1 || report.Findings[0].File != "fake.zip" || report.FilesScanned != 1 {
		t.Fatalf("report = %+v", report)
	}
}

func TestRuleTruePositives(t *testing.T) {
	hex64 := strings.Repeat("ab12", 16)
	alnum36 := strings.Repeat("aB3", 12)
	cases := []struct {
		name, text, rule string
	}{
		{"bearer header", "Authorization: Bearer abcdefghijklmnopqrstuvwxyz012345", "bearer-token"},
		{"bearer lowercase", "authorization: bearer abc123def456ghi789jkl012", "bearer-token"},
		{"bearer in curl", `curl -H "Authorization: Bearer abc123def456ghi789jkl012" https://x`, "bearer-token"},
		{"bearer har name/value", `{"name":"Authorization","value":"Bearer abcdefghijklmnopqrstuvwx"}`, "bearer-token"},
		{"bearer pretty har value", `            "value": "Bearer abcdefghijklmnopqrstuvwx"`, "bearer-token"},
		{"cookie session", "Cookie: theme=dark; sessionid=0123456789abcdef0123", "cookie-session"},
		{"set-cookie connect.sid", "Set-Cookie: connect.sid=s%3A0123456789abcdef0123; Path=/; HttpOnly", "cookie-session"},
		{"set-cookie PHPSESSID", "Set-Cookie: PHPSESSID=0123456789abcdef0123", "cookie-session"},
		{"cookie auth token", "Cookie: auth_token=0123456789abcdef0123", "cookie-session"},
		{"cookie sid", "Cookie: sid=0123456789abcdef0123", "cookie-session"},
		{"cookie har", `{"name":"Cookie","value":"a=b; auth_token=0123456789abcdef0123"}`, "cookie-session"},
		{"set-cookie har", `{"name":"Set-Cookie","value":"__Host-session=0123456789abcdef0123; Secure"}`, "cookie-session"},
		{"sk- key", "OPENAI=sk-proj-AbC123dEf456GhI789jKl012", "sk-api-key"},
		{"sk_live", "sk_live_0123456789abcdefABCD", "stripe-secret-key"},
		{"sk_test", "STRIPE=sk_test_0123456789abcdefABCD", "stripe-secret-key"},
		{"digitalocean", "dop_v1_" + hex64, "digitalocean-token"},
		{"npm", "//registry.npmjs.org/:_authToken=npm_" + alnum36, "npm-token"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			findings := scanText(t, tc.text)
			if !slices.Contains(ruleNames(findings), tc.rule) {
				t.Fatalf("%q: rules = %v, want %s", tc.text, ruleNames(findings), tc.rule)
			}
			for _, f := range findings {
				if f.Line != 1 || f.File != "f" {
					t.Fatalf("finding = %+v", f)
				}
			}
		})
	}
}

func TestRuleFalsePositives(t *testing.T) {
	cases := []struct {
		name, text string
	}{
		{"tokenizer identifier", "tokenizer = new Tokenizer(options)"},
		{"tokenizer prose", "The tokenizer splits the input into words before parsing it."},
		{"bearer in prose", "The Bearer of this letter is a trusted courier of the king."},
		{"bearer scheme prose", "Authorization uses the Bearer scheme defined by RFC 6750 for tokens."},
		{"bearer placeholder", "Authorization: Bearer <token>"},
		{"bearer env placeholder", "Authorization: Bearer ${ACCESS_TOKEN}"},
		{"bearer short token", "Authorization: Bearer abc123"},
		{"cookie theme", "Cookie: theme=dark"},
		{"cookie benign long", "Cookie: tracking=0123456789abcdef0123; theme=dark"},
		{"cookie short session", "Cookie: session=abc"},
		{"cookie sid inside word", "Cookie: residual=0123456789abcdef0123"},
		{"cookie prose", "Please accept the cookie policy; session handling is described below."},
		{"cookie har benign", `{"name":"Cookie","value":"theme=dark; lang=en-US"}`},
		{"sk- css class", ".sk-circle-fade-in-and-out-animation { display: none }"},
		{"sk- inside word", "task-runner-configuration-file-name-here and risk-adjusted-return-on-equity"},
		{"sk_live short", "sk_live_short"},
		{"dop short", "dop_v1_abcdef"},
		{"dop uppercase hex", "dop_v1_" + strings.Repeat("AB12", 16)},
		{"npm short", "npm_abc123"},
		{"npm config var", "npm install --npm_config_registry_setting_value_longer_than_thirty_six_xxx"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if findings := scanText(t, tc.text); len(findings) != 0 {
				t.Fatalf("%q: unexpected findings %v", tc.text, ruleNames(findings))
			}
		})
	}
}

func TestFindingsNeverCarryTheMatchedValue(t *testing.T) {
	const token = "abcdefghijklmnopqrstuvwxyz012345"
	findings := scanText(t, "Authorization: Bearer "+token+"\n")
	if len(findings) == 0 {
		t.Fatal("expected a finding")
	}
	if got := fmt.Sprintf("%+v", findings); strings.Contains(got, token) {
		t.Fatalf("finding leaks the value: %s", got)
	}
}
