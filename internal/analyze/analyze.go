// Package analyze provides text search and analysis for stash content.
package analyze

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/abdul-hamid-achik/file.cheap/internal/detect"
	"github.com/abdul-hamid-achik/file.cheap/internal/manifest"
)

// SearchResult is a single match from search or analysis.
type SearchResult struct {
	StashID   string  `json:"stash_id"`
	Score     float64 `json:"score"`
	Text      string  `json:"text"`
	File      string  `json:"file,omitempty"`
	Source    string  `json:"source,omitempty"`
}

// Analyzer provides search across stashes.
type Analyzer struct {
	stashRoot string
	vecgrepPath string
}

// NewAnalyzer creates an Analyzer for the given stash root.
func NewAnalyzer(stashRoot, vecgrepPath string) *Analyzer {
	return &Analyzer{
		stashRoot:   stashRoot,
		vecgrepPath: vecgrepPath,
	}
}

// IndexStash extracts searchable text from a stash and writes it to an index file.
// This is used for BM25 keyword search without external dependencies.
func (a *Analyzer) IndexStash(ctx context.Context, stashDir string) error {
	man, err := manifest.Load(stashDir)
	if err != nil {
		return fmt.Errorf("load manifest: %w", err)
	}

	contentDir := filepath.Join(stashDir, "content")
	result := detect.Detect(contentDir)

	// Write searchable text to index file
	indexPath := filepath.Join(stashDir, "searchable.txt")
	if err := os.WriteFile(indexPath, []byte(result.SearchableText), 0644); err != nil {
		return fmt.Errorf("write index: %w", err)
	}

	// Store detection result
	detectPath := filepath.Join(stashDir, "detection.json")
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal detection: %w", err)
	}
	if err := os.WriteFile(detectPath, data, 0644); err != nil {
		return fmt.Errorf("write detection: %w", err)
	}

	if man.Custom == nil {
		man.Custom = make(map[string]string)
	}
	man.Custom["indexed"] = "true"
	man.Custom["searchable_text_len"] = fmt.Sprintf("%d", len(result.SearchableText))
	man.Custom["bundle_type"] = string(result.Type)
	return man.Save(stashDir)
}

// Search performs a keyword search across all stashes.
// It searches the searchable.txt files in each stash directory.
func (a *Analyzer) Search(ctx context.Context, query string) ([]SearchResult, error) {
	entries, err := os.ReadDir(a.stashRoot)
	if err != nil {
		return nil, fmt.Errorf("read stash root: %w", err)
	}

	query = strings.ToLower(query)
	var results []SearchResult

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		stashDir := filepath.Join(a.stashRoot, entry.Name())
		indexPath := filepath.Join(stashDir, "searchable.txt")

		data, err := os.ReadFile(indexPath)
		if err != nil {
			continue // not indexed
		}

		text := string(data)
		score := scoreText(strings.ToLower(text), query)
		if score > 0 {
			// Extract a snippet around the match
			snippet := extractSnippet(text, query, 200)
			results = append(results, SearchResult{
				StashID: entry.Name(),
				Score:   score,
				Text:    snippet,
				Source:  "keyword",
			})
		}
	}

	return results, nil
}

// SearchStash searches within a single stash.
func (a *Analyzer) SearchStash(ctx context.Context, stashDir, query string) ([]SearchResult, error) {
	indexPath := filepath.Join(stashDir, "searchable.txt")
	data, err := os.ReadFile(indexPath)
	if err != nil {
		return nil, fmt.Errorf("stash not indexed, run 'fcheap analyze <stash-id>' first")
	}

	query = strings.ToLower(query)
	text := string(data)
	score := scoreText(strings.ToLower(text), query)
	if score == 0 {
		return nil, nil
	}

	snippet := extractSnippet(text, query, 500)
	return []SearchResult{
		{
			StashID: filepath.Base(stashDir),
			Score:   score,
			Text:    snippet,
			Source:  "keyword",
		},
	}, nil
}

// SearchWithVecgrep runs vecgrep as a subprocess for semantic search.
// Returns nil if vecgrep is not available.
func (a *Analyzer) SearchWithVecgrep(ctx context.Context, query string) ([]SearchResult, error) {
	bin := a.vecgrepPath
	if bin == "" {
		path, err := exec.LookPath("vecgrep")
		if err != nil {
			return nil, nil // vecgrep not installed
		}
		bin = path
	}

	// Run vecgrep search --json <query>
	cmd := exec.CommandContext(ctx, bin, "search", "--json", query)
	output, err := cmd.Output()
	if err != nil {
		return nil, nil // vecgrep failed, fall back to keyword
	}

	var vgrepResults []struct {
		File     string  `json:"file"`
		Score    float64 `json:"score"`
		Content  string  `json:"content"`
		Chunk    string  `json:"chunk"`
	}
	if err := json.Unmarshal(output, &vgrepResults); err != nil {
		return nil, nil
	}

	var results []SearchResult
	for _, r := range vgrepResults {
		stashID := extractStashIDFromPath(r.File, a.stashRoot)
		results = append(results, SearchResult{
			StashID: stashID,
			Score:   r.Score,
			Text:    r.Content,
			File:    r.File,
			Source:  "vecgrep",
		})
	}
	return results, nil
}

// scoreText computes a simple BM25-like score for text against a query.
func scoreText(text, query string) float64 {
	terms := strings.Fields(query)
	if len(terms) == 0 {
		return 0
	}

	var score float64
	for _, term := range terms {
		count := strings.Count(text, term)
		if count > 0 {
			// TF component
			tf := float64(count)
			score += tf
		}
	}
	return score
}

// extractSnippet finds the query in text and returns surrounding context.
func extractSnippet(text, query string, maxLen int) string {
	lower := strings.ToLower(text)
	idx := strings.Index(lower, strings.ToLower(query))
	if idx < 0 {
		// Try first term
		terms := strings.Fields(query)
		if len(terms) > 0 {
			idx = strings.Index(lower, terms[0])
		}
	}
	if idx < 0 {
		if len(text) > maxLen {
			return text[:maxLen] + "..."
		}
		return text
	}

	start := idx - maxLen/2
	if start < 0 {
		start = 0
	}
	end := start + maxLen
	if end > len(text) {
		end = len(text)
	}
	snippet := text[start:end]
	if start > 0 {
		snippet = "..." + snippet
	}
	if end < len(text) {
		snippet = snippet + "..."
	}
	return snippet
}

func extractStashIDFromPath(filePath, stashRoot string) string {
	abs, err := filepath.Abs(filePath)
	if err != nil {
		return ""
	}
	rel, err := filepath.Rel(stashRoot, abs)
	if err != nil {
		return filepath.Base(abs)
	}
	parts := strings.SplitN(rel, string(filepath.Separator), 2)
	if len(parts) > 0 {
		return parts[0]
	}
	return rel
}