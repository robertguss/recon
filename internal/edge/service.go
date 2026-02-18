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

	// Insert reverse for bidirectional relations
	if BidirectionalRelations[in.Relation] && validFromTypes[in.ToType] {
		_, err := s.db.ExecContext(ctx, `
INSERT OR IGNORE INTO edges (from_type, from_id, to_type, to_ref, relation, source, confidence, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?);
`, in.ToType, toIDFromRef(in.ToRef), in.FromType, fmt.Sprintf("%d", in.FromID), in.Relation, in.Source, in.Confidence, now)
		if err != nil {
			return Edge{}, fmt.Errorf("insert reverse edge: %w", err)
		}
	}

	return Edge{
		ID: id, FromType: in.FromType, FromID: in.FromID,
		ToType: in.ToType, ToRef: in.ToRef, Relation: in.Relation,
		Source: in.Source, Confidence: in.Confidence, CreatedAt: now,
	}, nil
}

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

	edges := make([]Edge, 0)
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

func toIDFromRef(ref string) int64 {
	var id int64
	fmt.Sscanf(ref, "%d", &id)
	return id
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
