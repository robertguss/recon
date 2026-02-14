package orient

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"time"

	"github.com/robertguss/recon/internal/db"
	"github.com/robertguss/recon/internal/index"
)

type BuildOptions struct {
	ModuleRoot   string
	MaxModules   int
	MaxDecisions int
}

type Payload struct {
	Project         ProjectInfo      `json:"project"`
	Freshness       Freshness        `json:"freshness"`
	Summary         Summary          `json:"summary"`
	Modules         []ModuleSummary  `json:"modules"`
	ActiveDecisions []DecisionDigest `json:"active_decisions"`
	Warnings        []string         `json:"warnings,omitempty"`
}

type ProjectInfo struct {
	Name       string `json:"name"`
	ModulePath string `json:"module_path"`
	Language   string `json:"language"`
}

type Freshness struct {
	IsStale       bool   `json:"is_stale"`
	Reason        string `json:"reason"`
	LastSyncAt    string `json:"last_sync_at,omitempty"`
	LastSyncCommit string `json:"last_sync_commit,omitempty"`
	CurrentCommit string `json:"current_commit,omitempty"`
}

type Summary struct {
	FileCount     int `json:"file_count"`
	SymbolCount   int `json:"symbol_count"`
	PackageCount  int `json:"package_count"`
	DecisionCount int `json:"decision_count"`
}

type ModuleSummary struct {
	Path      string `json:"path"`
	Name      string `json:"name"`
	FileCount int    `json:"file_count"`
	LineCount int    `json:"line_count"`
}

type DecisionDigest struct {
	ID         int64  `json:"id"`
	Title      string `json:"title"`
	Confidence string `json:"confidence"`
	UpdatedAt  string `json:"updated_at"`
	Drift      string `json:"drift_status"`
}

type Service struct {
	db *sql.DB
}

func NewService(conn *sql.DB) *Service {
	return &Service{db: conn}
}

func (s *Service) Build(ctx context.Context, opts BuildOptions) (Payload, error) {
	modulePath, err := index.ModulePath(opts.ModuleRoot)
	if err != nil {
		return Payload{}, err
	}

	payload := Payload{
		Project: ProjectInfo{
			Name:       filepath.Base(opts.ModuleRoot),
			ModulePath: modulePath,
			Language:   "go",
		},
	}

	if opts.MaxModules <= 0 {
		opts.MaxModules = 8
	}
	if opts.MaxDecisions <= 0 {
		opts.MaxDecisions = 5
	}

	if err := s.loadSummary(ctx, &payload); err != nil {
		return Payload{}, err
	}
	if err := s.loadModules(ctx, opts.MaxModules, &payload); err != nil {
		return Payload{}, err
	}
	if err := s.loadDecisions(ctx, opts.MaxDecisions, &payload); err != nil {
		return Payload{}, err
	}

	state, exists, err := db.LoadSyncState(ctx, s.db)
	if err != nil {
		return Payload{}, err
	}
	currentCommit, currentDirty := index.CurrentGitState(ctx, opts.ModuleRoot)

	switch {
	case !exists:
		payload.Freshness = Freshness{IsStale: true, Reason: "never_synced", CurrentCommit: currentCommit}
	case state.LastSyncCommit != "" && currentCommit != "" && state.LastSyncCommit != currentCommit:
		payload.Freshness = Freshness{
			IsStale:        true,
			Reason:         "git_head_changed_since_last_sync",
			LastSyncAt:     state.LastSyncAt.Format(time.RFC3339),
			LastSyncCommit: state.LastSyncCommit,
			CurrentCommit:  currentCommit,
		}
	case state.LastSyncDirty != currentDirty:
		payload.Freshness = Freshness{
			IsStale:        true,
			Reason:         "git_dirty_state_changed_since_last_sync",
			LastSyncAt:     state.LastSyncAt.Format(time.RFC3339),
			LastSyncCommit: state.LastSyncCommit,
			CurrentCommit:  currentCommit,
		}
	default:
		fingerprint, _, err := index.CurrentFingerprint(opts.ModuleRoot)
		if err != nil {
			payload.Warnings = append(payload.Warnings, fmt.Sprintf("fingerprint check failed: %v", err))
			payload.Freshness = Freshness{
				IsStale:        false,
				Reason:         "",
				LastSyncAt:     state.LastSyncAt.Format(time.RFC3339),
				LastSyncCommit: state.LastSyncCommit,
				CurrentCommit:  currentCommit,
			}
		} else if fingerprint != state.IndexFingerprint {
			payload.Freshness = Freshness{
				IsStale:        true,
				Reason:         "worktree_fingerprint_changed_since_last_sync",
				LastSyncAt:     state.LastSyncAt.Format(time.RFC3339),
				LastSyncCommit: state.LastSyncCommit,
				CurrentCommit:  currentCommit,
			}
		} else {
			payload.Freshness = Freshness{
				IsStale:        false,
				Reason:         "",
				LastSyncAt:     state.LastSyncAt.Format(time.RFC3339),
				LastSyncCommit: state.LastSyncCommit,
				CurrentCommit:  currentCommit,
			}
		}
	}

	return payload, nil
}

func (s *Service) loadSummary(ctx context.Context, payload *Payload) error {
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM files;").Scan(&payload.Summary.FileCount); err != nil {
		return fmt.Errorf("count files: %w", err)
	}
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM symbols;").Scan(&payload.Summary.SymbolCount); err != nil {
		return fmt.Errorf("count symbols: %w", err)
	}
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM packages;").Scan(&payload.Summary.PackageCount); err != nil {
		return fmt.Errorf("count packages: %w", err)
	}
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM decisions WHERE status = 'active';").Scan(&payload.Summary.DecisionCount); err != nil {
		return fmt.Errorf("count decisions: %w", err)
	}
	return nil
}

func (s *Service) loadModules(ctx context.Context, limit int, payload *Payload) error {
	rows, err := s.db.QueryContext(ctx, `
SELECT path, name, file_count, line_count
FROM packages
ORDER BY line_count DESC, path ASC
LIMIT ?;
`, limit)
	if err != nil {
		return fmt.Errorf("query modules: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var m ModuleSummary
		if err := rows.Scan(&m.Path, &m.Name, &m.FileCount, &m.LineCount); err != nil {
			return fmt.Errorf("scan module row: %w", err)
		}
		payload.Modules = append(payload.Modules, m)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate module rows: %w", err)
	}
	return nil
}

func (s *Service) loadDecisions(ctx context.Context, limit int, payload *Payload) error {
	rows, err := s.db.QueryContext(ctx, `
SELECT d.id, d.title, d.confidence, d.updated_at, COALESCE(e.drift_status, 'ok')
FROM decisions d
LEFT JOIN evidence e ON e.entity_type = 'decision' AND e.entity_id = d.id
WHERE d.status = 'active'
ORDER BY d.updated_at DESC
LIMIT ?;
`, limit)
	if err != nil {
		return fmt.Errorf("query decisions: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var d DecisionDigest
		if err := rows.Scan(&d.ID, &d.Title, &d.Confidence, &d.UpdatedAt, &d.Drift); err != nil {
			return fmt.Errorf("scan decision row: %w", err)
		}
		payload.ActiveDecisions = append(payload.ActiveDecisions, d)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate decision rows: %w", err)
	}
	return nil
}
