package cli

import (
	"bytes"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/file.cheap/internal/fcheap/output"
)

func TestDocsShowUsesEmbeddedContentOutsideCheckout(t *testing.T) {
	t.Chdir(t.TempDir())
	if found := findDocsDir(); found != "" {
		t.Fatalf("findDocsDir() = %q outside checkout, want empty", found)
	}

	oldPrinter, oldJSON := printer, jsonOutput
	t.Cleanup(func() {
		printer, jsonOutput = oldPrinter, oldJSON
	})
	var stdout bytes.Buffer
	printer = output.New(output.WithOutput(&stdout), output.WithNoColor(true))
	jsonOutput = false

	if err := docsShowCmd.RunE(docsShowCmd, []string{"cli/save"}); err != nil {
		t.Fatalf("docs show embedded page: %v", err)
	}
	if !strings.Contains(stdout.String(), "# save") {
		t.Fatalf("docs show output = %q, want embedded save page", stdout.String())
	}
	if err := docsShowCmd.RunE(docsShowCmd, []string{"../README"}); err == nil {
		t.Fatal("docs show traversal succeeded, want error")
	}
}

func TestDocsBuildInvocationUsesRequestedOutput(t *testing.T) {
	t.Chdir(t.TempDir())
	docsDir := filepath.Join(t.TempDir(), "docs")
	args, outputDir, err := docsBuildInvocation(docsDir, "site-output")
	if err != nil {
		t.Fatal(err)
	}
	wantOutput := filepath.Join(mustGetwd(t), "site-output")
	wantArgs := []string{"run", "docs:build", "--", "--outDir", wantOutput}
	if !reflect.DeepEqual(args, wantArgs) || outputDir != wantOutput {
		t.Fatalf("docsBuildInvocation = (%v, %q), want (%v, %q)", args, outputDir, wantArgs, wantOutput)
	}

	args, outputDir, err = docsBuildInvocation(docsDir, "")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(args, []string{"run", "docs:build"}) || outputDir != filepath.Join(docsDir, ".vitepress", "dist") {
		t.Fatalf("default docsBuildInvocation = (%v, %q)", args, outputDir)
	}
}

func TestOnlineDocsURLTargetsGuide(t *testing.T) {
	if onlineDocsURL != "https://file.cheap/guide/" {
		t.Fatalf("onlineDocsURL = %q, want canonical guide root", onlineDocsURL)
	}
}

func mustGetwd(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(".")
	if err != nil {
		t.Fatal(err)
	}
	return dir
}
