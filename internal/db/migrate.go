package db

import (
	"database/sql"
	"embed"
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/sqlite"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

var (
	newIOFSSource          = iofs.New
	newSQLiteWithInstance  = sqlite.WithInstance
	newMigratorWithInstance = migrate.NewWithInstance
	migrateUp              = func(m *migrate.Migrate) error { return m.Up() }
)

func RunMigrations(conn *sql.DB) error {
	src, err := newIOFSSource(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("open migrations fs: %w", err)
	}

	driver, err := newSQLiteWithInstance(conn, &sqlite.Config{})
	if err != nil {
		return fmt.Errorf("create sqlite migrate driver: %w", err)
	}

	migrator, err := newMigratorWithInstance("iofs", src, "sqlite", driver)
	if err != nil {
		return fmt.Errorf("create migrator: %w", err)
	}

	if err := migrateUp(migrator); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("apply migrations: %w", err)
	}
	return nil
}
