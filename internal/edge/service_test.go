package edge

import (
	"context"
	"database/sql"
	"testing"

	"github.com/robertguss/recon/internal/db"
)

func edgeTestDB(t *testing.T) (*sql.DB, func()) {
	t.Helper()
	root := t.TempDir()
	if _, err := db.EnsureReconDir(root); err != nil {
		t.Fatalf("EnsureReconDir: %v", err)
	}
	conn, err := db.Open(db.DBPath(root))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := db.RunMigrations(conn); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}
	return conn, func() { _ = conn.Close() }
}

func TestCreateEdge(t *testing.T) {
	conn, cleanup := edgeTestDB(t)
	defer cleanup()
	svc := NewService(conn)

	e, err := svc.Create(context.Background(), CreateInput{
		FromType:   "decision",
		FromID:     1,
		ToType:     "package",
		ToRef:      "internal/cli",
		Relation:   "affects",
		Source:     "manual",
		Confidence: "high",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if e.ID == 0 {
		t.Fatal("expected non-zero ID")
	}
	if e.FromType != "decision" || e.ToRef != "internal/cli" {
		t.Fatalf("unexpected edge: %+v", e)
	}
}

func TestCreateEdge_Duplicate(t *testing.T) {
	conn, cleanup := edgeTestDB(t)
	defer cleanup()
	svc := NewService(conn)

	input := CreateInput{
		FromType: "decision", FromID: 1,
		ToType: "package", ToRef: "internal/cli",
		Relation: "affects", Source: "manual", Confidence: "high",
	}
	if _, err := svc.Create(context.Background(), input); err != nil {
		t.Fatalf("Create: %v", err)
	}
	_, err := svc.Create(context.Background(), input)
	if err == nil {
		t.Fatal("expected duplicate error")
	}
}

func TestCreateEdge_Validation(t *testing.T) {
	conn, cleanup := edgeTestDB(t)
	defer cleanup()
	svc := NewService(conn)
	ctx := context.Background()

	tests := []struct {
		name  string
		input CreateInput
		want  string
	}{
		{"empty from_type", CreateInput{FromID: 1, ToType: "package", ToRef: "x", Relation: "affects"}, "from_type is required"},
		{"empty to_type", CreateInput{FromType: "decision", FromID: 1, ToRef: "x", Relation: "affects"}, "to_type is required"},
		{"empty to_ref", CreateInput{FromType: "decision", FromID: 1, ToType: "package", Relation: "affects"}, "to_ref is required"},
		{"empty relation", CreateInput{FromType: "decision", FromID: 1, ToType: "package", ToRef: "x"}, "relation is required"},
		{"invalid from_type", CreateInput{FromType: "bogus", FromID: 1, ToType: "package", ToRef: "x", Relation: "affects"}, "invalid from_type"},
		{"invalid to_type", CreateInput{FromType: "decision", FromID: 1, ToType: "bogus", ToRef: "x", Relation: "affects"}, "invalid to_type"},
		{"invalid relation", CreateInput{FromType: "decision", FromID: 1, ToType: "package", ToRef: "x", Relation: "bogus"}, "invalid relation"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.Create(ctx, tt.input)
			if err == nil {
				t.Fatal("expected error")
			}
			if !contains(err.Error(), tt.want) {
				t.Fatalf("expected %q in error, got %q", tt.want, err.Error())
			}
		})
	}
}

func TestDeleteEdge(t *testing.T) {
	conn, cleanup := edgeTestDB(t)
	defer cleanup()
	svc := NewService(conn)
	ctx := context.Background()

	e, _ := svc.Create(ctx, CreateInput{
		FromType: "decision", FromID: 1,
		ToType: "package", ToRef: "internal/cli",
		Relation: "affects", Source: "manual", Confidence: "high",
	})

	if err := svc.Delete(ctx, e.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	edges, _ := svc.ListFrom(ctx, "decision", 1)
	if len(edges) != 0 {
		t.Fatalf("expected 0 edges after delete, got %d", len(edges))
	}
}

func TestDeleteEdge_NotFound(t *testing.T) {
	conn, cleanup := edgeTestDB(t)
	defer cleanup()
	svc := NewService(conn)

	err := svc.Delete(context.Background(), 9999)
	if err == nil {
		t.Fatal("expected error for non-existent edge")
	}
}

func TestListFrom(t *testing.T) {
	conn, cleanup := edgeTestDB(t)
	defer cleanup()
	svc := NewService(conn)
	ctx := context.Background()

	svc.Create(ctx, CreateInput{
		FromType: "decision", FromID: 1,
		ToType: "package", ToRef: "internal/cli",
		Relation: "affects", Source: "manual", Confidence: "high",
	})
	svc.Create(ctx, CreateInput{
		FromType: "decision", FromID: 1,
		ToType: "package", ToRef: "internal/orient",
		Relation: "affects", Source: "auto", Confidence: "medium",
	})
	svc.Create(ctx, CreateInput{
		FromType: "pattern", FromID: 1,
		ToType: "package", ToRef: "internal/cli",
		Relation: "affects", Source: "manual", Confidence: "high",
	})

	edges, err := svc.ListFrom(ctx, "decision", 1)
	if err != nil {
		t.Fatalf("ListFrom: %v", err)
	}
	if len(edges) != 2 {
		t.Fatalf("expected 2 edges, got %d", len(edges))
	}
}

func TestListTo(t *testing.T) {
	conn, cleanup := edgeTestDB(t)
	defer cleanup()
	svc := NewService(conn)
	ctx := context.Background()

	svc.Create(ctx, CreateInput{
		FromType: "decision", FromID: 1,
		ToType: "package", ToRef: "internal/cli",
		Relation: "affects", Source: "manual", Confidence: "high",
	})
	svc.Create(ctx, CreateInput{
		FromType: "pattern", FromID: 1,
		ToType: "package", ToRef: "internal/cli",
		Relation: "affects", Source: "manual", Confidence: "high",
	})

	edges, err := svc.ListTo(ctx, "package", "internal/cli")
	if err != nil {
		t.Fatalf("ListTo: %v", err)
	}
	if len(edges) != 2 {
		t.Fatalf("expected 2 edges, got %d", len(edges))
	}
}

func TestCreateEdge_Bidirectional(t *testing.T) {
	conn, cleanup := edgeTestDB(t)
	defer cleanup()
	svc := NewService(conn)
	ctx := context.Background()

	_, err := svc.Create(ctx, CreateInput{
		FromType: "decision", FromID: 1,
		ToType: "decision", ToRef: "2",
		Relation: "related", Source: "manual", Confidence: "high",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Reverse edge should exist
	edges, err := svc.ListFrom(ctx, "decision", 2)
	if err != nil {
		t.Fatalf("ListFrom: %v", err)
	}
	if len(edges) != 1 {
		t.Fatalf("expected 1 reverse edge, got %d", len(edges))
	}
	if edges[0].ToRef != "1" || edges[0].Relation != "related" {
		t.Fatalf("unexpected reverse edge: %+v", edges[0])
	}
}

func TestDeleteEdge_Bidirectional(t *testing.T) {
	conn, cleanup := edgeTestDB(t)
	defer cleanup()
	svc := NewService(conn)
	ctx := context.Background()

	e, _ := svc.Create(ctx, CreateInput{
		FromType: "decision", FromID: 1,
		ToType: "decision", ToRef: "2",
		Relation: "related", Source: "manual", Confidence: "high",
	})

	// Delete the forward edge
	if err := svc.Delete(ctx, e.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// Reverse should also be gone
	edges, _ := svc.ListFrom(ctx, "decision", 2)
	if len(edges) != 0 {
		t.Fatalf("expected 0 edges after bidirectional delete, got %d", len(edges))
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
