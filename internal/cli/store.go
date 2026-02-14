package cli

import (
	"database/sql"
	"errors"
	"fmt"
	"os"

	"github.com/robertguss/recon/internal/db"
)

func openExistingDB(app *App) (*sql.DB, error) {
	path := db.DBPath(app.ModuleRoot)
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("database not initialized at %s; run `recon init` first", path)
		}
		return nil, fmt.Errorf("stat db file: %w", err)
	}

	conn, err := db.Open(path)
	if err != nil {
		return nil, err
	}
	return conn, nil
}
