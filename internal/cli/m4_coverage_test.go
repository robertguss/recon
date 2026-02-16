package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// initAndSync creates a temp module root, inits the DB, syncs, and returns
// the root path and App. The caller can pass extra file specs as alternating
// (relative-path, content) strings that are written before sync.
func m4Setup(t *testing.T, extras ...string) (string, *App) {
	t.Helper()
	root := setupModuleRoot(t)
	for i := 0; i < len(extras)-1; i += 2 {
		full := filepath.Join(root, extras[i])
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", extras[i], err)
		}
		if err := os.WriteFile(full, []byte(extras[i+1]), 0o644); err != nil {
			t.Fatalf("write %s: %v", extras[i], err)
		}
	}
	app := &App{Context: context.Background(), ModuleRoot: root}
	if _, _, err := runCommandWithCapture(t, newInitCommand(app), nil); err != nil {
		t.Fatalf("init: %v", err)
	}
	if _, _, err := runCommandWithCapture(t, newSyncCommand(app), nil); err != nil {
		t.Fatalf("sync: %v", err)
	}
	return root, app
}

// m4SetupNoInit returns a temp dir with a go.mod but no .recon directory.
func m4SetupNoInit(t *testing.T) (string, *App) {
	t.Helper()
	root := setupModuleRoot(t)
	app := &App{Context: context.Background(), ModuleRoot: root}
	return root, app
}

// m4SetupBrokenDB returns a setup with a corrupt/empty DB file.
func m4SetupBrokenDB(t *testing.T) (string, *App) {
	t.Helper()
	root := setupModuleRoot(t)
	if err := os.MkdirAll(filepath.Join(root, ".recon"), 0o755); err != nil {
		t.Fatalf("mkdir .recon: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".recon", "recon.db"), []byte{}, 0o644); err != nil {
		t.Fatalf("seed db: %v", err)
	}
	app := &App{Context: context.Background(), ModuleRoot: root}
	return root, app
}

// ---------------------------------------------------------------------------
// pattern.go coverage tests
// ---------------------------------------------------------------------------

func TestM4PatternTextPromoted(t *testing.T) {
	_, app := m4Setup(t)
	out, _, err := runCommandWithCapture(t, newPatternCommand(app), []string{
		"Pattern promoted text",
		"--description", "desc",
		"--evidence-summary", "go.mod exists",
		"--check-type", "file_exists",
		"--check-path", "go.mod",
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !strings.Contains(out, "Pattern promoted") {
		t.Fatalf("expected 'Pattern promoted', out=%q", out)
	}
	if !strings.Contains(out, "Verification: passed=true") {
		t.Fatalf("expected verification line, out=%q", out)
	}
}

func TestM4PatternTextNotPromoted(t *testing.T) {
	_, app := m4Setup(t)
	out, _, err := runCommandWithCapture(t, newPatternCommand(app), []string{
		"Pattern pending text",
		"--description", "desc",
		"--evidence-summary", "missing file",
		"--check-type", "file_exists",
		"--check-path", "nonexistent_file.txt",
	})
	if err == nil {
		t.Fatal("expected exit code 2 for failed verification")
	}
	if !strings.Contains(out, "Pattern pending") {
		t.Fatalf("expected 'Pattern pending', out=%q", out)
	}
	if !strings.Contains(out, "Verification: passed=false") {
		t.Fatalf("expected verification failed line, out=%q", out)
	}
}

func TestM4PatternJSONVerifyFailed(t *testing.T) {
	_, app := m4Setup(t)
	out, _, err := runCommandWithCapture(t, newPatternCommand(app), []string{
		"Pattern verify fail json",
		"--description", "desc",
		"--evidence-summary", "missing file",
		"--check-type", "file_exists",
		"--check-path", "nonexistent_file.txt",
		"--json",
	})
	if err == nil {
		t.Fatal("expected exit code 2 for failed verification")
	}
	if !strings.Contains(out, `"code": "verification_failed"`) {
		t.Fatalf("expected verification_failed envelope, out=%q", out)
	}
}

func TestM4PatternJSONInternalError(t *testing.T) {
	_, app := m4SetupBrokenDB(t)
	out, _, err := runCommandWithCapture(t, newPatternCommand(app), []string{
		"Pattern internal err",
		"--description", "desc",
		"--evidence-summary", "e",
		"--check-type", "file_exists",
		"--check-path", "go.mod",
		"--json",
	})
	if err == nil {
		t.Fatal("expected error for broken db")
	}
	if !strings.Contains(out, `"code"`) {
		t.Fatalf("expected JSON error envelope, out=%q", out)
	}
}

func TestM4PatternTextBuildCheckSpecError(t *testing.T) {
	_, app := m4Setup(t)
	_, _, err := runCommandWithCapture(t, newPatternCommand(app), []string{
		"Pattern bad spec text",
		"--description", "desc",
		"--evidence-summary", "e",
		"--check-type", "file_exists",
		"--check-path", "go.mod",
		"--check-spec", `{"path":"go.mod"}`,
	})
	if err == nil || !strings.Contains(err.Error(), "cannot combine --check-spec") {
		t.Fatalf("expected buildCheckSpec error, got %v", err)
	}
}

func TestM4PatternJSONBuildCheckSpecError(t *testing.T) {
	_, app := m4Setup(t)
	out, _, err := runCommandWithCapture(t, newPatternCommand(app), []string{
		"Pattern bad spec json",
		"--description", "desc",
		"--evidence-summary", "e",
		"--check-type", "file_exists",
		"--check-path", "go.mod",
		"--check-spec", `{"path":"go.mod"}`,
		"--json",
	})
	if err == nil {
		t.Fatal("expected error for spec conflict")
	}
	if !strings.Contains(out, `"code": "invalid_input"`) {
		t.Fatalf("expected invalid_input envelope, out=%q", out)
	}
}

func TestM4PatternTextNoDBError(t *testing.T) {
	_, app := m4SetupNoInit(t)
	_, _, err := runCommandWithCapture(t, newPatternCommand(app), []string{
		"Pattern no db",
		"--description", "desc",
		"--evidence-summary", "e",
		"--check-type", "file_exists",
		"--check-path", "go.mod",
	})
	if err == nil {
		t.Fatal("expected error for missing db")
	}
	if !strings.Contains(err.Error(), "run `recon init` first") {
		t.Fatalf("expected init error, got %v", err)
	}
}

func TestM4PatternJSONNoDBError(t *testing.T) {
	_, app := m4SetupNoInit(t)
	out, _, err := runCommandWithCapture(t, newPatternCommand(app), []string{
		"Pattern no db json",
		"--description", "desc",
		"--evidence-summary", "e",
		"--check-type", "file_exists",
		"--check-path", "go.mod",
		"--json",
	})
	if err == nil {
		t.Fatal("expected error for missing db")
	}
	if !strings.Contains(out, `"code": "not_initialized"`) {
		t.Fatalf("expected not_initialized envelope, out=%q", out)
	}
}

func TestM4PatternTextInternalError(t *testing.T) {
	_, app := m4SetupBrokenDB(t)
	_, _, err := runCommandWithCapture(t, newPatternCommand(app), []string{
		"Pattern internal err text",
		"--description", "desc",
		"--evidence-summary", "e",
		"--check-type", "file_exists",
		"--check-path", "go.mod",
	})
	if err == nil {
		t.Fatal("expected error for broken db")
	}
}

// ---------------------------------------------------------------------------
// decide.go coverage tests
// ---------------------------------------------------------------------------

func TestM4DecideDeleteTextSuccess(t *testing.T) {
	_, app := m4Setup(t)
	// Create a decision first
	if _, _, err := runCommandWithCapture(t, newDecideCommand(app), []string{
		"Delete me", "--reasoning", "r", "--evidence-summary", "e",
		"--check-type", "file_exists", "--check-path", "go.mod",
	}); err != nil {
		t.Fatalf("create decision: %v", err)
	}

	out, _, err := runCommandWithCapture(t, newDecideCommand(app), []string{"--delete", "1"})
	if err != nil {
		t.Fatalf("expected delete success, got %v", err)
	}
	if !strings.Contains(out, "Decision 1 archived.") {
		t.Fatalf("expected archived message, out=%q", out)
	}
}

func TestM4DecideDeleteTextNotFound(t *testing.T) {
	_, app := m4Setup(t)
	_, _, err := runCommandWithCapture(t, newDecideCommand(app), []string{"--delete", "999"})
	if err == nil {
		t.Fatal("expected error for deleting non-existent decision")
	}
}

func TestM4DecideUpdateTextSuccess(t *testing.T) {
	_, app := m4Setup(t)
	// Create a decision first
	if _, _, err := runCommandWithCapture(t, newDecideCommand(app), []string{
		"Update me", "--reasoning", "r", "--evidence-summary", "e",
		"--check-type", "file_exists", "--check-path", "go.mod",
	}); err != nil {
		t.Fatalf("create decision: %v", err)
	}

	out, _, err := runCommandWithCapture(t, newDecideCommand(app), []string{
		"--update", "1", "--confidence", "high",
	})
	if err != nil {
		t.Fatalf("expected update success, got %v", err)
	}
	if !strings.Contains(out, "Decision 1 confidence updated to high.") {
		t.Fatalf("expected update message, out=%q", out)
	}
}

func TestM4DecideUpdateTextError(t *testing.T) {
	_, app := m4Setup(t)
	_, _, err := runCommandWithCapture(t, newDecideCommand(app), []string{
		"--update", "999", "--confidence", "high",
	})
	if err == nil {
		t.Fatal("expected error for updating non-existent decision")
	}
}

func TestM4DecideDryRunTextPassed(t *testing.T) {
	_, app := m4Setup(t)
	out, _, err := runCommandWithCapture(t, newDecideCommand(app), []string{
		"--dry-run",
		"--check-type", "file_exists", "--check-path", "go.mod",
		"--evidence-summary", "e",
	})
	if err != nil {
		t.Fatalf("expected dry-run pass, got %v", err)
	}
	if !strings.Contains(out, "Dry run: passed") {
		t.Fatalf("expected dry run passed text, out=%q", out)
	}
}

func TestM4DecideDryRunTextFailed(t *testing.T) {
	_, app := m4Setup(t)
	out, _, err := runCommandWithCapture(t, newDecideCommand(app), []string{
		"--dry-run",
		"--check-type", "file_exists", "--check-path", "nonexistent.txt",
		"--evidence-summary", "e",
	})
	if err == nil {
		t.Fatal("expected exit code 2 for failed dry run")
	}
	if !strings.Contains(out, "Dry run: failed") {
		t.Fatalf("expected dry run failed text, out=%q", out)
	}
}

func TestM4DecideDryRunJSONPassed(t *testing.T) {
	_, app := m4Setup(t)
	out, _, err := runCommandWithCapture(t, newDecideCommand(app), []string{
		"--dry-run",
		"--check-type", "file_exists", "--check-path", "go.mod",
		"--evidence-summary", "e",
		"--json",
	})
	if err != nil {
		t.Fatalf("expected dry-run pass, got %v", err)
	}
	if !strings.Contains(out, `"passed": true`) {
		t.Fatalf("expected passed true, out=%q", out)
	}
}

func TestM4DecideDryRunJSONFailed(t *testing.T) {
	_, app := m4Setup(t)
	out, _, err := runCommandWithCapture(t, newDecideCommand(app), []string{
		"--dry-run",
		"--check-type", "file_exists", "--check-path", "nonexistent.txt",
		"--evidence-summary", "e",
		"--json",
	})
	if err == nil {
		t.Fatal("expected exit code 2 for failed dry run")
	}
	if !strings.Contains(out, `"code": "verification_failed"`) {
		t.Fatalf("expected verification_failed envelope, out=%q", out)
	}
}

func TestM4DecideDryRunJSONBuildCheckSpecError(t *testing.T) {
	_, app := m4Setup(t)
	out, _, err := runCommandWithCapture(t, newDecideCommand(app), []string{
		"--dry-run",
		"--check-type", "file_exists",
		"--check-path", "go.mod",
		"--check-spec", `{"path":"go.mod"}`,
		"--evidence-summary", "e",
		"--json",
	})
	if err == nil {
		t.Fatal("expected error for spec conflict")
	}
	if !strings.Contains(out, `"code": "invalid_input"`) {
		t.Fatalf("expected invalid_input envelope, out=%q", out)
	}
}

func TestM4DecideDryRunTextBuildCheckSpecError(t *testing.T) {
	_, app := m4Setup(t)
	_, _, err := runCommandWithCapture(t, newDecideCommand(app), []string{
		"--dry-run",
		"--check-type", "file_exists",
		"--check-path", "go.mod",
		"--check-spec", `{"path":"go.mod"}`,
		"--evidence-summary", "e",
	})
	if err == nil || !strings.Contains(err.Error(), "cannot combine --check-spec") {
		t.Fatalf("expected buildCheckSpec error, got %v", err)
	}
}

func TestM4DecideDryRunNoDBText(t *testing.T) {
	_, app := m4SetupNoInit(t)
	_, _, err := runCommandWithCapture(t, newDecideCommand(app), []string{
		"--dry-run",
		"--check-type", "file_exists", "--check-path", "go.mod",
		"--evidence-summary", "e",
	})
	if err == nil {
		t.Fatal("expected error for missing db")
	}
}

func TestM4DecideDryRunNoDBJSON(t *testing.T) {
	_, app := m4SetupNoInit(t)
	out, _, err := runCommandWithCapture(t, newDecideCommand(app), []string{
		"--dry-run",
		"--check-type", "file_exists", "--check-path", "go.mod",
		"--evidence-summary", "e",
		"--json",
	})
	if err == nil {
		t.Fatal("expected error for missing db")
	}
	if !strings.Contains(out, `"code": "not_initialized"`) {
		t.Fatalf("expected not_initialized envelope, out=%q", out)
	}
}

func TestM4DecideTextPromoted(t *testing.T) {
	_, app := m4Setup(t)
	out, _, err := runCommandWithCapture(t, newDecideCommand(app), []string{
		"Promoted decision text", "--reasoning", "r", "--evidence-summary", "e",
		"--check-type", "file_exists", "--check-path", "go.mod",
	})
	if err != nil {
		t.Fatalf("expected promoted success, got %v", err)
	}
	if !strings.Contains(out, "Decision promoted") {
		t.Fatalf("expected 'Decision promoted', out=%q", out)
	}
	if !strings.Contains(out, "Verification: passed=true") {
		t.Fatalf("expected verification passed, out=%q", out)
	}
}

func TestM4DecideTextNotPromoted(t *testing.T) {
	_, app := m4Setup(t)
	out, _, err := runCommandWithCapture(t, newDecideCommand(app), []string{
		"Not promoted text", "--reasoning", "r", "--evidence-summary", "e",
		"--check-type", "file_exists", "--check-path", "nonexistent_file.txt",
	})
	if err == nil {
		t.Fatal("expected exit code 2 for failed verification")
	}
	if !strings.Contains(out, "Decision pending") {
		t.Fatalf("expected 'Decision pending', out=%q", out)
	}
	if !strings.Contains(out, "Verification: passed=false") {
		t.Fatalf("expected verification failed, out=%q", out)
	}
}

func TestM4DecideMissingTitleText(t *testing.T) {
	_, app := m4Setup(t)
	_, _, err := runCommandWithCapture(t, newDecideCommand(app), []string{
		"--reasoning", "r", "--evidence-summary", "e",
		"--check-type", "file_exists", "--check-path", "go.mod",
	})
	if err == nil {
		t.Fatal("expected error for missing title")
	}
	var exitErr ExitError
	if ee, ok := err.(ExitError); ok {
		exitErr = ee
	}
	if exitErr.Code != 2 {
		t.Fatalf("expected exit code 2, got %d", exitErr.Code)
	}
}

func TestM4DecideMissingTitleJSON(t *testing.T) {
	_, app := m4Setup(t)
	out, _, err := runCommandWithCapture(t, newDecideCommand(app), []string{
		"--reasoning", "r", "--evidence-summary", "e",
		"--check-type", "file_exists", "--check-path", "go.mod",
		"--json",
	})
	if err == nil {
		t.Fatal("expected error for missing title")
	}
	if !strings.Contains(out, `"code": "missing_argument"`) {
		t.Fatalf("expected missing_argument envelope, out=%q", out)
	}
}

func TestM4DecideListTextWithItems(t *testing.T) {
	_, app := m4Setup(t)
	// Create a decision
	if _, _, err := runCommandWithCapture(t, newDecideCommand(app), []string{
		"Listed decision", "--reasoning", "r", "--evidence-summary", "e",
		"--check-type", "file_exists", "--check-path", "go.mod",
	}); err != nil {
		t.Fatalf("create decision: %v", err)
	}

	out, _, err := runCommandWithCapture(t, newDecideCommand(app), []string{"--list"})
	if err != nil {
		t.Fatalf("expected list success, got %v", err)
	}
	if !strings.Contains(out, "Listed decision") {
		t.Fatalf("expected decision in list, out=%q", out)
	}
	if !strings.Contains(out, "#") {
		t.Fatalf("expected # prefix in list, out=%q", out)
	}
}

func TestM4DecideListTextNoItems(t *testing.T) {
	_, app := m4Setup(t)
	out, _, err := runCommandWithCapture(t, newDecideCommand(app), []string{"--list"})
	if err != nil {
		t.Fatalf("expected list success, got %v", err)
	}
	if !strings.Contains(out, "No active decisions.") {
		t.Fatalf("expected empty list message, out=%q", out)
	}
}

func TestM4DecideListNoDBText(t *testing.T) {
	_, app := m4SetupNoInit(t)
	_, _, err := runCommandWithCapture(t, newDecideCommand(app), []string{"--list"})
	if err == nil {
		t.Fatal("expected error for missing db")
	}
}

func TestM4DecideListNoDBJSON(t *testing.T) {
	_, app := m4SetupNoInit(t)
	out, _, err := runCommandWithCapture(t, newDecideCommand(app), []string{"--list", "--json"})
	if err == nil {
		t.Fatal("expected error for missing db")
	}
	if !strings.Contains(out, `"code": "not_initialized"`) {
		t.Fatalf("expected not_initialized envelope, out=%q", out)
	}
}

func TestM4DecideDeleteNoDBText(t *testing.T) {
	_, app := m4SetupNoInit(t)
	_, _, err := runCommandWithCapture(t, newDecideCommand(app), []string{"--delete", "1"})
	if err == nil {
		t.Fatal("expected error for missing db")
	}
}

func TestM4DecideDeleteNoDBJSON(t *testing.T) {
	_, app := m4SetupNoInit(t)
	out, _, err := runCommandWithCapture(t, newDecideCommand(app), []string{"--delete", "1", "--json"})
	if err == nil {
		t.Fatal("expected error for missing db")
	}
	if !strings.Contains(out, `"code": "not_initialized"`) {
		t.Fatalf("expected not_initialized envelope, out=%q", out)
	}
}

func TestM4DecideDeleteJSONNotFound(t *testing.T) {
	_, app := m4Setup(t)
	out, _, err := runCommandWithCapture(t, newDecideCommand(app), []string{"--delete", "999", "--json"})
	if err == nil {
		t.Fatal("expected error for deleting non-existent decision")
	}
	if !strings.Contains(out, `"code": "not_found"`) {
		t.Fatalf("expected not_found envelope, out=%q", out)
	}
}

func TestM4DecideUpdateNoDBText(t *testing.T) {
	_, app := m4SetupNoInit(t)
	_, _, err := runCommandWithCapture(t, newDecideCommand(app), []string{
		"--update", "1", "--confidence", "high",
	})
	if err == nil {
		t.Fatal("expected error for missing db")
	}
}

func TestM4DecideUpdateNoDBJSON(t *testing.T) {
	_, app := m4SetupNoInit(t)
	out, _, err := runCommandWithCapture(t, newDecideCommand(app), []string{
		"--update", "1", "--confidence", "high", "--json",
	})
	if err == nil {
		t.Fatal("expected error for missing db")
	}
	if !strings.Contains(out, `"code": "not_initialized"`) {
		t.Fatalf("expected not_initialized envelope, out=%q", out)
	}
}

func TestM4DecideUpdateJSONError(t *testing.T) {
	_, app := m4Setup(t)
	out, _, err := runCommandWithCapture(t, newDecideCommand(app), []string{
		"--update", "999", "--confidence", "high", "--json",
	})
	if err == nil {
		t.Fatal("expected error for updating non-existent decision")
	}
	if !strings.Contains(out, `"code": "not_found"`) {
		t.Fatalf("expected not_found envelope, out=%q", out)
	}
}

func TestM4DecideUpdateJSONInvalidConfidence(t *testing.T) {
	_, app := m4Setup(t)
	out, _, err := runCommandWithCapture(t, newDecideCommand(app), []string{
		"--update", "1", "--confidence", "invalid", "--json",
	})
	if err == nil {
		t.Fatal("expected error for invalid confidence")
	}
	if !strings.Contains(out, `"code": "invalid_input"`) {
		t.Fatalf("expected invalid_input envelope, out=%q", out)
	}
}

func TestM4DecideListInternalErrorJSON(t *testing.T) {
	_, app := m4SetupBrokenDB(t)
	out, _, err := runCommandWithCapture(t, newDecideCommand(app), []string{"--list", "--json"})
	if err == nil {
		t.Fatal("expected error for broken db")
	}
	if !strings.Contains(out, `"code": "internal_error"`) {
		t.Fatalf("expected internal_error envelope, out=%q", out)
	}
}

func TestM4DecideListInternalErrorText(t *testing.T) {
	_, app := m4SetupBrokenDB(t)
	_, _, err := runCommandWithCapture(t, newDecideCommand(app), []string{"--list"})
	if err == nil {
		t.Fatal("expected error for broken db")
	}
}

// ---------------------------------------------------------------------------
// find.go runFindListMode coverage tests
// ---------------------------------------------------------------------------

func TestM4FindListTextWithPackageFilter(t *testing.T) {
	_, app := m4Setup(t)
	out, _, err := runCommandWithCapture(t, newFindCommand(app), []string{
		"--package", "pkg1",
	})
	if err != nil {
		t.Fatalf("expected list success, got %v", err)
	}
	if !strings.Contains(out, "Symbols (") {
		t.Fatalf("expected Symbols header, out=%q", out)
	}
	if !strings.Contains(out, "Ambig") {
		t.Fatalf("expected Ambig in list, out=%q", out)
	}
}

func TestM4FindListTextWithKindFilter(t *testing.T) {
	_, app := m4Setup(t)
	out, _, err := runCommandWithCapture(t, newFindCommand(app), []string{
		"--kind", "func",
	})
	if err != nil {
		t.Fatalf("expected list success, got %v", err)
	}
	if !strings.Contains(out, "Symbols (") {
		t.Fatalf("expected Symbols header, out=%q", out)
	}
}

func TestM4FindListJSONOutput(t *testing.T) {
	_, app := m4Setup(t)
	out, _, err := runCommandWithCapture(t, newFindCommand(app), []string{
		"--package", "pkg1", "--json",
	})
	if err != nil {
		t.Fatalf("expected list success, got %v", err)
	}
	if !strings.Contains(out, `"symbols"`) {
		t.Fatalf("expected symbols key, out=%q", out)
	}
}

func TestM4FindListTextNoDBError(t *testing.T) {
	_, app := m4SetupNoInit(t)
	_, _, err := runCommandWithCapture(t, newFindCommand(app), []string{
		"--package", "pkg1",
	})
	if err == nil {
		t.Fatal("expected error for missing db")
	}
}

func TestM4FindListJSONNoDBError(t *testing.T) {
	_, app := m4SetupNoInit(t)
	out, _, err := runCommandWithCapture(t, newFindCommand(app), []string{
		"--package", "pkg1", "--json",
	})
	if err == nil {
		t.Fatal("expected error for missing db")
	}
	if !strings.Contains(out, `"code": "not_initialized"`) {
		t.Fatalf("expected not_initialized envelope, out=%q", out)
	}
}

func TestM4FindListJSONInternalError(t *testing.T) {
	_, app := m4SetupBrokenDB(t)
	out, _, err := runCommandWithCapture(t, newFindCommand(app), []string{
		"--package", "pkg1", "--json",
	})
	if err == nil {
		t.Fatal("expected error for broken db")
	}
	if !strings.Contains(out, `"code"`) {
		t.Fatalf("expected JSON error envelope, out=%q", out)
	}
}

func TestM4FindListTextInternalError(t *testing.T) {
	_, app := m4SetupBrokenDB(t)
	_, _, err := runCommandWithCapture(t, newFindCommand(app), []string{
		"--package", "pkg1",
	})
	if err == nil {
		t.Fatal("expected error for broken db")
	}
}

func TestM4FindListTextReceiverLabel(t *testing.T) {
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
	write("main.go", "package main\ntype R struct{}\nfunc (r R) Method() {}\n")

	app := &App{Context: context.Background(), ModuleRoot: root}
	if _, _, err := runCommandWithCapture(t, newInitCommand(app), nil); err != nil {
		t.Fatalf("init: %v", err)
	}
	if _, _, err := runCommandWithCapture(t, newSyncCommand(app), nil); err != nil {
		t.Fatalf("sync: %v", err)
	}

	out, _, err := runCommandWithCapture(t, newFindCommand(app), []string{
		"--kind", "method",
	})
	if err != nil {
		t.Fatalf("expected list success, got %v", err)
	}
	if !strings.Contains(out, "R.Method") {
		t.Fatalf("expected receiver.method label, out=%q", out)
	}
}

// ---------------------------------------------------------------------------
// recall.go coverage tests
// ---------------------------------------------------------------------------

func TestM4RecallTextPatternEntityType(t *testing.T) {
	_, app := m4Setup(t)

	// Create a pattern so recall returns a pattern entity type
	if _, _, err := runCommandWithCapture(t, newPatternCommand(app), []string{
		"Recall pattern test",
		"--description", "desc",
		"--evidence-summary", "go.mod exists",
		"--check-type", "file_exists",
		"--check-path", "go.mod",
	}); err != nil {
		t.Fatalf("create pattern: %v", err)
	}

	out, _, err := runCommandWithCapture(t, newRecallCommand(app), []string{"Recall"})
	if err != nil {
		t.Fatalf("expected recall success, got %v", err)
	}
	if !strings.Contains(out, "[pattern]") {
		t.Fatalf("expected [pattern] label in output, out=%q", out)
	}
}

func TestM4RecallTextNoResults(t *testing.T) {
	_, app := m4Setup(t)
	out, _, err := runCommandWithCapture(t, newRecallCommand(app), []string{"zzz_no_match_zzz"})
	if err != nil {
		t.Fatalf("expected recall success, got %v", err)
	}
	if !strings.Contains(out, "No promoted knowledge found.") {
		t.Fatalf("expected no results message, out=%q", out)
	}
}

func TestM4RecallMissingArgText(t *testing.T) {
	_, app := m4Setup(t)
	_, _, err := runCommandWithCapture(t, newRecallCommand(app), []string{})
	if err == nil {
		t.Fatal("expected error for missing argument")
	}
}

func TestM4RecallNoDBText(t *testing.T) {
	_, app := m4SetupNoInit(t)
	_, _, err := runCommandWithCapture(t, newRecallCommand(app), []string{"test"})
	if err == nil {
		t.Fatal("expected error for missing db")
	}
}

// ---------------------------------------------------------------------------
// status.go coverage tests
// ---------------------------------------------------------------------------

func TestM4StatusTextWithSyncState(t *testing.T) {
	_, app := m4Setup(t)
	out, _, err := runCommandWithCapture(t, newStatusCommand(app), nil)
	if err != nil {
		t.Fatalf("expected status success, got %v", err)
	}
	if !strings.Contains(out, "Initialized: yes") {
		t.Fatalf("expected initialized line, out=%q", out)
	}
	if !strings.Contains(out, "Last sync:") {
		t.Fatalf("expected last sync line, out=%q", out)
	}
	// Should show a timestamp, not "never"
	if strings.Contains(out, "Last sync: never") {
		t.Fatalf("expected actual sync time, not 'never', out=%q", out)
	}
	if !strings.Contains(out, "Files:") {
		t.Fatalf("expected Files line, out=%q", out)
	}
	if !strings.Contains(out, "Decisions:") {
		t.Fatalf("expected Decisions line, out=%q", out)
	}
}

func TestM4StatusTextNeverSynced(t *testing.T) {
	root := setupModuleRoot(t)
	app := &App{Context: context.Background(), ModuleRoot: root}
	if _, _, err := runCommandWithCapture(t, newInitCommand(app), nil); err != nil {
		t.Fatalf("init: %v", err)
	}
	// Don't sync - just check status
	out, _, err := runCommandWithCapture(t, newStatusCommand(app), nil)
	if err != nil {
		t.Fatalf("expected status success, got %v", err)
	}
	if !strings.Contains(out, "Last sync: never") {
		t.Fatalf("expected 'Last sync: never', out=%q", out)
	}
}

func TestM4StatusTextNoDBError(t *testing.T) {
	_, app := m4SetupNoInit(t)
	_, _, err := runCommandWithCapture(t, newStatusCommand(app), nil)
	if err == nil {
		t.Fatal("expected error for missing db")
	}
}

func TestM4StatusJSONNoDBError(t *testing.T) {
	_, app := m4SetupNoInit(t)
	out, _, err := runCommandWithCapture(t, newStatusCommand(app), []string{"--json"})
	if err == nil {
		t.Fatal("expected error for missing db")
	}
	if !strings.Contains(out, `"code": "not_initialized"`) {
		t.Fatalf("expected not_initialized envelope, out=%q", out)
	}
}

func TestM4StatusJSONLoadSyncStateError(t *testing.T) {
	// Use a broken DB to trigger a loadSyncState error
	_, app := m4SetupBrokenDB(t)
	out, _, err := runCommandWithCapture(t, newStatusCommand(app), []string{"--json"})
	if err == nil {
		t.Fatal("expected error for broken db")
	}
	if !strings.Contains(out, `"code"`) {
		t.Fatalf("expected JSON error envelope, out=%q", out)
	}
}

func TestM4StatusTextLoadSyncStateError(t *testing.T) {
	_, app := m4SetupBrokenDB(t)
	_, _, err := runCommandWithCapture(t, newStatusCommand(app), nil)
	if err == nil {
		t.Fatal("expected error for broken db")
	}
}

// ---------------------------------------------------------------------------
// buildCheckSpec edge: empty check type with typed flags (line 312 default case)
// ---------------------------------------------------------------------------

func TestM4BuildCheckSpecEmptyTypeWithTypedFlags(t *testing.T) {
	_, err := buildCheckSpec("", "", "go.mod", "", "", "")
	if err == nil || !strings.Contains(err.Error(), "unsupported check type") {
		t.Fatalf("expected unsupported check type error for empty type, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// decide: buildCheckSpec error in propose mode (text)
// ---------------------------------------------------------------------------

func TestM4DecideProposeTextBuildCheckSpecError(t *testing.T) {
	_, app := m4Setup(t)
	_, _, err := runCommandWithCapture(t, newDecideCommand(app), []string{
		"Propose with conflict", "--reasoning", "r", "--evidence-summary", "e",
		"--check-type", "file_exists",
		"--check-path", "go.mod",
		"--check-spec", `{"path":"go.mod"}`,
	})
	if err == nil || !strings.Contains(err.Error(), "cannot combine --check-spec") {
		t.Fatalf("expected buildCheckSpec error, got %v", err)
	}
}

func TestM4DecideProposeNoDBText(t *testing.T) {
	_, app := m4SetupNoInit(t)
	_, _, err := runCommandWithCapture(t, newDecideCommand(app), []string{
		"No db decide", "--reasoning", "r", "--evidence-summary", "e",
		"--check-type", "file_exists", "--check-path", "go.mod",
	})
	if err == nil {
		t.Fatal("expected error for missing db")
	}
}

// ---------------------------------------------------------------------------
// find: missing arg text mode (no filters)
// ---------------------------------------------------------------------------

func TestM4FindMissingArgTextMode(t *testing.T) {
	_, app := m4Setup(t)
	_, _, err := runCommandWithCapture(t, newFindCommand(app), []string{})
	if err == nil {
		t.Fatal("expected error for missing symbol and no filters")
	}
}

func TestM4DecideUpdateTextMissingConfidence(t *testing.T) {
	_, app := m4Setup(t)
	// Create a decision first
	if _, _, err := runCommandWithCapture(t, newDecideCommand(app), []string{
		"Update conf test", "--reasoning", "r", "--evidence-summary", "e",
		"--check-type", "file_exists", "--check-path", "go.mod",
	}); err != nil {
		t.Fatalf("create decision: %v", err)
	}

	// Call --update without --confidence (text mode)
	_, _, err := runCommandWithCapture(t, newDecideCommand(app), []string{
		"--update", "1",
	})
	if err == nil {
		t.Fatal("expected error for missing --confidence")
	}
	ee, ok := err.(ExitError)
	if !ok || ee.Code != 2 {
		t.Fatalf("expected ExitError code 2, got %v", err)
	}
	if !strings.Contains(ee.Message, "--confidence is required") {
		t.Fatalf("expected confidence required message, got %q", ee.Message)
	}
}

func TestM4DecideUpdateJSONMissingConfidence(t *testing.T) {
	_, app := m4Setup(t)
	// Create a decision first
	if _, _, err := runCommandWithCapture(t, newDecideCommand(app), []string{
		"Update conf json test", "--reasoning", "r", "--evidence-summary", "e",
		"--check-type", "file_exists", "--check-path", "go.mod",
	}); err != nil {
		t.Fatalf("create decision: %v", err)
	}

	// Call --update without --confidence (JSON mode)
	out, _, err := runCommandWithCapture(t, newDecideCommand(app), []string{
		"--update", "1", "--json",
	})
	if err == nil {
		t.Fatal("expected error for missing --confidence")
	}
	if !strings.Contains(out, `"code": "missing_argument"`) {
		t.Fatalf("expected missing_argument envelope, out=%q", out)
	}
}

func TestM4FindMissingArgJSONMode(t *testing.T) {
	_, app := m4Setup(t)
	out, _, err := runCommandWithCapture(t, newFindCommand(app), []string{"--json"})
	if err == nil {
		t.Fatal("expected error for missing symbol and no filters")
	}
	if !strings.Contains(out, `"code": "missing_argument"`) {
		t.Fatalf("expected missing_argument envelope, out=%q", out)
	}
}

func TestM4FindJSONIncludesKnowledgeEdges(t *testing.T) {
	_, app := m4Setup(t)

	// Open the DB to seed a decision and edge pointing at pkg1
	conn, err := openExistingDB(app)
	if err != nil {
		t.Fatalf("openExistingDB: %v", err)
	}
	defer conn.Close()

	_, _ = conn.Exec(`INSERT INTO decisions(id,title,reasoning,confidence,status,created_at,updated_at)
		VALUES (1,'Use pkg1 pattern','Because reasons','high','active','2026-01-01T00:00:00Z','2026-01-01T00:00:00Z')`)
	_, _ = conn.Exec(`INSERT INTO edges(from_type,from_id,to_type,to_ref,relation,source,confidence,created_at)
		VALUES ('decision',1,'package','pkg1','affects','manual','high','2026-01-01T00:00:00Z')`)

	out, _, err := runCommandWithCapture(t, newFindCommand(app), []string{"Ambig", "--package", "pkg1", "--json"})
	if err != nil {
		t.Fatalf("find Ambig --json error: %v", err)
	}
	if !strings.Contains(out, `"knowledge"`) {
		t.Fatalf("expected knowledge field in JSON output, out=%q", out)
	}
	if !strings.Contains(out, `"Use pkg1 pattern"`) {
		t.Fatalf("expected decision title in knowledge, out=%q", out)
	}
	if !strings.Contains(out, `"affects"`) {
		t.Fatalf("expected relation in knowledge, out=%q", out)
	}
}
