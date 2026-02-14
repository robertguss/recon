package cli

import (
	"fmt"

	"github.com/robertguss/recon/internal/find"
	"github.com/spf13/cobra"
)

func newFindCommand(app *App) *cobra.Command {
	var jsonOut bool

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
						_ = writeJSON(map[string]any{"error": "not_found", "symbol": symbol, "suggestions": e.Suggestions})
					}
					return err
				case find.AmbiguousError:
					if jsonOut {
						_ = writeJSON(map[string]any{"error": "ambiguous", "symbol": symbol, "candidates": e.Candidates})
					}
					return err
				default:
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
			fmt.Println("\nBody:")
			fmt.Println(result.Symbol.Body)
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
	return cmd
}
