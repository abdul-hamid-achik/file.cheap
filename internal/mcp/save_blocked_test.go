package mcp

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/file.cheap/internal/secrets"
	"github.com/abdul-hamid-achik/file.cheap/internal/stash"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestBlockedSaveResultCarriesFindingsButNoValues(t *testing.T) {
	blocked := &stash.SecretsFoundError{
		Findings: []secrets.Finding{
			{File: "config.env", Rule: "aws-access-key", Line: 3},
			{File: "trace.zip!trace.network", Rule: "bearer-token", Line: 12},
		},
		Scan: secrets.Report{FilesScanned: 4, FilesSkipped: 1, SkippedReasons: map[string]int{secrets.SkipBudget: 1}},
	}
	res := blockedSaveResult(blocked)
	if !res.IsError {
		t.Fatal("a refused save must be a tool error")
	}
	text := res.Content[0].(*mcp.TextContent).Text
	var payload struct {
		Status  string `json:"status"`
		Saved   bool   `json:"saved"`
		Secrets struct {
			Found    int               `json:"found"`
			Rules    []string          `json:"rules"`
			Findings []secrets.Finding `json:"findings"`
			Scanned  int               `json:"files_scanned"`
			Skipped  int               `json:"files_skipped"`
			Reasons  map[string]int    `json:"skipped_reasons"`
		} `json:"secrets"`
	}
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		t.Fatalf("decode %q: %v", text, err)
	}
	if payload.Status != "blocked_secrets_found" || payload.Saved {
		t.Fatalf("payload = %+v", payload)
	}
	s := payload.Secrets
	if s.Found != 2 || len(s.Findings) != 2 || s.Findings[1].File != "trace.zip!trace.network" || s.Scanned != 4 || s.Skipped != 1 || s.Reasons["budget"] != 1 {
		t.Fatalf("secrets = %+v", s)
	}
	if !strings.Contains(strings.Join(s.Rules, ","), "bearer-token") {
		t.Fatalf("rules = %v", s.Rules)
	}
}
