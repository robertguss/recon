package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/robertguss/recon/internal/cli"
)

var (
	newRootCommand = cli.NewRootCommand
	stderr         io.Writer = os.Stderr
	exitFn                    = os.Exit
)

func main() {
	exitFn(run())
}

func run() int {
	ctx := context.Background()
	root, err := newRootCommand(ctx)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	if err := root.Execute(); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}
