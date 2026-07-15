package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/file.cheap/internal/agentguide"
	"github.com/abdul-hamid-achik/file.cheap/internal/fcheap/output"
	"github.com/spf13/cobra"
)

func TestAgentCommandHumanAndJSONOutput(t *testing.T) {
	oldPrinter, oldJSON := printer, jsonOutput
	t.Cleanup(func() {
		printer, jsonOutput = oldPrinter, oldJSON
	})

	var stdout bytes.Buffer
	jsonOutput = false
	printer = output.New(output.WithOutput(&stdout), output.WithNoColor(true))
	if err := agentCmd.RunE(agentCmd, nil); err != nil {
		t.Fatalf("agent human output: %v", err)
	}
	for _, want := range []string{"file.cheap agent guide", "Recommended flow", "Safety rules", "fcheap://agent-guide"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("agent output missing %q:\n%s", want, stdout.String())
		}
	}

	stdout.Reset()
	jsonOutput = true
	printer = output.New(output.WithJSON(true), output.WithOutput(&stdout), output.WithNoColor(true))
	if err := agentCmd.RunE(agentCmd, nil); err != nil {
		t.Fatalf("agent JSON output: %v", err)
	}
	var guide agentguide.Guide
	if err := json.Unmarshal(stdout.Bytes(), &guide); err != nil {
		t.Fatalf("decode agent JSON %q: %v", stdout.String(), err)
	}
	if guide.SchemaVersion != agentguide.SchemaVersion || guide.Product != "file.cheap" || len(guide.Capabilities) != 14 {
		t.Fatalf("agent JSON contract = %+v", guide)
	}
}

func TestStaticAgentAndDocsCommandsDoNotNeedConfig(t *testing.T) {
	for _, cmd := range []*cobra.Command{agentCmd, docsListCmd, docsShowCmd, docsOpenCmd} {
		if commandNeedsConfig(cmd) {
			t.Errorf("commandNeedsConfig(%s) = true, want false", cmd.CommandPath())
		}
	}
	if !commandNeedsConfig(saveCmd) || !commandNeedsConfig(mcpServeCmd) {
		t.Fatal("vault commands unexpectedly bypass configuration")
	}
}

func TestStaticAgentPreRunBypassesInvalidConfig(t *testing.T) {
	oldCfg, oldPrinter, oldRootCtx, oldRootCancel := cfg, printer, rootCtx, rootCancel
	t.Cleanup(func() {
		if rootCancel != nil {
			rootCancel()
		}
		cfg, printer, rootCtx, rootCancel = oldCfg, oldPrinter, oldRootCtx, oldRootCancel
	})

	// Loading a vault command with these relative XDG roots fails validation.
	// Static guidance must remain available because it reads neither location.
	t.Setenv("XDG_CONFIG_HOME", "relative-config")
	t.Setenv("XDG_DATA_HOME", "relative-data")
	cfg = nil
	if err := rootCmd.PersistentPreRunE(agentCmd, nil); err != nil {
		t.Fatalf("agent pre-run loaded invalid config: %v", err)
	}
	if printer == nil || cfg != nil {
		t.Fatalf("static pre-run printer=%v cfg=%+v", printer, cfg)
	}
	rootCmd.PersistentPostRun(agentCmd, nil)

	if err := rootCmd.PersistentPreRunE(saveCmd, nil); err == nil {
		t.Fatal("vault command unexpectedly ignored invalid config")
	}
	if rootCancel != nil {
		rootCancel()
	}
}

func TestRootHelpPointsAgentsAtSafeWorkingExamples(t *testing.T) {
	for _, want := range []string{"fcheap agent --json", "fcheap drop <stash-id> --force", "fcheap connect <stash-id> ~/projects/my-app"} {
		if !strings.Contains(rootCmd.Long, want) {
			t.Fatalf("root help missing %q:\n%s", want, rootCmd.Long)
		}
	}
}
