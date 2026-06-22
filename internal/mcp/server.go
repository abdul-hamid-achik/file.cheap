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
	"github.com/abdul-hamid-achik/file.cheap/internal/detect"
	"github.com/abdul-hamid-achik/file.cheap/internal/diff"
	"github.com/abdul-hamid-achik/file.cheap/internal/manifest"
	"github.com/abdul-hamid-achik/file.cheap/internal/stash"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Server wraps the MCP server with stash operations.
type Server struct {
	stashDir   string
	vecgrepPath string
	version    string
}

// NewServer creates a new MCP server.
func NewServer(stashDir, vecgrepPath, version string) *Server {
	return &Server{
		stashDir:    stashDir,
		vecgrepPath: vecgrepPath,
		version:     version,
	}
}

// Run starts the MCP server with the given transport.
func (s *Server) Run(ctx context.Context, transport mcp.Transport) error {
	srv := mcp.NewServer(&mcp.Implementation{
		Name:    "fcheap",
		Title:   "fcheap stash tools",
		Version: s.version,
	}, nil)
	s.registerTools(srv)
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
	}
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "fcheap_save",
		Description: "Save a file or directory to the stash vault. Returns the stash ID and manifest.",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: &f,
			OpenWorldHint:   &t,
			IdempotentHint: false,
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
		st, err := mgr.Save(ctx, &stash.SaveOptions{
			SourcePath: absPath,
			Name:       in.Name,
			Tags:       in.Tags,
			Tool:       in.Tool,
		})
		if err != nil {
			return toolError("save failed: %v", err), nil, nil
		}
		return textResult(st.Manifest), nil, nil
	})

	// fcheap_list
	type listInput struct {
		Tag string `json:"tag,omitempty" jsonschema:"Filter by tag"`
	}
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "fcheap_list",
		Description: "List all stashes, optionally filtered by tag.",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: &f,
			OpenWorldHint:   &f,
			IdempotentHint: true,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, in listInput) (*mcp.CallToolResult, any, error) {
		mgr, err := stash.NewManager(s.stashDir)
		if err != nil {
			return toolError("create stash manager: %v", err), nil, nil
		}
		stashes, err := mgr.List(ctx, in.Tag)
		if err != nil {
			return toolError("list failed: %v", err), nil, nil
		}
		var summaries []map[string]any
		for _, st := range stashes {
			summaries = append(summaries, map[string]any{
				"id":         st.Manifest.ID,
				"name":       st.Manifest.Name,
				"tool":       st.Manifest.Tool,
				"tags":       st.Manifest.Tags,
				"file_count": st.Manifest.FileCount,
				"total_size": st.Manifest.TotalSize,
				"created_at": st.Manifest.CreatedAt,
				"bundle_type": st.Manifest.BundleType,
			})
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
			IdempotentHint: true,
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
		Target  string `json:"target,omitempty" jsonschema:"Target directory (default: /tmp/<stash-id>)"`
	}
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "fcheap_restore",
		Description: "Restore a stash to a target directory. Extracts all files from the stash.",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: &f,
			OpenWorldHint:   &t,
			IdempotentHint: false,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, in restoreInput) (*mcp.CallToolResult, any, error) {
		mgr, err := stash.NewManager(s.stashDir)
		if err != nil {
			return toolError("create stash manager: %v", err), nil, nil
		}
		if err := mgr.Restore(ctx, in.StashID, in.Target); err != nil {
			return toolError("restore failed: %v", err), nil, nil
		}
		target := in.Target
		if target == "" {
			target = fmt.Sprintf("/tmp/%s", in.StashID)
		}
		return textResult(map[string]string{
			"stash_id": in.StashID,
			"target":   target,
			"status":   "restored",
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
			IdempotentHint: false,
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
		return textResult(map[string]string{
			"stash_id": in.StashID,
			"status":   "dropped",
		}), nil, nil
	})

	// fcheap_search
	type searchInput struct {
		Query string `json:"query" jsonschema:"Search query"`
	}
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "fcheap_search",
		Description: "Search across all indexed stashes. Returns matching snippets with scores.",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: &f,
			OpenWorldHint:   &f,
			IdempotentHint: true,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, in searchInput) (*mcp.CallToolResult, any, error) {
		an := analyze.NewAnalyzer(s.stashDir, s.vecgrepPath)
		results, err := an.Search(ctx, in.Query)
		if err != nil {
			return toolError("search failed: %v", err), nil, nil
		}
		// Also try vecgrep
		vgrepResults, _ := an.SearchWithVecgrep(ctx, in.Query)
		allResults := append(results, vgrepResults...)
		return textResult(allResults), nil, nil
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
			IdempotentHint: true,
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
		an := analyze.NewAnalyzer(s.stashDir, s.vecgrepPath)
		if err := an.IndexStash(ctx, stashDir); err != nil {
			return toolError("index failed: %v", err), nil, nil
		}

		// Get detection info
		contentDir := filepath.Join(stashDir, "content")
		detectResult := detect.Detect(contentDir)

		result := map[string]any{
			"stash_id":         in.StashID,
			"status":           "indexed",
			"bundle_type":      string(detectResult.Type),
			"searchable_files": len(detectResult.SearchableFiles),
		}

		if in.Query != "" {
			results, err := an.SearchStash(ctx, stashDir, in.Query)
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
			IdempotentHint: true,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, in diffInput) (*mcp.CallToolResult, any, error) {
		mgr, err := stash.NewManager(s.stashDir)
		if err != nil {
			return toolError("create stash manager: %v", err), nil, nil
		}
		stashDir := mgr.StashDir(in.StashID)
		result, err := diff.CompareStashToDir(stashDir, in.TargetDir)
		if err != nil {
			return toolError("diff failed: %v", err), nil, nil
		}
		return textResult(result), nil, nil
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
			IdempotentHint: true,
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

// Ensure manifest is imported (used in save tool)
var _ = manifest.SchemaVersion

// listDocPages returns all .md doc pages relative to the docs/ directory.
func listDocPages() []string {
	docsDir := findProjectDocsDir()
	if docsDir == "" {
		return nil
	}
	var pages []string
	filepath.WalkDir(docsDir, func(path string, d os.DirEntry, err error) error {
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