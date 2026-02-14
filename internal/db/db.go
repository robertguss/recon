package db

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

const (
	ReconDirName = ".recon"
	DBFileName   = "recon.db"
)

func ReconDir(root string) string {
	return filepath.Join(root, ReconDirName)
}

func DBPath(root string) string {
	return filepath.Join(ReconDir(root), DBFileName)
}

func EnsureReconDir(root string) (string, error) {
	dir := ReconDir(root)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create %s: %w", dir, err)
	}
	return dir, nil
}

func Open(path string) (*sql.DB, error) {
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite db: %w", err)
	}
	conn.SetMaxOpenConns(1)

	if _, err := conn.Exec("PRAGMA foreign_keys = ON;"); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}
	return conn, nil
}

func EnsureGitIgnore(root string) error {
	target := ".recon/recon.db"
	path := filepath.Join(root, ".gitignore")

	raw, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read .gitignore: %w", err)
	}
	if strings.Contains(string(raw), target) {
		return nil
	}

	var next string
	if len(raw) == 0 {
		next = target + "\n"
	} else {
		next = string(raw)
		if !strings.HasSuffix(next, "\n") {
			next += "\n"
		}
		next += target + "\n"
	}

	if err := os.WriteFile(path, []byte(next), 0o644); err != nil {
		return fmt.Errorf("write .gitignore: %w", err)
	}
	return nil
}
