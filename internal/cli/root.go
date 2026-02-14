package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/robertguss/recon/internal/index"
	"github.com/spf13/cobra"
)

var (
	osGetwd        = os.Getwd
	findModuleRoot = index.FindModuleRoot
)

type App struct {
	Context    context.Context
	ModuleRoot string
}

func NewRootCommand(ctx context.Context) (*cobra.Command, error) {
	cwd, err := osGetwd()
	if err != nil {
		return nil, fmt.Errorf("resolve cwd: %w", err)
	}

	moduleRoot, err := findModuleRoot(cwd)
	if err != nil {
		moduleRoot = cwd
	}

	app := &App{Context: ctx, ModuleRoot: moduleRoot}

	root := &cobra.Command{
		Use:           "recon",
		Short:         "Recon is a code intelligence and knowledge CLI for Go repositories",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.AddCommand(newInitCommand(app))
	root.AddCommand(newSyncCommand(app))
	root.AddCommand(newOrientCommand(app))
	root.AddCommand(newFindCommand(app))
	root.AddCommand(newDecideCommand(app))
	root.AddCommand(newRecallCommand(app))

	return root, nil
}
