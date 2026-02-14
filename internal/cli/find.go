package cli

import (
	"fmt"
	"strings"

	"github.com/robertguss/recon/internal/find"
	"github.com/spf13/cobra"
)

func newFindCommand(app *App) *cobra.Command {
	var (
		jsonOut      bool
		noBody       bool
		maxBodyLines int
	)

	cmd := &cobra.Command{
		Use:   "find <symbol>",
		Short: "Find exact symbol and direct in-project dependencies",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			symbol := args[0]
			conn, err := openExistingDB(app)
			if err != nil {
				return err
			}
			defer conn.Close()

			result, err := find.NewService(conn).FindExact(cmd.Context(), symbol)
			if err != nil {
				switch e := err.(type) {
				case find.NotFoundError:
					if jsonOut {
						details := map[string]any{
							"symbol":      symbol,
							"suggestions": e.Suggestions,
						}
						_ = writeJSONError("not_found", e.Error(), details)
					} else {
						fmt.Printf("symbol %q not found\n", symbol)
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
						_ = writeJSONError("ambiguous", e.Error(), details)
					} else {
						fmt.Printf("symbol %q is ambiguous (%d candidates)\n", symbol, len(e.Candidates))
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
	return cmd
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
