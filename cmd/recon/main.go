package main

import (
	"context"
	"fmt"
	"os"

	"github.com/robertguss/recon/internal/cli"
)

func main() {
	ctx := context.Background()
	root, err := cli.NewRootCommand(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}
