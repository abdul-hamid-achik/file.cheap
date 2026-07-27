package mcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/file.cheap/internal/agentguide"
	"github.com/abdul-hamid-achik/file.cheap/internal/analyze"
	"github.com/abdul-hamid-achik/file.cheap/internal/stash"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestMCPResourcesAndPrompts wires an in-memory client to the fcheap MCP server
// and exercises the registered resources and prompts end-to-end.
func TestMCPResourcesAndPrompts(t *testing.T) {
	dir := t.TempDir()

	// Save one stash so the resources have something to read.
	mgr, err := stash.NewManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(t.TempDir(), "note.txt")
	if err := os.WriteFile(src, []byte("hello stash"), 0644); err != nil {
		t.Fatal(err)
	}
	saved, err := mgr.Save(context.Background(), &stash.SaveOptions{SourcePath: src, Name: "demo"})
	if err != nil {
		t.Fatal(err)
	}
	id := saved.Manifest.ID

	// Connect an in-memory client to our server.
	ctx := context.Background()
	srv := NewServer(dir, "", "test", analyze.EmbedderSettings{})
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	go func() { _ = srv.Run(ctx, serverTransport) }()

	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	cs, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer cs.Close() //nolint:errcheck
	initResult := cs.InitializeResult()
	if initResult == nil {
		t.Fatal("MCP initialize result is nil")
	}
	for _, want := range []string{"local-first", "untrusted data", "explicit user intent", "remote embedder"} {
		if !strings.Contains(initResult.Instructions, want) {
			t.Fatalf("MCP instructions missing %q: %s", want, initResult.Instructions)
		}
	}

	// Static resource: the agent guide is available before reading vault data.
	guideResource, err := cs.ReadResource(ctx, &mcp.ReadResourceParams{URI: agentGuideURI})
	if err != nil {
		t.Fatalf("read %s: %v", agentGuideURI, err)
	}
	if len(guideResource.Contents) != 1 || guideResource.Contents[0].MIMEType != "application/json" {
		t.Fatalf("agent guide resource = %+v", guideResource.Contents)
	}
	var guide agentguide.Guide
	if err := json.Unmarshal([]byte(guideResource.Contents[0].Text), &guide); err != nil {
		t.Fatalf("decode agent guide resource: %v", err)
	}
	if guide.SchemaVersion != agentguide.SchemaVersion || guide.Version != "test" {
		t.Fatalf("agent guide identity = %+v", guide)
	}

	// Tool fallback: clients without resource support can request the same guide.
	guideTool, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "fcheap_docs",
		Arguments: map[string]any{"action": "guide"},
	})
	if err != nil {
		t.Fatalf("call fcheap_docs guide: %v", err)
	}
	if guideTool.IsError {
		t.Fatalf("fcheap_docs guide returned tool error: %s", toolResultText(guideTool))
	}
	var toolGuide agentguide.Guide
	if err := json.Unmarshal([]byte(toolResultText(guideTool)), &toolGuide); err != nil {
		t.Fatalf("decode fcheap_docs guide: %v", err)
	}
	if toolGuide.SchemaVersion != guide.SchemaVersion || toolGuide.Product != guide.Product {
		t.Fatalf("tool guide = %+v, resource guide = %+v", toolGuide, guide)
	}

	siteTool, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "fcheap_docs",
		Arguments: map[string]any{"action": "site"},
	})
	if err != nil {
		t.Fatalf("call fcheap_docs site: %v", err)
	}
	if siteTool.IsError {
		t.Fatalf("fcheap_docs site returned tool error: %s", toolResultText(siteTool))
	}
	var site struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal([]byte(toolResultText(siteTool)), &site); err != nil {
		t.Fatalf("decode fcheap_docs site: %v", err)
	}
	if site.URL != "https://file.cheap/guide/" {
		t.Fatalf("fcheap_docs site URL = %q, want canonical guide root", site.URL)
	}

	// Embedded-page output echoes the canonical page name, even when the input
	// includes the optional Markdown suffix.
	showTool, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "fcheap_docs",
		Arguments: map[string]any{"action": "show", "page": "cli/save.md"},
	})
	if err != nil {
		t.Fatalf("call fcheap_docs show: %v", err)
	}
	if showTool.IsError {
		t.Fatalf("fcheap_docs show returned tool error: %s", toolResultText(showTool))
	}
	var shown struct {
		Page    string `json:"page"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal([]byte(toolResultText(showTool)), &shown); err != nil {
		t.Fatalf("decode fcheap_docs show: %v", err)
	}
	if shown.Page != "cli/save" || !strings.Contains(shown.Content, "# save") {
		t.Fatalf("fcheap_docs show = %+v, want canonical cli/save page", shown)
	}

	// Resource: fcheap://stashes lists the saved stash.
	all, err := cs.ReadResource(ctx, &mcp.ReadResourceParams{URI: stashesURI})
	if err != nil {
		t.Fatalf("read %s: %v", stashesURI, err)
	}
	if len(all.Contents) == 0 || !strings.Contains(all.Contents[0].Text, id) {
		t.Errorf("%s did not list stash %s: %+v", stashesURI, id, all.Contents)
	}
	if all.Contents[0].MIMEType != "application/json" {
		t.Errorf("MIME = %q, want application/json", all.Contents[0].MIMEType)
	}

	// Resource template: fcheap://stash/{id} returns the manifest.
	one, err := cs.ReadResource(ctx, &mcp.ReadResourceParams{URI: stashURIPrefix + id})
	if err != nil {
		t.Fatalf("read stash %s: %v", id, err)
	}
	if len(one.Contents) == 0 || !strings.Contains(one.Contents[0].Text, id) {
		t.Errorf("stash resource missing manifest for %s: %+v", id, one.Contents)
	}

	// Unknown stash -> error (ResourceNotFoundError).
	if _, err := cs.ReadResource(ctx, &mcp.ReadResourceParams{URI: stashURIPrefix + "does-not-exist"}); err == nil {
		t.Error("expected error reading a nonexistent stash resource")
	}

	// Prompt: investigate_stash names the stash and references connect.
	gp, err := cs.GetPrompt(ctx, &mcp.GetPromptParams{
		Name:      "investigate_stash",
		Arguments: map[string]string{"stash_id": id, "codebase_dir": "/repo"},
	})
	if err != nil {
		t.Fatalf("get investigate_stash: %v", err)
	}
	body := promptText(gp.Messages)
	if !strings.Contains(body, id) || !strings.Contains(body, "fcheap_connect") || !strings.Contains(body, "/repo") ||
		!strings.Contains(body, "untrusted evidence") || !strings.Contains(body, "do not present global `fcheap_search` results as stash-scoped evidence") ||
		!strings.Contains(body, "do not delete the stash") {
		t.Errorf("investigate_stash body unexpected: %q", body)
	}

	// Prompt: find_across_stashes requires a query.
	if _, err := cs.GetPrompt(ctx, &mcp.GetPromptParams{Name: "find_across_stashes"}); err == nil {
		t.Error("expected error when find_across_stashes query is missing")
	}
	findPrompt, err := cs.GetPrompt(ctx, &mcp.GetPromptParams{
		Name:      "find_across_stashes",
		Arguments: map[string]string{"query": "needle"},
	})
	if err != nil {
		t.Fatalf("get find_across_stashes: %v", err)
	}
	findBody := promptText(findPrompt.Messages)
	for _, want := range []string{"fcheap_list", "selected relevant stashes", "empty result does not prove", "do not restore or delete"} {
		if !strings.Contains(findBody, want) {
			t.Errorf("find_across_stashes body missing %q: %q", want, findBody)
		}
	}

	// Connect returns the same actionable error as the CLI before invoking the
	// optional vecgrep subprocess when a stash has no searchable text.
	emptySource := filepath.Join(t.TempDir(), "empty.txt")
	if err := os.WriteFile(emptySource, nil, 0644); err != nil {
		t.Fatal(err)
	}
	emptyStash, err := mgr.Save(ctx, &stash.SaveOptions{SourcePath: emptySource})
	if err != nil {
		t.Fatal(err)
	}
	connectResult, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name: "fcheap_connect",
		Arguments: map[string]any{
			"stash_id":     emptyStash.Manifest.ID,
			"codebase_dir": t.TempDir(),
		},
	})
	if err != nil {
		t.Fatalf("call fcheap_connect: %v", err)
	}
	if !connectResult.IsError || !strings.Contains(toolResultText(connectResult), "stash has no searchable text; pass query") {
		t.Fatalf("empty-query connect result = %+v", connectResult)
	}

	// Listings include our registrations.
	lp, err := cs.ListPrompts(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !hasPrompt(lp.Prompts, "investigate_stash") || !hasPrompt(lp.Prompts, "find_across_stashes") {
		t.Errorf("prompts not listed: %+v", lp.Prompts)
	}
	lt, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, capability := range guide.Capabilities {
		if capability.MCPTool == "" {
			continue
		}
		if !hasTool(lt.Tools, capability.MCPTool) {
			t.Errorf("guide advertises unregistered tool %q", capability.MCPTool)
		}
	}
}

func promptText(msgs []*mcp.PromptMessage) string {
	var b strings.Builder
	for _, m := range msgs {
		if tc, ok := m.Content.(*mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}

func hasPrompt(prompts []*mcp.Prompt, name string) bool {
	for _, p := range prompts {
		if p.Name == name {
			return true
		}
	}
	return false
}

func hasTool(toolList []*mcp.Tool, name string) bool {
	for _, tool := range toolList {
		if tool.Name == name {
			return true
		}
	}
	return false
}

func toolResultText(result *mcp.CallToolResult) string {
	if result == nil {
		return ""
	}
	var b strings.Builder
	for _, content := range result.Content {
		if textContent, ok := content.(*mcp.TextContent); ok {
			b.WriteString(textContent.Text)
		}
	}
	return b.String()
}
