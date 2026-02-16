package cli

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

var (
	Version = "dev"
	Commit  = "unknown"
)

func newVersionCommand() *cobra.Command {
	var jsonOut bool

	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print recon version information",
		RunE: func(cmd *cobra.Command, args []string) error {
			if jsonOut {
				return writeJSON(map[string]string{
					"version":    Version,
					"commit":     Commit,
					"go_version": runtime.Version(),
				})
			}
			fmt.Printf("recon %s (commit %s, %s)\n", Version, Commit, runtime.Version())
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output JSON")
	return cmd
}
