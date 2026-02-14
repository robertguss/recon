package orient

import (
	"strings"
	"testing"
)

func TestRenderText(t *testing.T) {
	payload := Payload{
		Project: ProjectInfo{Name: "recon", ModulePath: "example.com/recon", Language: "go"},
		Freshness: Freshness{IsStale: true, Reason: "never_synced", LastSyncAt: "2026-01-01T00:00:00Z"},
		Summary: Summary{FileCount: 1, SymbolCount: 2, PackageCount: 3, DecisionCount: 4},
		Modules: []ModuleSummary{{Path: "internal/cli", Name: "cli", FileCount: 2, LineCount: 50}},
		ActiveDecisions: []DecisionDigest{{ID: 9, Title: "Use x", Confidence: "high", Drift: "ok", UpdatedAt: "now"}},
		Warnings: []string{"warn1"},
	}
	got := RenderText(payload)
	for _, needle := range []string{"Project: recon", "STALE CONTEXT: never_synced", "Modules:", "Active decisions:", "Warnings:"} {
		if !strings.Contains(got, needle) {
			t.Fatalf("render output missing %q: %s", needle, got)
		}
	}
}

func TestRenderTextEmptySections(t *testing.T) {
	got := RenderText(Payload{Project: ProjectInfo{Name: "x", ModulePath: "m", Language: "go"}})
	if !strings.Contains(got, "- (none)") {
		t.Fatalf("expected empty markers in output: %s", got)
	}
}
