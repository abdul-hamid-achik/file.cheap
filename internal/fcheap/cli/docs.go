package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	doccontent "github.com/abdul-hamid-achik/file.cheap/docs"
	"github.com/spf13/cobra"
)

const onlineDocsURL = "https://file.cheap/guide/"

var (
	docsPort   int
	docsOpen   bool
	docsOutput string
)

var docsCmd = &cobra.Command{
	Use:   "docs",
	Short: "Documentation commands",
	Long: `Read the embedded fcheap documentation or manage its VitePress source site.

Subcommands:
  serve   Start a local docs dev server (source checkout required)
  build   Build the docs site for production (source checkout required)
  preview Preview a built docs site (source checkout required)
  list    List all embedded doc pages
  show    Print an embedded doc page to stdout
  open    Open the online docs site in a browser`,
}

var docsServeCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start a local VitePress dev server",
	Long: `Start a local VitePress development server for the docs site.

Requires a file.cheap source checkout plus Bun. If node_modules is missing,
fcheap installs the exact dependencies from docs/bun.lock.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		docsDir := findDocsDir()
		if docsDir == "" {
			return docsSourceRequiredError()
		}

		if err := checkDocsDeps(docsDir); err != nil {
			return err
		}

		port := fmt.Sprintf("%d", docsPort)
		printer.Info("Starting docs dev server on port %s...", port)

		if docsOpen {
			go func() { _ = openBrowser(fmt.Sprintf("http://localhost:%s", port)) }()
		}

		bunCmd := exec.CommandContext(GetContext(), "bun", "run", "docs:dev", "--port", port)
		bunCmd.Dir = docsDir
		bunCmd.Stdout = os.Stdout
		bunCmd.Stderr = os.Stderr
		return bunCmd.Run()
	},
}

var docsBuildCmd = &cobra.Command{
	Use:   "build",
	Short: "Build the docs site for production",
	Long: `Build the VitePress docs site for production deployment.

The output goes to docs/.vitepress/dist/ by default, or the directory
specified by --output.

Requires a file.cheap source checkout plus Bun.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		docsDir := findDocsDir()
		if docsDir == "" {
			return docsSourceRequiredError()
		}

		if err := checkDocsDeps(docsDir); err != nil {
			return err
		}

		printer.Info("Building docs site...")

		bunArgs, distDir, err := docsBuildInvocation(docsDir, docsOutput)
		if err != nil {
			return err
		}
		bunCmd := exec.CommandContext(GetContext(), "bun", bunArgs...)
		bunCmd.Dir = docsDir
		bunCmd.Stdout = os.Stdout
		bunCmd.Stderr = os.Stderr
		if err := bunCmd.Run(); err != nil {
			return fmt.Errorf("docs build failed: %w", err)
		}

		printer.Success("Docs built to: %s", distDir)
		return nil
	},
}

var docsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all embedded doc pages",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		pages := doccontent.List()
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

The page argument is a canonical embedded path (without the .md extension).
Absolute and traversal paths are rejected.
Use 'fcheap docs list' to see available pages.

Examples:
  fcheap docs show guide/getting-started
  fcheap docs show cli/save
  fcheap docs show mcp/overview`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		page, err := doccontent.Read(args[0])
		if err != nil {
			return fmt.Errorf("%w (try 'fcheap docs list' for available pages)", err)
		}

		if jsonOutput {
			return printer.PrintResult(map[string]string{
				"page":    page.Name,
				"content": page.Content,
			})
		}

		printer.Printf("%s", page.Content)
		return nil
	},
}

var docsOpenCmd = &cobra.Command{
	Use:   "open",
	Short: "Open the online docs site in a browser",
	Long:  `Open https://file.cheap/guide/ (the online docs site) in the default browser.`,
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		printer.Info("Opening %s...", onlineDocsURL)
		return openBrowser(onlineDocsURL)
	},
}

var docsPreviewCmd = &cobra.Command{
	Use:   "preview",
	Short: "Preview the built docs site locally",
	Long: `Preview the built docs site using VitePress preview server.

Requires a file.cheap source checkout plus Bun. Run
'fcheap docs build' first to generate the dist output.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		docsDir := findDocsDir()
		if docsDir == "" {
			return docsSourceRequiredError()
		}

		if err := checkDocsDeps(docsDir); err != nil {
			return err
		}

		port := fmt.Sprintf("%d", docsPort)
		printer.Info("Starting docs preview server on port %s...", port)

		if docsOpen {
			go func() { _ = openBrowser(fmt.Sprintf("http://localhost:%s", port)) }()
		}

		bunCmd := exec.CommandContext(GetContext(), "bun", "run", "docs:preview", "--port", port)
		bunCmd.Dir = docsDir
		bunCmd.Stdout = os.Stdout
		bunCmd.Stderr = os.Stderr
		return bunCmd.Run()
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

// findDocsDir locates the VitePress source directory in a file.cheap checkout.
// Read-only list/show commands use embedded Markdown and do not call this.
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
		if isFcheapDocsDir(abs) {
			return abs
		}
	}
	return ""
}

// checkDocsDeps verifies that node_modules exists in the docs dir.
func checkDocsDeps(docsDir string) error {
	nodeModules := filepath.Join(docsDir, "node_modules")
	if _, err := os.Stat(nodeModules); err != nil {
		installArgs := []string{"install"}
		installName := "bun install"
		if _, lockErr := os.Stat(filepath.Join(docsDir, "bun.lock")); lockErr == nil {
			installArgs = []string{"install", "--frozen-lockfile"}
			installName = "bun install --frozen-lockfile"
		}
		printer.Warn("node_modules not found in docs/. Running %s...", installName)
		installCmd := exec.CommandContext(GetContext(), "bun", installArgs...)
		installCmd.Dir = docsDir
		installCmd.Stdout = os.Stdout
		installCmd.Stderr = os.Stderr
		if err := installCmd.Run(); err != nil {
			return fmt.Errorf("%s failed: %w", installName, err)
		}
	}
	return nil
}

func docsBuildInvocation(docsDir, outputDir string) ([]string, string, error) {
	args := []string{"run", "docs:build"}
	resolvedOutput := filepath.Join(docsDir, ".vitepress", "dist")
	if outputDir == "" {
		return args, resolvedOutput, nil
	}
	absOutput, err := filepath.Abs(outputDir)
	if err != nil {
		return nil, "", fmt.Errorf("resolve docs output directory: %w", err)
	}
	return append(args, "--outDir", absOutput), absOutput, nil
}

func docsSourceRequiredError() error {
	return fmt.Errorf("docs source directory not found: serve, build, and preview require a file.cheap source checkout")
}

func isFcheapDocsDir(dir string) bool {
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return false
	}
	if _, err := os.Stat(filepath.Join(dir, ".vitepress", "config.ts")); err != nil {
		return false
	}
	packageJSON, err := os.ReadFile(filepath.Join(dir, "package.json"))
	return err == nil && strings.Contains(string(packageJSON), `"name": "fcheap-docs"`)
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
