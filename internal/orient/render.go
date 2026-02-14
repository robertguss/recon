package orient

import (
	"fmt"
	"strings"
)

func RenderText(payload Payload) string {
	var b strings.Builder

	fmt.Fprintf(&b, "Project: %s\n", payload.Project.Name)
	fmt.Fprintf(&b, "Language: %s\n", payload.Project.Language)
	fmt.Fprintf(&b, "Module: %s\n", payload.Project.ModulePath)
	if len(payload.Architecture.EntryPoints) > 0 {
		fmt.Fprintf(&b, "Entry points: %s\n", strings.Join(payload.Architecture.EntryPoints, ", "))
	}
	if payload.Architecture.DependencyFlow != "" {
		fmt.Fprintf(&b, "Dependency flow: %s\n", payload.Architecture.DependencyFlow)
	}
	b.WriteString("\n")

	if payload.Freshness.IsStale {
		fmt.Fprintf(&b, "STALE CONTEXT: %s\n", payload.Freshness.Reason)
		if payload.Freshness.LastSyncAt != "" {
			fmt.Fprintf(&b, "Last sync: %s\n", payload.Freshness.LastSyncAt)
		}
		b.WriteString("\n")
	}

	fmt.Fprintf(&b, "Summary: files=%d symbols=%d packages=%d decisions=%d\n\n",
		payload.Summary.FileCount,
		payload.Summary.SymbolCount,
		payload.Summary.PackageCount,
		payload.Summary.DecisionCount,
	)

	b.WriteString("Modules:\n")
	if len(payload.Modules) == 0 {
		b.WriteString("- (none)\n")
	} else {
		for _, m := range payload.Modules {
			if m.Heat == "cold" {
				continue
			}
			fmt.Fprintf(&b, "- %s (%s): %d files, %d lines [%s]\n", m.Path, m.Name, m.FileCount, m.LineCount, strings.ToUpper(m.Heat))
		}
	}
	b.WriteString("\n")

	b.WriteString("Active decisions:\n")
	if len(payload.ActiveDecisions) == 0 {
		b.WriteString("- (none)\n")
	} else {
		for _, d := range payload.ActiveDecisions {
			fmt.Fprintf(&b, "- #%d %s [%s] drift=%s updated=%s\n", d.ID, d.Title, d.Confidence, d.Drift, d.UpdatedAt)
		}
	}

	if len(payload.Warnings) > 0 {
		b.WriteString("\nWarnings:\n")
		for _, w := range payload.Warnings {
			fmt.Fprintf(&b, "- %s\n", w)
		}
	}

	return strings.TrimSpace(b.String()) + "\n"
}
