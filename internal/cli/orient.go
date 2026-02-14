package cli

import (
	"fmt"
	"os"

	"github.com/robertguss/recon/internal/index"
	"github.com/robertguss/recon/internal/orient"
	"github.com/spf13/cobra"
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

			syncSvc := index.NewService(conn)
			orientSvc := orient.NewService(conn)

			payload, err := orientSvc.Build(cmd.Context(), orient.BuildOptions{ModuleRoot: app.ModuleRoot, MaxModules: 8, MaxDecisions: 5})
			if err != nil {
				return err
			}

			if payload.Freshness.IsStale {
				if isInteractiveTTY() {
					runSync, err := promptYesNo("Index looks stale. Run recon sync now? [Y/n]: ", true)
					if err != nil {
						return fmt.Errorf("read stale prompt: %w", err)
					}
					if runSync {
						if _, err := syncSvc.Sync(cmd.Context(), app.ModuleRoot); err != nil {
							return err
						}
						payload, err = orientSvc.Build(cmd.Context(), orient.BuildOptions{ModuleRoot: app.ModuleRoot, MaxModules: 8, MaxDecisions: 5})
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
