package recall

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

type RecallOptions struct {
	Limit int
	Kind  string // "decision", "pattern", or "" for all
}

type ConnectedEdge struct {
	ToType   string `json:"to_type"`
	ToRef    string `json:"to_ref"`
	Relation string `json:"relation"`
}

type Item struct {
	DecisionID      int64           `json:"decision_id,omitempty"`
	PatternID       int64           `json:"pattern_id,omitempty"`
	EntityType      string          `json:"entity_type"`
	Title           string          `json:"title"`
	Reasoning       string          `json:"reasoning"`
	Confidence      string          `json:"confidence"`
	UpdatedAt       string          `json:"updated_at"`
	EvidenceSummary string          `json:"evidence_summary"`
	EvidenceDrift   string          `json:"evidence_drift_status"`
	ConnectedEdges  []ConnectedEdge `json:"connected_edges,omitempty"`
}

type Result struct {
	Query string `json:"query"`
	Items []Item `json:"items"`
}

type Service struct {
	db *sql.DB
}

func NewService(conn *sql.DB) *Service {
	return &Service{db: conn}
}

func (s *Service) Recall(ctx context.Context, query string, opts RecallOptions) (Result, error) {
	if opts.Limit <= 0 {
		opts.Limit = 10
	}

	items, err := s.recallFTS(ctx, query, opts.Limit)
	if err != nil {
		items, err = s.recallLike(ctx, query, opts.Limit)
		if err != nil {
			return Result{}, err
		}
	}

	if opts.Kind != "" {
		items = filterByKind(items, opts.Kind)
	}
	s.enrichWithEdges(ctx, items)
	return Result{Query: query, Items: items}, nil
}

func filterByKind(items []Item, kind string) []Item {
	filtered := make([]Item, 0, len(items))
	for _, item := range items {
		if item.EntityType == kind {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

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

		for rows.Next() {
			var ce ConnectedEdge
			if err := rows.Scan(&ce.ToType, &ce.ToRef, &ce.Relation); err != nil {
				continue
			}
			items[i].ConnectedEdges = append(items[i].ConnectedEdges, ce)
		}
		rows.Close()
	}
}

func (s *Service) recallFTS(ctx context.Context, query string, limit int) ([]Item, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT
    search_index.entity_type,
    search_index.entity_id,
    search_index.title,
    COALESCE(d.reasoning, p.description, ''),
    COALESCE(d.confidence, p.confidence, 'medium'),
    COALESCE(d.updated_at, p.updated_at, ''),
    COALESCE(e.summary, ''),
    COALESCE(e.drift_status, 'ok')
FROM search_index
LEFT JOIN decisions d ON d.id = search_index.entity_id AND search_index.entity_type = 'decision'
LEFT JOIN patterns p ON p.id = search_index.entity_id AND search_index.entity_type = 'pattern'
LEFT JOIN evidence e ON e.entity_type = search_index.entity_type AND e.entity_id = search_index.entity_id
WHERE search_index MATCH ?
  AND (
    (search_index.entity_type = 'decision' AND d.status = 'active')
    OR (search_index.entity_type = 'pattern' AND p.status = 'active')
  )
ORDER BY rank
LIMIT ?;
	`, query, limit)
	if err != nil {
		if isMissingTableError(err, "patterns") {
			return s.recallFTSLegacy(ctx, query, limit)
		}
		return nil, fmt.Errorf("fts recall query: %w", err)
	}
	defer rows.Close()

	return scanItems(rows)
}

func (s *Service) recallFTSLegacy(ctx context.Context, query string, limit int) ([]Item, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT
    search_index.entity_type,
    search_index.entity_id,
    search_index.title,
    COALESCE(d.reasoning, ''),
    COALESCE(d.confidence, 'medium'),
    COALESCE(d.updated_at, ''),
    COALESCE(e.summary, ''),
    COALESCE(e.drift_status, 'ok')
FROM search_index
LEFT JOIN decisions d ON d.id = search_index.entity_id AND search_index.entity_type = 'decision'
LEFT JOIN evidence e ON e.entity_type = search_index.entity_type AND e.entity_id = search_index.entity_id
WHERE search_index MATCH ?
  AND search_index.entity_type = 'decision'
  AND d.status = 'active'
ORDER BY rank
LIMIT ?;
	`, query, limit)
	if err != nil {
		return nil, fmt.Errorf("fts recall query: %w", err)
	}
	defer rows.Close()

	return scanItems(rows)
}

func (s *Service) recallLike(ctx context.Context, query string, limit int) ([]Item, error) {
	like := "%" + query + "%"
	rows, err := s.db.QueryContext(ctx, `
SELECT 'decision' AS entity_type, d.id, d.title, d.reasoning, d.confidence, d.updated_at,
       COALESCE(e.summary, ''), COALESCE(e.drift_status, 'ok')
FROM decisions d
LEFT JOIN evidence e ON e.entity_type = 'decision' AND e.entity_id = d.id
WHERE d.status = 'active' AND (d.title LIKE ? OR d.reasoning LIKE ? OR e.summary LIKE ?)
UNION ALL
SELECT 'pattern' AS entity_type, p.id, p.title, p.description, p.confidence, p.updated_at,
       COALESCE(e2.summary, ''), COALESCE(e2.drift_status, 'ok')
FROM patterns p
LEFT JOIN evidence e2 ON e2.entity_type = 'pattern' AND e2.entity_id = p.id
WHERE p.status = 'active' AND (p.title LIKE ? OR p.description LIKE ? OR e2.summary LIKE ?)
ORDER BY updated_at DESC
LIMIT ?;
	`, like, like, like, like, like, like, limit)
	if err != nil {
		if isMissingTableError(err, "patterns") {
			return s.recallLikeLegacy(ctx, like, limit)
		}
		return nil, fmt.Errorf("fallback recall query: %w", err)
	}
	defer rows.Close()

	return scanItems(rows)
}

func (s *Service) recallLikeLegacy(ctx context.Context, like string, limit int) ([]Item, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT 'decision' AS entity_type, d.id, d.title, d.reasoning, d.confidence, d.updated_at,
       COALESCE(e.summary, ''), COALESCE(e.drift_status, 'ok')
FROM decisions d
LEFT JOIN evidence e ON e.entity_type = 'decision' AND e.entity_id = d.id
WHERE d.status = 'active' AND (d.title LIKE ? OR d.reasoning LIKE ? OR e.summary LIKE ?)
ORDER BY updated_at DESC
LIMIT ?;
	`, like, like, like, limit)
	if err != nil {
		return nil, fmt.Errorf("fallback recall query: %w", err)
	}
	defer rows.Close()

	return scanItems(rows)
}

func isMissingTableError(err error, table string) bool {
	if err == nil {
		return false
	}

	return strings.Contains(strings.ToLower(err.Error()), "no such table: "+strings.ToLower(table))
}

func scanItems(rows *sql.Rows) ([]Item, error) {
	items := make([]Item, 0, 8)
	for rows.Next() {
		var item Item
		var entityID int64
		if err := rows.Scan(
			&item.EntityType,
			&entityID,
			&item.Title,
			&item.Reasoning,
			&item.Confidence,
			&item.UpdatedAt,
			&item.EvidenceSummary,
			&item.EvidenceDrift,
		); err != nil {
			return nil, fmt.Errorf("scan recall row: %w", err)
		}
		switch item.EntityType {
		case "pattern":
			item.PatternID = entityID
		default:
			item.DecisionID = entityID
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate recall rows: %w", err)
	}
	return items, nil
}
