package cli

import (
	"fmt"

	"github.com/robertguss/recon/internal/index"
	"github.com/spf13/cobra"
)

func newSyncCommand(app *App) *cobra.Command {
	var jsonOut bool

	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Index Go source code into recon",
		RunE: func(cmd *cobra.Command, args []string) error {
			conn, err := openExistingDB(app)
			if err != nil {
				return err
			}
			defer conn.Close()

			result, err := index.NewService(conn).Sync(cmd.Context(), app.ModuleRoot)
			if err != nil {
				return err
			}

			if jsonOut {
				return writeJSON(result)
			}

			fmt.Printf("Synced %d files, %d symbols across %d packages\n", result.IndexedFiles, result.IndexedSymbols, result.IndexedPackages)
			fmt.Printf("Fingerprint: %s\n", result.Fingerprint)
			if result.Commit != "" {
				fmt.Printf("Git commit: %s dirty=%v\n", result.Commit, result.Dirty)
			}
			fmt.Printf("Synced at: %s\n", result.SyncedAt.Format("2006-01-02T15:04:05Z07:00"))
			return nil
		},
	}

	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output JSON")
	return cmd
}
