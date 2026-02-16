package cli

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/robertguss/recon/internal/edge"
	"github.com/robertguss/recon/internal/find"
	"github.com/spf13/cobra"
)

func newFindCommand(app *App) *cobra.Command {
	var (
		jsonOut       bool
		noBody        bool
		maxBodyLines  int
		packageFilter string
		fileFilter    string
		kindFilter    string
		limit         int
		listPackages  bool
	)

	cmd := &cobra.Command{
		Use:   "find [<symbol>]",
		Short: "Find exact symbol or list symbols by filter",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if listPackages {
				conn, connErr := openExistingDB(app)
				if connErr != nil {
					if jsonOut {
						return exitJSONCommandError(connErr)
					}
					return connErr
				}
				defer conn.Close()

				pkgs, err := find.NewService(conn).ListPackages(cmd.Context())
				if err != nil {
					if jsonOut {
						_ = writeJSONError("internal_error", err.Error(), nil)
						return ExitError{Code: 2}
					}
					return err
				}

				if jsonOut {
					return writeJSON(pkgs)
				}

				fmt.Printf("Packages (%d):\n", len(pkgs))
				for _, p := range pkgs {
					fmt.Printf("- %s  %d files  %d lines\n", p.Path, p.FileCount, p.LineCount)
				}
				return nil
			}

			normalizedKind, err := normalizeFindKind(kindFilter)
			if err != nil {
				if jsonOut {
					details := map[string]any{"kind": strings.TrimSpace(kindFilter)}
					_ = writeJSONError("invalid_input", err.Error(), details)
					return ExitError{Code: 2}
				}
				return ExitError{Code: 2, Message: err.Error()}
			}

			queryOptions := find.QueryOptions{
				PackagePath: strings.TrimSpace(packageFilter),
				FilePath:    normalizeFindPath(fileFilter),
				Kind:        normalizedKind,
			}

			// No symbol arg: check for list mode vs missing arg error
			if len(args) == 0 {
				hasFilters := queryOptions.PackagePath != "" || queryOptions.FilePath != "" || queryOptions.Kind != ""
				if !hasFilters {
					msg := "find requires a <symbol> argument or filter flags (--package, --file, --kind)"
					if jsonOut {
						_ = writeJSONError("missing_argument", msg, map[string]any{"command": "find"})
						return ExitError{Code: 2}
					}
					return ExitError{Code: 2, Message: msg}
				}
				return runFindListMode(cmd, app, queryOptions, limit, jsonOut)
			}

			symbol := args[0]
			if maxBodyLines < 0 {
				msg := "--max-body-lines must be >= 0"
				if jsonOut {
					details := map[string]any{"flag": "max_body_lines", "value": maxBodyLines}
					_ = writeJSONError("invalid_input", msg, details)
					return ExitError{Code: 2}
				}
				return ExitError{Code: 2, Message: msg}
			}

			conn, connErr := openExistingDB(app)
			if connErr != nil {
				if jsonOut {
					return exitJSONCommandError(connErr)
				}
				return connErr
			}
			defer conn.Close()

			result, findErr := find.NewService(conn).Find(cmd.Context(), symbol, queryOptions)
			err = findErr
			if err != nil {
				switch e := err.(type) {
				case find.NotFoundError:
					if jsonOut {
						details := map[string]any{
							"symbol":      symbol,
							"suggestions": e.Suggestions,
						}
						if len(e.Suggestions) == 0 {
							details["tip"] = "try --kind func|type|var|method|const to browse, or --list-packages to see indexed packages"
						}
						addFindFilterDetails(details, queryOptions)
						_ = writeJSONError("not_found", e.Error(), details)
					} else {
						if e.Filtered {
							fmt.Printf("symbol %q not found with provided filters\n", symbol)
						} else {
							fmt.Printf("symbol %q not found\n", symbol)
						}
						printFindFilters(queryOptions)
						if len(e.Suggestions) > 0 {
							fmt.Println("Suggestions:")
							for _, suggestion := range e.Suggestions {
								fmt.Printf("- %s\n", suggestion)
							}
						} else {
							fmt.Println("Tip: try --kind func|type|var|method|const to browse, or --list-packages to see indexed packages")
						}
					}
					return ExitError{Code: 2}
				case find.AmbiguousError:
					if jsonOut {
						details := map[string]any{
							"symbol":     symbol,
							"candidates": e.Candidates,
						}
						addFindFilterDetails(details, queryOptions)
						_ = writeJSONError("ambiguous", e.Error(), details)
					} else {
						fmt.Printf("symbol %q is ambiguous (%d candidates)\n", symbol, len(e.Candidates))
						printFindFilters(queryOptions)
						for _, candidate := range e.Candidates {
							label := symbol
							if candidate.Receiver != "" {
								label = candidate.Receiver + "." + symbol
							}
							fmt.Printf("- %s %s (%s, pkg %s)\n", candidate.Kind, label, candidate.FilePath, candidate.Package)
						}
					}
					return ExitError{Code: 2}
				default:
					if jsonOut {
						_ = writeJSONError("internal_error", err.Error(), nil)
						return ExitError{Code: 2}
					}
					return err
				}
			}

			if jsonOut {
				result.Knowledge = enrichFindKnowledge(cmd, conn, result.Symbol)
				return writeJSON(result)
			}

			fmt.Printf("%s %s (%s)\n", result.Symbol.Kind, result.Symbol.Name, result.Symbol.FilePath)
			fmt.Printf("Lines: %d-%d\n", result.Symbol.LineStart, result.Symbol.LineEnd)
			if result.Symbol.Receiver != "" {
				fmt.Printf("Receiver: %s\n", result.Symbol.Receiver)
			}
			if !noBody {
				fmt.Println("\nBody:")
				fmt.Println(truncateBody(result.Symbol.Body, maxBodyLines))
			}
			fmt.Println("\nDirect dependencies:")
			if len(result.Dependencies) == 0 {
				fmt.Println("- (none)")
			} else {
				for _, dep := range result.Dependencies {
					fmt.Printf("- %s %s (%s)\n", dep.Kind, dep.Name, dep.FilePath)
				}
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output JSON")
	cmd.Flags().BoolVar(&noBody, "no-body", false, "Omit symbol body in text output")
	cmd.Flags().IntVar(&maxBodyLines, "max-body-lines", 0, "Maximum symbol body lines in text output (0 = no limit)")
	cmd.Flags().StringVar(&packageFilter, "package", "", "Filter by package path when symbols are ambiguous")
	cmd.Flags().StringVar(&fileFilter, "file", "", "Filter by file path when symbols are ambiguous")
	cmd.Flags().StringVar(&kindFilter, "kind", "", "Filter by symbol kind (func, method, type, var, const)")
	cmd.Flags().IntVar(&limit, "limit", 50, "Maximum symbols in list mode")
	cmd.Flags().BoolVar(&listPackages, "list-packages", false, "List all indexed packages")
	return cmd
}

func runFindListMode(cmd *cobra.Command, app *App, opts find.QueryOptions, limit int, jsonOut bool) error {
	conn, err := openExistingDB(app)
	if err != nil {
		if jsonOut {
			return exitJSONCommandError(err)
		}
		return err
	}
	defer conn.Close()

	result, err := find.NewService(conn).List(cmd.Context(), opts, limit)
	if err != nil {
		if jsonOut {
			_ = writeJSONError("internal_error", err.Error(), nil)
			return ExitError{Code: 2}
		}
		return err
	}

	if jsonOut {
		return writeJSON(result)
	}

	fmt.Printf("Symbols (%d of %d):\n", len(result.Symbols), result.Total)
	for _, s := range result.Symbols {
		label := s.Name
		if s.Receiver != "" {
			label = s.Receiver + "." + s.Name
		}
		fmt.Printf("- %s %s (%s:%d-%d) pkg=%s\n", s.Kind, label, s.FilePath, s.LineStart, s.LineEnd, s.Package)
	}
	return nil
}

func normalizeFindPath(path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return ""
	}
	return filepath.ToSlash(filepath.Clean(trimmed))
}

func normalizeFindKind(kind string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(kind))
	if normalized == "" {
		return "", nil
	}
	switch normalized {
	case "func", "method", "type", "var", "const":
		return normalized, nil
	default:
		return "", fmt.Errorf("--kind must be one of: func, method, type, var, const")
	}
}

func addFindFilterDetails(details map[string]any, opts find.QueryOptions) {
	if opts.PackagePath != "" {
		details["package"] = opts.PackagePath
	}
	if opts.FilePath != "" {
		details["file"] = opts.FilePath
	}
	if opts.Kind != "" {
		details["kind"] = opts.Kind
	}
}

func printFindFilters(opts find.QueryOptions) {
	if opts.PackagePath != "" {
		fmt.Printf("Filter package: %s\n", opts.PackagePath)
	}
	if opts.FilePath != "" {
		fmt.Printf("Filter file: %s\n", opts.FilePath)
	}
	if opts.Kind != "" {
		fmt.Printf("Filter kind: %s\n", opts.Kind)
	}
}

func truncateBody(body string, maxLines int) string {
	if maxLines <= 0 {
		return body
	}
	lines := strings.Split(body, "\n")
	if len(lines) <= maxLines {
		return body
	}
	return strings.Join(append(lines[:maxLines], "... (truncated)"), "\n")
}

func enrichFindKnowledge(cmd *cobra.Command, conn *sql.DB, sym find.Symbol) []find.KnowledgeLink {
	edgeSvc := edge.NewService(conn)
	var links []find.KnowledgeLink

	// Edges pointing at this symbol's package
	if sym.Package != "" {
		pkgEdges, _ := edgeSvc.ListTo(cmd.Context(), "package", sym.Package)
		for _, e := range pkgEdges {
			links = append(links, edgeToKnowledgeLink(conn, e))
		}
	}

	// Edges pointing at this symbol directly
	symRef := sym.Package + "." + sym.Name
	symEdges, _ := edgeSvc.ListTo(cmd.Context(), "symbol", symRef)
	for _, e := range symEdges {
		links = append(links, edgeToKnowledgeLink(conn, e))
	}

	return links
}

func edgeToKnowledgeLink(conn *sql.DB, e edge.Edge) find.KnowledgeLink {
	link := find.KnowledgeLink{
		EntityType: e.FromType,
		EntityID:   e.FromID,
		Relation:   e.Relation,
		Confidence: e.Confidence,
	}
	// Resolve title from the source entity
	switch e.FromType {
	case "decision":
		_ = conn.QueryRow(`SELECT title FROM decisions WHERE id = ?`, e.FromID).Scan(&link.Title)
	case "pattern":
		_ = conn.QueryRow(`SELECT title FROM patterns WHERE id = ?`, e.FromID).Scan(&link.Title)
	}
	return link
}
