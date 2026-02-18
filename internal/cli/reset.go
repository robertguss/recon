package cli

import (
	"fmt"
	"os"

	"github.com/robertguss/recon/internal/db"
	"github.com/spf13/cobra"
)

func newResetCommand(app *App) *cobra.Command {
	var (
		force   bool
		jsonOut bool
	)

	cmd := &cobra.Command{
		Use:   "reset",
		Short: "Delete the recon database and start fresh",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			path := db.DBPath(app.ModuleRoot)

			if _, err := os.Stat(path); os.IsNotExist(err) {
				if jsonOut {
					return writeJSON(map[string]any{"reset": false, "reason": "not initialized"})
				}
				fmt.Println("Nothing to reset: database not initialized.")
				return nil
			}

			if !force && !app.NoPrompt {
				fmt.Printf("This will delete %s. Continue? [y/N] ", path)
				var confirm string
				fmt.Scan(&confirm)
				if confirm != "y" && confirm != "Y" {
					fmt.Println("Aborted.")
					return nil
				}
			}

			if err := os.Remove(path); err != nil {
				if jsonOut {
					_ = writeJSONError("internal_error", err.Error(), nil)
					return ExitError{Code: 2}
				}
				return fmt.Errorf("delete database: %w", err)
			}

			if jsonOut {
				return writeJSON(map[string]any{"reset": true, "path": path})
			}
			fmt.Printf("Database reset. Run `recon init` to reinitialize.\n")
			return nil
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "Skip confirmation prompt")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output JSON")
	return cmd
}
