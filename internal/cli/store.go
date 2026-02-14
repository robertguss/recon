package cli

import (
	"database/sql"
	"errors"
	"fmt"
	"os"

	"github.com/robertguss/recon/internal/db"
)

type dbNotInitializedError struct {
	Path string
}

func (e dbNotInitializedError) Error() string {
	return fmt.Sprintf("database not initialized at %s; run `recon init` first", e.Path)
}

func openExistingDB(app *App) (*sql.DB, error) {
	path := db.DBPath(app.ModuleRoot)
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, dbNotInitializedError{Path: path}
		}
		return nil, fmt.Errorf("stat db file: %w", err)
	}

	conn, err := db.Open(path)
	if err != nil {
		return nil, err
	}
	return conn, nil
}
