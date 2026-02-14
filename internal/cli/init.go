package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/robertguss/recon/internal/db"
	"github.com/spf13/cobra"
)

var runMigrations = db.RunMigrations

func newInitCommand(app *App) *cobra.Command {
	var jsonOut bool

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize recon storage in this repository",
		RunE: func(cmd *cobra.Command, args []string) error {
			goModPath := filepath.Join(app.ModuleRoot, "go.mod")
			if _, err := os.Stat(goModPath); err != nil {
				if errors.Is(err, os.ErrNotExist) {
					return fmt.Errorf("go.mod not found at %s; run `recon` from a Go module", app.ModuleRoot)
				}
				return fmt.Errorf("stat go.mod: %w", err)
			}

			if _, err := db.EnsureReconDir(app.ModuleRoot); err != nil {
				return err
			}

			path := db.DBPath(app.ModuleRoot)
			conn, err := db.Open(path)
			if err != nil {
				return err
			}
			defer conn.Close()

			if err := runMigrations(conn); err != nil {
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
