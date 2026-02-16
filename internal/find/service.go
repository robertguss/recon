package find

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
)

type Symbol struct {
	ID        int64  `json:"id"`
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Signature string `json:"signature"`
	Body      string `json:"body"`
	LineStart int    `json:"line_start"`
	LineEnd   int    `json:"line_end"`
	Receiver  string `json:"receiver,omitempty"`
	FilePath  string `json:"file_path"`
	Package   string `json:"package"`
}

type Result struct {
	Symbol       Symbol   `json:"symbol"`
	Dependencies []Symbol `json:"dependencies"`
}

type QueryOptions struct {
	PackagePath string `json:"package,omitempty"`
	FilePath    string `json:"file,omitempty"`
	Kind        string `json:"kind,omitempty"`
}

type Candidate struct {
	Kind     string `json:"kind"`
	Receiver string `json:"receiver,omitempty"`
	FilePath string `json:"file_path"`
	Package  string `json:"package"`
}

type NotFoundError struct {
	Symbol      string
	Suggestions []string
	Filtered    bool
	Filters     QueryOptions
}

func (e NotFoundError) Error() string {
	if e.Filtered {
		return fmt.Sprintf("symbol %q not found with provided filters", e.Symbol)
	}
	if len(e.Suggestions) == 0 {
		return fmt.Sprintf("symbol %q not found", e.Symbol)
	}
	return fmt.Sprintf("symbol %q not found (suggestions: %v)", e.Symbol, e.Suggestions)
}

type AmbiguousError struct {
	Symbol     string
	Candidates []Candidate
}

func (e AmbiguousError) Error() string {
	return fmt.Sprintf("symbol %q is ambiguous (%d candidates)", e.Symbol, len(e.Candidates))
}

type Service struct {
	db *sql.DB
}

func NewService(conn *sql.DB) *Service {
	return &Service{db: conn}
}

type ListResult struct {
	Symbols []Symbol `json:"symbols"`
	Total   int      `json:"total"`
	Limit   int      `json:"limit"`
}

type PackageSummary struct {
	Path      string `json:"path"`
	Name      string `json:"name"`
	FileCount int    `json:"file_count"`
	LineCount int    `json:"line_count"`
}

func (s *Service) ListPackages(ctx context.Context) ([]PackageSummary, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT path, name, file_count, line_count FROM packages ORDER BY line_count DESC`)
	if err != nil {
		return nil, fmt.Errorf("query packages: %w", err)
	}
	defer rows.Close()

	var pkgs []PackageSummary
	for rows.Next() {
		var p PackageSummary
		if err := rows.Scan(&p.Path, &p.Name, &p.FileCount, &p.LineCount); err != nil {
			return nil, fmt.Errorf("scan package: %w", err)
		}
		pkgs = append(pkgs, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate packages: %w", err)
	}
	return pkgs, nil
}

func (s *Service) List(ctx context.Context, opts QueryOptions, limit int) (ListResult, error) {
	opts = normalizeQueryOptions(opts)
	if !hasActiveFilters(opts) {
		return ListResult{}, fmt.Errorf("list mode requires at least one filter (--package, --file, or --kind)")
	}
	if limit <= 0 {
		limit = 50
	}

	where, args := buildListWhere(opts)

	// Get total count
	var total int
	countQuery := "SELECT COUNT(*) FROM symbols s JOIN files f ON f.id = s.file_id LEFT JOIN packages p ON p.id = f.package_id WHERE " + where
	if err := s.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return ListResult{}, fmt.Errorf("count list symbols: %w", err)
	}

	// Get limited results (no body)
	selectQuery := `
SELECT s.id, s.kind, s.name, COALESCE(s.signature, ''), '',
       s.line_start, s.line_end, COALESCE(s.receiver, ''), f.path, COALESCE(p.path, '.')
FROM symbols s
JOIN files f ON f.id = s.file_id
LEFT JOIN packages p ON p.id = f.package_id
WHERE ` + where + `
ORDER BY p.path, f.path, s.kind, s.name
LIMIT ?;`
	rows, err := s.db.QueryContext(ctx, selectQuery, append(args, limit)...)
	if err != nil {
		return ListResult{}, fmt.Errorf("query list symbols: %w", err)
	}
	defer rows.Close()

	symbols := make([]Symbol, 0, limit)
	for rows.Next() {
		var sym Symbol
		if err := rows.Scan(&sym.ID, &sym.Kind, &sym.Name, &sym.Signature, &sym.Body,
			&sym.LineStart, &sym.LineEnd, &sym.Receiver, &sym.FilePath, &sym.Package); err != nil {
			return ListResult{}, fmt.Errorf("scan list symbol: %w", err)
		}
		symbols = append(symbols, sym)
	}
	if err := rows.Err(); err != nil {
		return ListResult{}, fmt.Errorf("iterate list symbols: %w", err)
	}

	return ListResult{Symbols: symbols, Total: total, Limit: limit}, nil
}

func buildListWhere(opts QueryOptions) (string, []any) {
	clauses := []string{"1=1"}
	args := []any{}
	if opts.PackagePath != "" {
		if !strings.Contains(opts.PackagePath, "/") {
			// Short name: match exact or last path segment
			clauses = append(clauses, "(COALESCE(p.path, '.') = ? OR COALESCE(p.path, '.') LIKE ?)")
			args = append(args, opts.PackagePath, "%/"+opts.PackagePath)
		} else {
			clauses = append(clauses, "COALESCE(p.path, '.') = ?")
			args = append(args, opts.PackagePath)
		}
	}
	if opts.FilePath != "" {
		if !strings.Contains(opts.FilePath, "/") {
			// Filename-only: match basename using LIKE suffix
			clauses = append(clauses, "(f.path = ? OR f.path LIKE ?)")
			args = append(args, opts.FilePath, "%/"+opts.FilePath)
		} else {
			clauses = append(clauses, "(f.path = ? OR f.path LIKE ?)")
			args = append(args, opts.FilePath, "%"+opts.FilePath+"%")
		}
	}
	if opts.Kind != "" {
		clauses = append(clauses, "LOWER(s.kind) = ?")
		args = append(args, opts.Kind)
	}
	return strings.Join(clauses, " AND "), args
}

func (s *Service) FindExact(ctx context.Context, symbol string) (Result, error) {
	return s.Find(ctx, symbol, QueryOptions{})
}

func (s *Service) Find(ctx context.Context, symbol string, opts QueryOptions) (Result, error) {
	opts = normalizeQueryOptions(opts)
	filtersApplied := hasActiveFilters(opts)

	// Support "Receiver.Name" dot syntax
	var receiverFilter string
	if idx := strings.IndexByte(symbol, '.'); idx > 0 && idx < len(symbol)-1 {
		receiverFilter = symbol[:idx]
		symbol = symbol[idx+1:]
	}

	rows, err := s.db.QueryContext(ctx, `
SELECT s.id, s.kind, s.name, COALESCE(s.signature, ''), COALESCE(s.body, ''),
       s.line_start, s.line_end, COALESCE(s.receiver, ''), f.path, COALESCE(p.path, '.')
FROM symbols s
JOIN files f ON f.id = s.file_id
LEFT JOIN packages p ON p.id = f.package_id
WHERE s.name = ?
ORDER BY p.path, f.path, s.kind, s.receiver;
`, symbol)
	if err != nil {
		return Result{}, fmt.Errorf("query symbol: %w", err)
	}
	defer rows.Close()

	matches := make([]Symbol, 0, 4)
	for rows.Next() {
		var item Symbol
		if err := rows.Scan(
			&item.ID,
			&item.Kind,
			&item.Name,
			&item.Signature,
			&item.Body,
			&item.LineStart,
			&item.LineEnd,
			&item.Receiver,
			&item.FilePath,
			&item.Package,
		); err != nil {
			return Result{}, fmt.Errorf("scan symbol row: %w", err)
		}
		matches = append(matches, item)
	}
	if err := rows.Err(); err != nil {
		return Result{}, fmt.Errorf("iterate symbol rows: %w", err)
	}

	// Apply receiver filter from dot syntax
	if receiverFilter != "" {
		filtered := make([]Symbol, 0, len(matches))
		for _, m := range matches {
			if m.Receiver == receiverFilter {
				filtered = append(filtered, m)
			}
		}
		matches = filtered
	}

	if len(matches) == 0 {
		queryLabel := symbol
		if receiverFilter != "" {
			queryLabel = receiverFilter + "." + symbol
		}
		suggestions, err := s.suggestions(ctx, symbol)
		if err != nil {
			return Result{}, err
		}
		return Result{}, NotFoundError{Symbol: queryLabel, Suggestions: suggestions}
	}

	if filtersApplied {
		matches = filterMatches(matches, opts)
		if len(matches) == 0 {
			return Result{}, NotFoundError{Symbol: symbol, Suggestions: []string{}, Filtered: true, Filters: opts}
		}
	}

	if len(matches) > 1 {
		candidates := make([]Candidate, 0, len(matches))
		for _, m := range matches {
			candidates = append(candidates, Candidate{
				Kind:     m.Kind,
				Receiver: m.Receiver,
				FilePath: m.FilePath,
				Package:  m.Package,
			})
		}
		return Result{}, AmbiguousError{Symbol: symbol, Candidates: candidates}
	}

	sym := matches[0]
	deps, err := s.directDeps(ctx, sym.ID)
	if err != nil {
		return Result{}, err
	}

	return Result{Symbol: sym, Dependencies: deps}, nil
}

func normalizeQueryOptions(opts QueryOptions) QueryOptions {
	normalized := QueryOptions{
		PackagePath: strings.TrimSpace(opts.PackagePath),
		FilePath:    normalizeFilePath(opts.FilePath),
		Kind:        strings.ToLower(strings.TrimSpace(opts.Kind)),
	}
	return normalized
}

func normalizeFilePath(path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return ""
	}
	return filepath.ToSlash(filepath.Clean(trimmed))
}

func hasActiveFilters(opts QueryOptions) bool {
	return opts.PackagePath != "" || opts.FilePath != "" || opts.Kind != ""
}

func filterMatches(matches []Symbol, opts QueryOptions) []Symbol {
	filtered := make([]Symbol, 0, len(matches))
	for _, match := range matches {
		if opts.PackagePath != "" && !matchPackagePath(match.Package, opts.PackagePath) {
			continue
		}
		if opts.FilePath != "" && !matchFilePath(normalizeFilePath(match.FilePath), opts.FilePath) {
			continue
		}
		if opts.Kind != "" && strings.ToLower(match.Kind) != opts.Kind {
			continue
		}
		filtered = append(filtered, match)
	}
	return filtered
}

func matchPackagePath(pkgPath, filter string) bool {
	if pkgPath == filter {
		return true
	}
	// Short name: match last segment
	if !strings.Contains(filter, "/") {
		lastSeg := pkgPath
		if idx := strings.LastIndex(pkgPath, "/"); idx >= 0 {
			lastSeg = pkgPath[idx+1:]
		}
		return lastSeg == filter
	}
	return false
}

func matchFilePath(symbolPath, filter string) bool {
	if symbolPath == filter {
		return true
	}
	// No slash = suffix/filename match
	if !strings.Contains(filter, "/") {
		return filepath.Base(symbolPath) == filter
	}
	// Has slash = substring match against relative path
	return strings.Contains(symbolPath, filter) || strings.HasSuffix(symbolPath, filter)
}

func (s *Service) suggestions(ctx context.Context, symbol string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT DISTINCT name
FROM symbols
WHERE name LIKE ?
ORDER BY name
LIMIT 5;
`, symbol+"%")
	if err != nil {
		return nil, fmt.Errorf("query suggestions: %w", err)
	}
	defer rows.Close()

	out := make([]string, 0, 5)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("scan suggestion: %w", err)
		}
		out = append(out, name)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate suggestions: %w", err)
	}
	return out, nil
}

func (s *Service) directDeps(ctx context.Context, symbolID int64) ([]Symbol, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT DISTINCT s2.id, s2.kind, s2.name, COALESCE(s2.signature, ''), COALESCE(s2.body, ''),
       s2.line_start, s2.line_end, COALESCE(s2.receiver, ''), f2.path, COALESCE(p2.path, '.')
FROM symbol_deps d
JOIN symbols s2 ON s2.name = d.dep_name
JOIN files f2 ON f2.id = s2.file_id
LEFT JOIN packages p2 ON p2.id = f2.package_id
WHERE d.symbol_id = ?
  AND (d.dep_package = '' OR COALESCE(p2.path, '.') = d.dep_package)
  AND (d.dep_kind = '' OR s2.kind = d.dep_kind)
ORDER BY p2.path, f2.path, s2.name
LIMIT 25;
`, symbolID)
	if err != nil {
		return nil, fmt.Errorf("query dependencies: %w", err)
	}
	defer rows.Close()

	deps := make([]Symbol, 0, 8)
	for rows.Next() {
		var dep Symbol
		if err := rows.Scan(
			&dep.ID,
			&dep.Kind,
			&dep.Name,
			&dep.Signature,
			&dep.Body,
			&dep.LineStart,
			&dep.LineEnd,
			&dep.Receiver,
			&dep.FilePath,
			&dep.Package,
		); err != nil {
			return nil, fmt.Errorf("scan dependency row: %w", err)
		}
		deps = append(deps, dep)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate dependency rows: %w", err)
	}
	return deps, nil
}
