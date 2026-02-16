# SQLite Knowledge Graph Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to
> implement this plan task-by-task.

**Goal:** Add a graph layer to recon using a single `edges` table in SQLite,
connecting knowledge entities (decisions, patterns) to each other and to code
entities (packages, files, symbols).

**Architecture:** A new `internal/edge` package provides CRUD and traversal
operations on a generic `edges` table. The table uses `(from_type, from_id)` for
knowledge entity sources and `(to_type, to_ref)` text references for targets
(surviving sync). Auto-linking runs after `decide`/`pattern` creation, scanning
title+reasoning for package paths and symbol names. Orient nests knowledge under
affected modules. Recall walks 1-hop edges from search results.

**Tech Stack:** Go, SQLite (modernc.org/sqlite), Cobra CLI, golang-migrate,
go-sqlmock for error path testing

---

### Task 1: Add edges table migration

Create migration 000004 that adds the `edges` table and migrates existing
`pattern_files` data into it, then drops `pattern_files`.

**Files:**

- Create: `internal/db/migrations/000004_edges.up.sql`
- Create: `internal/db/migrations/000004_edges.down.sql`

**Step 1: Write the up migration**

Create `internal/db/migrations/000004_edges.up.sql`:

```sql
CREATE TABLE IF NOT EXISTS edges (
    id          INTEGER PRIMARY KEY,
    from_type   TEXT NOT NULL,
    from_id     INTEGER NOT NULL,
    to_type     TEXT NOT NULL,
    to_ref      TEXT NOT NULL,
    relation    TEXT NOT NULL,
    source      TEXT NOT NULL DEFAULT 'manual',
    confidence  TEXT NOT NULL DEFAULT 'medium',
    created_at  TEXT NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_edges_unique
    ON edges(from_type, from_id, to_type, to_ref, relation);
CREATE INDEX IF NOT EXISTS idx_edges_from ON edges(from_type, from_id);
CREATE INDEX IF NOT EXISTS idx_edges_to ON edges(to_type, to_ref);
CREATE INDEX IF NOT EXISTS idx_edges_relation ON edges(relation);

-- Migrate pattern_files into edges
INSERT INTO edges (from_type, from_id, to_type, to_ref, relation, source, confidence, created_at)
SELECT 'pattern', pf.pattern_id, 'file', pf.file_path, 'affects', 'auto', 'medium',
       COALESCE(p.created_at, datetime('now'))
FROM pattern_files pf
LEFT JOIN patterns p ON p.id = pf.pattern_id;

DROP TABLE IF EXISTS pattern_files;
```

**Step 2: Write the down migration**

Create `internal/db/migrations/000004_edges.down.sql`:

```sql
CREATE TABLE IF NOT EXISTS pattern_files (
    id         INTEGER PRIMARY KEY,
    pattern_id INTEGER REFERENCES patterns(id) ON DELETE CASCADE,
    file_path  TEXT NOT NULL
);

INSERT INTO pattern_files (pattern_id, file_path)
SELECT from_id, to_ref FROM edges
WHERE from_type = 'pattern' AND to_type = 'file' AND relation = 'affects';

DROP TABLE IF EXISTS edges;
```

**Step 3: Verify migrations apply cleanly**

Run: `just db-reset && just init && just sync`

Expected: No errors. `.recon/recon.db` has an `edges` table.

**Step 4: Verify the edges table exists with correct schema**

Run: `sqlite3 .recon/recon.db ".schema edges"`

Expected: Shows the CREATE TABLE and index statements.

**Step 5: Commit**

```bash
git add internal/db/migrations/000004_edges.up.sql internal/db/migrations/000004_edges.down.sql
git commit -m "feat(db): add edges table migration with pattern_files migration"
```

---

### Task 2: Create edge service with CRUD operations

New package `internal/edge` with a `Service` struct providing `Create`,
`Delete`, `ListFrom`, `ListTo`, and `ListAll`.

**Files:**

- Create: `internal/edge/service.go`
- Create: `internal/edge/service_test.go`

**Step 1: Write the failing tests**

Create `internal/edge/service_test.go`:

```go
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
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/edge/... -v`

Expected: FAIL — package does not exist

**Step 3: Write the edge service**

Create `internal/edge/service.go`:

```go
package edge

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

var validFromTypes = map[string]bool{
	"decision": true,
	"pattern":  true,
}

var validToTypes = map[string]bool{
	"decision": true,
	"pattern":  true,
	"package":  true,
	"file":     true,
	"symbol":   true,
}

var validRelations = map[string]bool{
	"affects":      true,
	"evidenced_by": true,
	"supersedes":   true,
	"contradicts":  true,
	"related":      true,
	"reinforces":   true,
}

// BidirectionalRelations are stored as two directed rows.
var BidirectionalRelations = map[string]bool{
	"contradicts": true,
	"related":     true,
}

type Edge struct {
	ID         int64  `json:"id"`
	FromType   string `json:"from_type"`
	FromID     int64  `json:"from_id"`
	ToType     string `json:"to_type"`
	ToRef      string `json:"to_ref"`
	Relation   string `json:"relation"`
	Source     string `json:"source"`
	Confidence string `json:"confidence"`
	CreatedAt  string `json:"created_at"`
}

type CreateInput struct {
	FromType   string
	FromID     int64
	ToType     string
	ToRef      string
	Relation   string
	Source     string
	Confidence string
}

// ErrNotFound is returned when an edge does not exist.
var ErrNotFound = fmt.Errorf("not found")

type Service struct {
	db *sql.DB
}

func NewService(conn *sql.DB) *Service {
	return &Service{db: conn}
}

func (s *Service) Create(ctx context.Context, in CreateInput) (Edge, error) {
	if err := validate(in); err != nil {
		return Edge{}, err
	}

	if in.Source == "" {
		in.Source = "manual"
	}
	if in.Confidence == "" {
		in.Confidence = "medium"
	}

	now := time.Now().UTC().Format(time.RFC3339)

	res, err := s.db.ExecContext(ctx, `
INSERT INTO edges (from_type, from_id, to_type, to_ref, relation, source, confidence, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?);
`, in.FromType, in.FromID, in.ToType, in.ToRef, in.Relation, in.Source, in.Confidence, now)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return Edge{}, fmt.Errorf("edge already exists: %s:%d -> %s:%s (%s)", in.FromType, in.FromID, in.ToType, in.ToRef, in.Relation)
		}
		return Edge{}, fmt.Errorf("insert edge: %w", err)
	}

	id, _ := res.LastInsertId()
	return Edge{
		ID: id, FromType: in.FromType, FromID: in.FromID,
		ToType: in.ToType, ToRef: in.ToRef, Relation: in.Relation,
		Source: in.Source, Confidence: in.Confidence, CreatedAt: now,
	}, nil
}

func (s *Service) Delete(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM edges WHERE id = ?;`, id)
	if err != nil {
		return fmt.Errorf("delete edge: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("edge %d: %w", id, ErrNotFound)
	}
	return nil
}

func (s *Service) ListFrom(ctx context.Context, fromType string, fromID int64) ([]Edge, error) {
	return s.query(ctx, `
SELECT id, from_type, from_id, to_type, to_ref, relation, source, confidence, created_at
FROM edges WHERE from_type = ? AND from_id = ?
ORDER BY relation, to_type, to_ref;
`, fromType, fromID)
}

func (s *Service) ListTo(ctx context.Context, toType, toRef string) ([]Edge, error) {
	return s.query(ctx, `
SELECT id, from_type, from_id, to_type, to_ref, relation, source, confidence, created_at
FROM edges WHERE to_type = ? AND to_ref = ?
ORDER BY relation, from_type, from_id;
`, toType, toRef)
}

func (s *Service) ListAll(ctx context.Context) ([]Edge, error) {
	return s.query(ctx, `
SELECT id, from_type, from_id, to_type, to_ref, relation, source, confidence, created_at
FROM edges ORDER BY from_type, from_id, relation, to_type, to_ref;
`)
}

func (s *Service) query(ctx context.Context, q string, args ...any) ([]Edge, error) {
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("query edges: %w", err)
	}
	defer rows.Close()

	var edges []Edge
	for rows.Next() {
		var e Edge
		if err := rows.Scan(&e.ID, &e.FromType, &e.FromID, &e.ToType, &e.ToRef,
			&e.Relation, &e.Source, &e.Confidence, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan edge: %w", err)
		}
		edges = append(edges, e)
	}
	return edges, rows.Err()
}

func validate(in CreateInput) error {
	if strings.TrimSpace(in.FromType) == "" {
		return fmt.Errorf("from_type is required")
	}
	if strings.TrimSpace(in.ToType) == "" {
		return fmt.Errorf("to_type is required")
	}
	if strings.TrimSpace(in.ToRef) == "" {
		return fmt.Errorf("to_ref is required")
	}
	if strings.TrimSpace(in.Relation) == "" {
		return fmt.Errorf("relation is required")
	}
	if !validFromTypes[in.FromType] {
		return fmt.Errorf("invalid from_type %q; must be one of: decision, pattern", in.FromType)
	}
	if !validToTypes[in.ToType] {
		return fmt.Errorf("invalid to_type %q; must be one of: decision, pattern, package, file, symbol", in.ToType)
	}
	if !validRelations[in.Relation] {
		return fmt.Errorf("invalid relation %q; must be one of: affects, evidenced_by, supersedes, contradicts, related, reinforces", in.Relation)
	}
	return nil
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/edge/... -v`

Expected: PASS

**Step 5: Run full test suite**

Run: `just test`

Expected: PASS

**Step 6: Commit**

```bash
git add internal/edge/service.go internal/edge/service_test.go
git commit -m "feat(edge): add edge service with CRUD operations"
```

---

### Task 3: Add bidirectional edge support to Create

When creating a `contradicts` or `related` edge, automatically insert the
reverse row. When deleting, also delete the reverse.

**Files:**

- Modify: `internal/edge/service.go` — update `Create` and `Delete`
- Modify: `internal/edge/service_test.go` — add bidirectional tests

**Step 1: Write the failing test**

Add to `internal/edge/service_test.go`:

```go
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
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/edge/... -run TestCreateEdge_Bidirectional -v`

Expected: FAIL — reverse edge not created

**Step 3: Update Create to insert reverse for bidirectional relations**

In `internal/edge/service.go`, update `Create` — after the initial insert and
before returning, check if the relation is bidirectional. If so and the target
is a knowledge entity (decision/pattern), insert the reverse row. For
bidirectional edges where `to_type` is not a knowledge entity (e.g.,
`contradicts` between a decision and a package), skip the reverse since packages
aren't `from_type` eligible.

```go
// After the initial insert succeeds:
if BidirectionalRelations[in.Relation] && validFromTypes[in.ToType] {
	_, err := s.db.ExecContext(ctx, `
INSERT OR IGNORE INTO edges (from_type, from_id, to_type, to_ref, relation, source, confidence, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?);
`, in.ToType, toIDFromRef(in.ToRef), in.FromType, fmt.Sprintf("%d", in.FromID), in.Relation, in.Source, in.Confidence, now)
	if err != nil {
		return Edge{}, fmt.Errorf("insert reverse edge: %w", err)
	}
}
```

Add helper:

```go
func toIDFromRef(ref string) int64 {
	var id int64
	fmt.Sscanf(ref, "%d", &id)
	return id
}
```

Update `Delete` to also remove the reverse:

```go
func (s *Service) Delete(ctx context.Context, id int64) error {
	// Fetch the edge first to check for bidirectional reverse
	var e Edge
	err := s.db.QueryRowContext(ctx, `
SELECT id, from_type, from_id, to_type, to_ref, relation FROM edges WHERE id = ?;
`, id).Scan(&e.ID, &e.FromType, &e.FromID, &e.ToType, &e.ToRef, &e.Relation)
	if err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("edge %d: %w", id, ErrNotFound)
		}
		return fmt.Errorf("fetch edge: %w", err)
	}

	if _, err := s.db.ExecContext(ctx, `DELETE FROM edges WHERE id = ?;`, id); err != nil {
		return fmt.Errorf("delete edge: %w", err)
	}

	// Delete reverse if bidirectional
	if BidirectionalRelations[e.Relation] && validFromTypes[e.ToType] {
		s.db.ExecContext(ctx, `
DELETE FROM edges WHERE from_type = ? AND from_id = ? AND to_type = ? AND to_ref = ? AND relation = ?;
`, e.ToType, toIDFromRef(e.ToRef), e.FromType, fmt.Sprintf("%d", e.FromID), e.Relation)
	}

	return nil
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/edge/... -v`

Expected: PASS

**Step 5: Commit**

```bash
git add internal/edge/service.go internal/edge/service_test.go
git commit -m "feat(edge): add bidirectional edge support for related and contradicts"
```

---

### Task 4: Add auto-linking service

New file in `internal/edge/` that scans title + reasoning text for known package
paths and distinctive symbol names, creating `affects` edges automatically.

**Files:**

- Create: `internal/edge/autolink.go`
- Create: `internal/edge/autolink_test.go`

**Step 1: Write the failing tests**

Create `internal/edge/autolink_test.go`:

```go
package edge

import (
	"context"
	"testing"
)

func seedPackages(t *testing.T, conn interface{ ExecContext(ctx context.Context, query string, args ...any) (interface{}, error) }, db *sql.DB) {
	t.Helper()
	now := "2024-01-01T00:00:00Z"
	db.ExecContext(context.Background(),
		`INSERT INTO packages (path, name, import_path, created_at, updated_at) VALUES ('internal/cli', 'cli', 'example.com/test/internal/cli', ?, ?)`, now, now)
	db.ExecContext(context.Background(),
		`INSERT INTO packages (path, name, import_path, created_at, updated_at) VALUES ('internal/orient', 'orient', 'example.com/test/internal/orient', ?, ?)`, now, now)
}

func seedExportedSymbol(t *testing.T, db *sql.DB, name string) {
	t.Helper()
	now := "2024-01-01T00:00:00Z"
	// Ensure package exists
	db.ExecContext(context.Background(),
		`INSERT OR IGNORE INTO packages (path, name, import_path, created_at, updated_at) VALUES ('internal/cli', 'cli', 'example.com/test/internal/cli', ?, ?)`, now, now)
	var pkgID int64
	db.QueryRowContext(context.Background(), `SELECT id FROM packages WHERE path = 'internal/cli'`).Scan(&pkgID)
	db.ExecContext(context.Background(),
		`INSERT OR IGNORE INTO files (package_id, path, language, lines, hash, created_at, updated_at) VALUES (?, 'internal/cli/exit_error.go', 'go', 20, 'abc', ?, ?)`, pkgID, now, now)
	var fileID int64
	db.QueryRowContext(context.Background(), `SELECT id FROM files WHERE path = 'internal/cli/exit_error.go'`).Scan(&fileID)
	db.ExecContext(context.Background(),
		`INSERT OR IGNORE INTO symbols (file_id, kind, name, signature, body, line_start, line_end, exported, receiver) VALUES (?, 'type', ?, '', '{}', 1, 5, 1, '')`, fileID, name)
}

func TestAutoLink_FindsPackagePaths(t *testing.T) {
	conn, cleanup := edgeTestDB(t)
	defer cleanup()
	now := "2024-01-01T00:00:00Z"
	conn.ExecContext(context.Background(),
		`INSERT INTO packages (path, name, import_path, created_at, updated_at) VALUES ('internal/cli', 'cli', 'example.com/test/internal/cli', ?, ?)`, now, now)
	conn.ExecContext(context.Background(),
		`INSERT INTO packages (path, name, import_path, created_at, updated_at) VALUES ('internal/orient', 'orient', 'example.com/test/internal/orient', ?, ?)`, now, now)

	linker := NewAutoLinker(conn)
	edges := linker.Detect(context.Background(), "decision", 1, "ExitError convention", "Used in internal/cli for all commands, also affects internal/orient")

	pkgRefs := map[string]bool{}
	for _, e := range edges {
		if e.ToType == "package" {
			pkgRefs[e.ToRef] = true
		}
	}
	if !pkgRefs["internal/cli"] {
		t.Fatal("expected internal/cli in auto-linked edges")
	}
	if !pkgRefs["internal/orient"] {
		t.Fatal("expected internal/orient in auto-linked edges")
	}
}

func TestAutoLink_FindsDistinctiveSymbols(t *testing.T) {
	conn, cleanup := edgeTestDB(t)
	defer cleanup()
	now := "2024-01-01T00:00:00Z"
	conn.ExecContext(context.Background(),
		`INSERT INTO packages (path, name, import_path, created_at, updated_at) VALUES ('internal/cli', 'cli', 'example.com/test/internal/cli', ?, ?)`, now, now)
	var pkgID int64
	conn.QueryRowContext(context.Background(), `SELECT id FROM packages WHERE path = 'internal/cli'`).Scan(&pkgID)
	conn.ExecContext(context.Background(),
		`INSERT INTO files (package_id, path, language, lines, hash, created_at, updated_at) VALUES (?, 'internal/cli/exit_error.go', 'go', 20, 'abc', ?, ?)`, pkgID, now, now)
	var fileID int64
	conn.QueryRowContext(context.Background(), `SELECT id FROM files WHERE path = 'internal/cli/exit_error.go'`).Scan(&fileID)
	conn.ExecContext(context.Background(),
		`INSERT INTO symbols (file_id, kind, name, signature, body, line_start, line_end, exported, receiver) VALUES (?, 'type', 'ExitError', '', '{}', 1, 5, 1, '')`, fileID)

	linker := NewAutoLinker(conn)
	edges := linker.Detect(context.Background(), "decision", 1, "ExitError is the standard error type", "All commands return ExitError")

	symRefs := map[string]bool{}
	for _, e := range edges {
		if e.ToType == "symbol" {
			symRefs[e.ToRef] = true
		}
	}
	if !symRefs["internal/cli.ExitError"] {
		t.Fatalf("expected internal/cli.ExitError in auto-linked edges, got %v", symRefs)
	}
}

func TestAutoLink_SkipsShortSymbolNames(t *testing.T) {
	conn, cleanup := edgeTestDB(t)
	defer cleanup()
	now := "2024-01-01T00:00:00Z"
	conn.ExecContext(context.Background(),
		`INSERT INTO packages (path, name, import_path, created_at, updated_at) VALUES ('internal/cli', 'cli', 'example.com/test/internal/cli', ?, ?)`, now, now)
	var pkgID int64
	conn.QueryRowContext(context.Background(), `SELECT id FROM packages WHERE path = 'internal/cli'`).Scan(&pkgID)
	conn.ExecContext(context.Background(),
		`INSERT INTO files (package_id, path, language, lines, hash, created_at, updated_at) VALUES (?, 'internal/cli/run.go', 'go', 10, 'abc', ?, ?)`, pkgID, now, now)
	var fileID int64
	conn.QueryRowContext(context.Background(), `SELECT id FROM files WHERE path = 'internal/cli/run.go'`).Scan(&fileID)
	conn.ExecContext(context.Background(),
		`INSERT INTO symbols (file_id, kind, name, signature, body, line_start, line_end, exported, receiver) VALUES (?, 'func', 'Run', '', '{}', 1, 5, 1, '')`, fileID)

	linker := NewAutoLinker(conn)
	edges := linker.Detect(context.Background(), "decision", 1, "Run function", "We use Run everywhere")

	for _, e := range edges {
		if e.ToType == "symbol" {
			t.Fatalf("should not auto-link short symbol name 'Run', got %+v", e)
		}
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/edge/... -run TestAutoLink -v`

Expected: FAIL — `NewAutoLinker` does not exist

**Step 3: Implement the auto-linker**

Create `internal/edge/autolink.go`:

```go
package edge

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"unicode"
)

const minSymbolNameLen = 6

type AutoLinker struct {
	db *sql.DB
}

func NewAutoLinker(conn *sql.DB) *AutoLinker {
	return &AutoLinker{db: conn}
}

// DetectedEdge represents an edge suggested by auto-linking.
type DetectedEdge struct {
	ToType   string
	ToRef    string
	Relation string
}

// Detect scans title and reasoning for known package paths and distinctive
// exported symbol names. Returns suggested edges (not yet persisted).
func (a *AutoLinker) Detect(ctx context.Context, fromType string, fromID int64, title, reasoning string) []DetectedEdge {
	text := title + " " + reasoning
	var edges []DetectedEdge
	seen := map[string]bool{}

	// Match package paths
	packages := a.loadPackagePaths(ctx)
	for _, pkg := range packages {
		if strings.Contains(text, pkg) {
			key := "package:" + pkg
			if !seen[key] {
				seen[key] = true
				edges = append(edges, DetectedEdge{ToType: "package", ToRef: pkg, Relation: "affects"})
			}
		}
	}

	// Match distinctive exported symbol names
	symbols := a.loadExportedSymbols(ctx)
	for _, sym := range symbols {
		if len(sym.Name) < minSymbolNameLen {
			continue
		}
		if !isDistinctive(sym.Name) {
			continue
		}
		if containsWord(text, sym.Name) {
			ref := sym.Package + "." + sym.Name
			key := "symbol:" + ref
			if !seen[key] {
				seen[key] = true
				edges = append(edges, DetectedEdge{ToType: "symbol", ToRef: ref, Relation: "affects"})
			}
		}
	}

	return edges
}

type indexedSymbol struct {
	Name    string
	Package string
}

func (a *AutoLinker) loadPackagePaths(ctx context.Context) []string {
	rows, err := a.db.QueryContext(ctx, `SELECT path FROM packages ORDER BY length(path) DESC;`)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var paths []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			continue
		}
		paths = append(paths, p)
	}
	return paths
}

func (a *AutoLinker) loadExportedSymbols(ctx context.Context) []indexedSymbol {
	rows, err := a.db.QueryContext(ctx, `
SELECT s.name, p.path
FROM symbols s
JOIN files f ON f.id = s.file_id
JOIN packages p ON p.id = f.package_id
WHERE s.exported = 1;
`)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var syms []indexedSymbol
	for rows.Next() {
		var sym indexedSymbol
		if err := rows.Scan(&sym.Name, &sym.Package); err != nil {
			continue
		}
		syms = append(syms, sym)
	}
	return syms
}

// containsWord checks if text contains name as a whole word (bounded by
// non-alphanumeric characters or string boundaries).
func containsWord(text, name string) bool {
	idx := 0
	for {
		pos := strings.Index(text[idx:], name)
		if pos == -1 {
			return false
		}
		start := idx + pos
		end := start + len(name)

		startOK := start == 0 || !isAlphaNum(rune(text[start-1]))
		endOK := end == len(text) || !isAlphaNum(rune(text[end]))

		if startOK && endOK {
			return true
		}
		idx = start + 1
	}
}

func isAlphaNum(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

// isDistinctive filters out common Go names that would cause false positives.
func isDistinctive(name string) bool {
	common := map[string]bool{
		"String": true, "Error": true, "Close": true, "Write": true,
		"Reader": true, "Writer": true, "Buffer": true, "Logger": true,
		"Config": true, "Option": true, "Result": true, "Status": true,
		"Server": true, "Client": true, "Handle": true,
	}
	return !common[name]
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/edge/... -run TestAutoLink -v`

Expected: PASS

**Step 5: Run full test suite**

Run: `just test`

Expected: PASS

**Step 6: Commit**

```bash
git add internal/edge/autolink.go internal/edge/autolink_test.go
git commit -m "feat(edge): add conservative auto-linking for package paths and symbols"
```

---

### Task 5: Integrate edge creation into decide command

Add `--affects` flag to `decide` that creates manual edges. Run auto-linking
after successful promotion.

**Files:**

- Modify: `internal/cli/decide.go` — add `--affects` flag and post-promote edge
  creation
- Modify: `internal/cli/decide_extra_test.go` or create new test file

**Step 1: Write the failing test**

Add a test to `internal/cli/commands_test.go` or a new
`internal/cli/decide_edges_test.go` that runs the decide command with
`--affects` and checks that edges were created. Follow the existing CLI test
pattern (execute cobra command, check output).

```go
func TestDecide_AffectsFlag_CreatesEdge(t *testing.T) {
	// Set up a test DB with packages indexed
	// Run: decide "test" --reasoning "r" --evidence-summary "e"
	//   --check-type file_exists --check-path go.mod --affects internal/cli
	// Then query edges table to verify edge was created
}
```

The exact test structure should follow the existing CLI test patterns in
`commands_test.go`. The key assertion: after a successful decide with
`--affects internal/cli`, the edges table should contain a row with
`from_type='decision'`, `to_type='package'`, `to_ref='internal/cli'`,
`relation='affects'`, `source='manual'`.

**Step 2: Run test to verify it fails**

Run: `go test ./internal/cli/... -run TestDecide_AffectsFlag -v`

Expected: FAIL — no `--affects` flag

**Step 3: Add --affects flag to decide command**

In `internal/cli/decide.go`, add a string slice variable:

```go
var affectsRefs []string
```

Register the flag:

```go
cmd.Flags().StringSliceVar(&affectsRefs, "affects", nil, "Package/file/symbol this decision affects (creates edges)")
```

After the successful promotion block (after `result.Promoted` is true and before
returning), create edges:

```go
if result.Promoted && len(affectsRefs) > 0 {
	edgeSvc := edge.NewService(conn)
	for _, ref := range affectsRefs {
		refType := inferRefType(ref)
		_, err := edgeSvc.Create(cmd.Context(), edge.CreateInput{
			FromType:   "decision",
			FromID:     result.DecisionID,
			ToType:     refType,
			ToRef:      ref,
			Relation:   "affects",
			Source:     "manual",
			Confidence: "high",
		})
		if err != nil && !jsonOut {
			fmt.Printf("  edge warning: %v\n", err)
		}
	}
}
```

Add `inferRefType` helper:

```go
func inferRefType(ref string) string {
	if strings.Contains(ref, ".go") {
		return "file"
	}
	if strings.Contains(ref, ".") && !strings.Contains(ref, "/") {
		return "symbol"
	}
	return "package"
}
```

Also add auto-linking after promotion:

```go
if result.Promoted {
	linker := edge.NewAutoLinker(conn)
	detected := linker.Detect(cmd.Context(), "decision", result.DecisionID, title, reasoning)
	for _, d := range detected {
		edgeSvc.Create(cmd.Context(), edge.CreateInput{
			FromType: "decision", FromID: result.DecisionID,
			ToType: d.ToType, ToRef: d.ToRef, Relation: d.Relation,
			Source: "auto", Confidence: "medium",
		})
	}
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/cli/... -v`

Expected: PASS

**Step 5: Run full test suite**

Run: `just test`

Expected: PASS

**Step 6: Commit**

```bash
git add internal/cli/decide.go
git commit -m "feat(decide): add --affects flag and auto-linking on promotion"
```

---

### Task 6: Integrate edge creation into pattern command

Mirror the decide integration — add `--affects` flag and auto-linking.

**Files:**

- Modify: `internal/cli/pattern.go` — add `--affects` flag and post-promote edge
  creation

**Step 1: Add --affects flag to pattern command**

Same pattern as Task 5. Add `affectsRefs` string slice, register flag, create
edges after promotion, run auto-linker.

**Step 2: Run tests**

Run: `just test`

Expected: PASS

**Step 3: Commit**

```bash
git add internal/cli/pattern.go
git commit -m "feat(pattern): add --affects flag and auto-linking on promotion"
```

---

### Task 7: Add edge-aware orient output

Modify orient to nest decisions/patterns under the modules they affect via
edges.

**Files:**

- Modify: `internal/orient/service.go` — add `loadModuleEdges` method, add edges
  to `ModuleSummary`
- Modify: `internal/orient/render.go` — render knowledge under modules
- Modify: `internal/orient/service_test.go` — test edge-aware output
- Modify: `internal/orient/render_test.go` — test rendering

**Step 1: Write the failing test**

Add to `internal/orient/service_test.go`:

```go
func TestBuild_ModulesIncludeEdges(t *testing.T) {
	// Set up test DB with:
	// - A package "internal/cli"
	// - A decision #1
	// - An edge: decision:1 -> package:internal/cli (affects)
	// Build the orient payload
	// Assert that the module for internal/cli has the decision listed
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/orient/... -run TestBuild_ModulesIncludeEdges -v`

Expected: FAIL — no edges field on ModuleSummary

**Step 3: Add edges to ModuleSummary struct**

In `internal/orient/service.go`:

```go
type ModuleKnowledge struct {
	ID         int64  `json:"id"`
	Type       string `json:"type"`       // "decision" or "pattern"
	Title      string `json:"title"`
	Confidence string `json:"confidence"`
}

// Add to ModuleSummary:
Knowledge []ModuleKnowledge `json:"knowledge,omitempty"`
```

Add `loadModuleEdges` method that queries edges + joins to decisions/patterns:

```go
func (s *Service) loadModuleEdges(ctx context.Context, payload *Payload) error {
	rows, err := s.db.QueryContext(ctx, `
SELECT e.to_ref, e.from_type, e.from_id,
       COALESCE(d.title, p.title, '') AS title,
       COALESCE(d.confidence, p.confidence, 'medium') AS confidence
FROM edges e
LEFT JOIN decisions d ON e.from_type = 'decision' AND e.from_id = d.id AND d.status = 'active'
LEFT JOIN patterns p ON e.from_type = 'pattern' AND e.from_id = p.id AND p.status = 'active'
WHERE e.to_type = 'package' AND e.relation = 'affects'
  AND (d.id IS NOT NULL OR p.id IS NOT NULL)
ORDER BY e.to_ref, e.from_type, confidence DESC;
`)
	if err != nil {
		// Non-fatal: edges table might not exist in older DBs
		return nil
	}
	defer rows.Close()

	moduleKnowledge := map[string][]ModuleKnowledge{}
	for rows.Next() {
		var pkgPath, fromType string
		var fromID int64
		var title, confidence string
		if err := rows.Scan(&pkgPath, &fromType, &fromID, &title, &confidence); err != nil {
			continue
		}
		moduleKnowledge[pkgPath] = append(moduleKnowledge[pkgPath], ModuleKnowledge{
			ID: fromID, Type: fromType, Title: title, Confidence: confidence,
		})
	}

	for i := range payload.Modules {
		if k, ok := moduleKnowledge[payload.Modules[i].Path]; ok {
			// Cap at 5 per module
			if len(k) > 5 {
				k = k[:5]
			}
			payload.Modules[i].Knowledge = k
		}
	}
	return nil
}
```

Call it in `Build` after `loadPatterns`:

```go
if err := s.loadModuleEdges(ctx, &payload); err != nil {
	return Payload{}, err
}
```

**Step 4: Update render.go**

In `internal/orient/render.go`, update the modules section to show knowledge:

```go
for _, m := range payload.Modules {
	fmt.Fprintf(&b, "- %s (%s): %d files, %d lines [%s]\n", m.Path, m.Name, m.FileCount, m.LineCount, strings.ToUpper(m.Heat))
	for _, k := range m.Knowledge {
		fmt.Fprintf(&b, "    %s #%d: %s [%s]\n", k.Type, k.ID, k.Title, k.Confidence)
	}
}
```

**Step 5: Run tests**

Run: `go test ./internal/orient/... -v`

Expected: PASS

**Step 6: Run full test suite**

Run: `just test`

Expected: PASS

**Step 7: Commit**

```bash
git add internal/orient/service.go internal/orient/render.go internal/orient/service_test.go internal/orient/render_test.go
git commit -m "feat(orient): nest decisions and patterns under affected modules via edges"
```

---

### Task 8: Add edge-aware recall output

After FTS search, walk 1-hop edges from each result and include connected
entities in the response.

**Files:**

- Modify: `internal/recall/service.go` — add edge walking after FTS results
- Modify: `internal/recall/service_test.go` — test edge-enriched output
- Modify: `internal/cli/recall.go` — render connected edges in text output

**Step 1: Write the failing test**

Add to `internal/recall/service_test.go`:

```go
func TestRecall_IncludesConnectedEdges(t *testing.T) {
	// Set up test DB with:
	// - A decision "ExitError convention"
	// - An edge from that decision to package "internal/cli"
	// - FTS index entry for the decision
	// Recall "ExitError"
	// Assert result includes connected edges
}
```

**Step 2: Run test to verify it fails**

**Step 3: Add `ConnectedEdges` field to recall.Item**

In `internal/recall/service.go`:

```go
type ConnectedEdge struct {
	ToType   string `json:"to_type"`
	ToRef    string `json:"to_ref"`
	Relation string `json:"relation"`
}

// Add to Item struct:
ConnectedEdges []ConnectedEdge `json:"connected_edges,omitempty"`
```

After FTS/LIKE query returns items, walk edges for each:

```go
func (s *Service) enrichWithEdges(ctx context.Context, items []Item) {
	for i := range items {
		entityType := items[i].EntityType
		var entityID int64
		if entityType == "pattern" {
			entityID = items[i].PatternID
		} else {
			entityID = items[i].DecisionID
		}

		rows, err := s.db.QueryContext(ctx, `
SELECT to_type, to_ref, relation FROM edges
WHERE from_type = ? AND from_id = ?
ORDER BY relation, to_type;
`, entityType, entityID)
		if err != nil {
			continue
		}
		defer rows.Close()

		for rows.Next() {
			var ce ConnectedEdge
			if err := rows.Scan(&ce.ToType, &ce.ToRef, &ce.Relation); err != nil {
				continue
			}
			items[i].ConnectedEdges = append(items[i].ConnectedEdges, ce)
		}
	}
}
```

Call after FTS results:

```go
func (s *Service) Recall(ctx context.Context, query string, opts RecallOptions) (Result, error) {
	// ... existing code ...
	s.enrichWithEdges(ctx, items)
	return Result{Query: query, Items: items}, nil
}
```

**Step 4: Update CLI recall text output**

In `internal/cli/recall.go`, after printing each item, show connected edges:

```go
for _, ce := range item.ConnectedEdges {
	fmt.Printf("    %s: %s (%s)\n", ce.Relation, ce.ToRef, ce.ToType)
}
```

**Step 5: Run tests**

Run: `go test ./internal/recall/... -v`

Expected: PASS

**Step 6: Run full test suite**

Run: `just test`

Expected: PASS

**Step 7: Commit**

```bash
git add internal/recall/service.go internal/recall/service_test.go internal/cli/recall.go
git commit -m "feat(recall): walk 1-hop edges from search results"
```

---

### Task 9: Add `recon edges` command

New CLI command for direct edge management: `--from`, `--to`, `--delete`,
`--list`.

**Files:**

- Create: `internal/cli/edges.go`
- Modify: `internal/cli/root.go` — register edges command

**Step 1: Write the edges command**

Create `internal/cli/edges.go`:

```go
package cli

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/robertguss/recon/internal/edge"
	"github.com/spf13/cobra"
)

func newEdgesCommand(app *App) *cobra.Command {
	var (
		jsonOut  bool
		fromRef  string
		toRef    string
		deleteID int64
		listAll  bool
	)

	cmd := &cobra.Command{
		Use:   "edges",
		Short: "Manage knowledge graph edges",
		RunE: func(cmd *cobra.Command, args []string) error {
			conn, err := openExistingDB(app)
			if err != nil {
				if jsonOut {
					return exitJSONCommandError(err)
				}
				return err
			}
			defer conn.Close()

			svc := edge.NewService(conn)

			// Delete mode
			if deleteID > 0 {
				err := svc.Delete(cmd.Context(), deleteID)
				if err != nil {
					if jsonOut {
						code := "internal_error"
						if errors.Is(err, edge.ErrNotFound) {
							code = "not_found"
						}
						_ = writeJSONError(code, err.Error(), map[string]any{"id": deleteID})
						return ExitError{Code: 2}
					}
					return err
				}
				if jsonOut {
					return writeJSON(map[string]any{"deleted": true, "id": deleteID})
				}
				fmt.Printf("Edge %d deleted.\n", deleteID)
				return nil
			}

			// From mode: edges --from decision:2
			if fromRef != "" {
				fromType, fromID, err := parseEntityRef(fromRef)
				if err != nil {
					if jsonOut {
						_ = writeJSONError("invalid_input", err.Error(), nil)
						return ExitError{Code: 2}
					}
					return err
				}
				edges, err := svc.ListFrom(cmd.Context(), fromType, fromID)
				if err != nil {
					if jsonOut {
						_ = writeJSONError("internal_error", err.Error(), nil)
						return ExitError{Code: 2}
					}
					return err
				}
				return renderEdges(edges, jsonOut)
			}

			// To mode: edges --to package:internal/cli
			if toRef != "" {
				parts := strings.SplitN(toRef, ":", 2)
				if len(parts) != 2 {
					msg := "invalid --to format; use type:ref (e.g., package:internal/cli)"
					if jsonOut {
						_ = writeJSONError("invalid_input", msg, nil)
						return ExitError{Code: 2}
					}
					return ExitError{Code: 2, Message: msg}
				}
				edges, err := svc.ListTo(cmd.Context(), parts[0], parts[1])
				if err != nil {
					if jsonOut {
						_ = writeJSONError("internal_error", err.Error(), nil)
						return ExitError{Code: 2}
					}
					return err
				}
				return renderEdges(edges, jsonOut)
			}

			// List all mode
			if listAll {
				edges, err := svc.ListAll(cmd.Context())
				if err != nil {
					if jsonOut {
						_ = writeJSONError("internal_error", err.Error(), nil)
						return ExitError{Code: 2}
					}
					return err
				}
				return renderEdges(edges, jsonOut)
			}

			msg := "edges requires --from, --to, --delete, or --list"
			if jsonOut {
				_ = writeJSONError("missing_argument", msg, nil)
				return ExitError{Code: 2}
			}
			return ExitError{Code: 2, Message: msg}
		},
	}

	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output JSON")
	cmd.Flags().StringVar(&fromRef, "from", "", "List edges from entity (e.g., decision:2)")
	cmd.Flags().StringVar(&toRef, "to", "", "List edges to entity (e.g., package:internal/cli)")
	cmd.Flags().Int64Var(&deleteID, "delete", 0, "Delete an edge by ID")
	cmd.Flags().BoolVar(&listAll, "list", false, "List all edges")

	return cmd
}

func parseEntityRef(ref string) (string, int64, error) {
	parts := strings.SplitN(ref, ":", 2)
	if len(parts) != 2 {
		return "", 0, fmt.Errorf("invalid entity ref %q; use type:id (e.g., decision:2)", ref)
	}
	id, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return "", 0, fmt.Errorf("invalid entity ID %q; must be an integer", parts[1])
	}
	return parts[0], id, nil
}

func renderEdges(edges []edge.Edge, jsonOut bool) error {
	if jsonOut {
		return writeJSON(edges)
	}
	if len(edges) == 0 {
		fmt.Println("No edges found.")
		return nil
	}
	for _, e := range edges {
		fmt.Printf("#%d %s:%d -[%s]-> %s:%s (source=%s, confidence=%s)\n",
			e.ID, e.FromType, e.FromID, e.Relation, e.ToType, e.ToRef, e.Source, e.Confidence)
	}
	return nil
}
```

**Step 2: Register in root.go**

Add to `internal/cli/root.go`:

```go
root.AddCommand(newEdgesCommand(app))
```

**Step 3: Run full test suite**

Run: `just test`

Expected: PASS

**Step 4: Manual smoke test**

Run:

```bash
just build
./bin/recon edges --list --json
./bin/recon edges --from decision:1 --json
```

Expected: Empty arrays (no edges yet). No errors.

**Step 5: Commit**

```bash
git add internal/cli/edges.go internal/cli/root.go
git commit -m "feat(cli): add edges command for direct edge management"
```

---

### Task 10: Add edge-enriched find output (JSON only)

When `find` returns a symbol in `--json` mode, include knowledge edges pointing
at that symbol or its package.

**Files:**

- Modify: `internal/find/service.go` — add optional edge enrichment
- Modify: `internal/cli/find.go` — pass edge data in JSON output

**Step 1: Write the failing test**

Add to `internal/find/service_test.go` a test that sets up a symbol, an edge
pointing at its package, calls `Find`, and checks that the result includes the
edge (via a new `Knowledge` field on `Result`).

**Step 2: Add Knowledge field to find.Result**

In `internal/find/service.go`:

```go
type KnowledgeLink struct {
	EntityType string `json:"entity_type"`
	EntityID   int64  `json:"entity_id"`
	Title      string `json:"title"`
	Relation   string `json:"relation"`
	Confidence string `json:"confidence"`
}

// Add to Result:
Knowledge []KnowledgeLink `json:"knowledge,omitempty"`
```

**Step 3: Enrich in CLI layer**

In `internal/cli/find.go`, after `Find` succeeds and before writing JSON, query
edges pointing at the symbol's package and the symbol itself. This keeps the
find service unaware of edges (clean separation).

```go
if jsonOut && result.Symbol.Package != "" {
	edgeSvc := edge.NewService(conn)
	pkgEdges, _ := edgeSvc.ListTo(cmd.Context(), "package", result.Symbol.Package)
	symRef := result.Symbol.Package + "." + result.Symbol.Name
	symEdges, _ := edgeSvc.ListTo(cmd.Context(), "symbol", symRef)
	// Merge and attach to result as knowledge links
}
```

**Step 4: Run tests**

Run: `just test`

Expected: PASS

**Step 5: Commit**

```bash
git add internal/find/service.go internal/cli/find.go
git commit -m "feat(find): include knowledge edges in JSON output"
```

---

## Summary of Tasks

| Task | Description                                     | Depends On |
| ---- | ----------------------------------------------- | ---------- |
| 1    | Edges table migration + pattern_files migration | —          |
| 2    | Edge service CRUD                               | 1          |
| 3    | Bidirectional edge support                      | 2          |
| 4    | Auto-linking service                            | 2          |
| 5    | Integrate edges into decide command             | 2, 4       |
| 6    | Integrate edges into pattern command            | 2, 4       |
| 7    | Edge-aware orient output                        | 2          |
| 8    | Edge-aware recall output                        | 2          |
| 9    | `recon edges` CLI command                       | 2          |
| 10   | Edge-enriched find output (JSON)                | 2          |

**Dependencies:** Tasks 1→2→3 are sequential. Tasks 4-10 all depend on Task 2
but are independent of each other and can be done in any order or in parallel.
