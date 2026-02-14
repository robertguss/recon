package db

import (
	"context"
	"database/sql"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	databasepkg "github.com/golang-migrate/migrate/v4/database"
	"github.com/golang-migrate/migrate/v4/database/sqlite"
	sourcepkg "github.com/golang-migrate/migrate/v4/source"
)

func TestPathsAndEnsureReconDir(t *testing.T) {
	root := t.TempDir()
	wantDir := filepath.Join(root, ".recon")
	if got := ReconDir(root); got != wantDir {
		t.Fatalf("ReconDir() = %q, want %q", got, wantDir)
	}
	wantDB := filepath.Join(wantDir, "recon.db")
	if got := DBPath(root); got != wantDB {
		t.Fatalf("DBPath() = %q, want %q", got, wantDB)
	}

	dir, err := EnsureReconDir(root)
	if err != nil {
		t.Fatalf("EnsureReconDir() error = %v", err)
	}
	if dir != wantDir {
		t.Fatalf("EnsureReconDir() = %q, want %q", dir, wantDir)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("stat recon dir: %v", err)
	}
}

func TestEnsureGitIgnore(t *testing.T) {
	root := t.TempDir()

	if err := EnsureGitIgnore(root); err != nil {
		t.Fatalf("EnsureGitIgnore() first call error = %v", err)
	}
	b, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	if err != nil {
		t.Fatalf("read .gitignore: %v", err)
	}
	if got := string(b); got != ".recon/recon.db\n" {
		t.Fatalf("unexpected .gitignore content: %q", got)
	}

	if err := EnsureGitIgnore(root); err != nil {
		t.Fatalf("EnsureGitIgnore() second call error = %v", err)
	}
	b, err = os.ReadFile(filepath.Join(root, ".gitignore"))
	if err != nil {
		t.Fatalf("read .gitignore after second call: %v", err)
	}
	if strings.Count(string(b), ".recon/recon.db") != 1 {
		t.Fatalf("entry duplicated: %q", string(b))
	}

	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("node_modules"), 0o644); err != nil {
		t.Fatalf("seed .gitignore: %v", err)
	}
	if err := EnsureGitIgnore(root); err != nil {
		t.Fatalf("EnsureGitIgnore() with missing newline error = %v", err)
	}
	b, err = os.ReadFile(filepath.Join(root, ".gitignore"))
	if err != nil {
		t.Fatalf("read .gitignore after append: %v", err)
	}
	if got := string(b); got != "node_modules\n.recon/recon.db\n" {
		t.Fatalf("unexpected newline handling: %q", got)
	}
}

func TestEnsureGitIgnoreReadAndWriteErrors(t *testing.T) {
	fileRoot := filepath.Join(t.TempDir(), "rootfile")
	if err := os.WriteFile(fileRoot, []byte("x"), 0o644); err != nil {
		t.Fatalf("write root file: %v", err)
	}
	if err := EnsureGitIgnore(fileRoot); err == nil || !strings.Contains(err.Error(), "read .gitignore") {
		t.Fatalf("expected read error, got %v", err)
	}

	root := t.TempDir()
	path := filepath.Join(root, ".gitignore")
	if err := os.WriteFile(path, []byte("seed\n"), 0o444); err != nil {
		t.Fatalf("write readonly .gitignore: %v", err)
	}
	if err := EnsureGitIgnore(root); err == nil || !strings.Contains(err.Error(), "write .gitignore") {
		t.Fatalf("expected write error, got %v", err)
	}
}

func TestOpenAndRunMigrationsAndSyncState(t *testing.T) {
	root := t.TempDir()
	if _, err := EnsureReconDir(root); err != nil {
		t.Fatalf("EnsureReconDir: %v", err)
	}
	conn, err := Open(DBPath(root))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer conn.Close()

	if err := RunMigrations(conn); err != nil {
		t.Fatalf("RunMigrations first call: %v", err)
	}
	if err := RunMigrations(conn); err != nil {
		t.Fatalf("RunMigrations second call: %v", err)
	}

	ctx := context.Background()
	if _, ok, err := LoadSyncState(ctx, conn); err != nil || ok {
		t.Fatalf("LoadSyncState before upsert = ok:%v err:%v", ok, err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	state := SyncState{
		LastSyncAt:       now,
		LastSyncCommit:   "abc123",
		LastSyncDirty:    true,
		IndexedFileCount: 4,
		IndexFingerprint: "fingerprint",
	}
	if err := UpsertSyncState(ctx, conn, state); err != nil {
		t.Fatalf("UpsertSyncState: %v", err)
	}

	got, ok, err := LoadSyncState(ctx, conn)
	if err != nil || !ok {
		t.Fatalf("LoadSyncState after upsert = ok:%v err:%v", ok, err)
	}
	if !got.LastSyncAt.Equal(now) || got.LastSyncCommit != state.LastSyncCommit || !got.LastSyncDirty || got.IndexedFileCount != state.IndexedFileCount || got.IndexFingerprint != state.IndexFingerprint {
		t.Fatalf("unexpected sync state: %+v", got)
	}

	if _, err := conn.Exec("UPDATE sync_state SET last_sync_at = 'not-a-time' WHERE id = 1;"); err != nil {
		t.Fatalf("set invalid time: %v", err)
	}
	if _, _, err := LoadSyncState(ctx, conn); err == nil || !strings.Contains(err.Error(), "parse sync timestamp") {
		t.Fatalf("expected parse error, got %v", err)
	}
}

func TestOpenAndMigrationErrors(t *testing.T) {
	dirAsDB := filepath.Join(t.TempDir(), "as-dir")
	if err := os.MkdirAll(dirAsDB, 0o755); err != nil {
		t.Fatalf("mkdir dirAsDB: %v", err)
	}
	if _, err := Open(dirAsDB); err == nil {
		t.Fatal("expected Open to fail for directory path")
	}

	origOpen := sqlOpen
	sqlOpen = func(string, string) (*sql.DB, error) {
		return nil, errors.New("open fail")
	}
	if _, err := Open("ignored"); err == nil || !strings.Contains(err.Error(), "open sqlite db") {
		t.Fatalf("expected wrapped open error, got %v", err)
	}
	sqlOpen = origOpen

	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("expected RunMigrations(nil) to panic")
			}
		}()
		_ = RunMigrations(nil)
	}()

	root := t.TempDir()
	if _, err := EnsureReconDir(root); err != nil {
		t.Fatalf("EnsureReconDir: %v", err)
	}
	conn, err := Open(DBPath(root))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}
	ctx := context.Background()
	state := SyncState{LastSyncAt: time.Now(), IndexFingerprint: "x"}
	if err := UpsertSyncState(ctx, conn, state); err == nil {
		t.Fatal("expected UpsertSyncState on closed db to fail")
	}
	if _, _, err := LoadSyncState(ctx, conn); err == nil {
		t.Fatal("expected LoadSyncState on closed db to fail")
	}
}

func TestBoolToInt(t *testing.T) {
	if boolToInt(true) != 1 {
		t.Fatal("boolToInt(true) != 1")
	}
	if boolToInt(false) != 0 {
		t.Fatal("boolToInt(false) != 0")
	}
}

func TestEnsureReconDirError(t *testing.T) {
	rootFile := filepath.Join(t.TempDir(), "as-file")
	if err := os.WriteFile(rootFile, []byte("x"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	_, err := EnsureReconDir(rootFile)
	if err == nil {
		t.Fatal("expected EnsureReconDir to fail")
	}
	if !errors.Is(err, os.ErrExist) && !strings.Contains(err.Error(), "create") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunMigrationsInjectedErrors(t *testing.T) {
	root := t.TempDir()
	if _, err := EnsureReconDir(root); err != nil {
		t.Fatalf("EnsureReconDir: %v", err)
	}
	conn, err := Open(DBPath(root))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer conn.Close()

	origSource := newIOFSSource
	origSQLite := newSQLiteWithInstance
	origMigrator := newMigratorWithInstance
	origMigrateUp := migrateUp
	defer func() {
		newIOFSSource = origSource
		newSQLiteWithInstance = origSQLite
		newMigratorWithInstance = origMigrator
		migrateUp = origMigrateUp
	}()

	newIOFSSource = func(fs.FS, string) (sourcepkg.Driver, error) {
		return nil, errors.New("source fail")
	}
	if err := RunMigrations(conn); err == nil || !strings.Contains(err.Error(), "open migrations fs") {
		t.Fatalf("expected source error, got %v", err)
	}

	newIOFSSource = origSource
	newSQLiteWithInstance = func(*sql.DB, *sqlite.Config) (databasepkg.Driver, error) {
		return nil, errors.New("sqlite driver fail")
	}
	if err := RunMigrations(conn); err == nil || !strings.Contains(err.Error(), "create sqlite migrate driver") {
		t.Fatalf("expected sqlite driver error, got %v", err)
	}

	newSQLiteWithInstance = origSQLite
	newMigratorWithInstance = func(string, sourcepkg.Driver, string, databasepkg.Driver) (*migrate.Migrate, error) {
		return nil, errors.New("migrator fail")
	}
	if err := RunMigrations(conn); err == nil || !strings.Contains(err.Error(), "create migrator") {
		t.Fatalf("expected migrator error, got %v", err)
	}

	newMigratorWithInstance = origMigrator
	migrateUp = func(*migrate.Migrate) error { return errors.New("up fail") }
	if err := RunMigrations(conn); err == nil || !strings.Contains(err.Error(), "apply migrations") {
		t.Fatalf("expected migrate up error, got %v", err)
	}

	migrateUp = func(*migrate.Migrate) error { return migrate.ErrNoChange }
	if err := RunMigrations(conn); err != nil {
		t.Fatalf("expected ErrNoChange to be ignored, got %v", err)
	}
}

func TestRunMigrationsUpgradesLegacySymbolDepsSchema(t *testing.T) {
	root := t.TempDir()
	conn, err := Open(filepath.Join(root, "legacy.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer conn.Close()

	if _, err := conn.Exec(`
CREATE TABLE symbols (
    id INTEGER PRIMARY KEY
);
CREATE TABLE symbol_deps (
    id INTEGER PRIMARY KEY,
    symbol_id INTEGER REFERENCES symbols(id) ON DELETE CASCADE,
    dep_name TEXT NOT NULL,
    UNIQUE(symbol_id, dep_name)
);
CREATE TABLE schema_migrations (version uint64, dirty bool);
INSERT INTO schema_migrations (version, dirty) VALUES (1, 0);
INSERT INTO symbols (id) VALUES (1);
INSERT INTO symbol_deps (id, symbol_id, dep_name) VALUES (1, 1, 'Helper');
`); err != nil {
		t.Fatalf("seed legacy schema: %v", err)
	}

	if err := RunMigrations(conn); err != nil {
		t.Fatalf("RunMigrations upgrade: %v", err)
	}

	colRows, err := conn.Query(`PRAGMA table_info(symbol_deps);`)
	if err != nil {
		t.Fatalf("table_info symbol_deps: %v", err)
	}
	defer colRows.Close()

	cols := map[string]bool{}
	for colRows.Next() {
		var (
			cid       int
			name      string
			ctype     string
			notnull   int
			dfltValue sql.NullString
			pk        int
		)
		if err := colRows.Scan(&cid, &name, &ctype, &notnull, &dfltValue, &pk); err != nil {
			t.Fatalf("scan table_info row: %v", err)
		}
		cols[name] = true
	}
	if err := colRows.Err(); err != nil {
		t.Fatalf("iterate table_info rows: %v", err)
	}
	if !cols["dep_package"] || !cols["dep_kind"] {
		t.Fatalf("expected dep_package and dep_kind columns, got %#v", cols)
	}

	var depPackage, depKind string
	if err := conn.QueryRow(`SELECT dep_package, dep_kind FROM symbol_deps WHERE id = 1;`).Scan(&depPackage, &depKind); err != nil {
		t.Fatalf("query migrated dep columns: %v", err)
	}
	if depPackage != "" || depKind != "" {
		t.Fatalf("expected migrated dep columns to default empty strings, got package=%q kind=%q", depPackage, depKind)
	}

	if _, err := conn.Exec(`
INSERT INTO symbol_deps (symbol_id, dep_name, dep_package, dep_kind)
VALUES (1, 'Helper', 'internal/util', 'func');
`); err != nil {
		t.Fatalf("expected extended unique key to allow contextual duplicate, got %v", err)
	}
	if _, err := conn.Exec(`
INSERT INTO symbol_deps (symbol_id, dep_name, dep_package, dep_kind)
VALUES (1, 'Helper', 'internal/util', 'func');
`); err == nil {
		t.Fatal("expected duplicate contextual dependency insert to fail")
	}
}
