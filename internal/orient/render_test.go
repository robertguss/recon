package orient

import (
	"strings"
	"testing"
)

func TestRenderText(t *testing.T) {
	payload := Payload{
		Project:         ProjectInfo{Name: "recon", ModulePath: "example.com/recon", Language: "go"},
		Freshness:       Freshness{IsStale: true, Reason: "never_synced", LastSyncAt: "2026-01-01T00:00:00Z"},
		Summary:         Summary{FileCount: 1, SymbolCount: 2, PackageCount: 3, DecisionCount: 4},
		Modules:         []ModuleSummary{{Path: "internal/cli", Name: "cli", FileCount: 2, LineCount: 50}},
		ActiveDecisions: []DecisionDigest{{ID: 9, Title: "Use x", Confidence: "high", Drift: "ok", UpdatedAt: "now"}},
		Warnings:        []string{"warn1"},
	}
	got := RenderText(payload)
	for _, needle := range []string{"Project: recon", "STALE CONTEXT: never_synced", "Modules:", "Active decisions:", "Warnings:"} {
		if !strings.Contains(got, needle) {
			t.Fatalf("render output missing %q: %s", needle, got)
		}
	}
}

func TestRenderTextColdModulesAnnotated(t *testing.T) {
	payload := Payload{
		Project: ProjectInfo{Name: "x", ModulePath: "m", Language: "go"},
		Modules: []ModuleSummary{
			{Path: "hot", Name: "hot", FileCount: 1, LineCount: 10, Heat: "hot"},
			{Path: "cold", Name: "cold", FileCount: 1, LineCount: 5, Heat: "cold"},
		},
	}
	got := RenderText(payload)
	if !strings.Contains(got, "hot (hot)") {
		t.Fatalf("expected hot module in output: %s", got)
	}
	if !strings.Contains(got, "[HOT]") {
		t.Fatalf("expected [HOT] annotation: %s", got)
	}
	if !strings.Contains(got, "cold (cold)") {
		t.Fatalf("expected cold module in output: %s", got)
	}
	if !strings.Contains(got, "[COLD]") {
		t.Fatalf("expected [COLD] annotation: %s", got)
	}
}

func TestRenderTextAllColdModules(t *testing.T) {
	payload := Payload{
		Project: ProjectInfo{Name: "x", ModulePath: "m", Language: "go"},
		Modules: []ModuleSummary{
			{Path: "a", Name: "a", FileCount: 1, LineCount: 10, Heat: "cold"},
			{Path: "b", Name: "b", FileCount: 2, LineCount: 20, Heat: "cold"},
		},
	}
	got := RenderText(payload)
	if strings.Contains(got, "Modules:\n- (none)") {
		t.Fatalf("should not say (none) in Modules when modules exist, got:\n%s", got)
	}
	if !strings.Contains(got, "a (a)") || !strings.Contains(got, "b (b)") {
		t.Fatalf("expected all cold modules listed, got:\n%s", got)
	}
	if !strings.Contains(got, "[COLD]") {
		t.Fatalf("expected [COLD] annotation, got:\n%s", got)
	}
}

func TestRenderTextDependencyFlowTruncation(t *testing.T) {
	payload := Payload{
		Project: ProjectInfo{Name: "x", ModulePath: "m", Language: "go"},
		Modules: []ModuleSummary{
			{Path: "internal/cli", Name: "cli", FileCount: 5, LineCount: 100, Heat: "hot"},
			{Path: "internal/db", Name: "db", FileCount: 2, LineCount: 50, Heat: "hot"},
		},
		Architecture: Architecture{
			DependencyFlow: []DependencyEdge{
				{From: "internal/cli", To: []string{"internal/db"}},
				{From: "internal/cli", To: []string{"internal/util"}},        // internal/util not in modules
				{From: "internal/something", To: []string{"internal/other"}}, // neither in modules
			},
		},
	}
	got := RenderText(payload)
	// Should show the inter-module dep
	if !strings.Contains(got, "internal/cli → internal/db") {
		t.Fatalf("expected inter-module dep in output: %s", got)
	}
	// Should show "+2 more" for the two non-inter-module edges
	if !strings.Contains(got, "(+2 more)") {
		t.Fatalf("expected (+2 more) in output: %s", got)
	}
}

func TestRenderTextDecisionReasoning(t *testing.T) {
	payload := Payload{
		Project: ProjectInfo{Name: "x", ModulePath: "m", Language: "go"},
		ActiveDecisions: []DecisionDigest{
			{ID: 1, Title: "Use Cobra for CLI", Confidence: "high", Drift: "ok", UpdatedAt: "now", Reasoning: "standard Go CLI framework with broad ecosystem support"},
		},
		ActivePatterns: []PatternDigest{
			{ID: 1, Title: "Function-var injection", Confidence: "medium", Drift: "ok", UpdatedAt: "now", Reasoning: "override package-level vars in tests for isolation"},
		},
	}
	got := RenderText(payload)
	if !strings.Contains(got, "standard Go CLI framework") {
		t.Fatalf("expected decision reasoning in output: %s", got)
	}
	if !strings.Contains(got, "override package-level vars") {
		t.Fatalf("expected pattern reasoning in output: %s", got)
	}
}

func TestRenderTextDecisionWithoutReasoning(t *testing.T) {
	payload := Payload{
		Project: ProjectInfo{Name: "x", ModulePath: "m", Language: "go"},
		ActiveDecisions: []DecisionDigest{
			{ID: 1, Title: "No reasoning here", Confidence: "high", Drift: "ok", UpdatedAt: "now"},
		},
	}
	got := RenderText(payload)
	// Should still render fine, just no reasoning line
	if !strings.Contains(got, "No reasoning here") {
		t.Fatalf("expected decision title in output: %s", got)
	}
}

func TestRenderTextEmptySections(t *testing.T) {
	got := RenderText(Payload{Project: ProjectInfo{Name: "x", ModulePath: "m", Language: "go"}})
	if !strings.Contains(got, "- (none)") {
		t.Fatalf("expected empty markers in output: %s", got)
	}
}
