package cli

import (
	"fmt"
	"path/filepath"
	"strings"

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
	)

	cmd := &cobra.Command{
		Use:   "find <symbol>",
		Short: "Find exact symbol and direct in-project dependencies",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
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

			normalizedKind, err := normalizeFindKind(kindFilter)
			if err != nil {
				if jsonOut {
					details := map[string]any{"kind": strings.TrimSpace(kindFilter)}
					_ = writeJSONError("invalid_input", err.Error(), details)
					return ExitError{Code: 2}
				}
				return ExitError{Code: 2, Message: err.Error()}
			}

			conn, err := openExistingDB(app)
			if err != nil {
				if jsonOut {
					return exitJSONCommandError(err)
				}
				return err
			}
			defer conn.Close()

			queryOptions := find.QueryOptions{
				PackagePath: strings.TrimSpace(packageFilter),
				FilePath:    normalizeFindPath(fileFilter),
				Kind:        normalizedKind,
			}

			result, err := find.NewService(conn).Find(cmd.Context(), symbol, queryOptions)
			if err != nil {
				switch e := err.(type) {
				case find.NotFoundError:
					if jsonOut {
						details := map[string]any{
							"symbol":      symbol,
							"suggestions": e.Suggestions,
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
	return cmd
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
