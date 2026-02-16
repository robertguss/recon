package orient

import (
	"context"
	"database/sql"
	"fmt"
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

func TestBuildModuleHeat(t *testing.T) {
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
	if err := os.MkdirAll(filepath.Join(root, "pkg"), 0o755); err != nil {
		t.Fatalf("mkdir pkg: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\nfunc main(){}\n"), 0o644); err != nil {
		t.Fatalf("write main.go: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "pkg", "a.go"), []byte("package pkg\nfunc A(){}\n"), 0o644); err != nil {
		t.Fatalf("write pkg/a.go: %v", err)
	}
	run("init")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Tester")
	run("add", ".")
	run("commit", "-m", "init")

	// Make multiple recent commits touching main.go to make root "hot"
	for i := 0; i < 5; i++ {
		content := fmt.Sprintf("package main\nfunc main(){}\n// change %d\n", i)
		if err := os.WriteFile(filepath.Join(root, "main.go"), []byte(content), 0o644); err != nil {
			t.Fatalf("write main.go change %d: %v", i, err)
		}
		run("add", "main.go")
		run("commit", "-m", fmt.Sprintf("change %d", i))
	}

	conn := setupOrientDB(t, root)
	defer conn.Close()
	if _, err := index.NewService(conn).Sync(context.Background(), root); err != nil {
		t.Fatalf("sync: %v", err)
	}

	payload, err := NewService(conn).Build(context.Background(), BuildOptions{ModuleRoot: root})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	for _, m := range payload.Modules {
		if m.Path == "." && m.Heat != "hot" {
			t.Fatalf("expected root module to be hot, got %s", m.Heat)
		}
	}
}

func TestBuildArchitectureSection(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/recon\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "cmd", "recon"), 0o755); err != nil {
		t.Fatalf("mkdir cmd/recon: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "cmd", "recon", "main.go"), []byte("package main\nfunc main(){}\n"), 0o644); err != nil {
		t.Fatalf("write main.go: %v", err)
	}
	conn := setupOrientDB(t, root)
	defer conn.Close()

	if _, err := index.NewService(conn).Sync(context.Background(), root); err != nil {
		t.Fatalf("sync: %v", err)
	}

	payload, err := NewService(conn).Build(context.Background(), BuildOptions{ModuleRoot: root})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(payload.Architecture.EntryPoints) == 0 {
		t.Fatal("expected entry points")
	}
	found := false
	for _, ep := range payload.Architecture.EntryPoints {
		if strings.Contains(ep, "cmd/recon/main.go") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected cmd/recon/main.go in entry points, got %v", payload.Architecture.EntryPoints)
	}
}

func TestOrientShowsActivePatterns(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/recon\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	conn := setupOrientDB(t, root)
	defer conn.Close()

	// Seed an active pattern
	_, _ = conn.Exec(`INSERT INTO patterns(id,title,description,confidence,status,created_at,updated_at) VALUES (1,'Error wrapping','Use fmt.Errorf with %%w','high','active','2026-01-01T00:00:00Z','2026-01-01T00:00:00Z');`)
	_, _ = conn.Exec(`INSERT INTO evidence(entity_type,entity_id,summary,drift_status) VALUES ('pattern',1,'grep finds %%w usage','ok');`)

	payload, err := NewService(conn).Build(context.Background(), BuildOptions{ModuleRoot: root})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(payload.ActivePatterns) == 0 {
		t.Fatal("expected active patterns in orient output")
	}
	if payload.ActivePatterns[0].Title != "Error wrapping" {
		t.Fatalf("expected 'Error wrapping', got %q", payload.ActivePatterns[0].Title)
	}

	// Verify text rendering includes patterns
	text := RenderText(payload)
	if !strings.Contains(text, "Error wrapping") {
		t.Fatalf("expected text to contain pattern title, got:\n%s", text)
	}
}

func TestRenderTextAllSections(t *testing.T) {
	payload := Payload{
		Project:      ProjectInfo{Name: "proj", Language: "go", ModulePath: "example.com/proj"},
		Architecture: Architecture{EntryPoints: []string{"cmd/main.go"}, DependencyFlow: []DependencyEdge{{From: "cmd", To: []string{"pkg"}}}},
		Freshness:    Freshness{IsStale: true, Reason: "stale", LastSyncAt: "2026-01-01T00:00:00Z"},
		Summary:      Summary{FileCount: 1, SymbolCount: 2, PackageCount: 1, DecisionCount: 1},
		Modules:      []ModuleSummary{{Path: "cmd", Name: "cmd"}, {Path: "pkg", Name: "pkg"}},
		ActiveDecisions: []DecisionDigest{
			{ID: 1, Title: "d1", Confidence: "high", Drift: "ok", UpdatedAt: "2026-01-01T00:00:00Z"},
		},
		ActivePatterns: []PatternDigest{
			{ID: 1, Title: "p1", Confidence: "medium", Drift: "ok"},
		},
		RecentActivity: []RecentFile{
			{File: "main.go", LastModified: "2026-01-01T00:00:00Z"},
		},
		Warnings: []string{"something is wrong"},
	}
	text := RenderText(payload)
	for _, want := range []string{
		"Entry points: cmd/main.go",
		"Dependency flow: cmd → pkg",
		"STALE CONTEXT: stale",
		"Last sync: 2026-01-01T00:00:00Z",
		"- cmd (cmd)",
		"- #1 d1",
		"Active patterns:",
		"- #1 p1",
		"Recent activity:",
		"- main.go",
		"Warnings:",
		"- something is wrong",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected %q in text, got:\n%s", want, text)
		}
	}
}

func TestRenderDependencyFlowEdges(t *testing.T) {
	// Empty edges should not render dependency flow
	payload := Payload{
		Architecture: Architecture{
			DependencyFlow: []DependencyEdge{},
		},
	}
	text := RenderText(payload)
	if strings.Contains(text, "Dependency flow") {
		t.Fatal("expected no dependency flow for empty edges")
	}

	// Single edge (with matching modules)
	payload.Modules = []ModuleSummary{{Path: "cmd", Name: "cmd"}, {Path: "pkg", Name: "pkg"}}
	payload.Architecture.DependencyFlow = []DependencyEdge{{From: "cmd", To: []string{"pkg"}}}
	text = RenderText(payload)
	if !strings.Contains(text, "Dependency flow:") || !strings.Contains(text, "pkg") {
		t.Fatalf("expected single dep flow in text, got:\n%s", text)
	}

	// Multi dep
	payload.Modules = []ModuleSummary{{Path: "cmd", Name: "cmd"}, {Path: "pkg1", Name: "pkg1"}, {Path: "pkg2", Name: "pkg2"}}
	payload.Architecture.DependencyFlow = []DependencyEdge{{From: "cmd", To: []string{"pkg1", "pkg2"}}}
	text = RenderText(payload)
	if !strings.Contains(text, "pkg1") || !strings.Contains(text, "pkg2") {
		t.Fatalf("expected multi dep flow in text, got:\n%s", text)
	}

	// Edges without matching modules show count
	payload.Modules = []ModuleSummary{}
	payload.Architecture.DependencyFlow = []DependencyEdge{{From: "a", To: []string{"b"}}}
	text = RenderText(payload)
	if !strings.Contains(text, "1 edges (none between top modules)") {
		t.Fatalf("expected fallback count message, got:\n%s", text)
	}
}

func TestBuild_DependencyFlowIsStructured(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/recon\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	conn := setupOrientDB(t, root)
	defer conn.Close()

	// Seed packages and imports directly to ensure dependency edges exist
	now := "2026-01-01T00:00:00Z"
	conn.Exec(`INSERT INTO packages(id,path,name,import_path,file_count,line_count,created_at,updated_at) VALUES (1,'cmd/recon','main','example.com/recon/cmd/recon',1,10,?,?)`, now, now)
	conn.Exec(`INSERT INTO packages(id,path,name,import_path,file_count,line_count,created_at,updated_at) VALUES (2,'pkg','pkg','example.com/recon/pkg',1,5,?,?)`, now, now)
	conn.Exec(`INSERT INTO files(id,package_id,path,language,lines,hash,created_at,updated_at) VALUES (1,1,'cmd/recon/main.go','go',10,'h1',?,?)`, now, now)
	conn.Exec(`INSERT INTO files(id,package_id,path,language,lines,hash,created_at,updated_at) VALUES (2,2,'pkg/pkg.go','go',5,'h2',?,?)`, now, now)
	conn.Exec(`INSERT INTO imports(from_file_id,to_path,to_package_id,alias,import_type) VALUES (1,'example.com/recon/pkg',2,'pkg','local')`)

	payload, err := NewService(conn).Build(context.Background(), BuildOptions{ModuleRoot: root})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	if len(payload.Architecture.DependencyFlow) == 0 {
		t.Fatal("expected structured dependency flow")
	}
	edge := payload.Architecture.DependencyFlow[0]
	if edge.From == "" || len(edge.To) == 0 {
		t.Fatal("expected non-empty from and to fields")
	}
}

func TestBuildLoadPatternsError(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/recon\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	conn := setupOrientDB(t, root)
	defer conn.Close()

	// Create tables needed for summary, modules, decisions, but make patterns table broken
	_, _ = conn.Exec(`DROP TABLE IF EXISTS patterns;`)
	if _, err := NewService(conn).Build(context.Background(), BuildOptions{ModuleRoot: root}); err == nil || !strings.Contains(err.Error(), "query patterns") {
		t.Fatalf("expected patterns error, got %v", err)
	}
}

func TestBuildRecentActivityCapsAtFive(t *testing.T) {
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

	// Create 7 unique files, each committed separately
	for i := 0; i < 7; i++ {
		name := fmt.Sprintf("file%d.go", i)
		content := fmt.Sprintf("package main\nfunc F%d(){}\n", i)
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
		run("add", name)
		run("commit", "-m", fmt.Sprintf("add %s", name))
	}

	conn := setupOrientDB(t, root)
	defer conn.Close()
	if _, err := index.NewService(conn).Sync(context.Background(), root); err != nil {
		t.Fatalf("sync: %v", err)
	}

	payload, err := NewService(conn).Build(context.Background(), BuildOptions{ModuleRoot: root})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(payload.RecentActivity) != 5 {
		t.Fatalf("expected exactly 5 recent activity entries, got %d", len(payload.RecentActivity))
	}
}

func TestBuildRecentActivity(t *testing.T) {
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
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write main.go: %v", err)
	}
	run("init")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Tester")
	run("add", ".")
	run("commit", "-m", "init")

	conn := setupOrientDB(t, root)
	defer conn.Close()
	if _, err := index.NewService(conn).Sync(context.Background(), root); err != nil {
		t.Fatalf("sync: %v", err)
	}

	payload, err := NewService(conn).Build(context.Background(), BuildOptions{ModuleRoot: root})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(payload.RecentActivity) == 0 {
		t.Fatal("expected recent activity")
	}
	if payload.RecentActivity[0].File != "main.go" && payload.RecentActivity[0].File != "go.mod" {
		t.Fatalf("unexpected recent activity file: %s", payload.RecentActivity[0].File)
	}
}

func TestBuild_StaleFreshnessIncludesSummary(t *testing.T) {
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

	// Record the first commit as the "sync" commit
	firstCommit := strings.TrimSpace(gitOutput(t, root, "rev-parse", "HEAD"))

	conn := setupOrientDB(t, root)
	defer conn.Close()

	// Make a second commit so HEAD differs from sync commit
	if err := os.WriteFile(filepath.Join(root, "extra.go"), []byte("package main\nfunc Extra(){}\n"), 0o644); err != nil {
		t.Fatalf("write extra.go: %v", err)
	}
	run("add", "extra.go")
	run("commit", "-m", "add extra")

	now := time.Now().UTC()
	_, dirty := index.CurrentGitState(context.Background(), root)
	fp, _, _ := index.CurrentFingerprint(root)
	if err := db.UpsertSyncState(context.Background(), conn, db.SyncState{
		LastSyncAt:       now,
		LastSyncCommit:   firstCommit,
		LastSyncDirty:    dirty,
		IndexFingerprint: fp,
		IndexedFileCount: 1,
	}); err != nil {
		t.Fatalf("UpsertSyncState: %v", err)
	}

	payload, err := NewService(conn).Build(context.Background(), BuildOptions{ModuleRoot: root})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if !payload.Freshness.IsStale {
		t.Fatal("expected stale payload")
	}
	if payload.Freshness.Reason != "git_head_changed_since_last_sync" {
		t.Fatalf("expected git_head_changed_since_last_sync, got %q", payload.Freshness.Reason)
	}
	if payload.Freshness.StaleSummary == "" {
		t.Fatal("expected stale summary when index is stale due to git head change")
	}
	if !strings.Contains(payload.Freshness.StaleSummary, "1 commits") {
		t.Fatalf("expected '1 commits' in summary, got %q", payload.Freshness.StaleSummary)
	}
	if !strings.Contains(payload.Freshness.StaleSummary, "1 files changed") {
		t.Fatalf("expected '1 files changed' in summary, got %q", payload.Freshness.StaleSummary)
	}
}

func TestBuild_ModulesIncludeEdges(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/recon\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	conn := setupOrientDB(t, root)
	defer conn.Close()

	now := "2026-01-01T00:00:00Z"
	// Seed a package
	conn.Exec(`INSERT INTO packages(id,path,name,import_path,file_count,line_count,created_at,updated_at) VALUES (1,'internal/cli','cli','example.com/recon/internal/cli',5,200,?,?)`, now, now)
	// Seed a decision
	conn.Exec(`INSERT INTO decisions(id,title,reasoning,confidence,status,created_at,updated_at) VALUES (1,'ExitError convention','All commands use ExitError','high','active',?,?)`, now, now)
	// Seed an edge: decision:1 -> package:internal/cli (affects)
	conn.Exec(`INSERT INTO edges(from_type,from_id,to_type,to_ref,relation,source,confidence,created_at) VALUES ('decision',1,'package','internal/cli','affects','manual','high',?)`, now)

	payload, err := NewService(conn).Build(context.Background(), BuildOptions{ModuleRoot: root})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	var found bool
	for _, m := range payload.Modules {
		if m.Path == "internal/cli" {
			if len(m.Knowledge) == 0 {
				t.Fatal("expected knowledge entries for internal/cli module")
			}
			if m.Knowledge[0].Type != "decision" || m.Knowledge[0].ID != 1 || m.Knowledge[0].Title != "ExitError convention" {
				t.Fatalf("unexpected knowledge: %+v", m.Knowledge[0])
			}
			found = true
		}
	}
	if !found {
		t.Fatal("expected internal/cli module in payload")
	}

	// Verify text rendering includes the knowledge
	text := RenderText(payload)
	if !strings.Contains(text, "decision #1: ExitError convention") {
		t.Fatalf("expected knowledge in text output, got:\n%s", text)
	}
}

func gitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return string(out)
}
