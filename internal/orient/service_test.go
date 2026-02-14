package orient

import (
	"context"
	"database/sql"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/robertguss/recon/internal/db"
	"github.com/robertguss/recon/internal/index"
)

func setupOrientDB(t *testing.T, root string) *sql.DB {
	t.Helper()
	if _, err := db.EnsureReconDir(root); err != nil {
		t.Fatalf("EnsureReconDir: %v", err)
	}
	conn, err := db.Open(db.DBPath(root))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := db.RunMigrations(conn); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}
	return conn
}

func TestBuildNeverSyncedAndSummary(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/recon\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	conn := setupOrientDB(t, root)
	defer conn.Close()

	svc := NewService(conn)
	payload, err := svc.Build(context.Background(), BuildOptions{ModuleRoot: root})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if !payload.Freshness.IsStale || payload.Freshness.Reason != "never_synced" {
		t.Fatalf("unexpected freshness: %+v", payload.Freshness)
	}
	if payload.Project.ModulePath != "example.com/recon" || payload.Project.Language != "go" {
		t.Fatalf("unexpected project info: %+v", payload.Project)
	}
}

func TestBuildFreshnessModes(t *testing.T) {
	root := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v (%s)", args, err, string(out))
		}
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/recon\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\nfunc main(){}\n"), 0o644); err != nil {
		t.Fatalf("write main.go: %v", err)
	}
	run("init")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Tester")
	run("add", ".")
	run("commit", "-m", "init")
	conn := setupOrientDB(t, root)
	defer conn.Close()

	if _, err := conn.Exec(`INSERT INTO packages(path,name,import_path,file_count,line_count,created_at,updated_at) VALUES ('.','main','example.com/recon',1,1,'x','x');`); err != nil {
		t.Fatalf("seed package: %v", err)
	}
	if _, err := conn.Exec(`INSERT INTO decisions(title,reasoning,confidence,status,created_at,updated_at) VALUES ('d1','r','high','active','x','2026-01-01T00:00:00Z');`); err != nil {
		t.Fatalf("seed decision: %v", err)
	}
	if _, err := conn.Exec(`INSERT INTO evidence(entity_type,entity_id,summary,drift_status) VALUES ('decision',1,'e','drifting');`); err != nil {
		t.Fatalf("seed evidence: %v", err)
	}

	ctx := context.Background()
	commit, dirty := index.CurrentGitState(ctx, root)
	fp, _, err := index.CurrentFingerprint(root)
	if err != nil {
		t.Fatalf("fingerprint: %v", err)
	}
	now := time.Now().UTC()
	if err := db.UpsertSyncState(ctx, conn, db.SyncState{LastSyncAt: now, LastSyncCommit: commit, LastSyncDirty: dirty, IndexFingerprint: fp, IndexedFileCount: 1}); err != nil {
		t.Fatalf("UpsertSyncState: %v", err)
	}

	svc := NewService(conn)
	payload, err := svc.Build(ctx, BuildOptions{ModuleRoot: root, MaxModules: 1, MaxDecisions: 1})
	if err != nil {
		t.Fatalf("Build clean error: %v", err)
	}
	if payload.Freshness.IsStale {
		t.Fatalf("expected fresh payload, got %+v", payload.Freshness)
	}
	if len(payload.Modules) != 1 || len(payload.ActiveDecisions) != 1 {
		t.Fatalf("expected limits applied, got modules=%d decisions=%d", len(payload.Modules), len(payload.ActiveDecisions))
	}

	if err := db.UpsertSyncState(ctx, conn, db.SyncState{LastSyncAt: now, LastSyncCommit: "different", LastSyncDirty: dirty, IndexFingerprint: fp, IndexedFileCount: 1}); err != nil {
		t.Fatalf("UpsertSyncState head mismatch: %v", err)
	}
	payload, err = svc.Build(ctx, BuildOptions{ModuleRoot: root})
	if err != nil {
		t.Fatalf("Build head mismatch error: %v", err)
	}
	if !payload.Freshness.IsStale || payload.Freshness.Reason != "git_head_changed_since_last_sync" {
		t.Fatalf("expected git head stale, got %+v", payload.Freshness)
	}

	if err := db.UpsertSyncState(ctx, conn, db.SyncState{LastSyncAt: now, LastSyncCommit: commit, LastSyncDirty: !dirty, IndexFingerprint: fp, IndexedFileCount: 1}); err != nil {
		t.Fatalf("UpsertSyncState dirty mismatch: %v", err)
	}
	payload, err = svc.Build(ctx, BuildOptions{ModuleRoot: root})
	if err != nil {
		t.Fatalf("Build dirty mismatch error: %v", err)
	}
	if !payload.Freshness.IsStale || payload.Freshness.Reason != "git_dirty_state_changed_since_last_sync" {
		t.Fatalf("expected dirty stale, got %+v", payload.Freshness)
	}

	if err := db.UpsertSyncState(ctx, conn, db.SyncState{LastSyncAt: now, LastSyncCommit: commit, LastSyncDirty: dirty, IndexFingerprint: "wrong", IndexedFileCount: 1}); err != nil {
		t.Fatalf("UpsertSyncState fingerprint mismatch: %v", err)
	}
	payload, err = svc.Build(ctx, BuildOptions{ModuleRoot: root})
	if err != nil {
		t.Fatalf("Build fingerprint mismatch error: %v", err)
	}
	if !payload.Freshness.IsStale || payload.Freshness.Reason != "worktree_fingerprint_changed_since_last_sync" {
		t.Fatalf("expected fingerprint stale, got %+v", payload.Freshness)
	}
}

func TestBuildFingerprintWarningAndErrors(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/recon\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\nfunc main(){}\n"), 0o644); err != nil {
		t.Fatalf("write main.go: %v", err)
	}
	conn := setupOrientDB(t, root)
	defer conn.Close()

	ctx := context.Background()
	commit, dirty := index.CurrentGitState(ctx, root)
	fp, _, err := index.CurrentFingerprint(root)
	if err != nil {
		t.Fatalf("fingerprint: %v", err)
	}
	if err := db.UpsertSyncState(ctx, conn, db.SyncState{LastSyncAt: time.Now().UTC(), LastSyncCommit: commit, LastSyncDirty: dirty, IndexFingerprint: fp, IndexedFileCount: 1}); err != nil {
		t.Fatalf("UpsertSyncState: %v", err)
	}

	if err := os.Chmod(filepath.Join(root, "main.go"), 0o000); err != nil {
		t.Fatalf("chmod unreadable: %v", err)
	}
	defer os.Chmod(filepath.Join(root, "main.go"), 0o644)

	payload, err := NewService(conn).Build(ctx, BuildOptions{ModuleRoot: root})
	if err != nil {
		t.Fatalf("Build with fingerprint warning error: %v", err)
	}
	if payload.Freshness.IsStale {
		t.Fatalf("expected non-stale on fingerprint warning, got %+v", payload.Freshness)
	}
	if len(payload.Warnings) == 0 || !strings.Contains(payload.Warnings[0], "fingerprint check failed") {
		t.Fatalf("expected fingerprint warning, got %+v", payload.Warnings)
	}

	badRoot := t.TempDir()
	conn2 := setupOrientDB(t, badRoot)
	defer conn2.Close()
	if _, err := NewService(conn2).Build(ctx, BuildOptions{ModuleRoot: badRoot}); err == nil {
		t.Fatal("expected module path error without go.mod")
	}

	dbNoSchema, err := db.Open(filepath.Join(t.TempDir(), "x.db"))
	if err != nil {
		t.Fatalf("open dbNoSchema: %v", err)
	}
	defer dbNoSchema.Close()
	if _, err := NewService(dbNoSchema).Build(ctx, BuildOptions{ModuleRoot: root}); err == nil {
		t.Fatal("expected summary query error for unmigrated db")
	}
}

func TestBuildWithGitRepoHeadBranchExplicit(t *testing.T) {
	root := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v (%s)", args, err, string(out))
		}
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/recon\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	run("init")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Tester")
	run("add", "go.mod")
	run("commit", "-m", "init")

	conn := setupOrientDB(t, root)
	defer conn.Close()
	commit, dirty := index.CurrentGitState(context.Background(), root)
	if commit == "" {
		t.Fatal("expected commit hash")
	}
	if err := db.UpsertSyncState(context.Background(), conn, db.SyncState{LastSyncAt: time.Now().UTC(), LastSyncCommit: "old", LastSyncDirty: dirty, IndexFingerprint: "x"}); err != nil {
		t.Fatalf("upsert sync state: %v", err)
	}
	payload, err := NewService(conn).Build(context.Background(), BuildOptions{ModuleRoot: root})
	if err != nil {
		t.Fatalf("Build git head mismatch: %v", err)
	}
	if payload.Freshness.Reason != "git_head_changed_since_last_sync" {
		t.Fatalf("expected head change stale reason, got %+v", payload.Freshness)
	}
}
