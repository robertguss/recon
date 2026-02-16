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
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/robertguss/recon/internal/db"
)

var (
	collectEligibleFiles = CollectEligibleGoFiles
	importPathUnquote    = strconv.Unquote
)

type SyncDiff struct {
	FilesAdded     int `json:"files_added"`
	FilesRemoved   int `json:"files_removed"`
	FilesModified  int `json:"files_modified"`
	SymbolsBefore  int `json:"symbols_before"`
	SymbolsAfter   int `json:"symbols_after"`
	PackagesBefore int `json:"packages_before"`
	PackagesAfter  int `json:"packages_after"`
}

type SyncResult struct {
	IndexedFiles    int       `json:"indexed_files"`
	IndexedSymbols  int       `json:"indexed_symbols"`
	IndexedPackages int       `json:"indexed_packages"`
	Fingerprint     string    `json:"fingerprint"`
	Commit          string    `json:"commit"`
	Dirty           bool      `json:"dirty"`
	SyncedAt        time.Time `json:"synced_at"`
	Diff            *SyncDiff `json:"diff,omitempty"`
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

	files, err := collectEligibleFiles(moduleRoot)
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

	// Capture previous state for diff computation
	var prevFiles, prevSymbols, prevPackages int
	_ = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM files").Scan(&prevFiles)
	_ = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM symbols").Scan(&prevSymbols)
	_ = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM packages").Scan(&prevPackages)

	prevHashes := map[string]string{}
	if prevFiles > 0 {
		hashRows, hashErr := tx.QueryContext(ctx, "SELECT path, hash FROM files")
		if hashErr == nil {
			for hashRows.Next() {
				var p, h string
				hashRows.Scan(&p, &h)
				prevHashes[p] = h
			}
			hashRows.Close()
		}
	}

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

		localImportAliases := map[string]string{}

		for _, imp := range parsed.Imports {
			toPath, err := importPathUnquote(imp.Path.Value)
			if err != nil {
				toPath = strings.Trim(imp.Path.Value, "\"")
			}
			alias := ""
			if imp.Name != nil {
				alias = imp.Name.Name
			} else {
				alias = path.Base(toPath)
			}

			importType := "external"
			var toPkgID any
			localPkgPath := ""
			if toPath == modulePath || strings.HasPrefix(toPath, modulePath+"/") {
				importType = "local"
				rel := strings.TrimPrefix(toPath, modulePath)
				rel = strings.TrimPrefix(rel, "/")
				if rel == "" {
					rel = "."
				}
				localPkgPath = rel
				if localStats, ok := packageStats[rel]; ok {
					toPkgID = localStats.ID
				}
			}
			if alias != "" && alias != "_" && alias != "." {
				localImportAliases[alias] = localPkgPath
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
			records := symbolRecordsFromDeclWithContext(fset, file.Content, decl, depContext{
				PackagePath:  pkgPath,
				LocalImports: localImportAliases,
			})
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

				for _, dep := range rec.DepRefs {
					if _, err := tx.ExecContext(ctx, `
INSERT OR IGNORE INTO symbol_deps (symbol_id, dep_name, dep_package, dep_kind)
VALUES (?, ?, ?, ?);
`, symbolID, dep.Name, dep.PackagePath, dep.Kind); err != nil {
						return SyncResult{}, fmt.Errorf("insert symbol dep %s: %w", dep.Name, err)
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

	// Compute diff if there was previous data
	var diff *SyncDiff
	if prevFiles > 0 || prevSymbols > 0 || prevPackages > 0 {
		newPaths := map[string]string{}
		for _, f := range files {
			newPaths[f.RelPath] = f.Hash
		}

		added, removed, modified := 0, 0, 0
		for p, oldHash := range prevHashes {
			newHash, exists := newPaths[p]
			if !exists {
				removed++
			} else if oldHash != newHash {
				modified++
			}
		}
		for p := range newPaths {
			if _, existed := prevHashes[p]; !existed {
				added++
			}
		}

		diff = &SyncDiff{
			FilesAdded:     added,
			FilesRemoved:   removed,
			FilesModified:  modified,
			SymbolsBefore:  prevSymbols,
			SymbolsAfter:   symbolCount,
			PackagesBefore: prevPackages,
			PackagesAfter:  len(packageStats),
		}
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
		Diff:            diff,
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
	DepRefs   []depRef
}

type depRef struct {
	Name        string
	PackagePath string
	Kind        string
}

type depContext struct {
	PackagePath  string
	LocalImports map[string]string
}

func symbolRecordsFromDecl(fset *token.FileSet, src []byte, decl ast.Decl) []symbolRecord {
	return symbolRecordsFromDeclWithContext(fset, src, decl, depContext{})
}

func symbolRecordsFromDeclWithContext(fset *token.FileSet, src []byte, decl ast.Decl, ctx depContext) []symbolRecord {
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
			DepRefs:   collectCallDeps(d.Body, ctx),
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
	depsWithContext := collectCallDeps(body, depContext{})
	if depsWithContext == nil {
		return nil
	}

	nameSet := map[string]struct{}{}
	for _, dep := range depsWithContext {
		if dep.Name != "" {
			nameSet[dep.Name] = struct{}{}
		}
	}

	deps := make([]string, 0, len(nameSet))
	for name := range nameSet {
		deps = append(deps, name)
	}
	sort.Strings(deps)
	return deps
}

func collectCallDeps(body *ast.BlockStmt, ctx depContext) []depRef {
	if body == nil {
		return nil
	}
	set := map[string]depRef{}
	addDep := func(dep depRef) {
		key := dep.Name + "\x00" + dep.PackagePath + "\x00" + dep.Kind
		set[key] = dep
	}

	currentPackage := strings.TrimSpace(ctx.PackagePath)
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		switch fn := call.Fun.(type) {
		case *ast.Ident:
			if fn.Name != "" {
				addDep(depRef{Name: fn.Name, PackagePath: currentPackage, Kind: "func"})
			}
		case *ast.SelectorExpr:
			if fn.Sel != nil && fn.Sel.Name != "" {
				if ident, ok := fn.X.(*ast.Ident); ok {
					if pkgPath, found := ctx.LocalImports[ident.Name]; found {
						if pkgPath != "" {
							addDep(depRef{Name: fn.Sel.Name, PackagePath: pkgPath, Kind: "func"})
						}
						return true
					}

					addDep(depRef{Name: fn.Sel.Name, PackagePath: currentPackage, Kind: "method"})
				}
			}
		}
		return true
	})

	deps := make([]depRef, 0, len(set))
	for _, dep := range set {
		deps = append(deps, dep)
	}
	sort.Slice(deps, func(i, j int) bool {
		if deps[i].Name != deps[j].Name {
			return deps[i].Name < deps[j].Name
		}
		if deps[i].PackagePath != deps[j].PackagePath {
			return deps[i].PackagePath < deps[j].PackagePath
		}
		return deps[i].Kind < deps[j].Kind
	})
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
