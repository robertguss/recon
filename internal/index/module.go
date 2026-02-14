package index

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var moduleAbsPath = filepath.Abs

func FindModuleRoot(start string) (string, error) {
	current, err := moduleAbsPath(start)
	if err != nil {
		return "", fmt.Errorf("resolve absolute path: %w", err)
	}

	for {
		candidate := filepath.Join(current, "go.mod")
		if _, err := os.Stat(candidate); err == nil {
			return current, nil
		}

		parent := filepath.Dir(current)
		if parent == current {
			return "", errors.New("go.mod not found in current directory or parents")
		}
		current = parent
	}
}

func ModulePath(moduleRoot string) (string, error) {
	f, err := os.Open(filepath.Join(moduleRoot, "go.mod"))
	if err != nil {
		return "", fmt.Errorf("open go.mod: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "module ") {
			path := strings.TrimSpace(strings.TrimPrefix(line, "module "))
			if path == "" {
				break
			}
			return path, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("read go.mod: %w", err)
	}
	return "", errors.New("module path not found in go.mod")
}
