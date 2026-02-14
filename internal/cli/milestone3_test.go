package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestJSONModeDBErrorsAreEnveloped(t *testing.T) {
	root := setupModuleRoot(t)
	app := &App{Context: context.Background(), ModuleRoot: root}

	tests := []struct {
		name string
		args []string
		cmd  func() interface {
			SetArgs([]string)
			ExecuteContext(context.Context) error
		}
	}{
		{name: "orient", args: []string{"--json"}, cmd: func() interface {
			SetArgs([]string)
			ExecuteContext(context.Context) error
		} {
			return newOrientCommand(app)
		}},
		{name: "find", args: []string{"Alpha", "--json"}, cmd: func() interface {
			SetArgs([]string)
			ExecuteContext(context.Context) error
		} {
			return newFindCommand(app)
		}},
		{name: "recall", args: []string{"Alpha", "--json"}, cmd: func() interface {
			SetArgs([]string)
			ExecuteContext(context.Context) error
		} {
			return newRecallCommand(app)
		}},
		{name: "sync", args: []string{"--json"}, cmd: func() interface {
			SetArgs([]string)
			ExecuteContext(context.Context) error
		} {
			return newSyncCommand(app)
		}},
		{name: "decide", args: []string{"x", "--reasoning", "r", "--evidence-summary", "e", "--check-type", "file_exists", "--check-path", "go.mod", "--json"}, cmd: func() interface {
			SetArgs([]string)
			ExecuteContext(context.Context) error
		} {
			return newDecideCommand(app)
		}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out, _, err := runCommandWithCapture(t, tc.cmd(), tc.args)
			if err == nil {
				t.Fatalf("expected error for %s without initialized DB", tc.name)
			}
			if !strings.Contains(out, `"error"`) || !strings.Contains(out, `"code": "not_initialized"`) {
				t.Fatalf("expected not_initialized JSON envelope, out=%q err=%v", out, err)
			}
		})
	}
}

func TestFindJSONNotFoundSuggestionsArray(t *testing.T) {
	root := setupModuleRoot(t)
	app := &App{Context: context.Background(), ModuleRoot: root}
	if _, _, err := runCommandWithCapture(t, newInitCommand(app), nil); err != nil {
		t.Fatalf("init: %v", err)
	}
	if _, _, err := runCommandWithCapture(t, newSyncCommand(app), nil); err != nil {
		t.Fatalf("sync: %v", err)
	}

	out, _, err := runCommandWithCapture(t, newFindCommand(app), []string{"DefinitelyMissingSymbol", "--json"})
	if err == nil {
		t.Fatal("expected not_found error")
	}
	if !strings.Contains(out, `"suggestions": []`) {
		t.Fatalf("expected suggestions array, out=%q", out)
	}
}

func TestFindRejectsNegativeMaxBodyLines(t *testing.T) {
	root := setupModuleRoot(t)
	app := &App{Context: context.Background(), ModuleRoot: root}

	out, _, err := runCommandWithCapture(t, newFindCommand(app), []string{"Alpha", "--max-body-lines", "-1", "--json"})
	if err == nil {
		t.Fatal("expected invalid_input error")
	}
	if !strings.Contains(out, `"code": "invalid_input"`) || !strings.Contains(out, `"max_body_lines"`) {
		t.Fatalf("expected invalid_input envelope for max-body-lines, out=%q err=%v", out, err)
	}

	_, _, err = runCommandWithCapture(t, newFindCommand(app), []string{"Alpha", "--max-body-lines", "-1"})
	if err == nil || !strings.Contains(err.Error(), "--max-body-lines") {
		t.Fatalf("expected text validation error, got %v", err)
	}
}

func TestFindDependencyPrecision(t *testing.T) {
	root := t.TempDir()
	write := func(path, body string) {
		t.Helper()
		full := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", path, err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	write("go.mod", "module example.com/recon\n")
	write("main.go", `package main

import (
	"time"
)

func GenerateSessionID() string {
	return time.Now().Format(time.RFC3339)
}
`)
	write("internal/formatter/table.go", `package formatter

type TableFormatter struct{}

func (f TableFormatter) Format(value string) string {
	return value
}
`)
	write("internal/formatter/tsv.go", `package formatter

type TSVFormatter struct{}

func (f TSVFormatter) Format(value string) string {
	return value
}
`)

	app := &App{Context: context.Background(), ModuleRoot: root}
	if _, _, err := runCommandWithCapture(t, newInitCommand(app), nil); err != nil {
		t.Fatalf("init: %v", err)
	}
	if _, _, err := runCommandWithCapture(t, newSyncCommand(app), nil); err != nil {
		t.Fatalf("sync: %v", err)
	}

	out, _, err := runCommandWithCapture(t, newFindCommand(app), []string{"GenerateSessionID", "--json"})
	if err != nil {
		t.Fatalf("find GenerateSessionID: %v out=%q", err, out)
	}

	var result struct {
		Dependencies []struct {
			Name string `json:"name"`
		} `json:"dependencies"`
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("unmarshal result: %v\nout=%s", err, out)
	}
	if len(result.Dependencies) != 0 {
		t.Fatalf("expected no in-project Format dependencies, got %+v", result.Dependencies)
	}
}

func TestFindDisambiguationFilters(t *testing.T) {
	root := t.TempDir()
	write := func(path, body string) {
		t.Helper()
		full := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", path, err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	write("go.mod", "module example.com/recon\n")
	write("main.go", `package main

type T struct{}

func (T) Ambig() {}
`)
	write("pkg1/a.go", "package pkg1\n\nfunc Ambig() {}\n")
	write("pkg2/a.go", "package pkg2\n\nfunc Ambig() {}\n")

	app := &App{Context: context.Background(), ModuleRoot: root}
	if _, _, err := runCommandWithCapture(t, newInitCommand(app), nil); err != nil {
		t.Fatalf("init: %v", err)
	}
	if _, _, err := runCommandWithCapture(t, newSyncCommand(app), nil); err != nil {
		t.Fatalf("sync: %v", err)
	}

	out, _, err := runCommandWithCapture(t, newFindCommand(app), []string{"Ambig", "--json"})
	if err == nil || !strings.Contains(out, `"code": "ambiguous"`) {
		t.Fatalf("expected ambiguous without filters, out=%q err=%v", out, err)
	}

	out, _, err = runCommandWithCapture(t, newFindCommand(app), []string{"Ambig", "--package", "pkg1", "--json"})
	if err != nil {
		t.Fatalf("expected package filter to resolve ambiguity, out=%q err=%v", out, err)
	}
	var byPackage struct {
		Symbol struct {
			Package string `json:"package"`
		} `json:"symbol"`
	}
	if err := json.Unmarshal([]byte(out), &byPackage); err != nil {
		t.Fatalf("unmarshal package-filter result: %v", err)
	}
	if byPackage.Symbol.Package != "pkg1" {
		t.Fatalf("expected package pkg1, got %q", byPackage.Symbol.Package)
	}

	out, _, err = runCommandWithCapture(t, newFindCommand(app), []string{"Ambig", "--file", "pkg2/a.go", "--json"})
	if err != nil {
		t.Fatalf("expected file filter to resolve ambiguity, out=%q err=%v", out, err)
	}
	var byFile struct {
		Symbol struct {
			FilePath string `json:"file_path"`
		} `json:"symbol"`
	}
	if err := json.Unmarshal([]byte(out), &byFile); err != nil {
		t.Fatalf("unmarshal file-filter result: %v", err)
	}
	if byFile.Symbol.FilePath != "pkg2/a.go" {
		t.Fatalf("expected file pkg2/a.go, got %q", byFile.Symbol.FilePath)
	}

	out, _, err = runCommandWithCapture(t, newFindCommand(app), []string{"Ambig", "--kind", "method", "--json"})
	if err != nil {
		t.Fatalf("expected kind filter to resolve ambiguity, out=%q err=%v", out, err)
	}
	var byKind struct {
		Symbol struct {
			Kind string `json:"kind"`
		} `json:"symbol"`
	}
	if err := json.Unmarshal([]byte(out), &byKind); err != nil {
		t.Fatalf("unmarshal kind-filter result: %v", err)
	}
	if byKind.Symbol.Kind != "method" {
		t.Fatalf("expected kind method, got %q", byKind.Symbol.Kind)
	}

	out, _, err = runCommandWithCapture(t, newFindCommand(app), []string{"Ambig", "--package", "missing", "--json"})
	if err == nil || !strings.Contains(out, `"code": "not_found"`) || !strings.Contains(out, "provided filters") {
		t.Fatalf("expected filtered not_found envelope, out=%q err=%v", out, err)
	}
}

func TestDecideInvalidInputDoesNotPersist(t *testing.T) {
	root := setupModuleRoot(t)
	app := &App{Context: context.Background(), ModuleRoot: root}
	if _, _, err := runCommandWithCapture(t, newInitCommand(app), nil); err != nil {
		t.Fatalf("init: %v", err)
	}

	out, _, err := runCommandWithCapture(t, newDecideCommand(app), []string{
		"invalid", "--reasoning", "r", "--evidence-summary", "e",
		"--check-type", "nope", "--check-spec", `{}`,
		"--json",
	})
	if err == nil {
		t.Fatal("expected invalid_input error")
	}
	if !strings.Contains(out, `"code": "invalid_input"`) {
		t.Fatalf("expected invalid_input envelope, out=%q err=%v", out, err)
	}
	if strings.Contains(out, "proposal_id") {
		t.Fatalf("did not expect proposal_id for invalid input, out=%q", out)
	}

	conn, err := openExistingDB(app)
	if err != nil {
		t.Fatalf("openExistingDB: %v", err)
	}
	defer conn.Close()

	var proposalCount int
	if err := conn.QueryRow(`SELECT COUNT(*) FROM proposals;`).Scan(&proposalCount); err != nil {
		t.Fatalf("count proposals: %v", err)
	}
	if proposalCount != 0 {
		t.Fatalf("expected zero persisted proposals, got %d", proposalCount)
	}

	var evidenceCount int
	if err := conn.QueryRow(`SELECT COUNT(*) FROM evidence;`).Scan(&evidenceCount); err != nil {
		t.Fatalf("count evidence: %v", err)
	}
	if evidenceCount != 0 {
		t.Fatalf("expected zero persisted evidence rows, got %d", evidenceCount)
	}
}
