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
	entityDataJSON, err := json.Marshal(entityData)
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

	baselineJSON, _ := json.Marshal(outcome.Baseline)
	lastResultJSON, _ := json.Marshal(map[string]any{"passed": outcome.Passed, "details": outcome.Details})

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
		return ProposePatternResult{}, fmt.Errorf("commit pending pattern tx: %w", err)
	}

	return ProposePatternResult{ProposalID: proposalID, Promoted: false, VerificationPassed: false, VerificationDetails: outcome.Details}, nil
}
