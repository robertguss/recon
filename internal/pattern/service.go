package pattern

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/robertguss/recon/internal/knowledge"
)

var jsonMarshal = json.Marshal

type ProposePatternInput struct {
	Title           string
	Description     string
	Example         string
	Confidence      string
	EvidenceSummary string
	CheckType       string
	CheckSpec       string
	ModuleRoot      string
}

type ProposePatternResult struct {
	ProposalID          int64  `json:"proposal_id"`
	PatternID           int64  `json:"pattern_id,omitempty"`
	Promoted            bool   `json:"promoted"`
	VerificationPassed  bool   `json:"verification_passed"`
	VerificationDetails string `json:"verification_details"`
}

type Service struct {
	db *sql.DB
}

func NewService(conn *sql.DB) *Service {
	return &Service{db: conn}
}

func (s *Service) ProposeAndVerifyPattern(ctx context.Context, in ProposePatternInput) (ProposePatternResult, error) {
	if strings.TrimSpace(in.Title) == "" {
		return ProposePatternResult{}, fmt.Errorf("title is required")
	}
	if strings.TrimSpace(in.EvidenceSummary) == "" {
		return ProposePatternResult{}, fmt.Errorf("evidence summary is required")
	}
	if strings.TrimSpace(in.CheckType) == "" {
		return ProposePatternResult{}, fmt.Errorf("check type is required")
	}
	if strings.TrimSpace(in.CheckSpec) == "" {
		return ProposePatternResult{}, fmt.Errorf("check spec is required")
	}

	confidence := strings.TrimSpace(in.Confidence)
	if confidence == "" {
		confidence = "medium"
	}

	knowledgeSvc := knowledge.NewService(s.db)
	outcome := knowledgeSvc.RunCheckPublic(ctx, in.CheckType, in.CheckSpec, in.ModuleRoot)

	now := time.Now().UTC().Format(time.RFC3339)

	entityData := map[string]any{
		"title":            in.Title,
		"description":      in.Description,
		"example":          in.Example,
		"confidence":       confidence,
		"evidence_summary": in.EvidenceSummary,
		"check_type":       in.CheckType,
		"check_spec":       in.CheckSpec,
	}
	entityDataJSON, err := jsonMarshal(entityData)
	if err != nil {
		return ProposePatternResult{}, fmt.Errorf("marshal proposal data: %w", err)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ProposePatternResult{}, fmt.Errorf("begin pattern tx: %w", err)
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx, `
INSERT INTO proposals (session_id, entity_type, entity_data, status, proposed_at)
VALUES (NULL, 'pattern', ?, 'pending', ?);
`, string(entityDataJSON), now)
	if err != nil {
		return ProposePatternResult{}, fmt.Errorf("insert proposal: %w", err)
	}
	proposalID, _ := res.LastInsertId()

	baselineJSON, _ := jsonMarshal(outcome.Baseline)
	lastResultJSON, _ := jsonMarshal(map[string]any{"passed": outcome.Passed, "details": outcome.Details})

	if outcome.Passed {
		patternRes, err := tx.ExecContext(ctx, `
INSERT INTO patterns (title, description, confidence, status, created_at, updated_at)
VALUES (?, ?, ?, 'active', ?, ?);
`, in.Title, in.Description, confidence, now, now)
		if err != nil {
			return ProposePatternResult{}, fmt.Errorf("insert pattern: %w", err)
		}
		patternID, _ := patternRes.LastInsertId()

		if _, err := tx.ExecContext(ctx, `
INSERT INTO evidence (entity_type, entity_id, summary, check_type, check_spec, baseline, last_verified_at, last_result, drift_status)
VALUES ('pattern', ?, ?, ?, ?, ?, ?, ?, 'ok');
`, patternID, in.EvidenceSummary, in.CheckType, in.CheckSpec, string(baselineJSON), now, string(lastResultJSON)); err != nil {
			return ProposePatternResult{}, fmt.Errorf("insert pattern evidence: %w", err)
		}

		if _, err := tx.ExecContext(ctx, `
UPDATE proposals SET status = 'promoted', verified_at = ?, promoted_at = ? WHERE id = ?;
`, now, now, proposalID); err != nil {
			return ProposePatternResult{}, fmt.Errorf("update proposal: %w", err)
		}

		if _, err := tx.ExecContext(ctx, `
INSERT INTO search_index (title, content, entity_type, entity_id)
VALUES (?, ?, 'pattern', ?);
`, in.Title, in.Description+"\n"+in.Example+"\n"+in.EvidenceSummary, patternID); err != nil {
			return ProposePatternResult{}, fmt.Errorf("insert search index: %w", err)
		}

		if err := tx.Commit(); err != nil {
			_ = tx.Rollback()
			return ProposePatternResult{}, fmt.Errorf("commit pattern tx: %w", err)
		}
		return ProposePatternResult{ProposalID: proposalID, PatternID: patternID, Promoted: true, VerificationPassed: true, VerificationDetails: outcome.Details}, nil
	}

	// Not promoted
	if _, err := tx.ExecContext(ctx, `
INSERT INTO evidence (entity_type, entity_id, summary, check_type, check_spec, baseline, last_verified_at, last_result, drift_status)
VALUES ('proposal', ?, ?, ?, ?, ?, ?, ?, 'broken');
`, proposalID, "verification failed: "+outcome.Details, in.CheckType, in.CheckSpec, string(baselineJSON), now, string(lastResultJSON)); err != nil {
		return ProposePatternResult{}, fmt.Errorf("insert proposal evidence: %w", err)
	}

	if err := tx.Commit(); err != nil {
		_ = tx.Rollback()
		return ProposePatternResult{}, fmt.Errorf("commit pending pattern tx: %w", err)
	}

	return ProposePatternResult{ProposalID: proposalID, Promoted: false, VerificationPassed: false, VerificationDetails: outcome.Details}, nil
}

var ErrNotFound = fmt.Errorf("not found")

type UpdatePatternInput struct {
	Title       string
	Description string
}

func (s *Service) UpdatePattern(ctx context.Context, id int64, in UpdatePatternInput) error {
	if strings.TrimSpace(in.Title) == "" && strings.TrimSpace(in.Description) == "" {
		return fmt.Errorf("at least one field (title, description) is required")
	}

	now := time.Now().UTC().Format(time.RFC3339)

	setClauses := []string{"updated_at = ?"}
	args := []any{now}

	if strings.TrimSpace(in.Title) != "" {
		setClauses = append(setClauses, "title = ?")
		args = append(args, strings.TrimSpace(in.Title))
	}
	if strings.TrimSpace(in.Description) != "" {
		setClauses = append(setClauses, "description = ?")
		args = append(args, strings.TrimSpace(in.Description))
	}
	args = append(args, id)

	query := "UPDATE patterns SET " + strings.Join(setClauses, ", ") +
		" WHERE id = ? AND status = 'active';"
	res, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("update pattern: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("pattern %d: %w", id, ErrNotFound)
	}

	var title, description, example, evidenceSummary string
	if err := s.db.QueryRowContext(ctx,
		`SELECT p.title, p.description,
		        COALESCE(json_extract(pr.entity_data, '$.example'), ''),
		        COALESCE(e.summary, '')
		 FROM patterns p
		 LEFT JOIN evidence e ON e.entity_type = 'pattern' AND e.entity_id = p.id
		 LEFT JOIN proposals pr ON pr.entity_type = 'pattern'
		     AND pr.status = 'promoted'
		     AND e.summary IS NOT NULL
		     AND json_extract(pr.entity_data, '$.evidence_summary') = e.summary
		 WHERE p.id = ?`, id,
	).Scan(&title, &description, &example, &evidenceSummary); err != nil {
		return fmt.Errorf("read updated pattern for reindex: %w", err)
	}

	if _, err := s.db.ExecContext(ctx,
		`UPDATE search_index SET title = ?, content = ? WHERE entity_type = 'pattern' AND entity_id = ?`,
		title, description+"\n"+example+"\n"+evidenceSummary, id,
	); err != nil {
		return fmt.Errorf("reindex pattern: %w", err)
	}

	return nil
}

func (s *Service) ArchivePattern(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE patterns SET status = 'archived', updated_at = ? WHERE id = ? AND status = 'active';`,
		time.Now().UTC().Format(time.RFC3339), id)
	if err != nil {
		return fmt.Errorf("archive pattern: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("pattern %d: %w", id, ErrNotFound)
	}
	return nil
}

type PatternListItem struct {
	ID         int64  `json:"id"`
	Title      string `json:"title"`
	Confidence string `json:"confidence"`
	Status     string `json:"status"`
	Drift      string `json:"drift_status"`
	UpdatedAt  string `json:"updated_at"`
}

func (s *Service) ListPatterns(ctx context.Context) ([]PatternListItem, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT p.id, p.title, p.confidence, p.status, COALESCE(e.drift_status, 'ok'), p.updated_at
FROM patterns p
LEFT JOIN evidence e ON e.entity_type = 'pattern' AND e.entity_id = p.id
WHERE p.status = 'active'
ORDER BY p.updated_at DESC;
`)
	if err != nil {
		return nil, fmt.Errorf("query patterns: %w", err)
	}
	defer rows.Close()
	items := []PatternListItem{}
	for rows.Next() {
		var item PatternListItem
		if err := rows.Scan(&item.ID, &item.Title, &item.Confidence, &item.Status, &item.Drift, &item.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan pattern: %w", err)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
