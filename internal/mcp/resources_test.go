package mcp

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
	if !strings.Contains(body, id) || !strings.Contains(body, "fcheap_connect") || !strings.Contains(body, "/repo") {
		t.Errorf("investigate_stash body unexpected: %q", body)
	}

	// Prompt: find_across_stashes requires a query.
	if _, err := cs.GetPrompt(ctx, &mcp.GetPromptParams{Name: "find_across_stashes"}); err == nil {
		t.Error("expected error when find_across_stashes query is missing")
	}

	// Listings include our registrations.
	lp, err := cs.ListPrompts(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !hasPrompt(lp.Prompts, "investigate_stash") || !hasPrompt(lp.Prompts, "find_across_stashes") {
		t.Errorf("prompts not listed: %+v", lp.Prompts)
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
