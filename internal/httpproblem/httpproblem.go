// Package httpproblem extracts a short, safe description from an RFC 9457
// problem+json error body returned by the artifact service. It never echoes a
// non-JSON body, bounds every field, strips control characters, and redacts
// caller-supplied secrets so a failure message cannot leak a credential.
package httpproblem

import (
	"encoding/json"
	"io"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	// MaxBodyBytes bounds how much of an error body is ever read.
	MaxBodyBytes = 64 * 1024

	maxCodeRunes   = 64
	maxTitleRunes  = 120
	maxDetailRunes = 300
	redacted       = "[redacted]"
)

// Problem is the sanitized subset of an RFC 9457 body the CLI surfaces.
type Problem struct {
	Code   string
	Title  string
	Detail string
}

type wire struct {
	Type   string `json:"type"`
	Code   string `json:"code"`
	Title  string `json:"title"`
	Detail string `json:"detail"`
}

// Read decodes a problem document from at most MaxBodyBytes of body. Any
// non-JSON, empty, or unrecognizable body yields a zero Problem. Occurrences of
// each non-empty secret are replaced before the fields are truncated.
func Read(body io.Reader, secrets ...string) Problem {
	data, err := io.ReadAll(io.LimitReader(body, MaxBodyBytes+1))
	if err != nil || len(data) > MaxBodyBytes {
		return Problem{}
	}
	var decoded wire
	if err := json.Unmarshal(data, &decoded); err != nil {
		return Problem{}
	}
	code := decoded.Code
	if code == "" {
		code = decoded.Type
	}
	return Problem{
		Code:   clean(code, maxCodeRunes, secrets),
		Title:  clean(decoded.Title, maxTitleRunes, secrets),
		Detail: clean(decoded.Detail, maxDetailRunes, secrets),
	}
}

// Suffix renders the problem for appending to an error message, for example
// ` (producer_quota_exceeded): Artifact exceeds the producer quota: ...`.
// It is empty when the body carried nothing usable.
func (p Problem) Suffix() string {
	var out strings.Builder
	if p.Code != "" {
		out.WriteString(" (" + p.Code + ")")
	}
	switch {
	case p.Title != "" && p.Detail != "":
		out.WriteString(": " + p.Title + ": " + p.Detail)
	case p.Title != "":
		out.WriteString(": " + p.Title)
	case p.Detail != "":
		out.WriteString(": " + p.Detail)
	}
	return out.String()
}

func clean(value string, maxRunes int, secrets []string) string {
	for _, secret := range secrets {
		if secret != "" {
			value = strings.ReplaceAll(value, secret, redacted)
		}
	}
	if !utf8.ValidString(value) {
		value = strings.ToValidUTF8(value, "?")
	}
	var out strings.Builder
	count := 0
	space := false
	for _, r := range value {
		if unicode.IsSpace(r) {
			space = out.Len() > 0
			continue
		}
		if unicode.IsControl(r) || !unicode.IsPrint(r) {
			continue
		}
		if space {
			out.WriteRune(' ')
			count++
			space = false
		}
		if count >= maxRunes {
			out.WriteString("...")
			break
		}
		out.WriteRune(r)
		count++
	}
	return out.String()
}
