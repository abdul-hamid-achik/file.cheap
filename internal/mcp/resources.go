package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/abdul-hamid-achik/file.cheap/internal/manifest"
	"github.com/abdul-hamid-achik/file.cheap/internal/stash"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	stashesURI     = "fcheap://stashes"
	stashURIPrefix = "fcheap://stash/"
)

// registerResources exposes stashes as first-class MCP resources, so an agent
// can read stash metadata by URI (and embed it as context) without spending a
// tool call. Mirrors the data returned by fcheap_list / fcheap_info.
func (s *Server) registerResources(srv *mcp.Server) {
	// fcheap://stashes — the whole stash index as a JSON array.
	srv.AddResource(&mcp.Resource{
		Name:        "stashes",
		URI:         stashesURI,
		Title:       "All stashes",
		Description: "JSON index of every stash: id, name, tool, tags, file count, size, created_at, bundle type, and secret/video flags.",
		MIMEType:    "application/json",
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		mgr, err := stash.NewManager(s.stashDir)
		if err != nil {
			return nil, fmt.Errorf("create stash manager: %w", err)
		}
		stashes, err := mgr.ListFiltered(ctx, stash.ListOptions{})
		if err != nil {
			return nil, fmt.Errorf("list stashes: %w", err)
		}
		summaries := make([]map[string]any, 0, len(stashes))
		for _, st := range stashes {
			summaries = append(summaries, stashSummary(st.Manifest))
		}
		return jsonResource(req.Params.URI, summaries)
	})

	// fcheap://stash/{id} — one stash's full manifest (provenance + file list).
	srv.AddResourceTemplate(&mcp.ResourceTemplate{
		Name:        "stash",
		URITemplate: "fcheap://stash/{id}",
		Title:       "Stash manifest",
		Description: "Full manifest for a single stash: provenance, file list with hashes, tags, compression, detected secrets, and bundle metadata.",
		MIMEType:    "application/json",
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		id := strings.Trim(strings.TrimPrefix(req.Params.URI, stashURIPrefix), "/")
		if id == "" || strings.Contains(id, "/") {
			return nil, fmt.Errorf("invalid stash resource URI: %s", req.Params.URI)
		}
		mgr, err := stash.NewManager(s.stashDir)
		if err != nil {
			return nil, fmt.Errorf("create stash manager: %w", err)
		}
		st, err := mgr.Info(ctx, id)
		if err != nil {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		return jsonResource(req.Params.URI, st.Manifest)
	})
}

// registerPrompts exposes reusable agent workflows as MCP prompts, so a user can
// kick off a multi-step investigation with one slash command.
func (s *Server) registerPrompts(srv *mcp.Server) {
	// investigate_stash — the flagship vidtrace-evidence -> codebase workflow.
	srv.AddPrompt(&mcp.Prompt{
		Name:        "investigate_stash",
		Title:       "Investigate a stash",
		Description: "Plan an end-to-end investigation of a saved stash: read its manifest, index and search its contents, and (for bug-report bundles) connect it to a codebase to surface the file:line candidates most likely responsible.",
		Arguments: []*mcp.PromptArgument{
			{Name: "stash_id", Description: "The stash ID to investigate", Required: true},
			{Name: "codebase_dir", Description: "Optional codebase to connect the stash to (absolute path)"},
		},
	}, func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		id := req.Params.Arguments["stash_id"]
		if id == "" {
			return nil, fmt.Errorf("stash_id is required")
		}
		codebase := req.Params.Arguments["codebase_dir"]

		var b strings.Builder
		fmt.Fprintf(&b, "Investigate stash %q in the fcheap vault and report your findings.\n\n", id)
		b.WriteString("Work through these steps, using the fcheap MCP tools and resources:\n")
		fmt.Fprintf(&b, "1. Read the manifest — resource `fcheap://stash/%s` (or the `fcheap_info` tool) — to understand provenance, the file list, and the bundle type.\n", id)
		fmt.Fprintf(&b, "2. Call `fcheap_analyze` on %s to index it, then `fcheap_search` to surface its most relevant content.\n", id)
		if codebase != "" {
			fmt.Fprintf(&b, "3. Call `fcheap_connect` with stash_id=%s and codebase_dir=%s to map the stashed evidence to file:line candidates.\n", id, codebase)
		} else {
			b.WriteString("3. If the stash is a bug report (or otherwise derives from a codebase), call `fcheap_connect` with the codebase path to map the evidence to file:line candidates.\n")
		}
		b.WriteString("4. Summarize what the stash contains, anything notable (e.g. detected secrets), and your best hypothesis with supporting evidence.\n")

		return &mcp.GetPromptResult{
			Description: fmt.Sprintf("Investigation plan for stash %s", id),
			Messages: []*mcp.PromptMessage{
				{Role: "user", Content: &mcp.TextContent{Text: b.String()}},
			},
		}, nil
	})

	// find_across_stashes — multi-stash search synthesis.
	srv.AddPrompt(&mcp.Prompt{
		Name:        "find_across_stashes",
		Title:       "Find across all stashes",
		Description: "Search every indexed stash for a query and synthesize where the answer lives.",
		Arguments: []*mcp.PromptArgument{
			{Name: "query", Description: "What to look for across all stashes", Required: true},
			{Name: "mode", Description: "Search mode: keyword, semantic, or hybrid (optional)"},
		},
	}, func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		query := req.Params.Arguments["query"]
		if query == "" {
			return nil, fmt.Errorf("query is required")
		}
		modeNote := ""
		if mode := req.Params.Arguments["mode"]; mode != "" {
			modeNote = fmt.Sprintf(" (mode=%s)", mode)
		}

		text := fmt.Sprintf(
			"Search all indexed fcheap stashes for %q%s and tell me where it lives.\n\n"+
				"1. Call `fcheap_search` with query=%q%s.\n"+
				"2. For the strongest hits, read the surrounding context (the `fcheap://stash/{id}` resource or `fcheap_info`).\n"+
				"3. Synthesize: which stash and file best answers the query, and why. If nothing is indexed yet, run `fcheap_analyze` on the relevant stashes first and retry.\n",
			query, modeNote, query, modeNote)

		return &mcp.GetPromptResult{
			Description: fmt.Sprintf("Cross-stash search for %q", query),
			Messages: []*mcp.PromptMessage{
				{Role: "user", Content: &mcp.TextContent{Text: text}},
			},
		}, nil
	})
}

// stashSummary builds the compact JSON view of a stash shared by the
// fcheap_list tool and the fcheap://stashes resource.
func stashSummary(m *manifest.Manifest) map[string]any {
	item := map[string]any{
		"id":          m.ID,
		"name":        m.Name,
		"tool":        m.Tool,
		"tags":        m.Tags,
		"file_count":  m.FileCount,
		"total_size":  m.TotalSize,
		"created_at":  m.CreatedAt,
		"bundle_type": m.BundleType,
	}
	if m.Compression != "" {
		item["compression"] = m.Compression
	}
	if v := m.Custom["secrets_found"]; v != "" {
		item["secrets_found"] = v
	}
	if v := m.VideoSummary(); v != "" {
		item["video"] = v
	}
	return item
}

// jsonResource marshals v as the JSON body of an MCP resource read.
func jsonResource(uri string, v any) (*mcp.ReadResourceResult, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal resource %s: %w", uri, err)
	}
	return &mcp.ReadResourceResult{
		Contents: []*mcp.ResourceContents{{
			URI:      uri,
			MIMEType: "application/json",
			Text:     string(data),
		}},
	}, nil
}
