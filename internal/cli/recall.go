package cli

import (
	"fmt"

	"github.com/robertguss/recon/internal/recall"
	"github.com/spf13/cobra"
)

func newRecallCommand(app *App) *cobra.Command {
	var (
		jsonOut bool
		limit   int
	)

	cmd := &cobra.Command{
		Use:   "recall <query>",
		Short: "Search promoted knowledge",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				msg := "recall requires a <query> argument"
				if jsonOut {
					_ = writeJSONError("missing_argument", msg, map[string]any{"command": "recall"})
					return ExitError{Code: 2}
				}
				return ExitError{Code: 2, Message: msg}
			}
			query := args[0]

			conn, err := openExistingDB(app)
			if err != nil {
				if jsonOut {
					return exitJSONCommandError(err)
				}
				return err
			}
			defer conn.Close()

			result, err := recall.NewService(conn).Recall(cmd.Context(), query, recall.RecallOptions{Limit: limit})
			if err != nil {
				if jsonOut {
					return exitJSONCommandError(err)
				}
				return err
			}

			if jsonOut {
				return writeJSON(result)
			}

			if len(result.Items) == 0 {
				fmt.Println("No promoted knowledge found.")
				return nil
			}
			for _, item := range result.Items {
				id := item.DecisionID
				label := "decision"
				if item.EntityType == "pattern" {
					id = item.PatternID
					label = "pattern"
				}
				fmt.Printf("- [%s] #%d %s [%s] drift=%s\n", label, id, item.Title, item.Confidence, item.EvidenceDrift)
				fmt.Printf("  %s\n", item.EvidenceSummary)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output JSON")
	cmd.Flags().IntVar(&limit, "limit", 10, "Maximum results")
	return cmd
}
