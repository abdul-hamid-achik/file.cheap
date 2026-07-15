package agentguide

import (
	"fmt"
	"strings"
)

// Render returns the concise human-readable form of a Guide.
func Render(guide Guide) string {
	var b strings.Builder
	fmt.Fprintf(&b, "file.cheap agent guide (%s)\n\n", guide.Version)
	fmt.Fprintf(&b, "%s\nMode: %s\n\n", guide.Purpose, guide.Mode)

	b.WriteString("Recommended flow\n")
	for i, step := range guide.RecommendedFlow {
		fmt.Fprintf(&b, "%d. %s — %s\n", i+1, step.ID, step.Guidance)
		if len(step.CLI) > 0 {
			fmt.Fprintf(&b, "   CLI: %s\n", strings.Join(step.CLI, " | "))
		}
		if len(step.MCP) > 0 {
			fmt.Fprintf(&b, "   MCP: %s\n", strings.Join(step.MCP, ", "))
		}
	}

	b.WriteString("\nSafety rules\n")
	for _, rule := range guide.SafetyRules {
		fmt.Fprintf(&b, "- %s: %s\n", rule.ID, rule.Requirement)
	}

	fmt.Fprintf(&b, "\nMCP: %s %s (%s)\n", guide.MCP.Command, strings.Join(guide.MCP.Args, " "), guide.MCP.Transport)
	fmt.Fprintf(&b, "Full machine-readable guide: fcheap agent --json or %s\n", guide.MCP.Resources[0])
	fmt.Fprintf(&b, "Documentation: %s\n", guide.Docs.Site)
	return b.String()
}
