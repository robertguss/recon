package cli

import (
	"context"
	"database/sql"
	"fmt"
	"os"

	"github.com/robertguss/recon/internal/index"
	"github.com/robertguss/recon/internal/orient"
	"github.com/spf13/cobra"
)

var (
	isInteractive = isInteractiveTTY
	askYesNo      = promptYesNo
	buildOrient   = func(ctx context.Context, conn *sql.DB, moduleRoot string) (orient.Payload, error) {
		return orient.NewService(conn).Build(ctx, orient.BuildOptions{ModuleRoot: moduleRoot, MaxModules: 8, MaxDecisions: 5})
	}
	runOrientSync = func(ctx context.Context, conn *sql.DB, moduleRoot string) error {
		_, err := index.NewService(conn).Sync(ctx, moduleRoot)
		return err
	}
)

func newOrientCommand(app *App) *cobra.Command {
	var (
		jsonOut    bool
		jsonStrict bool
		syncNow    bool
		autoSync   bool
	)

	cmd := &cobra.Command{
		Use:   "orient",
		Short: "Serve startup context for this repository",
		RunE: func(cmd *cobra.Command, args []string) error {
			if jsonStrict {
				jsonOut = true
			}

			conn, err := openExistingDB(app)
			if err != nil {
				if jsonOut {
					return exitJSONCommandError(err)
				}
				return err
			}
			defer conn.Close()

			syncedInRun := false
			if syncNow {
				if err := runOrientSync(cmd.Context(), conn, app.ModuleRoot); err != nil {
					if jsonOut {
						return exitJSONCommandError(err)
					}
					return err
				}
				syncedInRun = true
			}

			payload, err := buildOrient(cmd.Context(), conn, app.ModuleRoot)
			if err != nil {
				if jsonOut {
					return exitJSONCommandError(err)
				}
				return err
			}

			if payload.Freshness.IsStale {
				if autoSync && !syncedInRun {
					if err := runOrientSync(cmd.Context(), conn, app.ModuleRoot); err != nil {
						if jsonOut {
							return exitJSONCommandError(err)
						}
						return err
					}
					payload, err = buildOrient(cmd.Context(), conn, app.ModuleRoot)
					if err != nil {
						if jsonOut {
							return exitJSONCommandError(err)
						}
						return err
					}
				} else if !jsonOut && !app.NoPrompt && isInteractive() {
					runSync, err := askYesNo("Index looks stale. Run recon sync now? [Y/n]: ", true)
					if err != nil {
						return fmt.Errorf("read stale prompt: %w", err)
					}
					if runSync {
						if err := runOrientSync(cmd.Context(), conn, app.ModuleRoot); err != nil {
							return err
						}
						payload, err = buildOrient(cmd.Context(), conn, app.ModuleRoot)
						if err != nil {
							return err
						}
					}
				} else if !jsonStrict {
					fmt.Fprintf(os.Stderr, "warning: stale context (%s)\n", payload.Freshness.Reason)
				}
			}

			if jsonOut {
				return writeJSON(payload)
			}

			fmt.Print(orient.RenderText(payload))
			return nil
		},
	}

	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output JSON")
	cmd.Flags().BoolVar(&jsonStrict, "json-strict", false, "Output JSON only (suppresses warnings; implies --json)")
	cmd.Flags().BoolVar(&syncNow, "sync", false, "Run sync before building orient context")
	cmd.Flags().BoolVar(&autoSync, "auto-sync", false, "Automatically run sync when stale instead of prompting")
	return cmd
}
