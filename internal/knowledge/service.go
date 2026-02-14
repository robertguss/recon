package knowledge

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/robertguss/recon/internal/index"
)

var marshalJSON = json.Marshal

type ProposeDecisionInput struct {
	Title           string
	Reasoning       string
	Confidence      string
	EvidenceSummary string
	CheckType       string
	CheckSpec       string
	ModuleRoot      string
}

type ProposeDecisionResult struct {
	ProposalID          int64  `json:"proposal_id"`
	DecisionID          int64  `json:"decision_id,omitempty"`
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

func (s *Service) ProposeAndVerifyDecision(ctx context.Context, in ProposeDecisionInput) (ProposeDecisionResult, error) {
	if strings.TrimSpace(in.Title) == "" {
		return ProposeDecisionResult{}, fmt.Errorf("title is required")
	}
	if strings.TrimSpace(in.Reasoning) == "" {
		return ProposeDecisionResult{}, fmt.Errorf("reasoning is required")
	}
	if strings.TrimSpace(in.EvidenceSummary) == "" {
		return ProposeDecisionResult{}, fmt.Errorf("evidence summary is required")
	}
	if strings.TrimSpace(in.CheckType) == "" {
		return ProposeDecisionResult{}, fmt.Errorf("check type is required")
	}
	if strings.TrimSpace(in.CheckSpec) == "" {
		return ProposeDecisionResult{}, fmt.Errorf("check spec is required")
	}

	confidence := strings.TrimSpace(in.Confidence)
	if confidence == "" {
		confidence = "medium"
	}

	now := time.Now().UTC().Format(time.RFC3339)
	entityData := map[string]any{
		"title":            in.Title,
		"reasoning":        in.Reasoning,
		"confidence":       confidence,
		"evidence_summary": in.EvidenceSummary,
		"check_type":       in.CheckType,
		"check_spec":       in.CheckSpec,
	}
	entityDataJSON, err := marshalJSON(entityData)
	if err != nil {
		return ProposeDecisionResult{}, fmt.Errorf("marshal proposal data: %w", err)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ProposeDecisionResult{}, fmt.Errorf("begin decision tx: %w", err)
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx, `
INSERT INTO proposals (session_id, entity_type, entity_data, status, proposed_at)
VALUES (NULL, 'decision', ?, 'pending', ?);
`, string(entityDataJSON), now)
	if err != nil {
		return ProposeDecisionResult{}, fmt.Errorf("insert proposal: %w", err)
	}
	proposalID, err := res.LastInsertId()
	if err != nil {
		return ProposeDecisionResult{}, fmt.Errorf("read proposal id: %w", err)
	}

	outcome := runCheckOutcome{Passed: false, Details: "unknown"}
	outcome, err = s.runCheck(ctx, in)
	if err != nil {
		outcome = runCheckOutcome{Passed: false, Details: err.Error(), Baseline: map[string]any{"error": err.Error()}}
	}

	baselineJSON, err := marshalJSON(outcome.Baseline)
	if err != nil {
		return ProposeDecisionResult{}, fmt.Errorf("marshal baseline: %w", err)
	}
	lastResultJSON, err := marshalJSON(map[string]any{
		"passed":  outcome.Passed,
		"details": outcome.Details,
	})
	if err != nil {
		return ProposeDecisionResult{}, fmt.Errorf("marshal check result: %w", err)
	}

	verifiedAt := time.Now().UTC().Format(time.RFC3339)
	if outcome.Passed {
		decisionRes, err := tx.ExecContext(ctx, `
INSERT INTO decisions (title, reasoning, confidence, status, created_at, updated_at)
VALUES (?, ?, ?, 'active', ?, ?);
`, in.Title, in.Reasoning, confidence, verifiedAt, verifiedAt)
		if err != nil {
			return ProposeDecisionResult{}, fmt.Errorf("insert decision: %w", err)
		}
		decisionID, err := decisionRes.LastInsertId()
		if err != nil {
			return ProposeDecisionResult{}, fmt.Errorf("read decision id: %w", err)
		}

		if _, err := tx.ExecContext(ctx, `
INSERT INTO evidence (
    entity_type,
    entity_id,
    summary,
    check_type,
    check_spec,
    baseline,
    last_verified_at,
    last_result,
    drift_status
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'ok');
`, "decision", decisionID, in.EvidenceSummary, in.CheckType, in.CheckSpec, string(baselineJSON), verifiedAt, string(lastResultJSON)); err != nil {
			return ProposeDecisionResult{}, fmt.Errorf("insert decision evidence: %w", err)
		}

		if _, err := tx.ExecContext(ctx, `
UPDATE proposals
SET status = 'promoted', verified_at = ?, promoted_at = ?
WHERE id = ?;
`, verifiedAt, verifiedAt, proposalID); err != nil {
			return ProposeDecisionResult{}, fmt.Errorf("update proposal status to promoted: %w", err)
		}

		if _, err := tx.ExecContext(ctx, `
INSERT INTO search_index (title, content, entity_type, entity_id)
VALUES (?, ?, 'decision', ?);
`, in.Title, in.Reasoning+"\n"+in.EvidenceSummary, decisionID); err != nil {
			return ProposeDecisionResult{}, fmt.Errorf("insert search index: %w", err)
		}

		if err := tx.Commit(); err != nil {
			return ProposeDecisionResult{}, fmt.Errorf("commit decision tx: %w", err)
		}

		return ProposeDecisionResult{
			ProposalID:          proposalID,
			DecisionID:          decisionID,
			Promoted:            true,
			VerificationPassed:  true,
			VerificationDetails: outcome.Details,
		}, nil
	}

	if _, err := tx.ExecContext(ctx, `
INSERT INTO evidence (
    entity_type,
    entity_id,
    summary,
    check_type,
    check_spec,
    baseline,
    last_verified_at,
    last_result,
    drift_status
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'broken');
`, "proposal", proposalID, "verification failed: "+outcome.Details, in.CheckType, in.CheckSpec, string(baselineJSON), verifiedAt, string(lastResultJSON)); err != nil {
		return ProposeDecisionResult{}, fmt.Errorf("insert proposal evidence: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
UPDATE proposals
SET status = 'pending', verified_at = ?
WHERE id = ?;
`, verifiedAt, proposalID); err != nil {
		return ProposeDecisionResult{}, fmt.Errorf("update proposal status to pending: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return ProposeDecisionResult{}, fmt.Errorf("commit pending proposal tx: %w", err)
	}

	return ProposeDecisionResult{
		ProposalID:          proposalID,
		Promoted:            false,
		VerificationPassed:  false,
		VerificationDetails: outcome.Details,
	}, nil
}

type runCheckOutcome struct {
	Passed   bool
	Details  string
	Baseline map[string]any
}

func (s *Service) runCheck(ctx context.Context, in ProposeDecisionInput) (runCheckOutcome, error) {
	switch in.CheckType {
	case "file_exists":
		return s.runFileExists(in.CheckSpec, in.ModuleRoot)
	case "symbol_exists":
		return s.runSymbolExists(ctx, in.CheckSpec)
	case "grep_pattern":
		return s.runGrepPattern(in.CheckSpec, in.ModuleRoot)
	default:
		return runCheckOutcome{}, fmt.Errorf("unsupported check type %q", in.CheckType)
	}
}

func (s *Service) runFileExists(specRaw string, moduleRoot string) (runCheckOutcome, error) {
	var spec struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(specRaw), &spec); err != nil {
		return runCheckOutcome{}, fmt.Errorf("parse file_exists check spec: %w", err)
	}
	if strings.TrimSpace(spec.Path) == "" {
		return runCheckOutcome{}, fmt.Errorf("file_exists requires spec.path")
	}

	target := spec.Path
	if !filepath.IsAbs(target) {
		target = filepath.Join(moduleRoot, target)
	}
	_, err := os.Stat(target)
	exists := err == nil

	return runCheckOutcome{
		Passed:  exists,
		Details: fmt.Sprintf("file %s exists=%v", spec.Path, exists),
		Baseline: map[string]any{
			"path":   spec.Path,
			"exists": exists,
		},
	}, nil
}

func (s *Service) runSymbolExists(ctx context.Context, specRaw string) (runCheckOutcome, error) {
	var spec struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal([]byte(specRaw), &spec); err != nil {
		return runCheckOutcome{}, fmt.Errorf("parse symbol_exists check spec: %w", err)
	}
	if strings.TrimSpace(spec.Name) == "" {
		return runCheckOutcome{}, fmt.Errorf("symbol_exists requires spec.name")
	}

	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM symbols WHERE name = ?;`, spec.Name).Scan(&count); err != nil {
		return runCheckOutcome{}, fmt.Errorf("query symbol count: %w", err)
	}
	passed := count > 0

	return runCheckOutcome{
		Passed:  passed,
		Details: fmt.Sprintf("symbol %s count=%d", spec.Name, count),
		Baseline: map[string]any{
			"name":  spec.Name,
			"count": count,
		},
	}, nil
}

func (s *Service) runGrepPattern(specRaw string, moduleRoot string) (runCheckOutcome, error) {
	var spec struct {
		Pattern string `json:"pattern"`
		Scope   string `json:"scope"`
	}
	if err := json.Unmarshal([]byte(specRaw), &spec); err != nil {
		return runCheckOutcome{}, fmt.Errorf("parse grep_pattern check spec: %w", err)
	}
	if strings.TrimSpace(spec.Pattern) == "" {
		return runCheckOutcome{}, fmt.Errorf("grep_pattern requires spec.pattern")
	}

	re, err := regexp.Compile(spec.Pattern)
	if err != nil {
		return runCheckOutcome{}, fmt.Errorf("compile regex pattern: %w", err)
	}

	files, err := index.CollectEligibleGoFiles(moduleRoot)
	if err != nil {
		return runCheckOutcome{}, fmt.Errorf("load files for grep_pattern: %w", err)
	}

	total := 0
	matched := 0
	for _, f := range files {
		if spec.Scope != "" {
			baseMatch, _ := filepath.Match(spec.Scope, filepath.Base(f.RelPath))
			relMatch, _ := filepath.Match(spec.Scope, f.RelPath)
			if !baseMatch && !relMatch {
				continue
			}
		}
		total++
		if re.Match(f.Content) {
			matched++
		}
	}

	passed := matched > 0
	return runCheckOutcome{
		Passed:  passed,
		Details: fmt.Sprintf("grep pattern matched %d of %d files", matched, total),
		Baseline: map[string]any{
			"pattern": spec.Pattern,
			"scope":   spec.Scope,
			"matched": matched,
			"total":   total,
		},
	}, nil
}
