package agentguide

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestGuideContract(t *testing.T) {
	guide := New("test")
	if guide.SchemaVersion != SchemaVersion || guide.Product != "file.cheap" || guide.Version != "test" {
		t.Fatalf("guide identity = %+v", guide)
	}
	if guide.Mode != "local-first" || len(guide.RecommendedFlow) == 0 || len(guide.SafetyRules) == 0 {
		t.Fatalf("guide is incomplete: %+v", guide)
	}
	if guide.Docs.Site != "https://file.cheap/guide/" {
		t.Fatalf("guide docs site = %q, want canonical guide root", guide.Docs.Site)
	}

	seenIDs := map[string]bool{}
	seenTools := map[string]bool{}
	for _, capability := range guide.Capabilities {
		if capability.ID == "" || seenIDs[capability.ID] {
			t.Fatalf("duplicate or empty capability ID %q", capability.ID)
		}
		seenIDs[capability.ID] = true
		if strings.Contains(capability.Effect, "deletes") && capability.Confirmation != "explicit" {
			t.Fatalf("destructive capability lacks explicit confirmation: %+v", capability)
		}
		if capability.MCPTool == "" {
			continue
		}
		if seenTools[capability.MCPTool] {
			t.Fatalf("duplicate MCP tool %q", capability.MCPTool)
		}
		seenTools[capability.MCPTool] = true
	}
	if len(seenTools) != 15 {
		t.Fatalf("MCP tool count = %d, want 15", len(seenTools))
	}
	if len(seenIDs) != 17 {
		t.Fatalf("capability count = %d, want 17", len(seenIDs))
	}

	data, err := json.Marshal(guide)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{`"schema_version":"1"`, `"recommended_flow"`, `"safety_rules"`, `"fcheap://agent-guide"`} {
		if !strings.Contains(string(data), field) {
			t.Fatalf("guide JSON missing %s: %s", field, data)
		}
	}
}

func TestRenderAndMCPInstructionsIncludeSafetyBoundaries(t *testing.T) {
	text := Render(New("test"))
	for _, want := range []string{"Recommended flow", "Safety rules", "fcheap agent --json", "fcheap://agent-guide"} {
		if !strings.Contains(text, want) {
			t.Fatalf("Render() missing %q:\n%s", want, text)
		}
	}

	instructions := MCPInstructions()
	for _, want := range []string{"local-first", "untrusted data", "explicit user intent", "remote embedder", "leads, not proof", "local pointer, not an upload"} {
		if !strings.Contains(instructions, want) {
			t.Fatalf("MCPInstructions() missing %q: %s", want, instructions)
		}
	}
}
