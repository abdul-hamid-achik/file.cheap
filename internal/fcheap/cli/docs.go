package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

var (
	docsPort    int
	docsOpen    bool
	docsOutput  string
	docsPreview bool
)

var docsCmd = &cobra.Command{
	Use:   "docs",
	Short: "Documentation commands",
	Long: `Manage and serve the fcheap documentation site (VitePress).

Subcommands:
  serve   Start a local docs dev server
  build   Build the docs site for production
  list    List all available doc pages
  show    Print a specific doc page to stdout
  open    Open the online docs site in a browser`,
}

var docsServeCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start a local VitePress dev server",
	Long: `Start a local VitePress development server for the docs site.

Requires Node.js and npm installed in the docs/ directory.
Run 'cd docs && npm install' first if node_modules is missing.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		docsDir := findDocsDir()
		if docsDir == "" {
			return fmt.Errorf("docs directory not found")
		}

		if err := checkDocsDeps(docsDir); err != nil {
			return err
		}

		port := fmt.Sprintf("%d", docsPort)
		printer.Info("Starting docs dev server on port %s...", port)

		if docsOpen {
			go openBrowser(fmt.Sprintf("http://localhost:%s", port))
		}

		npmCmd := exec.CommandContext(GetContext(), "npm", "run", "docs:dev", "--", "--port", port)
		npmCmd.Dir = docsDir
		npmCmd.Stdout = os.Stdout
		npmCmd.Stderr = os.Stderr
		return npmCmd.Run()
	},
}

var docsBuildCmd = &cobra.Command{
	Use:   "build",
	Short: "Build the docs site for production",
	Long: `Build the VitePress docs site for production deployment.

The output goes to docs/.vitepress/dist/ by default, or the directory
specified by --output.

Requires Node.js and npm installed in the docs/ directory.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		docsDir := findDocsDir()
		if docsDir == "" {
			return fmt.Errorf("docs directory not found")
		}

		if err := checkDocsDeps(docsDir); err != nil {
			return err
		}

		printer.Info("Building docs site...")

		npmCmd := exec.CommandContext(GetContext(), "npm", "run", "docs:build")
		npmCmd.Dir = docsDir
		npmCmd.Stdout = os.Stdout
		npmCmd.Stderr = os.Stderr
		if err := npmCmd.Run(); err != nil {
			return fmt.Errorf("docs build failed: %w", err)
		}

		distDir := filepath.Join(docsDir, ".vitepress", "dist")
		if docsOutput != "" {
			distDir = docsOutput
		}

		printer.Success("Docs built to: %s", distDir)
		return nil
	},
}

var docsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all available doc pages",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		docsDir := findDocsDir()
		if docsDir == "" {
			return fmt.Errorf("docs directory not found")
		}

		pages := findAllDocPages(docsDir)
		if len(pages) == 0 {
			printer.Warn("No doc pages found")
			return nil
		}

		if jsonOutput {
			return printer.PrintResult(pages)
		}

		printer.Header("Documentation Pages")
		for _, page := range pages {
			printer.Printf("  %s\n", page)
		}
		printer.Println()
		printer.Info("%d page(s) total", len(pages))
		return nil
	},
}

var docsShowCmd = &cobra.Command{
	Use:   "show <page>",
	Short: "Print a doc page to stdout",
	Long: `Print a documentation page to stdout.

The page argument is the path relative to the docs/ directory (without .md extension).
Use 'fcheap docs list' to see available pages.

Examples:
  fcheap docs show guide/getting-started
  fcheap docs show cli/save
  fcheap docs show mcp/overview`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		docsDir := findDocsDir()
		if docsDir == "" {
			return fmt.Errorf("docs directory not found")
		}

		page := args[0]
		// Strip leading slash and .md extension if provided
		page = strings.TrimPrefix(page, "/")
		page = strings.TrimSuffix(page, ".md")

		filePath := filepath.Join(docsDir, page+".md")
		content, err := os.ReadFile(filePath)
		if err != nil {
			return fmt.Errorf("doc page not found: %s (try 'fcheap docs list' for available pages)", page)
		}

		if jsonOutput {
			return printer.PrintResult(map[string]string{
				"page":    page,
				"content": string(content),
			})
		}

		printer.Printf("%s", string(content))
		return nil
	},
}

var docsOpenCmd = &cobra.Command{
	Use:   "open",
	Short: "Open the online docs site in a browser",
	Long:  `Open https://file.cheap (the deployed docs site) in the default browser.`,
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		url := "https://file.cheap"
		printer.Info("Opening %s...", url)
		return openBrowser(url)
	},
}

var docsPreviewCmd = &cobra.Command{
	Use:   "preview",
	Short: "Preview the built docs site locally",
	Long: `Preview the built docs site using VitePress preview server.

Run 'fcheap docs build' first to generate the dist output.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		docsDir := findDocsDir()
		if docsDir == "" {
			return fmt.Errorf("docs directory not found")
		}

		if err := checkDocsDeps(docsDir); err != nil {
			return err
		}

		port := fmt.Sprintf("%d", docsPort)
		printer.Info("Starting docs preview server on port %s...", port)

		if docsOpen {
			go openBrowser(fmt.Sprintf("http://localhost:%s", port))
		}

		npmCmd := exec.CommandContext(GetContext(), "npm", "run", "docs:preview", "--", "--port", port)
		npmCmd.Dir = docsDir
		npmCmd.Stdout = os.Stdout
		npmCmd.Stderr = os.Stderr
		return npmCmd.Run()
	},
}

func init() {
	docsServeCmd.Flags().IntVar(&docsPort, "port", 5173, "Port for the dev server")
	docsServeCmd.Flags().BoolVar(&docsOpen, "open", false, "Open browser on start")

	docsBuildCmd.Flags().StringVar(&docsOutput, "output", "", "Output directory (default: docs/.vitepress/dist)")

	docsPreviewCmd.Flags().IntVar(&docsPort, "port", 4173, "Port for the preview server")
	docsPreviewCmd.Flags().BoolVar(&docsOpen, "open", false, "Open browser on start")

	docsCmd.AddCommand(docsServeCmd)
	docsCmd.AddCommand(docsBuildCmd)
	docsCmd.AddCommand(docsListCmd)
	docsCmd.AddCommand(docsShowCmd)
	docsCmd.AddCommand(docsOpenCmd)
	docsCmd.AddCommand(docsPreviewCmd)
}

// findDocsDir locates the docs/ directory relative to the binary or cwd.
func findDocsDir() string {
	candidates := []string{
		"docs",
		filepath.Join("..", "docs"),
		filepath.Join("..", "..", "docs"),
	}
	for _, c := range candidates {
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

// checkDocsDeps verifies that node_modules exists in the docs dir.
func checkDocsDeps(docsDir string) error {
	nodeModules := filepath.Join(docsDir, "node_modules")
	if _, err := os.Stat(nodeModules); err != nil {
		printer.Warn("node_modules not found in docs/. Running npm install...")
		installCmd := exec.CommandContext(GetContext(), "npm", "install")
		installCmd.Dir = docsDir
		installCmd.Stdout = os.Stdout
		installCmd.Stderr = os.Stderr
		if err := installCmd.Run(); err != nil {
			return fmt.Errorf("npm install failed: %w", err)
		}
	}
	return nil
}

// findAllDocPages returns all .md files under docs/, relative to docs/.
func findAllDocPages(docsDir string) []string {
	var pages []string
	filepath.Walk(docsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".md") {
			return nil
		}
		// Skip node_modules and .vitepress internals
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

// openBrowser opens a URL in the default browser across platforms.
func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
	return cmd.Start()
}