package cli

import (
	"errors"
	"io"
	"os"
	"testing"
)

func TestClassifyJSONCommandError(t *testing.T) {
	code, details := classifyJSONCommandError(dbNotInitializedError{Path: "/tmp/x.db"})
	if code != "not_initialized" {
		t.Fatalf("expected not_initialized, got %q", code)
	}
	if details == nil {
		t.Fatal("expected details for not_initialized")
	}

	code, details = classifyJSONCommandError(errors.New("boom"))
	if code != "internal_error" || details != nil {
		t.Fatalf("expected internal_error with nil details, code=%q details=%v", code, details)
	}
}

func TestExitJSONCommandError(t *testing.T) {
	origStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w

	retErr := exitJSONCommandError(dbNotInitializedError{Path: "/tmp/test.db"})

	_ = w.Close()
	os.Stdout = origStdout
	data, _ := io.ReadAll(r)
	_ = r.Close()

	exitErr, ok := retErr.(ExitError)
	if !ok || exitErr.Code != 2 {
		t.Fatalf("expected ExitError code 2, got %#v", retErr)
	}
	if string(data) == "" {
		t.Fatal("expected JSON output")
	}
}
