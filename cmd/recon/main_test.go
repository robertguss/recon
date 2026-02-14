package main

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/robertguss/recon/internal/cli"
	"github.com/spf13/cobra"
)

func TestRunSuccess(t *testing.T) {
	origNewRoot := newRootCommand
	origStderr := stderr
	defer func() {
		newRootCommand = origNewRoot
		stderr = origStderr
	}()

	newRootCommand = func(context.Context) (*cobra.Command, error) {
		return &cobra.Command{Use: "recon"}, nil
	}
	var buf bytes.Buffer
	stderr = &buf

	if code := run(); code != 0 {
		t.Fatalf("run() code = %d, want 0", code)
	}
	if buf.Len() != 0 {
		t.Fatalf("expected no stderr, got %q", buf.String())
	}
}

func TestRunRootConstructionError(t *testing.T) {
	origNewRoot := newRootCommand
	origStderr := stderr
	defer func() {
		newRootCommand = origNewRoot
		stderr = origStderr
	}()

	newRootCommand = func(context.Context) (*cobra.Command, error) {
		return nil, errors.New("boom")
	}
	var buf bytes.Buffer
	stderr = &buf

	if code := run(); code != 1 {
		t.Fatalf("run() code = %d, want 1", code)
	}
	if got := buf.String(); got == "" {
		t.Fatal("expected stderr output")
	}
}

func TestRunExecuteError(t *testing.T) {
	origNewRoot := newRootCommand
	origStderr := stderr
	defer func() {
		newRootCommand = origNewRoot
		stderr = origStderr
	}()

	newRootCommand = func(context.Context) (*cobra.Command, error) {
		cmd := &cobra.Command{Use: "recon", RunE: func(*cobra.Command, []string) error { return errors.New("execute fail") }}
		return cmd, nil
	}
	var buf bytes.Buffer
	stderr = &buf

	if code := run(); code != 1 {
		t.Fatalf("run() code = %d, want 1", code)
	}
	if got := buf.String(); got == "" {
		t.Fatal("expected stderr output")
	}
}

func TestRunExecuteExitError(t *testing.T) {
	origNewRoot := newRootCommand
	origStderr := stderr
	defer func() {
		newRootCommand = origNewRoot
		stderr = origStderr
	}()

	newRootCommand = func(context.Context) (*cobra.Command, error) {
		cmd := &cobra.Command{
			Use: "recon",
			RunE: func(*cobra.Command, []string) error {
				return cli.ExitError{Code: 2}
			},
		}
		return cmd, nil
	}
	var buf bytes.Buffer
	stderr = &buf

	if code := run(); code != 2 {
		t.Fatalf("run() code = %d, want 2", code)
	}
	if got := buf.String(); got != "" {
		t.Fatalf("expected no stderr output, got %q", got)
	}
}

func TestRunExecuteExitErrorWithMessage(t *testing.T) {
	origNewRoot := newRootCommand
	origStderr := stderr
	defer func() {
		newRootCommand = origNewRoot
		stderr = origStderr
	}()

	newRootCommand = func(context.Context) (*cobra.Command, error) {
		cmd := &cobra.Command{
			Use: "recon",
			RunE: func(*cobra.Command, []string) error {
				return cli.ExitError{Code: 3, Message: "intentional"}
			},
		}
		return cmd, nil
	}
	var buf bytes.Buffer
	stderr = &buf

	if code := run(); code != 3 {
		t.Fatalf("run() code = %d, want 3", code)
	}
	if got := buf.String(); got == "" || !bytes.Contains([]byte(got), []byte("intentional")) {
		t.Fatalf("expected stderr message, got %q", got)
	}
}

func TestRunExecuteExitErrorZeroCodeFallback(t *testing.T) {
	origNewRoot := newRootCommand
	origStderr := stderr
	defer func() {
		newRootCommand = origNewRoot
		stderr = origStderr
	}()

	newRootCommand = func(context.Context) (*cobra.Command, error) {
		cmd := &cobra.Command{
			Use: "recon",
			RunE: func(*cobra.Command, []string) error {
				return cli.ExitError{Code: 0}
			},
		}
		return cmd, nil
	}
	var buf bytes.Buffer
	stderr = &buf

	if code := run(); code != 1 {
		t.Fatalf("run() code = %d, want 1", code)
	}
	if got := buf.String(); got != "" {
		t.Fatalf("expected no stderr message, got %q", got)
	}
}

func TestMainCallsExit(t *testing.T) {
	origExit := exitFn
	origNewRoot := newRootCommand
	defer func() {
		exitFn = origExit
		newRootCommand = origNewRoot
	}()

	exitCode := -1
	exitFn = func(code int) {
		exitCode = code
	}
	newRootCommand = func(context.Context) (*cobra.Command, error) {
		return &cobra.Command{Use: "recon"}, nil
	}

	main()
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0", exitCode)
	}
}
