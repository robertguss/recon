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
	if len(payload.Architecture.DependencyFlow) > 0 {
		// Collect top module paths for filtering
		topModules := map[string]bool{}
		for _, m := range payload.Modules {
			topModules[m.Path] = true
		}

		// Filter to only inter-module deps (both from and to in top modules)
		var interModuleDeps []string
		for _, edge := range payload.Architecture.DependencyFlow {
			if !topModules[edge.From] {
				continue
			}
			relevantTos := make([]string, 0, len(edge.To))
			for _, to := range edge.To {
				if topModules[to] {
					relevantTos = append(relevantTos, to)
				}
			}
			if len(relevantTos) == 0 {
				continue
			}
			if len(relevantTos) == 1 {
				interModuleDeps = append(interModuleDeps, edge.From+" → "+relevantTos[0])
			} else {
				interModuleDeps = append(interModuleDeps, edge.From+" → {"+strings.Join(relevantTos, ", ")+"}")
			}
		}

		totalEdges := len(payload.Architecture.DependencyFlow)
		if len(interModuleDeps) > 0 {
			fmt.Fprintf(&b, "Dependency flow: %s", strings.Join(interModuleDeps, "; "))
			if totalEdges > len(interModuleDeps) {
				fmt.Fprintf(&b, " (+%d more)", totalEdges-len(interModuleDeps))
			}
			fmt.Fprintln(&b)
		} else {
			fmt.Fprintf(&b, "Dependency flow: %d edges (none between top modules)\n", totalEdges)
		}
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
			fmt.Fprintf(&b, "- %s (%s): %d files, %d lines [%s]\n", m.Path, m.Name, m.FileCount, m.LineCount, strings.ToUpper(m.Heat))
			for _, k := range m.Knowledge {
				conf := k.Confidence
				if k.EdgeConfidence != "" && k.EdgeConfidence != k.Confidence {
					conf = k.Confidence + ", edge=" + k.EdgeConfidence
				}
				fmt.Fprintf(&b, "    %s #%d: %s [%s]\n", k.Type, k.ID, k.Title, conf)
			}
		}
	}
	b.WriteString("\n")

	b.WriteString("Active decisions:\n")
	if len(payload.ActiveDecisions) == 0 {
		b.WriteString("- (none)\n")
	} else {
		for _, d := range payload.ActiveDecisions {
			fmt.Fprintf(&b, "- #%d %s [%s] drift=%s updated=%s\n", d.ID, d.Title, d.Confidence, d.Drift, d.UpdatedAt)
			if d.Reasoning != "" {
				fmt.Fprintf(&b, "  Why: %s\n", d.Reasoning)
			}
		}
	}

	if len(payload.ActivePatterns) > 0 {
		b.WriteString("\nActive patterns:\n")
		for _, p := range payload.ActivePatterns {
			fmt.Fprintf(&b, "- #%d %s [%s] drift=%s\n", p.ID, p.Title, p.Confidence, p.Drift)
			if p.Reasoning != "" {
				fmt.Fprintf(&b, "  Why: %s\n", p.Reasoning)
			}
		}
	}

	if len(payload.RecentActivity) > 0 {
		b.WriteString("\nRecent activity:\n")
		for _, a := range payload.RecentActivity {
			fmt.Fprintf(&b, "- %s (%s)\n", a.File, a.LastModified)
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
