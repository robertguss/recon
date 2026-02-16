package cli

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// helpers: seed a decision + edge into the DB
// ---------------------------------------------------------------------------

func seedEdge(t *testing.T, app *App) {
	t.Helper()
	conn, err := openExistingDB(app)
	if err != nil {
		t.Fatalf("openExistingDB: %v", err)
	}
	defer conn.Close()

	_, err = conn.Exec(`INSERT INTO decisions(id,title,reasoning,confidence,status,created_at,updated_at)
		VALUES (1,'Test Decision','r','high','active','2026-01-01T00:00:00Z','2026-01-01T00:00:00Z')`)
	if err != nil {
		t.Fatalf("seed decision: %v", err)
	}
	_, err = conn.Exec(`INSERT INTO edges(from_type,from_id,to_type,to_ref,relation,source,confidence,created_at)
		VALUES ('decision',1,'package','internal/cli','affects','manual','high','2026-01-01T00:00:00Z')`)
	if err != nil {
		t.Fatalf("seed edge: %v", err)
	}
}

// ---------------------------------------------------------------------------
// edges --from
// ---------------------------------------------------------------------------

func TestEdgesFromTextValid(t *testing.T) {
	_, app := m4Setup(t)
	seedEdge(t, app)

	out, _, err := runCommandWithCapture(t, newEdgesCommand(app), []string{"--from", "decision:1"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !strings.Contains(out, "decision:1") {
		t.Fatalf("expected edge line with decision:1, out=%q", out)
	}
	if !strings.Contains(out, "affects") {
		t.Fatalf("expected relation in output, out=%q", out)
	}
}

func TestEdgesFromJSONValid(t *testing.T) {
	_, app := m4Setup(t)
	seedEdge(t, app)

	out, _, err := runCommandWithCapture(t, newEdgesCommand(app), []string{"--from", "decision:1", "--json"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !strings.Contains(out, `"from_type"`) {
		t.Fatalf("expected JSON field from_type, out=%q", out)
	}
	if !strings.Contains(out, `"affects"`) {
		t.Fatalf("expected relation in JSON, out=%q", out)
	}
}

func TestEdgesFromTextInvalidRef(t *testing.T) {
	_, app := m4Setup(t)

	_, _, err := runCommandWithCapture(t, newEdgesCommand(app), []string{"--from", "invalid"})
	if err == nil {
		t.Fatal("expected error for invalid ref (no colon)")
	}
}

func TestEdgesFromJSONInvalidRef(t *testing.T) {
	_, app := m4Setup(t)

	out, _, err := runCommandWithCapture(t, newEdgesCommand(app), []string{"--from", "invalid", "--json"})
	if err == nil {
		t.Fatal("expected error for invalid ref")
	}
	if !strings.Contains(out, `"code": "invalid_input"`) {
		t.Fatalf("expected invalid_input envelope, out=%q", out)
	}
}

func TestEdgesFromTextNonIntegerID(t *testing.T) {
	_, app := m4Setup(t)

	_, _, err := runCommandWithCapture(t, newEdgesCommand(app), []string{"--from", "decision:abc"})
	if err == nil {
		t.Fatal("expected error for non-integer ID")
	}
	if !strings.Contains(err.Error(), "integer") {
		t.Fatalf("expected integer error message, got %v", err)
	}
}

func TestEdgesFromJSONNonIntegerID(t *testing.T) {
	_, app := m4Setup(t)

	out, _, err := runCommandWithCapture(t, newEdgesCommand(app), []string{"--from", "decision:abc", "--json"})
	if err == nil {
		t.Fatal("expected error for non-integer ID")
	}
	if !strings.Contains(out, `"code": "invalid_input"`) {
		t.Fatalf("expected invalid_input envelope, out=%q", out)
	}
}

// ---------------------------------------------------------------------------
// edges --to
// ---------------------------------------------------------------------------

func TestEdgesToTextValid(t *testing.T) {
	_, app := m4Setup(t)
	seedEdge(t, app)

	out, _, err := runCommandWithCapture(t, newEdgesCommand(app), []string{"--to", "package:internal/cli"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !strings.Contains(out, "affects") {
		t.Fatalf("expected relation in output, out=%q", out)
	}
}

func TestEdgesToJSONValid(t *testing.T) {
	_, app := m4Setup(t)
	seedEdge(t, app)

	out, _, err := runCommandWithCapture(t, newEdgesCommand(app), []string{"--to", "package:internal/cli", "--json"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !strings.Contains(out, `"to_ref"`) {
		t.Fatalf("expected JSON field to_ref, out=%q", out)
	}
}

func TestEdgesToTextInvalidRef(t *testing.T) {
	_, app := m4Setup(t)

	_, _, err := runCommandWithCapture(t, newEdgesCommand(app), []string{"--to", "invalid"})
	if err == nil {
		t.Fatal("expected error for invalid ref (no colon)")
	}
}

func TestEdgesToJSONInvalidRef(t *testing.T) {
	_, app := m4Setup(t)

	out, _, err := runCommandWithCapture(t, newEdgesCommand(app), []string{"--to", "invalid", "--json"})
	if err == nil {
		t.Fatal("expected error for invalid ref")
	}
	if !strings.Contains(out, `"code": "invalid_input"`) {
		t.Fatalf("expected invalid_input envelope, out=%q", out)
	}
}

// ---------------------------------------------------------------------------
// edges --delete
// ---------------------------------------------------------------------------

func TestEdgesDeleteTextSuccess(t *testing.T) {
	_, app := m4Setup(t)
	seedEdge(t, app)

	// Get the edge ID (should be 1)
	out, _, err := runCommandWithCapture(t, newEdgesCommand(app), []string{"--delete", "1"})
	if err != nil {
		t.Fatalf("expected delete success, got %v", err)
	}
	if !strings.Contains(out, "Edge 1 deleted.") {
		t.Fatalf("expected 'Edge 1 deleted.', out=%q", out)
	}
}

func TestEdgesDeleteJSONSuccess(t *testing.T) {
	_, app := m4Setup(t)
	seedEdge(t, app)

	out, _, err := runCommandWithCapture(t, newEdgesCommand(app), []string{"--delete", "1", "--json"})
	if err != nil {
		t.Fatalf("expected delete success, got %v", err)
	}
	if !strings.Contains(out, `"deleted"`) {
		t.Fatalf("expected deleted field, out=%q", out)
	}
	if !strings.Contains(out, `"id"`) {
		t.Fatalf("expected id field, out=%q", out)
	}
}

func TestEdgesDeleteTextNotFound(t *testing.T) {
	_, app := m4Setup(t)

	_, _, err := runCommandWithCapture(t, newEdgesCommand(app), []string{"--delete", "999"})
	if err == nil {
		t.Fatal("expected error for deleting non-existent edge")
	}
}

func TestEdgesDeleteJSONNotFound(t *testing.T) {
	_, app := m4Setup(t)

	out, _, err := runCommandWithCapture(t, newEdgesCommand(app), []string{"--delete", "999", "--json"})
	if err == nil {
		t.Fatal("expected error for deleting non-existent edge")
	}
	if !strings.Contains(out, `"code": "not_found"`) {
		t.Fatalf("expected not_found envelope, out=%q", out)
	}
}

// ---------------------------------------------------------------------------
// edges --list
// ---------------------------------------------------------------------------

func TestEdgesListTextWithEdges(t *testing.T) {
	_, app := m4Setup(t)
	seedEdge(t, app)

	out, _, err := runCommandWithCapture(t, newEdgesCommand(app), []string{"--list"})
	if err != nil {
		t.Fatalf("expected list success, got %v", err)
	}
	if !strings.Contains(out, "affects") {
		t.Fatalf("expected edge in list, out=%q", out)
	}
}

func TestEdgesListJSONWithEdges(t *testing.T) {
	_, app := m4Setup(t)
	seedEdge(t, app)

	out, _, err := runCommandWithCapture(t, newEdgesCommand(app), []string{"--list", "--json"})
	if err != nil {
		t.Fatalf("expected list success, got %v", err)
	}
	if !strings.Contains(out, `"from_type"`) {
		t.Fatalf("expected JSON edge data, out=%q", out)
	}
}

func TestEdgesListTextEmpty(t *testing.T) {
	_, app := m4Setup(t)

	out, _, err := runCommandWithCapture(t, newEdgesCommand(app), []string{"--list"})
	if err != nil {
		t.Fatalf("expected list success, got %v", err)
	}
	if !strings.Contains(out, "No edges found.") {
		t.Fatalf("expected 'No edges found.', out=%q", out)
	}
}

// ---------------------------------------------------------------------------
// edges: no flags
// ---------------------------------------------------------------------------

func TestEdgesNoFlagsText(t *testing.T) {
	_, app := m4Setup(t)

	_, _, err := runCommandWithCapture(t, newEdgesCommand(app), []string{})
	if err == nil {
		t.Fatal("expected error for no flags")
	}
	if !strings.Contains(err.Error(), "--from") && !strings.Contains(err.Error(), "--create") {
		t.Fatalf("expected missing flag error, got %v", err)
	}
}

func TestEdgesNoFlagsJSON(t *testing.T) {
	_, app := m4Setup(t)

	out, _, err := runCommandWithCapture(t, newEdgesCommand(app), []string{"--json"})
	if err == nil {
		t.Fatal("expected error for no flags")
	}
	if !strings.Contains(out, `"code": "missing_argument"`) {
		t.Fatalf("expected missing_argument envelope, out=%q", out)
	}
}

// ---------------------------------------------------------------------------
// edges: no DB
// ---------------------------------------------------------------------------

func TestEdgesNoDBText(t *testing.T) {
	_, app := m4SetupNoInit(t)

	_, _, err := runCommandWithCapture(t, newEdgesCommand(app), []string{"--list"})
	if err == nil {
		t.Fatal("expected error for missing db")
	}
	if !strings.Contains(err.Error(), "run `recon init` first") {
		t.Fatalf("expected init error, got %v", err)
	}
}

func TestEdgesNoDBJSON(t *testing.T) {
	_, app := m4SetupNoInit(t)

	out, _, err := runCommandWithCapture(t, newEdgesCommand(app), []string{"--list", "--json"})
	if err == nil {
		t.Fatal("expected error for missing db")
	}
	if !strings.Contains(out, `"code": "not_initialized"`) {
		t.Fatalf("expected not_initialized envelope, out=%q", out)
	}
}
