// Package mcp exposes fcheap stash operations as MCP tools for AI agents.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/abdul-hamid-achik/file.cheap/internal/analyze"
	"github.com/abdul-hamid-achik/file.cheap/internal/diff"
	"github.com/abdul-hamid-achik/file.cheap/internal/stash"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Server wraps the MCP server with stash operations.
type Server struct {
	stashDir    string
	vecgrepPath string
	version     string
	emb         analyze.EmbedderSettings
}

// NewServer creates a new MCP server.
func NewServer(stashDir, vecgrepPath, version string, emb analyze.EmbedderSettings) *Server {
	return &Server{
		stashDir:    stashDir,
		vecgrepPath: vecgrepPath,
		version:     version,
		emb:         emb,
	}
}

// analyzer builds an embedder-aware analyzer for this server.
func (s *Server) analyzer() *analyze.Analyzer {
	return analyze.NewAnalyzer(s.stashDir, s.vecgrepPath).WithEmbedder(s.emb)
}

// Run starts the MCP server with the given transport.
func (s *Server) Run(ctx context.Context, transport mcp.Transport) error {
	srv := mcp.NewServer(&mcp.Implementation{
		Name:    "fcheap",
		Title:   "fcheap stash tools",
		Version: s.version,
	}, nil)
	s.registerTools(srv)
	s.registerResources(srv)
	s.registerPrompts(srv)
	return srv.Run(ctx, transport)
}

func (s *Server) registerTools(srv *mcp.Server) {
	f := false
	t := true

	// fcheap_save
	type saveInput struct {
		Path   string   `json:"path" jsonschema:"Absolute path to the file or directory to save"`
		Name   string   `json:"name,omitempty" jsonschema:"Display name for the stash"`
		Tags   []string `json:"tags,omitempty" jsonschema:"Tags for categorization"`
		Tool   string   `json:"tool,omitempty" jsonschema:"Tool that produced the content (e.g., vidtrace)"`
		Source string   `json:"source,omitempty" jsonschema:"Original artifact this stash derives from (provenance)"`
	}
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "fcheap_save",
		Description: "Save a file or directory to the stash vault. Returns the stash ID and manifest.",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: &f,
			OpenWorldHint:   &t,
			IdempotentHint:  false,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, in saveInput) (*mcp.CallToolResult, any, error) {
		mgr, err := stash.NewManager(s.stashDir)
		if err != nil {
			return toolError("create stash manager: %v", err), nil, nil
		}
		absPath, err := filepath.Abs(in.Path)
		if err != nil {
			return toolError("resolve path: %v", err), nil, nil
		}
		if _, err := os.Stat(absPath); err != nil {
			return toolError("path not found: %v", err), nil, nil
		}
		opts := &stash.SaveOptions{
			SourcePath: absPath,
			Name:       in.Name,
			Tags:       in.Tags,
			Tool:       in.Tool,
		}
		if in.Source != "" {
			opts.Custom = map[string]string{"source": in.Source}
		}
		st, err := mgr.Save(ctx, opts)
		if err != nil {
			return toolError("save failed: %v", err), nil, nil
		}
		out := map[string]any{"manifest": st.Manifest}
		if len(st.Secrets) > 0 {
			out["secrets_warning"] = fmt.Sprintf("%d potential secret(s) detected — review before sharing", len(st.Secrets))
			out["secrets"] = st.Secrets
		}
		return textResult(out), nil, nil
	})

	// fcheap_list
	type listInput struct {
		Tag   string `json:"tag,omitempty" jsonschema:"Filter by tag"`
		Tool  string `json:"tool,omitempty" jsonschema:"Filter by tool (e.g. vidtrace)"`
		Since string `json:"since,omitempty" jsonschema:"Only stashes newer than 24h, 7d, 2w, or 2026-06-01"`
		Limit int    `json:"limit,omitempty" jsonschema:"Maximum number of stashes"`
	}
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "fcheap_list",
		Description: "List stashes, optionally filtered by tag, tool, and age. Newest first.",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: &f,
			OpenWorldHint:   &f,
			IdempotentHint:  true,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, in listInput) (*mcp.CallToolResult, any, error) {
		mgr, err := stash.NewManager(s.stashDir)
		if err != nil {
			return toolError("create stash manager: %v", err), nil, nil
		}
		opts := stash.ListOptions{Tag: in.Tag, Tool: in.Tool, Limit: in.Limit}
		if in.Since != "" {
			since, perr := stash.ParseSince(in.Since)
			if perr != nil {
				return toolError("%v", perr), nil, nil
			}
			opts.Since = since
		}
		stashes, err := mgr.ListFiltered(ctx, opts)
		if err != nil {
			return toolError("list failed: %v", err), nil, nil
		}
		summaries := make([]map[string]any, 0, len(stashes))
		for _, st := range stashes {
			summaries = append(summaries, stashSummary(st.Manifest))
		}
		return textResult(summaries), nil, nil
	})

	// fcheap_info
	type infoInput struct {
		StashID string `json:"stash_id" jsonschema:"The stash ID to inspect"`
	}
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "fcheap_info",
		Description: "Get detailed info about a stash including file list and metadata.",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: &f,
			OpenWorldHint:   &f,
			IdempotentHint:  true,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, in infoInput) (*mcp.CallToolResult, any, error) {
		mgr, err := stash.NewManager(s.stashDir)
		if err != nil {
			return toolError("create stash manager: %v", err), nil, nil
		}
		st, err := mgr.Info(ctx, in.StashID)
		if err != nil {
			return toolError("info failed: %v", err), nil, nil
		}
		return textResult(st.Manifest), nil, nil
	})

	// fcheap_restore
	type restoreInput struct {
		StashID string `json:"stash_id" jsonschema:"The stash ID to restore"`
		Target  string `json:"target,omitempty" jsonschema:"Target directory (default: a fresh, unique temp directory, reported in the result)"`
	}
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "fcheap_restore",
		Description: "Restore a stash to a target directory. Extracts all files from the stash.",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: &f,
			OpenWorldHint:   &t,
			IdempotentHint:  false,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, in restoreInput) (*mcp.CallToolResult, any, error) {
		mgr, err := stash.NewManager(s.stashDir)
		if err != nil {
			return toolError("create stash manager: %v", err), nil, nil
		}
		res, err := mgr.Restore(ctx, in.StashID, in.Target)
		if err != nil {
			return toolError("restore failed: %v", err), nil, nil
		}
		return textResult(map[string]any{
			"stash_id":   in.StashID,
			"target":     res.Target,
			"file_count": res.FileCount,
			"verified":   res.Verified,
			"mismatches": res.Mismatches,
			"status":     "restored",
		}), nil, nil
	})

	// fcheap_drop
	type dropInput struct {
		StashID string `json:"stash_id" jsonschema:"The stash ID to drop"`
		Force   bool   `json:"force" jsonschema:"Must be true to confirm deletion"`
	}
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "fcheap_drop",
		Description: "Permanently delete a stash and all its files. Requires force=true.",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: &t,
			OpenWorldHint:   &f,
			IdempotentHint:  false,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, in dropInput) (*mcp.CallToolResult, any, error) {
		if !in.Force {
			return toolError("force=true is required to drop a stash"), nil, nil
		}
		mgr, err := stash.NewManager(s.stashDir)
		if err != nil {
			return toolError("create stash manager: %v", err), nil, nil
		}
		if err := mgr.Drop(ctx, in.StashID); err != nil {
			return toolError("drop failed: %v", err), nil, nil
		}
		// Best-effort: remove any indexed documents for this stash.
		_ = analyze.NewAnalyzer(s.stashDir, s.vecgrepPath).DropIndex(in.StashID)
		return textResult(map[string]string{
			"stash_id": in.StashID,
			"status":   "dropped",
		}), nil, nil
	})

	// fcheap_search
	type searchInput struct {
		Query string `json:"query" jsonschema:"Search query"`
		Limit int    `json:"limit,omitempty" jsonschema:"Maximum number of results (default 20)"`
		Mode  string `json:"mode,omitempty" jsonschema:"Search mode: keyword, semantic, or hybrid (default: hybrid if an embedder is configured, else keyword)"`
	}
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "fcheap_search",
		Description: "Search across all indexed stashes. Returns matching snippets with scores. Supports keyword (BM25), semantic (vector), and hybrid search when an embedder is configured.",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: &f,
			OpenWorldHint:   &f,
			IdempotentHint:  true,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, in searchInput) (*mcp.CallToolResult, any, error) {
		an := s.analyzer()
		results, err := an.Search(ctx, in.Query, in.Limit, in.Mode)
		if err != nil {
			return toolError("search failed: %v", err), nil, nil
		}
		return textResult(results), nil, nil
	})

	// fcheap_analyze
	type analyzeInput struct {
		StashID string `json:"stash_id" jsonschema:"The stash ID to analyze"`
		Query   string `json:"query,omitempty" jsonschema:"Optional search query within the stash"`
	}
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "fcheap_analyze",
		Description: "Index a stash for search and optionally search within it. Detects bundle type (vidtrace, generic) and extracts searchable text.",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: &f,
			OpenWorldHint:   &f,
			IdempotentHint:  true,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, in analyzeInput) (*mcp.CallToolResult, any, error) {
		mgr, err := stash.NewManager(s.stashDir)
		if err != nil {
			return toolError("create stash manager: %v", err), nil, nil
		}
		stashDir := mgr.StashDir(in.StashID)
		if !mgr.Exists(in.StashID) {
			return toolError("stash not found: %s", in.StashID), nil, nil
		}
		an := s.analyzer()
		idx, err := an.IndexStash(ctx, stashDir)
		if err != nil {
			return toolError("index failed: %v", err), nil, nil
		}

		result := map[string]any{
			"stash_id":      in.StashID,
			"status":        "indexed",
			"bundle_type":   idx.BundleType,
			"files_indexed": idx.FilesIndex,
		}

		if in.Query != "" {
			results, err := an.SearchStash(ctx, stashDir, in.Query, 0, "")
			if err != nil {
				result["search_error"] = err.Error()
			} else {
				result["search_results"] = results
			}
		}
		return textResult(result), nil, nil
	})

	// fcheap_diff
	type diffInput struct {
		StashID   string `json:"stash_id" jsonschema:"The stash ID to compare"`
		TargetDir string `json:"target_dir" jsonschema:"Target directory to compare against"`
	}
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "fcheap_diff",
		Description: "Compare a stash against a target directory. Shows files only in stash, only in target, and changed files.",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: &f,
			OpenWorldHint:   &t,
			IdempotentHint:  true,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, in diffInput) (*mcp.CallToolResult, any, error) {
		mgr, err := stash.NewManager(s.stashDir)
		if err != nil {
			return toolError("create stash manager: %v", err), nil, nil
		}
		if !mgr.Exists(in.StashID) {
			return toolError("stash not found: %s", in.StashID), nil, nil
		}
		stashDir := mgr.StashDir(in.StashID)
		result, err := diff.CompareStashToDir(stashDir, in.TargetDir)
		if err != nil {
			return toolError("diff failed: %v", err), nil, nil
		}
		return textResult(result), nil, nil
	})

	// fcheap_connect
	type connectInput struct {
		StashID  string `json:"stash_id" jsonschema:"The stash ID whose content drives the code search"`
		Codebase string `json:"codebase_dir" jsonschema:"Absolute path to the codebase directory to search"`
		Query    string `json:"query,omitempty" jsonschema:"Override the query auto-extracted from the stash"`
		Limit    int    `json:"limit,omitempty" jsonschema:"Max code matches (default 10)"`
		Index    bool   `json:"index,omitempty" jsonschema:"Build the vecgrep index for the codebase first"`
		Mode     string `json:"mode,omitempty" jsonschema:"vecgrep search mode: semantic, keyword, or hybrid (default hybrid)"`
	}
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "fcheap_connect",
		Description: "Connect a stash to a codebase: run semantic code search (vecgrep) over the codebase using the stashed artifact's text (e.g. a vidtrace bug report) to surface the file:line candidates most likely responsible for the bug.",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: &f,
			OpenWorldHint:   &t,
			IdempotentHint:  true,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, in connectInput) (*mcp.CallToolResult, any, error) {
		mgr, err := stash.NewManager(s.stashDir)
		if err != nil {
			return toolError("create stash manager: %v", err), nil, nil
		}
		if !mgr.Exists(in.StashID) {
			return toolError("stash not found: %s", in.StashID), nil, nil
		}
		an := analyze.NewAnalyzer(s.stashDir, s.vecgrepPath)
		query := in.Query
		if query == "" {
			q, err := an.StashQuery(mgr.StashDir(in.StashID), 2000)
			if err != nil {
				return toolError("derive query from stash: %v", err), nil, nil
			}
			query = q
		}
		matches, err := an.VecgrepSearchIn(ctx, in.Codebase, query, in.Limit, in.Index, in.Mode)
		if err != nil {
			return toolError("connect failed: %v", err), nil, nil
		}
		return textResult(&analyze.ConnectResult{
			StashID: in.StashID, Codebase: in.Codebase, Query: query, Matches: matches,
		}), nil, nil
	})

	// fcheap_vacuum
	type vacuumInput struct{}
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "fcheap_vacuum",
		Description: "Remove orphaned metadata- and search-index entries for stashes whose directory no longer exists, then compact the database.",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: &f,
			OpenWorldHint:   &f,
			IdempotentHint:  true,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, in vacuumInput) (*mcp.CallToolResult, any, error) {
		mgr, err := stash.NewManager(s.stashDir)
		if err != nil {
			return toolError("create stash manager: %v", err), nil, nil
		}
		an := analyze.NewAnalyzer(s.stashDir, s.vecgrepPath)
		res, err := mgr.Vacuum(ctx, an.DropIndex)
		if err != nil {
			return toolError("vacuum failed: %v", err), nil, nil
		}
		return textResult(res), nil, nil
	})

	// fcheap_docs
	type docsInput struct {
		Action string `json:"action" jsonschema:"Action: 'list' (list all doc pages), 'show' (show a specific page), or 'site' (get the docs site URL)"`
		Page   string `json:"page,omitempty" jsonschema:"Doc page path (for action=show), e.g. 'guide/getting-started', 'cli/save', 'mcp/overview'"`
	}
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "fcheap_docs",
		Description: "Access fcheap documentation. Use action='list' to list all doc pages, action='show' with a page path to read a specific page, or action='site' to get the online docs URL.",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: &f,
			OpenWorldHint:   &f,
			IdempotentHint:  true,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, in docsInput) (*mcp.CallToolResult, any, error) {
		switch in.Action {
		case "list":
			pages := listDocPages()
			return textResult(map[string]any{
				"pages": pages,
				"count": len(pages),
			}), nil, nil
		case "show":
			if in.Page == "" {
				return toolError("page is required for action=show"), nil, nil
			}
			content, err := readDocPage(in.Page)
			if err != nil {
				return toolError("%v", err), nil, nil
			}
			return textResult(map[string]string{
				"page":    in.Page,
				"content": content,
			}), nil, nil
		case "site":
			return textResult(map[string]string{
				"url":   "https://file.cheap",
				"local": "fcheap docs serve",
			}), nil, nil
		default:
			return toolError("unknown action: %s (use 'list', 'show', or 'site')", in.Action), nil, nil
		}
	})
}

// --- helpers ---

func toolError(format string, args ...any) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: fmt.Sprintf("Error: "+format, args...)},
		},
		IsError: true,
	}
}

func textResult(v any) *mcp.CallToolResult {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Error marshaling result: %v", err)},
			},
			IsError: true,
		}
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(data)},
		},
	}
}

// listDocPages returns all .md doc pages relative to the docs/ directory.
func listDocPages() []string {
	docsDir := findProjectDocsDir()
	if docsDir == "" {
		return nil
	}
	var pages []string
	_ = filepath.WalkDir(docsDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".md") {
			return nil
		}
		if strings.Contains(path, "node_modules") || strings.Contains(path, ".vitepress") {
			return nil
		}
		rel, err := filepath.Rel(docsDir, path)
		if err != nil {
			return nil
		}
		pages = append(pages, rel)
		return nil
	})
	sort.Strings(pages)
	return pages
}

// readDocPage reads a doc page by relative path (with or without .md extension).
func readDocPage(page string) (string, error) {
	docsDir := findProjectDocsDir()
	if docsDir == "" {
		return "", fmt.Errorf("docs directory not found")
	}
	page = strings.TrimPrefix(page, "/")
	page = strings.TrimSuffix(page, ".md")
	filePath := filepath.Join(docsDir, page+".md")
	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("doc page not found: %s", page)
	}
	return string(content), nil
}

// findProjectDocsDir locates the docs/ directory relative to the working directory.
func findProjectDocsDir() string {
	for _, c := range []string{"docs", "../docs", "../../docs"} {
		abs, err := filepath.Abs(c)
		if err != nil {
			continue
		}
		if info, err := os.Stat(abs); err == nil && info.IsDir() {
			if _, err := os.Stat(filepath.Join(abs, ".vitepress", "config.ts")); err == nil {
				return abs
			}
		}
	}
	return ""
}
