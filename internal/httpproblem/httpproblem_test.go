package httpproblem

import (
	"strings"
	"testing"
)

func TestReadExtractsProblemFields(t *testing.T) {
	t.Parallel()
	body := `{"type":"https://file.cheap/problems/quota","code":"producer_quota_exceeded","title":"Artifact exceeds the producer quota","detail":"producer 'cairntrace' allows up to 100 bytes","status":413}`
	got := Read(strings.NewReader(body))
	if got.Code != "producer_quota_exceeded" || got.Title == "" || !strings.Contains(got.Detail, "allows up to 100 bytes") {
		t.Fatalf("unexpected problem: %#v", got)
	}
	if want := " (producer_quota_exceeded): Artifact exceeds the producer quota: producer 'cairntrace' allows up to 100 bytes"; got.Suffix() != want {
		t.Fatalf("suffix = %q, want %q", got.Suffix(), want)
	}
}

func TestReadFallsBackToTypeWhenCodeIsMissing(t *testing.T) {
	t.Parallel()
	if got := Read(strings.NewReader(`{"type":"about:blank","title":"Bad"}`)); got.Code != "about:blank" {
		t.Fatalf("code = %q", got.Code)
	}
}

func TestReadIgnoresNonJSONAndOversizedBodies(t *testing.T) {
	t.Parallel()
	if got := Read(strings.NewReader("<html>secret page</html>")); got != (Problem{}) || got.Suffix() != "" {
		t.Fatalf("non-JSON body must be ignored: %#v", got)
	}
	huge := `{"detail":"` + strings.Repeat("a", MaxBodyBytes) + `"}`
	if got := Read(strings.NewReader(huge)); got != (Problem{}) {
		t.Fatalf("oversized body must be ignored: %#v", got)
	}
}

func TestReadSanitizesBoundsAndRedacts(t *testing.T) {
	t.Parallel()
	token := "tok_" + strings.Repeat("s", 40)
	body := `{"code":"bad\u001b[31m","title":"line\none\u0000\ttwo","detail":"echoed ` + token + ` ` + strings.Repeat("x", 1000) + `"}`
	got := Read(strings.NewReader(body), token)
	for _, field := range []string{got.Code, got.Title, got.Detail} {
		for _, r := range field {
			if r < ' ' || r == 0x7f {
				t.Fatalf("control character survived in %q", field)
			}
		}
	}
	if got.Title != "line one two" {
		t.Fatalf("title = %q", got.Title)
	}
	if strings.Contains(got.Detail, token) || !strings.Contains(got.Detail, "[redacted]") {
		t.Fatalf("token was not redacted: %q", got.Detail)
	}
	if len([]rune(got.Detail)) > maxDetailRunes+3 {
		t.Fatalf("detail not bounded: %d runes", len([]rune(got.Detail)))
	}
}
