package recall

import (
	"context"
	"database/sql"
	"fmt"
)

type RecallOptions struct {
	Limit int
}

type Item struct {
	DecisionID       int64  `json:"decision_id"`
	Title            string `json:"title"`
	Reasoning        string `json:"reasoning"`
	Confidence       string `json:"confidence"`
	UpdatedAt        string `json:"updated_at"`
	EvidenceSummary  string `json:"evidence_summary"`
	EvidenceDrift    string `json:"evidence_drift_status"`
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

	return Result{Query: query, Items: items}, nil
}

func (s *Service) recallFTS(ctx context.Context, query string, limit int) ([]Item, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT d.id, d.title, d.reasoning, d.confidence, d.updated_at,
       COALESCE(e.summary, ''), COALESCE(e.drift_status, 'ok')
FROM search_index
JOIN decisions d ON d.id = search_index.entity_id AND search_index.entity_type = 'decision'
LEFT JOIN evidence e ON e.entity_type = 'decision' AND e.entity_id = d.id
WHERE search_index MATCH ?
ORDER BY d.updated_at DESC
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
SELECT d.id, d.title, d.reasoning, d.confidence, d.updated_at,
       COALESCE(e.summary, ''), COALESCE(e.drift_status, 'ok')
FROM decisions d
LEFT JOIN evidence e ON e.entity_type = 'decision' AND e.entity_id = d.id
WHERE d.status = 'active' AND (d.title LIKE ? OR d.reasoning LIKE ? OR e.summary LIKE ?)
ORDER BY d.updated_at DESC
LIMIT ?;
`, like, like, like, limit)
	if err != nil {
		return nil, fmt.Errorf("fallback recall query: %w", err)
	}
	defer rows.Close()

	return scanItems(rows)
}

func scanItems(rows *sql.Rows) ([]Item, error) {
	items := make([]Item, 0, 8)
	for rows.Next() {
		var item Item
		if err := rows.Scan(
			&item.DecisionID,
			&item.Title,
			&item.Reasoning,
			&item.Confidence,
			&item.UpdatedAt,
			&item.EvidenceSummary,
			&item.EvidenceDrift,
		); err != nil {
			return nil, fmt.Errorf("scan recall row: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate recall rows: %w", err)
	}
	return items, nil
}
