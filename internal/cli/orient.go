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
	var jsonOut bool

	cmd := &cobra.Command{
		Use:   "orient",
		Short: "Serve startup context for this repository",
		RunE: func(cmd *cobra.Command, args []string) error {
			conn, err := openExistingDB(app)
			if err != nil {
				return err
			}
			defer conn.Close()

			payload, err := buildOrient(cmd.Context(), conn, app.ModuleRoot)
			if err != nil {
				return err
			}

			if payload.Freshness.IsStale {
				if isInteractive() {
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
				} else {
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
	return cmd
}
