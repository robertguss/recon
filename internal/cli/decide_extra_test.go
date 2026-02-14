package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildCheckSpec(t *testing.T) {
	spec, err := buildCheckSpec("file_exists", `{"path":"go.mod"}`, "", "", "", "")
	if err != nil || spec != `{"path":"go.mod"}` {
		t.Fatalf("expected raw spec passthrough, spec=%q err=%v", spec, err)
	}

	spec, err = buildCheckSpec("file_exists", "", "go.mod", "", "", "")
	if err != nil || spec != `{"path":"go.mod"}` {
		t.Fatalf("expected file_exists typed spec, spec=%q err=%v", spec, err)
	}

	spec, err = buildCheckSpec("symbol_exists", "", "", "Alpha", "", "")
	if err != nil || spec != `{"name":"Alpha"}` {
		t.Fatalf("expected symbol_exists typed spec, spec=%q err=%v", spec, err)
	}

	spec, err = buildCheckSpec("grep_pattern", "", "", "", "package", "*.go")
	if err != nil || spec != `{"pattern":"package","scope":"*.go"}` {
		t.Fatalf("expected grep_pattern typed spec, spec=%q err=%v", spec, err)
	}

	for _, tc := range []struct {
		name      string
		checkType string
		checkSpec string
		checkPath string
		checkSym  string
		checkPat  string
		checkScp  string
		wantErr   string
	}{
		{
			name:      "conflict raw and typed",
			checkType: "file_exists",
			checkSpec: `{"path":"go.mod"}`,
			checkPath: "go.mod",
			wantErr:   "cannot combine --check-spec",
		},
		{
			name:      "missing all spec inputs",
			checkType: "file_exists",
			wantErr:   "either --check-spec or typed check flags",
		},
		{
			name:      "file exists rejects extra typed fields",
			checkType: "file_exists",
			checkPath: "go.mod",
			checkSym:  "Alpha",
			wantErr:   "file_exists only supports --check-path",
		},
		{
			name:      "file exists requires path",
			checkType: "file_exists",
			checkSym:  "Alpha",
			wantErr:   "--check-path is required",
		},
		{
			name:      "symbol exists requires symbol",
			checkType: "symbol_exists",
			checkScp:  "*.go",
			wantErr:   "--check-symbol is required",
		},
		{
			name:      "symbol exists rejects extra typed fields",
			checkType: "symbol_exists",
			checkSym:  "Alpha",
			checkPath: "go.mod",
			wantErr:   "symbol_exists only supports --check-symbol",
		},
		{
			name:      "grep pattern requires pattern",
			checkType: "grep_pattern",
			checkScp:  "*.go",
			wantErr:   "--check-pattern is required",
		},
		{
			name:      "grep pattern rejects file path",
			checkType: "grep_pattern",
			checkPat:  "package",
			checkPath: "go.mod",
			wantErr:   "grep_pattern supports --check-pattern",
		},
		{
			name:      "unsupported check type",
			checkType: "nope",
			checkPath: "go.mod",
			wantErr:   `unsupported check type "nope"`,
		},
		{
			name:      "unsupported check type with raw spec",
			checkType: "nope",
			checkSpec: `{}`,
			wantErr:   `unsupported check type "nope"`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := buildCheckSpec(tc.checkType, tc.checkSpec, tc.checkPath, tc.checkSym, tc.checkPat, tc.checkScp)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestMarshalCheckSpecError(t *testing.T) {
	_, err := marshalCheckSpec(func() {})
	if err == nil || !strings.Contains(err.Error(), "marshal check spec") {
		t.Fatalf("expected marshal error, got %v", err)
	}
}

func TestClassifyDecideHelpers(t *testing.T) {
	code, details := classifyDecideError("nope", assertErr(`unsupported check type "nope"`))
	if code != "invalid_input" {
		t.Fatalf("expected invalid_input, got %q", code)
	}
	if details == nil {
		t.Fatal("expected invalid_input details")
	}

	code, details = classifyDecideError("file_exists", assertErr("insert proposal: broken"))
	if code != "internal_error" || details != nil {
		t.Fatalf("expected internal_error with nil details, code=%q details=%v", code, details)
	}

	for _, tc := range []struct {
		msg  string
		want string
	}{
		{`unsupported check type "x"`, "invalid_input"},
		{"parse file_exists check spec: bad json", "invalid_input"},
		{"grep_pattern requires spec.pattern", "invalid_input"},
		{"file_exists requires spec path", "invalid_input"},
		{"compile regex pattern: bad", "invalid_input"},
		{"verification failed for unknown reason", "verification_failed"},
	} {
		got := classifyDecideMessage(tc.msg)
		if got != tc.want {
			t.Fatalf("classifyDecideMessage(%q)=%q want=%q", tc.msg, got, tc.want)
		}
	}
}

func TestDecideCommandJSONInternalErrorPaths(t *testing.T) {
	root := setupModuleRoot(t)
	app := &App{Context: context.Background(), ModuleRoot: root}

	out, _, err := runCommandWithCapture(t, newDecideCommand(app), []string{
		"missing db", "--reasoning", "r", "--evidence-summary", "e",
		"--check-type", "file_exists", "--check-path", "go.mod", "--json",
	})
	if err == nil || !strings.Contains(out, `"code": "not_initialized"`) {
		t.Fatalf("expected not_initialized for missing db, out=%q err=%v", out, err)
	}

	brokenRoot := setupModuleRoot(t)
	if err := os.MkdirAll(filepath.Join(brokenRoot, ".recon"), 0o755); err != nil {
		t.Fatalf("mkdir .recon: %v", err)
	}
	if err := os.WriteFile(filepath.Join(brokenRoot, ".recon", "recon.db"), []byte{}, 0o644); err != nil {
		t.Fatalf("seed db file: %v", err)
	}
	brokenApp := &App{Context: context.Background(), ModuleRoot: brokenRoot}
	out, _, err = runCommandWithCapture(t, newDecideCommand(brokenApp), []string{
		"broken db", "--reasoning", "r", "--evidence-summary", "e",
		"--check-type", "file_exists", "--check-spec", `{"path":"go.mod"}`, "--json",
	})
	if err == nil || !strings.Contains(out, `"code": "internal_error"`) {
		t.Fatalf("expected internal_error for service failure, out=%q err=%v", out, err)
	}
}

func TestDecideCommandBuildCheckSpecTextError(t *testing.T) {
	root := setupModuleRoot(t)
	app := &App{Context: context.Background(), ModuleRoot: root}

	_, _, err := runCommandWithCapture(t, newDecideCommand(app), []string{
		"conflict", "--reasoning", "r", "--evidence-summary", "e",
		"--check-type", "file_exists", "--check-spec", `{"path":"go.mod"}`, "--check-path", "go.mod",
	})
	if err == nil || !strings.Contains(err.Error(), "cannot combine --check-spec") {
		t.Fatalf("expected text-mode build spec error, got %v", err)
	}
}

func TestTruncateBody(t *testing.T) {
	if got := truncateBody("a\nb", 0); got != "a\nb" {
		t.Fatalf("maxLines=0 should return full body, got %q", got)
	}
	if got := truncateBody("a\nb", 4); got != "a\nb" {
		t.Fatalf("when line count <= max, got %q", got)
	}
	if got := truncateBody("a\nb\nc", 2); got != "a\nb\n... (truncated)" {
		t.Fatalf("unexpected truncation output: %q", got)
	}
}

func TestFindNotFoundWithoutSuggestions(t *testing.T) {
	root := setupModuleRoot(t)
	app := &App{Context: context.Background(), ModuleRoot: root}
	if _, _, err := runCommandWithCapture(t, newInitCommand(app), nil); err != nil {
		t.Fatalf("init: %v", err)
	}
	if _, _, err := runCommandWithCapture(t, newSyncCommand(app), nil); err != nil {
		t.Fatalf("sync: %v", err)
	}

	out, _, err := runCommandWithCapture(t, newFindCommand(app), []string{"zzz"})
	if err == nil {
		t.Fatal("expected find not_found error")
	}
	if strings.Contains(out, "Suggestions:") {
		t.Fatalf("expected no suggestions list, out=%q", out)
	}
}

func TestFindDefaultErrorTextBranch(t *testing.T) {
	root := setupModuleRoot(t)
	app := &App{Context: context.Background(), ModuleRoot: root}
	if _, _, err := runCommandWithCapture(t, newInitCommand(app), nil); err != nil {
		t.Fatalf("init: %v", err)
	}
	if _, _, err := runCommandWithCapture(t, newSyncCommand(app), nil); err != nil {
		t.Fatalf("sync: %v", err)
	}
	conn, err := openExistingDB(app)
	if err != nil {
		t.Fatalf("openExistingDB: %v", err)
	}
	if _, err := conn.Exec(`DROP TABLE symbols;`); err != nil {
		_ = conn.Close()
		t.Fatalf("drop symbols: %v", err)
	}
	_ = conn.Close()

	if _, _, err := runCommandWithCapture(t, newFindCommand(app), []string{"Alpha"}); err == nil {
		t.Fatal("expected default text find error")
	}
}

type assertErr string

func (e assertErr) Error() string { return string(e) }
