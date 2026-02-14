package cli

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/robertguss/recon/internal/index"
	"github.com/robertguss/recon/internal/orient"
)

func TestJSONModeInternalErrorsAreEnveloped(t *testing.T) {
	root := setupModuleRoot(t)
	app := &App{Context: context.Background(), ModuleRoot: root}
	if _, _, err := runCommandWithCapture(t, newInitCommand(app), nil); err != nil {
		t.Fatalf("init: %v", err)
	}
	if _, _, err := runCommandWithCapture(t, newSyncCommand(app), nil); err != nil {
		t.Fatalf("sync: %v", err)
	}

	origRunSync := runSync
	origBuildOrient := buildOrient
	origRunOrientSync := runOrientSync
	defer func() {
		runSync = origRunSync
		buildOrient = origBuildOrient
		runOrientSync = origRunOrientSync
	}()

	runSync = func(context.Context, *sql.DB, string) (index.SyncResult, error) {
		return index.SyncResult{}, errors.New("sync exploded")
	}
	out, _, err := runCommandWithCapture(t, newSyncCommand(app), []string{"--json"})
	if err == nil || !strings.Contains(out, `"code": "internal_error"`) {
		t.Fatalf("expected sync internal_error envelope, out=%q err=%v", out, err)
	}

	buildOrient = func(context.Context, *sql.DB, string) (orient.Payload, error) {
		return orient.Payload{}, errors.New("orient exploded")
	}
	out, _, err = runCommandWithCapture(t, newOrientCommand(app), []string{"--json"})
	if err == nil || !strings.Contains(out, `"code": "internal_error"`) {
		t.Fatalf("expected orient internal_error envelope, out=%q err=%v", out, err)
	}

	buildOrient = func(context.Context, *sql.DB, string) (orient.Payload, error) {
		return orient.Payload{Freshness: orient.Freshness{IsStale: true, Reason: "stale"}}, nil
	}
	runOrientSync = func(context.Context, *sql.DB, string) error {
		return errors.New("auto sync exploded")
	}
	out, _, err = runCommandWithCapture(t, newOrientCommand(app), []string{"--auto-sync", "--json"})
	if err == nil || !strings.Contains(out, `"code": "internal_error"`) {
		t.Fatalf("expected orient auto-sync internal_error envelope, out=%q err=%v", out, err)
	}

	conn, err := openExistingDB(app)
	if err != nil {
		t.Fatalf("openExistingDB: %v", err)
	}
	if _, err := conn.Exec(`DROP TABLE decisions;`); err != nil {
		_ = conn.Close()
		t.Fatalf("drop decisions: %v", err)
	}
	_ = conn.Close()

	out, _, err = runCommandWithCapture(t, newRecallCommand(app), []string{"x", "--json"})
	if err == nil || !strings.Contains(out, `"code": "internal_error"`) {
		t.Fatalf("expected recall internal_error envelope, out=%q err=%v", out, err)
	}
}

func TestFindRejectsInvalidKind(t *testing.T) {
	root := setupModuleRoot(t)
	app := &App{Context: context.Background(), ModuleRoot: root}

	out, _, err := runCommandWithCapture(t, newFindCommand(app), []string{"Alpha", "--kind", "bogus", "--json"})
	if err == nil || !strings.Contains(out, `"code": "invalid_input"`) {
		t.Fatalf("expected invalid_input for bad kind in json mode, out=%q err=%v", out, err)
	}

	_, _, err = runCommandWithCapture(t, newFindCommand(app), []string{"Alpha", "--kind", "bogus"})
	if err == nil || !strings.Contains(err.Error(), "--kind must be one of") {
		t.Fatalf("expected text invalid kind error, got %v", err)
	}
}

func TestFindFilteredNotFoundTextOutput(t *testing.T) {
	root := t.TempDir()
	write := func(path, body string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(filepath.Join(root, path)), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", path, err)
		}
		if err := os.WriteFile(filepath.Join(root, path), []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	write("go.mod", "module example.com/recon\n")
	write("main.go", "package main\nfunc Ambig() {}\n")
	write("pkg1/a.go", "package pkg1\nfunc Ambig() {}\n")

	app := &App{Context: context.Background(), ModuleRoot: root}
	if _, _, err := runCommandWithCapture(t, newInitCommand(app), nil); err != nil {
		t.Fatalf("init: %v", err)
	}
	if _, _, err := runCommandWithCapture(t, newSyncCommand(app), nil); err != nil {
		t.Fatalf("sync: %v", err)
	}

	out, _, err := runCommandWithCapture(t, newFindCommand(app), []string{"Ambig", "--package", "missing"})
	if err == nil || !strings.Contains(out, "not found with provided filters") || !strings.Contains(out, "Filter package:") {
		t.Fatalf("expected filtered text output, out=%q err=%v", out, err)
	}
}

func TestFindAmbiguousWithFilterDetails(t *testing.T) {
	root := setupModuleRoot(t)
	app := &App{Context: context.Background(), ModuleRoot: root}
	if _, _, err := runCommandWithCapture(t, newInitCommand(app), nil); err != nil {
		t.Fatalf("init: %v", err)
	}
	if _, _, err := runCommandWithCapture(t, newSyncCommand(app), nil); err != nil {
		t.Fatalf("sync: %v", err)
	}

	out, _, err := runCommandWithCapture(t, newFindCommand(app), []string{"Ambig", "--kind", "func", "--json"})
	if err == nil || !strings.Contains(out, `"code": "ambiguous"`) || !strings.Contains(out, `"kind": "func"`) {
		t.Fatalf("expected ambiguous json output with kind filter details, out=%q err=%v", out, err)
	}

	out, _, err = runCommandWithCapture(t, newFindCommand(app), []string{"Ambig", "--kind", "func"})
	if err == nil || !strings.Contains(out, "Filter kind: func") {
		t.Fatalf("expected text output with kind filter hint, out=%q err=%v", out, err)
	}
}

func TestOrientSyncAndAutoSyncAvoidDoubleSync(t *testing.T) {
	root := setupModuleRoot(t)
	app := &App{Context: context.Background(), ModuleRoot: root}
	if _, _, err := runCommandWithCapture(t, newInitCommand(app), nil); err != nil {
		t.Fatalf("init: %v", err)
	}

	origBuildOrient := buildOrient
	origRunOrientSync := runOrientSync
	defer func() {
		buildOrient = origBuildOrient
		runOrientSync = origRunOrientSync
	}()

	syncCalls := 0
	runOrientSync = func(context.Context, *sql.DB, string) error {
		syncCalls++
		return nil
	}
	buildOrient = func(context.Context, *sql.DB, string) (orient.Payload, error) {
		return orient.Payload{Freshness: orient.Freshness{IsStale: true, Reason: "stale"}}, nil
	}

	out, _, err := runCommandWithCapture(t, newOrientCommand(app), []string{"--sync", "--auto-sync", "--json"})
	if err != nil {
		t.Fatalf("orient --sync --auto-sync --json: %v out=%q", err, out)
	}
	if syncCalls != 1 {
		t.Fatalf("expected one sync call, got %d", syncCalls)
	}
}

func TestOrientTextModeSyncAndAutoSyncErrors(t *testing.T) {
	root := setupModuleRoot(t)
	app := &App{Context: context.Background(), ModuleRoot: root}
	if _, _, err := runCommandWithCapture(t, newInitCommand(app), nil); err != nil {
		t.Fatalf("init: %v", err)
	}

	origBuildOrient := buildOrient
	origRunOrientSync := runOrientSync
	defer func() {
		buildOrient = origBuildOrient
		runOrientSync = origRunOrientSync
	}()

	runOrientSync = func(context.Context, *sql.DB, string) error {
		return errors.New("sync text fail")
	}
	if _, _, err := runCommandWithCapture(t, newOrientCommand(app), []string{"--sync"}); err == nil {
		t.Fatal("expected text-mode --sync error")
	}

	buildOrient = func(context.Context, *sql.DB, string) (orient.Payload, error) {
		return orient.Payload{Freshness: orient.Freshness{IsStale: true, Reason: "stale"}}, nil
	}
	runOrientSync = func(context.Context, *sql.DB, string) error {
		return errors.New("auto sync text fail")
	}
	if _, _, err := runCommandWithCapture(t, newOrientCommand(app), []string{"--auto-sync"}); err == nil {
		t.Fatal("expected text-mode --auto-sync sync error")
	}

	buildCalls := 0
	buildOrient = func(context.Context, *sql.DB, string) (orient.Payload, error) {
		buildCalls++
		if buildCalls == 1 {
			return orient.Payload{Freshness: orient.Freshness{IsStale: true, Reason: "stale"}}, nil
		}
		return orient.Payload{}, errors.New("rebuild text fail")
	}
	runOrientSync = func(context.Context, *sql.DB, string) error {
		return nil
	}
	if _, _, err := runCommandWithCapture(t, newOrientCommand(app), []string{"--auto-sync"}); err == nil {
		t.Fatal("expected text-mode --auto-sync rebuild error")
	}
}
