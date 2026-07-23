package mcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/file.cheap/internal/analyze"
	"github.com/abdul-hamid-achik/file.cheap/internal/artifactref"
	"github.com/abdul-hamid-achik/file.cheap/internal/stash"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestArtifactRefToolReturnsStrictLocalContract(t *testing.T) {
	ctx := context.Background()
	vault := filepath.Join(t.TempDir(), "vault")
	mgr, err := stash.NewManager(vault)
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(t.TempDir(), "run.json")
	if err := os.WriteFile(source, []byte(`{"status":"failed"}`), 0600); err != nil {
		t.Fatal(err)
	}
	saved, err := mgr.Save(ctx, &stash.SaveOptions{SourcePath: source})
	if err != nil {
		t.Fatal(err)
	}

	server := NewServer(vault, "", "test", analyze.EmbedderSettings{})
	clientTransport, serverTransport := mcpsdk.NewInMemoryTransports()
	go func() { _ = server.Run(ctx, serverTransport) }()
	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "artifact-ref-test", Version: "0"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer session.Close() //nolint:errcheck

	result, err := session.CallTool(ctx, &mcpsdk.CallToolParams{
		Name: "fcheap_artifact_ref",
		Arguments: map[string]any{
			"stash_id":         saved.Manifest.ID,
			"kind":             "cairntrace.run",
			"producer_tool":    "cairntrace",
			"producer_version": "1.8.0",
			"native_schema":    "urn:cairntrace.dev:run:v1",
			"native_id":        "run_01",
			"entrypoint":       "run.json",
		},
	})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool error: %s", toolResultText(result))
	}
	var got artifactref.ArtifactRefV1
	if err := json.Unmarshal([]byte(toolResultText(result)), &got); err != nil {
		t.Fatalf("decode tool JSON: %v", err)
	}
	if err := got.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if got.ArtifactID != saved.Manifest.ID || got.Kind != "cairntrace.run" ||
		got.Producer == nil || got.Producer.Tool != "cairntrace" {
		t.Fatalf("result = %+v", got)
	}
	for _, forbidden := range []string{`"integrity"`, `"web_url"`, "token="} {
		if strings.Contains(toolResultText(result), forbidden) {
			t.Fatalf("tool result contains %q: %s", forbidden, toolResultText(result))
		}
	}

	structuredJSON, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatalf("marshal StructuredContent: %v", err)
	}
	var structured artifactref.ArtifactRefV1
	if err := json.Unmarshal(structuredJSON, &structured); err != nil {
		t.Fatalf("decode StructuredContent: %v", err)
	}
	if structured.URI != got.URI {
		t.Fatalf("StructuredContent = %#v, want URI %q", result.StructuredContent, got.URI)
	}
}

func TestArtifactRefToolValidatesInputAndIsReadOnly(t *testing.T) {
	ctx := context.Background()
	vault := filepath.Join(t.TempDir(), "vault")
	mgr, err := stash.NewManager(vault)
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(t.TempDir(), "evidence.txt")
	if err := os.WriteFile(source, []byte("evidence"), 0600); err != nil {
		t.Fatal(err)
	}
	saved, err := mgr.Save(ctx, &stash.SaveOptions{SourcePath: source})
	if err != nil {
		t.Fatal(err)
	}

	server := NewServer(vault, "", "test", analyze.EmbedderSettings{})
	clientTransport, serverTransport := mcpsdk.NewInMemoryTransports()
	go func() { _ = server.Run(ctx, serverTransport) }()
	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "artifact-ref-test", Version: "0"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer session.Close() //nolint:errcheck

	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	var found *mcpsdk.Tool
	for _, tool := range tools.Tools {
		if tool.Name == "fcheap_artifact_ref" {
			found = tool
			break
		}
	}
	if found == nil {
		t.Fatal("fcheap_artifact_ref is not registered")
	}
	if found.Annotations == nil || found.Annotations.DestructiveHint == nil || *found.Annotations.DestructiveHint ||
		found.Annotations.OpenWorldHint == nil || *found.Annotations.OpenWorldHint || !found.Annotations.IdempotentHint {
		t.Fatalf("annotations = %+v, want local read-only idempotent tool", found.Annotations)
	}
	inputSchema, err := json.Marshal(found.InputSchema)
	if err != nil {
		t.Fatalf("marshal input schema: %v", err)
	}
	for _, field := range []string{
		`"stash_id"`,
		`"kind"`,
		`"producer_tool"`,
		`"producer_version"`,
		`"native_schema"`,
		`"native_id"`,
		`"entrypoint"`,
	} {
		if !strings.Contains(string(inputSchema), field) {
			t.Errorf("input schema missing %s: %s", field, inputSchema)
		}
	}

	invalid, err := session.CallTool(ctx, &mcpsdk.CallToolParams{
		Name: "fcheap_artifact_ref",
		Arguments: map[string]any{
			"stash_id":   saved.Manifest.ID,
			"entrypoint": "../run.json",
		},
	})
	if err != nil {
		t.Fatalf("call invalid input: %v", err)
	}
	if !invalid.IsError || !strings.Contains(toolResultText(invalid), ".producer.tool") {
		t.Fatalf("invalid result = %+v", invalid)
	}
	if !mgr.Exists(saved.Manifest.ID) {
		t.Fatal("read-only artifact ref tool changed the stash")
	}

	missing, err := session.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "fcheap_artifact_ref",
		Arguments: map[string]any{"stash_id": "missing-stash"},
	})
	if err != nil {
		t.Fatalf("call missing stash: %v", err)
	}
	if !missing.IsError || !strings.Contains(toolResultText(missing), "artifact ref failed") {
		t.Fatalf("missing result = %+v", missing)
	}
}
