package cli

import (
	"fmt"

	"github.com/robertguss/recon/internal/db"
	"github.com/spf13/cobra"
)

type statusPayload struct {
	Initialized bool         `json:"initialized"`
	LastSyncAt  string       `json:"last_sync_at,omitempty"`
	Counts      statusCounts `json:"counts"`
}

type statusCounts struct {
	Files             int `json:"files"`
	Symbols           int `json:"symbols"`
	Packages          int `json:"packages"`
	Decisions         int `json:"decisions"`
	DecisionsDrifting int `json:"decisions_drifting"`
	Patterns          int `json:"patterns"`
}

func newStatusCommand(app *App) *cobra.Command {
	var jsonOut bool

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Quick health check for recon state",
		RunE: func(cmd *cobra.Command, args []string) error {
			conn, err := openExistingDB(app)
			if err != nil {
				if jsonOut {
					return exitJSONCommandError(err)
				}
				return err
			}
			defer conn.Close()

			var payload statusPayload
			payload.Initialized = true

			state, exists, err := db.LoadSyncState(cmd.Context(), conn)
			if err != nil {
				if jsonOut {
					return exitJSONCommandError(err)
				}
				return err
			}
			if exists {
				payload.LastSyncAt = state.LastSyncAt.Format("2006-01-02T15:04:05Z07:00")
			}

			ctx := cmd.Context()
			_ = conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM files").Scan(&payload.Counts.Files)
			_ = conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM symbols").Scan(&payload.Counts.Symbols)
			_ = conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM packages").Scan(&payload.Counts.Packages)
			_ = conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM decisions WHERE status = 'active'").Scan(&payload.Counts.Decisions)
			_ = conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM evidence WHERE entity_type = 'decision' AND drift_status != 'ok'").Scan(&payload.Counts.DecisionsDrifting)
			_ = conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM patterns WHERE status = 'active'").Scan(&payload.Counts.Patterns)

			if jsonOut {
				return writeJSON(payload)
			}

			fmt.Printf("Initialized: yes\n")
			if payload.LastSyncAt != "" {
				fmt.Printf("Last sync: %s\n", payload.LastSyncAt)
			} else {
				fmt.Printf("Last sync: never\n")
			}
			fmt.Printf("Files: %d | Symbols: %d | Packages: %d\n",
				payload.Counts.Files, payload.Counts.Symbols, payload.Counts.Packages)
			fmt.Printf("Decisions: %d (%d drifting) | Patterns: %d\n",
				payload.Counts.Decisions, payload.Counts.DecisionsDrifting, payload.Counts.Patterns)
			return nil
		},
	}

	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output JSON")
	return cmd
}
