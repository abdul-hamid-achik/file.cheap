// Package mcp exposes fcheap stash operations as MCP tools for AI agents.
package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/abdul-hamid-achik/file.cheap/internal/agentguide"
	"github.com/abdul-hamid-achik/file.cheap/internal/analyze"
	"github.com/abdul-hamid-achik/file.cheap/internal/artifactref"
	"github.com/abdul-hamid-achik/file.cheap/internal/cleanup"
	"github.com/abdul-hamid-achik/file.cheap/internal/diff"
	"github.com/abdul-hamid-achik/file.cheap/internal/stash"
	doccontent "github.com/abdul-hamid-achik/file.cheap/platform/docs"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Server wraps the MCP server with stash operations.
type Server struct {
	stashDir    string
	vecgrepPath string
	version     string
	emb         analyze.EmbedderSettings
}

type cleanupSkip struct {
	ID     string `json:"id"`
	Reason string `json:"reason"`
}

type smartCleanupResult struct {
	Analysis  *stash.CleanupResult `json:"analysis"`
	Applied   bool                 `json:"applied"`
	Dropped   []string             `json:"dropped"`
	Reclaimed int64                `json:"reclaimed"`
	Skipped   []cleanupSkip        `json:"skipped"`
	Failed    []stash.SweepFailure `json:"failed"`
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
	}, &mcp.ServerOptions{
		Instructions: agentguide.MCPInstructions(),
	})
	s.registerTools(srv)
	s.registerResources(srv)
	s.registerPrompts(srv)
	return srv.Run(ctx, transport)
}

func (s *Server) registerTools(srv *mcp.Server) {
	f := false
	t := true
	embeddingOpenWorld := s.emb.Enabled()

	// fcheap_save
	type saveInput struct {
		Path   string   `json:"path" jsonschema:"Absolute path to the file or directory to save"`
		Name   string   `json:"name,omitempty" jsonschema:"Display name for the stash"`
		Tags   []string `json:"tags,omitempty" jsonschema:"Tags for categorization"`
		Tool   string   `json:"tool,omitempty" jsonschema:"Tool that produced the content (e.g., vidtrace)"`
		Source string   `json:"source,omitempty" jsonschema:"Original artifact this stash derives from (provenance)"`
		TTL    string   `json:"ttl,omitempty" jsonschema:"Time-to-live for this stash (e.g. 7d, 24h, 30d, or 2026-12-31); empty = never expires"`
		Index  bool     `json:"index,omitempty" jsonschema:"Index the stash for search immediately after saving (so it's searchable without a separate fcheap_analyze call)"`
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
			TTL:        in.TTL,
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
		// Best-effort auto-index so the stash is searchable without a separate
		// fcheap_analyze call. A save that succeeds is never failed by indexing.
		if in.Index {
			an := s.analyzer()
			if idx, ierr := an.IndexStash(ctx, mgr.StashDir(st.Manifest.ID)); ierr != nil {
				out["index_error"] = ierr.Error()
			} else {
				out["indexed"] = map[string]any{
					"bundle_type":   idx.BundleType,
					"files_indexed": idx.FilesIndex,
				}
			}
		}
		return textResult(out), nil, nil
	})

	// fcheap_list
	type listInput struct {
		Tag            string   `json:"tag,omitempty" jsonschema:"Filter by tag (single; merged with tags)"`
		Tags           []string `json:"tags,omitempty" jsonschema:"Filter by tags — AND across entries (stash must contain every tag)"`
		Tool           string   `json:"tool,omitempty" jsonschema:"Filter by tool (e.g. vidtrace)"`
		Since          string   `json:"since,omitempty" jsonschema:"Only stashes newer than 24h, 7d, 2w, or 2026-06-01"`
		Limit          int      `json:"limit,omitempty" jsonschema:"Maximum number of stashes"`
		IncludeExpired bool     `json:"include_expired,omitempty" jsonschema:"Include expired stashes, which are hidden by default"`
	}
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "fcheap_list",
		Description: "List active stashes, optionally filtered by tag, tool, and age. Newest first; include expired stashes only when requested.",
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
		opts := stash.ListOptions{Tag: in.Tag, Tags: in.Tags, Tool: in.Tool, Limit: in.Limit, IncludeExpired: in.IncludeExpired}
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

	// fcheap_artifact_ref
	type artifactRefInput struct {
		StashID         string `json:"stash_id" jsonschema:"The existing local stash ID to reference"`
		Kind            string `json:"kind,omitempty" jsonschema:"Artifact kind override; defaults to a safe kind derived from the stash bundle"`
		ProducerTool    string `json:"producer_tool,omitempty" jsonschema:"Tool that produced the native artifact; required when any producer metadata is supplied"`
		ProducerVersion string `json:"producer_version,omitempty" jsonschema:"Version of the producer tool"`
		NativeSchema    string `json:"native_schema,omitempty" jsonschema:"Absolute schema URI for the native artifact"`
		NativeID        string `json:"native_id,omitempty" jsonschema:"Producer-native artifact ID"`
		Entrypoint      string `json:"entrypoint,omitempty" jsonschema:"Safe relative path to the native descriptor inside the stash"`
	}
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "fcheap_artifact_ref",
		Description: "Return a stable, credential-free ArtifactRefV1 for an existing local stash. This is read-only and does not upload or sign anything.",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: &f,
			OpenWorldHint:   &f,
			IdempotentHint:  true,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, in artifactRefInput) (*mcp.CallToolResult, any, error) {
		mgr, err := stash.NewManager(s.stashDir)
		if err != nil {
			return toolError("create stash manager: %v", err), nil, nil
		}
		st, err := mgr.Info(ctx, in.StashID)
		if err != nil {
			return toolError("artifact ref failed: %v", err), nil, nil
		}
		ref, err := artifactref.NewLocal(st.Manifest.ID, st.Manifest.BundleType, artifactref.LocalOptions{
			Kind: in.Kind,
			Producer: artifactref.Producer{
				Tool:         in.ProducerTool,
				Version:      in.ProducerVersion,
				NativeSchema: in.NativeSchema,
				NativeID:     in.NativeID,
				Entrypoint:   in.Entrypoint,
			},
		})
		if err != nil {
			return toolError("%v", err), nil, nil
		}
		return textResult(ref), nil, nil
	})

	// fcheap_restore
	type restoreInput struct {
		StashID       string `json:"stash_id" jsonschema:"The stash ID to restore"`
		Target        string `json:"target,omitempty" jsonschema:"Target directory (default: a fresh, unique temp directory, reported in the result)"`
		AllowMismatch bool   `json:"allow_mismatch,omitempty" jsonschema:"Accept an unverified restore instead of returning a tool error (default: false)"`
	}
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "fcheap_restore",
		Description: "Restore a stash, verify file hashes, and report the target. Defaults to a fresh temporary directory; hash mismatches are tool errors unless explicitly allowed.",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: &t,
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
		status := "restored"
		if !res.Verified {
			status = "restored_unverified"
			if len(res.Mismatches) > 0 {
				status = "restored_with_mismatches"
			}
		}
		result := textResult(map[string]any{
			"stash_id":   in.StashID,
			"target":     res.Target,
			"file_count": res.FileCount,
			"verified":   res.Verified,
			"mismatches": res.Mismatches,
			"status":     status,
		})
		if !res.Verified && !in.AllowMismatch {
			result.IsError = true
		}
		return result, nil, nil
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
		failed := []stash.SweepFailure{}
		if err := analyze.NewAnalyzer(s.stashDir, s.vecgrepPath).DropIndex(in.StashID); err != nil {
			failed = append(failed, stash.SweepFailure{ID: in.StashID, Stage: "index", Error: err.Error()})
		}
		status := "dropped"
		if len(failed) > 0 {
			status = "dropped_with_failures"
		}
		result := textResult(map[string]any{
			"stash_id": in.StashID,
			"status":   status,
			"failed":   failed,
		})
		if len(failed) > 0 {
			result.IsError = true
		}
		return result, nil, nil
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
			OpenWorldHint:   &embeddingOpenWorld,
			IdempotentHint:  true,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, in searchInput) (*mcp.CallToolResult, any, error) {
		an := s.analyzer()
		results, err := an.Search(ctx, in.Query, in.Limit, in.Mode)
		if err != nil {
			if errors.Is(err, analyze.ErrNotIndexed) {
				// Not indexed is data (empty), not a tool failure.
				return textResult([]any{}), nil, nil
			}
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
			OpenWorldHint:   &embeddingOpenWorld,
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
		Description: "Connect a stash to a codebase: use optional vecgrep with the stashed artifact's text to rank related file:line candidates for investigation. Matches are leads, not proof.",
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
			q, err := an.StashQueryContext(ctx, mgr.StashDir(in.StashID), 2000)
			if err != nil {
				return toolError("derive query from stash: %v", err), nil, nil
			}
			query = q
		}
		if query == "" {
			return toolError("stash has no searchable text; pass query"), nil, nil
		}
		vres, err := an.VecgrepSearchIn(ctx, in.Codebase, query, in.Limit, in.Index, in.Mode)
		if err != nil {
			return toolError("connect failed: %v", err), nil, nil
		}
		return textResult(&analyze.ConnectResult{
			StashID: in.StashID, Codebase: in.Codebase, Query: query, Matches: vres.Matches, IndexStatus: vres.IndexStatus,
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

	// fcheap_ttl
	type ttlInput struct {
		StashID string `json:"stash_id" jsonschema:"The stash ID to set the TTL on"`
		TTL     string `json:"ttl" jsonschema:"Time-to-live (e.g. 7d, 24h, 30d, or 2026-12-31); empty string clears the TTL (makes the stash permanent)"`
	}
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "fcheap_ttl",
		Description: "Set or update stash expiry metadata. Reaching the expiry time only hides the stash from default listings; no automatic deletion occurs. Pass an empty TTL to clear expiry, or use fcheap_sweep to deliberately drop expired stashes.",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: &f,
			OpenWorldHint:   &f,
			IdempotentHint:  true,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, in ttlInput) (*mcp.CallToolResult, any, error) {
		mgr, err := stash.NewManager(s.stashDir)
		if err != nil {
			return toolError("create stash manager: %v", err), nil, nil
		}
		if !mgr.Exists(in.StashID) {
			return toolError("stash not found: %s", in.StashID), nil, nil
		}
		if err := mgr.SetExpiry(ctx, in.StashID, in.TTL); err != nil {
			return toolError("set ttl failed: %v", err), nil, nil
		}
		st, _ := mgr.Info(ctx, in.StashID)
		expiresAt := ""
		if st != nil && st.Manifest != nil {
			expiresAt = st.Manifest.ExpiresAt
		}
		return textResult(map[string]string{
			"stash_id":   in.StashID,
			"expires_at": expiresAt,
		}), nil, nil
	})

	// fcheap_sweep
	type sweepInput struct {
		Apply      bool   `json:"apply,omitempty" jsonschema:"Actually drop expired stashes (default: dry-run)"`
		KeepTag    string `json:"keep_tag,omitempty" jsonschema:"Tag that exempts a stash from sweeping (default: keep)"`
		IncludeTag string `json:"include_tag,omitempty" jsonschema:"Only sweep stashes with this tag (e.g. codemap-snapshot)"`
	}
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "fcheap_sweep",
		Description: "Find and optionally drop stashes whose TTL has expired. By default a dry-run; pass apply=true to actually delete expired stashes. Stashes with the keep tag (default: keep) are never swept.",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: &t,
			OpenWorldHint:   &f,
			IdempotentHint:  true,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, in sweepInput) (*mcp.CallToolResult, any, error) {
		mgr, err := stash.NewManager(s.stashDir)
		if err != nil {
			return toolError("create stash manager: %v", err), nil, nil
		}
		keepTag := in.KeepTag
		if keepTag == "" {
			keepTag = "keep"
		}
		an := analyze.NewAnalyzer(s.stashDir, s.vecgrepPath)
		res, err := mgr.SweepExpiredFiltered(ctx, in.Apply, keepTag, in.IncludeTag, an.DropIndex)
		if err != nil {
			return toolError("sweep failed: %v", err), nil, nil
		}
		out := textResult(res)
		if len(res.Failed) > 0 {
			out.IsError = true
		}
		return out, nil, nil
	})

	// fcheap_cleanup
	type cleanupInput struct {
		Apply      bool     `json:"apply,omitempty" jsonschema:"Actually drop stashes (default: dry-run). Smart mode auto-deletes only explicit TTL expirations and documented regenerable cache tools"`
		KeepTag    string   `json:"keep_tag,omitempty" jsonschema:"Tag that exempts a stash from cleanup (default: keep)"`
		Tool       string   `json:"tool,omitempty" jsonschema:"Scoring mode: only analyze stashes from this tool"`
		Tag        string   `json:"tag,omitempty" jsonschema:"Scoring mode: only analyze stashes with this tag"`
		DropOnly   bool     `json:"drop_only,omitempty" jsonschema:"Scoring mode: only show stashes scored as drop (default: show all)"`
		Expired    bool     `json:"expired,omitempty" jsonschema:"Scoring mode: include stashes with an expired TTL even if not yet swept"`
		Smart      bool     `json:"smart,omitempty" jsonschema:"Use category-based smart analysis (expired/orphaned/superseded/duplicate/branch-gone/stale/keep) instead of scoring mode"`
		Categories []string `json:"categories,omitempty" jsonschema:"Smart mode: filter to specific categories (comma-separated: expired,orphaned,superseded,duplicate,branch-gone,stale,keep)"`
		StaleDays  int      `json:"stale_days,omitempty" jsonschema:"Smart mode: days without access to be considered stale (0 = disabled)"`
	}
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "fcheap_cleanup",
		Description: "Analyze stashes for cleanup. Two modes: (1) scoring mode (default) scores each stash 0-100 on droppability with weighted heuristics and returns verdicts (drop/review/keep); (2) smart mode (smart=true) categorizes each stash into exactly one cleanup category (expired/orphaned/superseded/duplicate/branch-gone/stale/keep). By default a dry-run; pass apply=true to drop candidates.",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: &t,
			OpenWorldHint:   &f,
			IdempotentHint:  true,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, in cleanupInput) (*mcp.CallToolResult, any, error) {
		mgr, err := stash.NewManager(s.stashDir)
		if err != nil {
			return toolError("create stash manager: %v", err), nil, nil
		}

		if in.Smart {
			// Smart mode: category-based analysis.
			result, err := mgr.AnalyzeCleanup(ctx, stash.CleanupOptions{
				StaleDays:  in.StaleDays,
				Categories: in.Categories,
			})
			if err != nil {
				return toolError("cleanup failed: %v", err), nil, nil
			}
			out := &smartCleanupResult{
				Analysis: result,
				Applied:  in.Apply,
				Dropped:  []string{},
				Skipped:  []cleanupSkip{},
				Failed:   []stash.SweepFailure{},
			}
			// Apply the reviewed plan conservatively. A missing source or older
			// checkpoint is not deletion consent for evidence-bearing stashes.
			if in.Apply {
				an := analyze.NewAnalyzer(s.stashDir, s.vecgrepPath)
				keepTag := in.KeepTag
				if keepTag == "" {
					keepTag = "keep"
				}
				for _, rec := range result.Recommendations {
					if rec.Category == stash.CatKeep {
						continue
					}
					st, err := mgr.Info(ctx, rec.ID)
					if err != nil {
						out.Failed = append(out.Failed, stash.SweepFailure{ID: rec.ID, Stage: "inspect", Error: err.Error()})
						continue
					}
					if st.Manifest.HasTag(keepTag) {
						out.Skipped = append(out.Skipped, cleanupSkip{ID: rec.ID, Reason: "protected by keep tag"})
						continue
					}
					if !mcpSmartCleanupAutoDeletable(rec) {
						out.Skipped = append(out.Skipped, cleanupSkip{ID: rec.ID, Reason: "requires review; not expired or a regenerable cache"})
						continue
					}
					if err := mgr.Drop(ctx, rec.ID); err != nil {
						out.Failed = append(out.Failed, stash.SweepFailure{ID: rec.ID, Stage: "drop", Error: err.Error()})
						continue
					}
					out.Dropped = append(out.Dropped, rec.ID)
					out.Reclaimed += rec.Size
					if err := an.DropIndex(rec.ID); err != nil {
						out.Failed = append(out.Failed, stash.SweepFailure{ID: rec.ID, Stage: "index", Error: err.Error()})
					}
				}
			}
			sort.Strings(out.Dropped)
			toolResult := textResult(out)
			if len(out.Failed) > 0 {
				toolResult.IsError = true
			}
			return toolResult, nil, nil
		}

		// Scoring mode (default): heuristic analysis.
		keepTag := in.KeepTag
		if keepTag == "" {
			keepTag = "keep"
		}
		an := analyze.NewAnalyzer(s.stashDir, s.vecgrepPath)
		result, err := cleanup.Run(ctx, mgr, an.DropIndex, cleanup.Options{
			Apply:    in.Apply,
			KeepTag:  keepTag,
			Tool:     in.Tool,
			Tag:      in.Tag,
			DropOnly: in.DropOnly,
			Expired:  in.Expired,
		})
		if err != nil {
			return toolError("cleanup failed: %v", err), nil, nil
		}
		return scoringCleanupResult(result), nil, nil
	})

	// fcheap_docs
	type docsInput struct {
		Action string `json:"action" jsonschema:"Action: 'guide' (agent operating guide), 'list' (list all doc pages), 'show' (show a specific page), or 'site' (get the docs site URL)"`
		Page   string `json:"page,omitempty" jsonschema:"Canonical embedded doc page for action=show, e.g. 'guide/getting-started'; absolute and traversal paths are rejected"`
	}
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "fcheap_docs",
		Description: "Access read-only fcheap guidance and documentation embedded in this server. Use action='guide' for the versioned agent operating guide, 'list' to list pages, 'show' to read one canonical page, or 'site' for the online docs URL.",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: &f,
			OpenWorldHint:   &f,
			IdempotentHint:  true,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, in docsInput) (*mcp.CallToolResult, any, error) {
		switch in.Action {
		case "guide":
			return textResult(agentguide.New(s.version)), nil, nil
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
			embeddedPage, err := doccontent.Read(in.Page)
			if err != nil {
				return toolError("%v", err), nil, nil
			}
			return textResult(map[string]string{
				"page":    embeddedPage.Name,
				"content": embeddedPage.Content,
			}), nil, nil
		case "site":
			return textResult(map[string]string{
				"url":               "https://file.cheap/guide/",
				"local":             "fcheap docs serve",
				"local_requirement": "file.cheap source checkout with Bun",
			}), nil, nil
		default:
			return toolError("unknown action: %s (use 'guide', 'list', 'show', or 'site')", in.Action), nil, nil
		}
	})
}

func mcpSmartCleanupAutoDeletable(rec stash.CleanupRecommendation) bool {
	if rec.Category == stash.CatExpired {
		return true
	}
	return rec.Tool == "codemap" || rec.Tool == "vecgrep"
}

func scoringCleanupResult(result *cleanup.Result) *mcp.CallToolResult {
	out := textResult(result)
	if result != nil && len(result.Failed) > 0 {
		out.IsError = true
	}
	return out
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
	structured := v
	if trimmed := bytes.TrimSpace(data); len(trimmed) == 0 || trimmed[0] != '{' {
		// MCP structuredContent must be a JSON object. Preserve object-shaped
		// DTOs directly and wrap arrays/scalars under a stable result key.
		structured = map[string]any{"result": v}
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(data)},
		},
		StructuredContent: structured,
	}
}

// listDocPages returns all Markdown pages embedded in the installed binary.
func listDocPages() []string {
	return doccontent.List()
}

// readDocPage reads a canonical embedded page and rejects traversal.
func readDocPage(page string) (string, error) {
	embeddedPage, err := doccontent.Read(page)
	if err != nil {
		return "", err
	}
	return embeddedPage.Content, nil
}
