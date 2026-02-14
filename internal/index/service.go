package index

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/robertguss/recon/internal/db"
)

type SyncResult struct {
	IndexedFiles    int       `json:"indexed_files"`
	IndexedSymbols  int       `json:"indexed_symbols"`
	IndexedPackages int       `json:"indexed_packages"`
	Fingerprint     string    `json:"fingerprint"`
	Commit          string    `json:"commit"`
	Dirty           bool      `json:"dirty"`
	SyncedAt        time.Time `json:"synced_at"`
}

type Service struct {
	db *sql.DB
}

func NewService(conn *sql.DB) *Service {
	return &Service{db: conn}
}

func (s *Service) Sync(ctx context.Context, moduleRoot string) (SyncResult, error) {
	modulePath, err := ModulePath(moduleRoot)
	if err != nil {
		return SyncResult{}, err
	}

	files, err := CollectEligibleGoFiles(moduleRoot)
	if err != nil {
		return SyncResult{}, err
	}
	fingerprint := ComputeFingerprint(files)
	commit, dirty := CurrentGitState(ctx, moduleRoot)
	now := time.Now().UTC()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return SyncResult{}, fmt.Errorf("begin sync tx: %w", err)
	}
	defer tx.Rollback()

	for _, q := range []string{
		"DELETE FROM symbol_deps;",
		"DELETE FROM imports;",
		"DELETE FROM symbols;",
		"DELETE FROM files;",
		"DELETE FROM packages;",
	} {
		if _, err := tx.ExecContext(ctx, q); err != nil {
			return SyncResult{}, fmt.Errorf("reset index tables: %w", err)
		}
	}

	type pkgStats struct {
		ID        int64
		Name      string
		Import    string
		FileCount int
		LineCount int
	}
	packageStats := map[string]*pkgStats{}
	symbolCount := 0

	for _, file := range files {
		fset := token.NewFileSet()
		parsed, err := parser.ParseFile(fset, file.AbsPath, file.Content, parser.ParseComments)
		if err != nil {
			return SyncResult{}, fmt.Errorf("parse %s: %w", file.RelPath, err)
		}

		pkgPath := filepath.ToSlash(filepath.Dir(file.RelPath))
		if pkgPath == "." {
			pkgPath = "."
		}
		importPath := modulePath
		if pkgPath != "." {
			importPath = modulePath + "/" + pkgPath
		}

		stats := packageStats[pkgPath]
		if stats == nil {
			res, err := tx.ExecContext(ctx, `
INSERT INTO packages (path, name, import_path, file_count, line_count, created_at, updated_at)
VALUES (?, ?, ?, 0, 0, ?, ?);
`, pkgPath, parsed.Name.Name, importPath, now.Format(time.RFC3339), now.Format(time.RFC3339))
			if err != nil {
				return SyncResult{}, fmt.Errorf("insert package %s: %w", pkgPath, err)
			}
			pkgID, err := res.LastInsertId()
			if err != nil {
				return SyncResult{}, fmt.Errorf("read package id: %w", err)
			}
			stats = &pkgStats{ID: pkgID, Name: parsed.Name.Name, Import: importPath}
			packageStats[pkgPath] = stats
		}
		stats.FileCount++
		stats.LineCount += file.Lines

		res, err := tx.ExecContext(ctx, `
INSERT INTO files (package_id, path, language, lines, hash, created_at, updated_at)
VALUES (?, ?, 'go', ?, ?, ?, ?);
`, stats.ID, file.RelPath, file.Lines, file.Hash, now.Format(time.RFC3339), now.Format(time.RFC3339))
		if err != nil {
			return SyncResult{}, fmt.Errorf("insert file %s: %w", file.RelPath, err)
		}
		fileID, err := res.LastInsertId()
		if err != nil {
			return SyncResult{}, fmt.Errorf("read file id: %w", err)
		}

		for _, imp := range parsed.Imports {
			toPath, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				toPath = strings.Trim(imp.Path.Value, "\"")
			}
			alias := ""
			if imp.Name != nil {
				alias = imp.Name.Name
			}

			importType := "external"
			var toPkgID any
			if toPath == modulePath || strings.HasPrefix(toPath, modulePath+"/") {
				importType = "local"
				rel := strings.TrimPrefix(toPath, modulePath)
				rel = strings.TrimPrefix(rel, "/")
				if rel == "" {
					rel = "."
				}
				if localStats, ok := packageStats[rel]; ok {
					toPkgID = localStats.ID
				}
			}

			if _, err := tx.ExecContext(ctx, `
INSERT INTO imports (from_file_id, to_path, to_package_id, alias, import_type)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT(from_file_id, to_path) DO UPDATE SET
    to_package_id = excluded.to_package_id,
    alias = excluded.alias,
    import_type = excluded.import_type;
`, fileID, toPath, toPkgID, alias, importType); err != nil {
				return SyncResult{}, fmt.Errorf("insert import %s: %w", toPath, err)
			}
		}

		for _, decl := range parsed.Decls {
			records := symbolRecordsFromDecl(fset, file.Content, decl)
			for _, rec := range records {
				if _, err := tx.ExecContext(ctx, `
INSERT INTO symbols (file_id, kind, name, signature, body, line_start, line_end, exported, receiver)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(file_id, kind, name, receiver) DO UPDATE SET
    signature = excluded.signature,
    body = excluded.body,
    line_start = excluded.line_start,
    line_end = excluded.line_end,
    exported = excluded.exported;
`, fileID, rec.Kind, rec.Name, rec.Signature, rec.Body, rec.LineStart, rec.LineEnd, boolToInt(rec.Exported), rec.Receiver); err != nil {
					return SyncResult{}, fmt.Errorf("insert symbol %s: %w", rec.Name, err)
				}

				var symbolID int64
				if err := tx.QueryRowContext(ctx, `
SELECT id FROM symbols WHERE file_id = ? AND kind = ? AND name = ? AND receiver = ?;
`, fileID, rec.Kind, rec.Name, rec.Receiver).Scan(&symbolID); err != nil {
					return SyncResult{}, fmt.Errorf("resolve symbol id for %s: %w", rec.Name, err)
				}
				symbolCount++

				for _, dep := range rec.DepNames {
					if _, err := tx.ExecContext(ctx, `
INSERT OR IGNORE INTO symbol_deps (symbol_id, dep_name)
VALUES (?, ?);
`, symbolID, dep); err != nil {
						return SyncResult{}, fmt.Errorf("insert symbol dep %s: %w", dep, err)
					}
				}
			}
		}
	}

	for pkgPath, stats := range packageStats {
		if _, err := tx.ExecContext(ctx, `
UPDATE packages
SET file_count = ?, line_count = ?, updated_at = ?
WHERE path = ?;
`, stats.FileCount, stats.LineCount, now.Format(time.RFC3339), pkgPath); err != nil {
			return SyncResult{}, fmt.Errorf("update package stats for %s: %w", pkgPath, err)
		}
	}

	if err := db.UpsertSyncState(ctx, tx, db.SyncState{
		LastSyncAt:       now,
		LastSyncCommit:   commit,
		LastSyncDirty:    dirty,
		IndexedFileCount: len(files),
		IndexFingerprint: fingerprint,
	}); err != nil {
		return SyncResult{}, err
	}

	if err := tx.Commit(); err != nil {
		return SyncResult{}, fmt.Errorf("commit sync tx: %w", err)
	}

	return SyncResult{
		IndexedFiles:    len(files),
		IndexedSymbols:  symbolCount,
		IndexedPackages: len(packageStats),
		Fingerprint:     fingerprint,
		Commit:          commit,
		Dirty:           dirty,
		SyncedAt:        now,
	}, nil
}

type symbolRecord struct {
	Kind      string
	Name      string
	Signature string
	Body      string
	LineStart int
	LineEnd   int
	Exported  bool
	Receiver  string
	DepNames  []string
}

func symbolRecordsFromDecl(fset *token.FileSet, src []byte, decl ast.Decl) []symbolRecord {
	records := make([]symbolRecord, 0, 4)

	switch d := decl.(type) {
	case *ast.FuncDecl:
		rec := symbolRecord{
			Kind:      "func",
			Name:      d.Name.Name,
			Signature: exprString(d.Type),
			Body:      textForPos(fset, src, d.Pos(), d.End()),
			LineStart: fset.Position(d.Pos()).Line,
			LineEnd:   fset.Position(d.End()).Line,
			Exported:  ast.IsExported(d.Name.Name),
			Receiver:  receiverName(d),
			DepNames:  collectCallNames(d.Body),
		}
		if rec.Receiver != "" {
			rec.Kind = "method"
		}
		records = append(records, rec)
	case *ast.GenDecl:
		kind := strings.ToLower(d.Tok.String())
		if kind != "type" && kind != "const" && kind != "var" {
			return records
		}
		for _, spec := range d.Specs {
			switch s := spec.(type) {
			case *ast.TypeSpec:
				records = append(records, symbolRecord{
					Kind:      "type",
					Name:      s.Name.Name,
					Signature: exprString(s.Type),
					Body:      textForPos(fset, src, s.Pos(), s.End()),
					LineStart: fset.Position(s.Pos()).Line,
					LineEnd:   fset.Position(s.End()).Line,
					Exported:  ast.IsExported(s.Name.Name),
				})
			case *ast.ValueSpec:
				for _, n := range s.Names {
					records = append(records, symbolRecord{
						Kind:      kind,
						Name:      n.Name,
						Signature: exprString(s.Type),
						Body:      textForPos(fset, src, s.Pos(), s.End()),
						LineStart: fset.Position(s.Pos()).Line,
						LineEnd:   fset.Position(s.End()).Line,
						Exported:  ast.IsExported(n.Name),
					})
				}
			}
		}
	}

	return records
}

func collectCallNames(body *ast.BlockStmt) []string {
	if body == nil {
		return nil
	}
	set := map[string]struct{}{}
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		switch fn := call.Fun.(type) {
		case *ast.Ident:
			if fn.Name != "" {
				set[fn.Name] = struct{}{}
			}
		case *ast.SelectorExpr:
			if fn.Sel != nil && fn.Sel.Name != "" {
				set[fn.Sel.Name] = struct{}{}
			}
		}
		return true
	})

	deps := make([]string, 0, len(set))
	for name := range set {
		deps = append(deps, name)
	}
	sort.Strings(deps)
	return deps
}

func receiverName(d *ast.FuncDecl) string {
	if d.Recv == nil || len(d.Recv.List) == 0 {
		return ""
	}
	recv := d.Recv.List[0].Type
	switch t := recv.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		if id, ok := t.X.(*ast.Ident); ok {
			return "*" + id.Name
		}
	}
	return exprString(recv)
}

func exprString(expr any) string {
	if expr == nil {
		return ""
	}
	var b bytes.Buffer
	if err := printer.Fprint(&b, token.NewFileSet(), expr); err != nil {
		return ""
	}
	return b.String()
}

func textForPos(fset *token.FileSet, src []byte, start, end token.Pos) string {
	if !start.IsValid() || !end.IsValid() {
		return ""
	}
	file := fset.File(start)
	if file == nil {
		return ""
	}
	offStart := file.Offset(start)
	offEnd := file.Offset(end)
	if offStart < 0 || offEnd > len(src) || offStart >= offEnd {
		return ""
	}
	return string(src[offStart:offEnd])
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
