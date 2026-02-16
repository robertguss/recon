package cli

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/robertguss/recon/internal/db"
	"github.com/robertguss/recon/internal/edge"
)

func TestEdgesCreate_Bidirectional(t *testing.T) {
	_, app := m4Setup(t)

	// Create a bidirectional edge: decision:1 -> decision:2 with relation "related"
	out, _, err := runCommandWithCapture(t, newEdgesCommand(app), []string{
		"--create",
		"--from", "decision:1",
		"--to", "decision:2",
		"--relation", "related",
		"--json",
	})
	if err != nil {
		t.Fatalf("expected success, got %v; out=%s", err, out)
	}

	// Parse the created edge from JSON output
	var created edge.Edge
	if err := json.Unmarshal([]byte(out), &created); err != nil {
		t.Fatalf("unmarshal created edge: %v; out=%s", err, out)
	}
	if created.Relation != "related" {
		t.Fatalf("expected relation 'related', got %q", created.Relation)
	}

	// Verify both forward and reverse edges exist in the DB
	conn, err := db.Open(db.DBPath(app.ModuleRoot))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer conn.Close()

	svc := edge.NewService(conn)

	// Forward: decision:1 -> decision:2
	forward, err := svc.ListFrom(context.Background(), "decision", 1)
	if err != nil {
		t.Fatalf("ListFrom decision:1: %v", err)
	}
	foundForward := false
	for _, e := range forward {
		if e.ToType == "decision" && e.ToRef == "2" && e.Relation == "related" {
			foundForward = true
		}
	}
	if !foundForward {
		t.Fatalf("expected forward edge decision:1 -> decision:2 (related), got %+v", forward)
	}

	// Reverse: decision:2 -> decision:1
	reverse, err := svc.ListFrom(context.Background(), "decision", 2)
	if err != nil {
		t.Fatalf("ListFrom decision:2: %v", err)
	}
	foundReverse := false
	for _, e := range reverse {
		if e.ToType == "decision" && e.ToRef == "1" && e.Relation == "related" {
			foundReverse = true
		}
	}
	if !foundReverse {
		t.Fatalf("expected reverse edge decision:2 -> decision:1 (related), got %+v", reverse)
	}
}

func TestEdgesCreate_ErrorJSON(t *testing.T) {
	_, app := m4Setup(t)

	// Use an invalid from_type to force edge.Service.Create to return an error
	out, _, err := runCommandWithCapture(t, newEdgesCommand(app), []string{
		"--create",
		"--from", "bogus:1",
		"--to", "decision:2",
		"--relation", "related",
		"--json",
	})
	if err == nil {
		t.Fatal("expected error for invalid from_type, got success")
	}

	// Verify JSON error output is produced
	if !strings.Contains(out, `"error"`) || !strings.Contains(out, "internal_error") {
		t.Fatalf("expected JSON error output with internal_error, got: %s", out)
	}
	if !strings.Contains(out, "invalid from_type") {
		t.Fatalf("expected 'invalid from_type' in JSON error output, got: %s", out)
	}
}

func TestEdgesCreate_RequiresFromAndTo(t *testing.T) {
	_, app := m4Setup(t)

	// Missing --to
	_, _, err := runCommandWithCapture(t, newEdgesCommand(app), []string{
		"--create", "--from", "decision:1", "--json",
	})
	if err == nil {
		t.Fatal("expected error when --to is missing")
	}

	// Missing --from
	_, _, err = runCommandWithCapture(t, newEdgesCommand(app), []string{
		"--create", "--to", "decision:2", "--json",
	})
	if err == nil {
		t.Fatal("expected error when --from is missing")
	}
}
