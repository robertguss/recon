package index

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type SourceFile struct {
	AbsPath string
	RelPath string
	Content []byte
	Hash    string
	Lines   int
}

func CollectEligibleGoFiles(moduleRoot string) ([]SourceFile, error) {
	files := make([]SourceFile, 0, 128)

	err := filepath.WalkDir(moduleRoot, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if d.IsDir() {
			if shouldSkipDir(moduleRoot, path, d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}

		name := d.Name()
		if !strings.HasSuffix(name, ".go") {
			return nil
		}
		if strings.HasSuffix(name, "_test.go") {
			return nil
		}

		rel, err := filepath.Rel(moduleRoot, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)

		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if isGeneratedGoFile(content) {
			return nil
		}

		sum := sha256.Sum256(content)
		files = append(files, SourceFile{
			AbsPath: path,
			RelPath: rel,
			Content: content,
			Hash:    hex.EncodeToString(sum[:]),
			Lines:   bytes.Count(content, []byte("\n")) + 1,
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk module files: %w", err)
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].RelPath < files[j].RelPath
	})
	return files, nil
}

func CurrentFingerprint(moduleRoot string) (string, int, error) {
	files, err := CollectEligibleGoFiles(moduleRoot)
	if err != nil {
		return "", 0, err
	}
	return ComputeFingerprint(files), len(files), nil
}

func ComputeFingerprint(files []SourceFile) string {
	h := sha256.New()
	for _, f := range files {
		_, _ = h.Write([]byte(f.RelPath))
		_, _ = h.Write([]byte("\x00"))
		_, _ = h.Write([]byte(f.Hash))
		_, _ = h.Write([]byte("\x00"))
	}
	return hex.EncodeToString(h.Sum(nil))
}

func shouldSkipDir(moduleRoot, path, name string) bool {
	if path != moduleRoot && strings.HasPrefix(name, ".") {
		return true
	}
	if name == "vendor" || name == "testdata" || name == ".recon" {
		return true
	}
	return false
}

func isGeneratedGoFile(content []byte) bool {
	prefix := content
	if len(prefix) > 4096 {
		prefix = prefix[:4096]
	}
	s := string(prefix)
	return strings.Contains(s, "Code generated") && strings.Contains(s, "DO NOT EDIT")
}
