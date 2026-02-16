package orient

import (
	"context"
	"database/sql"
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
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
	Architecture    Architecture     `json:"architecture"`
	Freshness       Freshness        `json:"freshness"`
	Summary         Summary          `json:"summary"`
	Modules         []ModuleSummary  `json:"modules"`
	ActiveDecisions []DecisionDigest `json:"active_decisions"`
	ActivePatterns  []PatternDigest  `json:"active_patterns"`
	RecentActivity  []RecentFile     `json:"recent_activity"`
	Warnings        []string         `json:"warnings,omitempty"`
}

type RecentFile struct {
	File         string `json:"file"`
	LastModified string `json:"last_modified"`
}

type DependencyEdge struct {
	From string   `json:"from"`
	To   []string `json:"to"`
}

type Architecture struct {
	EntryPoints    []string         `json:"entry_points"`
	DependencyFlow []DependencyEdge `json:"dependency_flow"`
}

type ProjectInfo struct {
	Name       string `json:"name"`
	ModulePath string `json:"module_path"`
	Language   string `json:"language"`
}

type Freshness struct {
	IsStale        bool   `json:"is_stale"`
	Reason         string `json:"reason"`
	LastSyncAt     string `json:"last_sync_at,omitempty"`
	LastSyncCommit string `json:"last_sync_commit,omitempty"`
	CurrentCommit  string `json:"current_commit,omitempty"`
	StaleSummary   string `json:"stale_summary,omitempty"`
}

type Summary struct {
	FileCount     int `json:"file_count"`
	SymbolCount   int `json:"symbol_count"`
	PackageCount  int `json:"package_count"`
	DecisionCount int `json:"decision_count"`
}

type ModuleKnowledge struct {
	ID             int64  `json:"id"`
	Type           string `json:"type"`
	Title          string `json:"title"`
	Confidence     string `json:"confidence"`
	EdgeConfidence string `json:"edge_confidence"`
}

type ModuleSummary struct {
	Path          string            `json:"path"`
	Name          string            `json:"name"`
	FileCount     int               `json:"file_count"`
	LineCount     int               `json:"line_count"`
	Heat          string            `json:"heat"`
	RecentCommits int               `json:"recent_commits"`
	Knowledge     []ModuleKnowledge `json:"knowledge,omitempty"`
}

type DecisionDigest struct {
	ID         int64  `json:"id"`
	Title      string `json:"title"`
	Confidence string `json:"confidence"`
	UpdatedAt  string `json:"updated_at"`
	Drift      string `json:"drift_status"`
}

type PatternDigest struct {
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
		Modules:         []ModuleSummary{},
		ActiveDecisions: []DecisionDigest{},
		ActivePatterns:  []PatternDigest{},
		RecentActivity:  []RecentFile{},
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
	if err := s.loadPatterns(ctx, 5, &payload); err != nil {
		return Payload{}, err
	}
	if err := s.loadArchitecture(ctx, &payload); err != nil {
		return Payload{}, err
	}
	s.loadModuleEdges(ctx, &payload)
	s.loadModuleHeat(ctx, opts.ModuleRoot, &payload)
	s.loadRecentActivity(ctx, opts.ModuleRoot, &payload)

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
			StaleSummary:   computeStaleSummary(ctx, opts.ModuleRoot, state.LastSyncCommit, currentCommit),
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

func (s *Service) loadPatterns(ctx context.Context, limit int, payload *Payload) error {
	rows, err := s.db.QueryContext(ctx, `
SELECT p.id, p.title, p.confidence, p.updated_at, COALESCE(e.drift_status, 'ok')
FROM patterns p
LEFT JOIN evidence e ON e.entity_type = 'pattern' AND e.entity_id = p.id
WHERE p.status = 'active'
ORDER BY p.updated_at DESC
LIMIT ?;
`, limit)
	if err != nil {
		return fmt.Errorf("query patterns: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var p PatternDigest
		if err := rows.Scan(&p.ID, &p.Title, &p.Confidence, &p.UpdatedAt, &p.Drift); err != nil {
			return fmt.Errorf("scan pattern row: %w", err)
		}
		payload.ActivePatterns = append(payload.ActivePatterns, p)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate pattern rows: %w", err)
	}
	return nil
}

func (s *Service) loadModuleEdges(ctx context.Context, payload *Payload) {
	rows, err := s.db.QueryContext(ctx, `
SELECT e.to_ref, e.from_type, e.from_id,
       COALESCE(d.title, p.title, '') AS title,
       COALESCE(d.confidence, p.confidence, 'medium') AS confidence,
       COALESCE(e.confidence, 'medium') AS edge_confidence
FROM edges e
LEFT JOIN decisions d ON e.from_type = 'decision' AND e.from_id = d.id AND d.status = 'active'
LEFT JOIN patterns p ON e.from_type = 'pattern' AND e.from_id = p.id AND p.status = 'active'
WHERE e.to_type = 'package' AND e.relation = 'affects'
  AND (d.id IS NOT NULL OR p.id IS NOT NULL)
ORDER BY e.to_ref, e.from_type, confidence DESC;
`)
	if err != nil {
		return // Non-fatal: edges table might not exist in older DBs
	}
	defer rows.Close()

	moduleKnowledge := map[string][]ModuleKnowledge{}
	for rows.Next() {
		var pkgPath, fromType string
		var fromID int64
		var title, confidence, edgeConfidence string
		if err := rows.Scan(&pkgPath, &fromType, &fromID, &title, &confidence, &edgeConfidence); err != nil {
			continue
		}
		moduleKnowledge[pkgPath] = append(moduleKnowledge[pkgPath], ModuleKnowledge{
			ID: fromID, Type: fromType, Title: title, Confidence: confidence, EdgeConfidence: edgeConfidence,
		})
	}

	for i := range payload.Modules {
		if k, ok := moduleKnowledge[payload.Modules[i].Path]; ok {
			if len(k) > 5 {
				k = k[:5]
			}
			payload.Modules[i].Knowledge = k
		}
	}
}

func (s *Service) loadArchitecture(ctx context.Context, payload *Payload) error {
	rows, err := s.db.QueryContext(ctx, `
SELECT f.path
FROM files f
JOIN packages p ON p.id = f.package_id
WHERE p.name = 'main' AND f.path LIKE '%main.go'
ORDER BY f.path;
`)
	if err != nil {
		return fmt.Errorf("query entry points: %w", err)
	}
	defer rows.Close()

	entryPoints := []string{}
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return fmt.Errorf("scan entry point: %w", err)
		}
		entryPoints = append(entryPoints, path)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate entry points: %w", err)
	}

	depRows, err := s.db.QueryContext(ctx, `
SELECT DISTINCT p1.path AS from_pkg, p2.path AS to_pkg
FROM imports i
JOIN files f ON f.id = i.from_file_id
JOIN packages p1 ON p1.id = f.package_id
JOIN packages p2 ON p2.id = i.to_package_id
WHERE p1.id != p2.id
ORDER BY p1.path, p2.path;
`)
	if err != nil {
		return fmt.Errorf("query dependency flow: %w", err)
	}
	defer depRows.Close()

	flowParts := map[string][]string{}
	for depRows.Next() {
		var from, to string
		if err := depRows.Scan(&from, &to); err != nil {
			return fmt.Errorf("scan dep flow: %w", err)
		}
		flowParts[from] = append(flowParts[from], to)
	}
	if err := depRows.Err(); err != nil {
		return fmt.Errorf("iterate dep flow: %w", err)
	}

	edges := make([]DependencyEdge, 0, len(flowParts))
	for from, tos := range flowParts {
		sort.Strings(tos)
		edges = append(edges, DependencyEdge{From: from, To: tos})
	}
	sort.Slice(edges, func(i, j int) bool {
		return edges[i].From < edges[j].From
	})
	payload.Architecture = Architecture{EntryPoints: entryPoints, DependencyFlow: edges}
	return nil
}

func (s *Service) loadModuleHeat(ctx context.Context, moduleRoot string, payload *Payload) {
	cmd := exec.CommandContext(ctx, "git", "-C", moduleRoot, "log", "--since=30 days ago", "--name-only", "--pretty=format:")
	out, err := cmd.Output()
	if err != nil {
		return // Non-fatal: heat is optional
	}

	counts := map[string]int{}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		dir := filepath.Dir(line)
		if dir == "." {
			counts["."]++
		} else {
			for _, m := range payload.Modules {
				if strings.HasPrefix(filepath.ToSlash(dir), m.Path) || (m.Path == "." && !strings.Contains(dir, "/")) {
					counts[m.Path]++
					break
				}
			}
		}
	}

	for i := range payload.Modules {
		c := counts[payload.Modules[i].Path]
		payload.Modules[i].RecentCommits = c
		switch {
		case c >= 4:
			payload.Modules[i].Heat = "hot"
		case c >= 1:
			payload.Modules[i].Heat = "warm"
		default:
			payload.Modules[i].Heat = "cold"
		}
	}
}

func (s *Service) loadRecentActivity(ctx context.Context, moduleRoot string, payload *Payload) {
	cmd := exec.CommandContext(ctx, "git", "-C", moduleRoot, "log", "-n", "20", "--pretty=format:%aI", "--name-only", "--diff-filter=ACMR")
	out, err := cmd.Output()
	if err != nil {
		return // Non-fatal
	}

	seen := map[string]bool{}
	activity := []RecentFile{}
	var currentDate string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// ISO date lines start with digit and have dash at position 4
		if len(line) > 10 && line[4] == '-' {
			currentDate = line
			continue
		}
		if !seen[line] && currentDate != "" {
			seen[line] = true
			activity = append(activity, RecentFile{File: line, LastModified: currentDate})
			if len(activity) >= 5 {
				break
			}
		}
	}
	payload.RecentActivity = activity
}

func computeStaleSummary(ctx context.Context, moduleRoot, fromCommit, toCommit string) string {
	cmd := exec.CommandContext(ctx, "git", "-C", moduleRoot, "rev-list", "--count", fromCommit+".."+toCommit)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	commitCount := strings.TrimSpace(string(out))

	cmd2 := exec.CommandContext(ctx, "git", "-C", moduleRoot, "diff", "--name-only", fromCommit+".."+toCommit)
	out2, _ := cmd2.Output()
	fileCount := 0
	for _, line := range strings.Split(string(out2), "\n") {
		if strings.TrimSpace(line) != "" {
			fileCount++
		}
	}

	return fmt.Sprintf("%s commits, %d files changed since last sync", commitCount, fileCount)
}
