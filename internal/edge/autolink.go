package edge

import (
	"context"
	"database/sql"
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
