package cli

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/robertguss/recon/internal/index"
	"github.com/spf13/cobra"
)

var runSync = func(ctx context.Context, conn *sql.DB, moduleRoot string) (index.SyncResult, error) {
	return index.NewService(conn).Sync(ctx, moduleRoot)
}

func newSyncCommand(app *App) *cobra.Command {
	var jsonOut bool

	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Index Go source code into recon",
		RunE: func(cmd *cobra.Command, args []string) error {
			conn, err := openExistingDB(app)
			if err != nil {
				if jsonOut {
					return exitJSONCommandError(err)
				}
				return err
			}
			defer conn.Close()

			result, err := runSync(cmd.Context(), conn, app.ModuleRoot)
			if err != nil {
				if jsonOut {
					return exitJSONCommandError(err)
				}
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
