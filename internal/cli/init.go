package cli

import (
	"fmt"

	"github.com/robertguss/recon/internal/db"
	"github.com/spf13/cobra"
)

func newInitCommand(app *App) *cobra.Command {
	var jsonOut bool

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize recon storage in this repository",
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, err := db.EnsureReconDir(app.ModuleRoot); err != nil {
				return err
			}

			path := db.DBPath(app.ModuleRoot)
			conn, err := db.Open(path)
			if err != nil {
				return err
			}
			defer conn.Close()

			if err := db.RunMigrations(conn); err != nil {
				return err
			}
			if err := db.EnsureGitIgnore(app.ModuleRoot); err != nil {
				return err
			}

			if jsonOut {
				return writeJSON(map[string]any{
					"ok":          true,
					"module_root": app.ModuleRoot,
					"db_path":     path,
				})
			}

			fmt.Printf("Initialized recon at %s\n", path)
			return nil
		},
	}

	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output JSON")
	return cmd
}
