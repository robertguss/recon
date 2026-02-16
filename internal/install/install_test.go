package install

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// readOnlyRoot returns a temp directory that blocks writes (on non-Windows).
// The cleanup function restores permissions so t.TempDir() can remove it.
func readOnlyRoot(t *testing.T) (string, func()) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("read-only directory test not supported on Windows")
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o444); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	return dir, func() { os.Chmod(dir, 0o755) }
}

func TestInstallHook(t *testing.T) {
	root := t.TempDir()

	if err := InstallHook(root); err != nil {
		t.Fatalf("InstallHook: %v", err)
	}

	path := filepath.Join(root, ".claude", "hooks", "recon-orient.sh")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat hook: %v", err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("expected 0755, got %o", info.Mode().Perm())
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read hook: %v", err)
	}

	want, err := assetsFS.ReadFile("assets/hook.sh")
	if err != nil {
		t.Fatalf("read embedded: %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("hook content mismatch:\ngot:  %q\nwant: %q", got, want)
	}
}

func TestInstallSkill(t *testing.T) {
	root := t.TempDir()

	if err := InstallSkill(root); err != nil {
		t.Fatalf("InstallSkill: %v", err)
	}

	path := filepath.Join(root, ".claude", "skills", "recon", "SKILL.md")
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read skill: %v", err)
	}

	want, err := assetsFS.ReadFile("assets/SKILL.md")
	if err != nil {
		t.Fatalf("read embedded: %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("skill content mismatch")
	}
}

func TestInstallClaudeSection(t *testing.T) {
	t.Run("creates CLAUDE.md when missing", func(t *testing.T) {
		root := t.TempDir()

		if err := InstallClaudeSection(root); err != nil {
			t.Fatalf("InstallClaudeSection: %v", err)
		}

		got, err := os.ReadFile(filepath.Join(root, "CLAUDE.md"))
		if err != nil {
			t.Fatalf("read CLAUDE.md: %v", err)
		}

		want, err := assetsFS.ReadFile("assets/CLAUDE_SECTION.md")
		if err != nil {
			t.Fatalf("read embedded: %v", err)
		}
		if string(got) != string(want) {
			t.Fatalf("content mismatch:\ngot:  %q\nwant: %q", got, want)
		}
	})

	t.Run("appends to existing CLAUDE.md", func(t *testing.T) {
		root := t.TempDir()
		existing := "# My Project\n\nSome content.\n"
		if err := os.WriteFile(filepath.Join(root, "CLAUDE.md"), []byte(existing), 0o644); err != nil {
			t.Fatalf("write existing: %v", err)
		}

		if err := InstallClaudeSection(root); err != nil {
			t.Fatalf("InstallClaudeSection: %v", err)
		}

		got, err := os.ReadFile(filepath.Join(root, "CLAUDE.md"))
		if err != nil {
			t.Fatalf("read CLAUDE.md: %v", err)
		}

		if !strings.HasPrefix(string(got), existing) {
			t.Fatal("existing content was not preserved")
		}
		if !strings.Contains(string(got), "## Recon (Code Intelligence)") {
			t.Fatal("recon section was not appended")
		}
	})

	t.Run("replaces existing Recon section", func(t *testing.T) {
		root := t.TempDir()
		existing := "# My Project\n\nSome content.\n\n## Recon (Code Intelligence)\n\nOld content here.\n"
		if err := os.WriteFile(filepath.Join(root, "CLAUDE.md"), []byte(existing), 0o644); err != nil {
			t.Fatalf("write existing: %v", err)
		}

		if err := InstallClaudeSection(root); err != nil {
			t.Fatalf("InstallClaudeSection: %v", err)
		}

		got, err := os.ReadFile(filepath.Join(root, "CLAUDE.md"))
		if err != nil {
			t.Fatalf("read CLAUDE.md: %v", err)
		}

		if strings.Contains(string(got), "Old content here.") {
			t.Fatal("old recon section was not replaced")
		}
		if !strings.Contains(string(got), "## Recon (Code Intelligence)") {
			t.Fatal("new recon section missing")
		}
		if !strings.Contains(string(got), "# My Project") {
			t.Fatal("non-recon content was removed")
		}
		// Should not have duplicate sections
		if strings.Count(string(got), "## Recon (Code Intelligence)") != 1 {
			t.Fatal("duplicate recon sections found")
		}
	})
}

func TestInstallSettings(t *testing.T) {
	t.Run("creates settings.json when missing", func(t *testing.T) {
		root := t.TempDir()

		if err := InstallSettings(root); err != nil {
			t.Fatalf("InstallSettings: %v", err)
		}

		path := filepath.Join(root, ".claude", "settings.json")
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read settings: %v", err)
		}

		var settings map[string]any
		if err := json.Unmarshal(got, &settings); err != nil {
			t.Fatalf("parse settings: %v", err)
		}

		hooks, ok := settings["hooks"].(map[string]any)
		if !ok {
			t.Fatal("missing hooks key")
		}
		sessionStart, ok := hooks["SessionStart"].([]any)
		if !ok {
			t.Fatal("missing SessionStart key")
		}
		if len(sessionStart) != 1 {
			t.Fatalf("expected 1 SessionStart entry, got %d", len(sessionStart))
		}
	})

	t.Run("merges into existing settings without hooks", func(t *testing.T) {
		root := t.TempDir()
		if err := os.MkdirAll(filepath.Join(root, ".claude"), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		existing := `{"permissions": {"allow": ["Read"]}}`
		if err := os.WriteFile(filepath.Join(root, ".claude", "settings.json"), []byte(existing), 0o644); err != nil {
			t.Fatalf("write existing: %v", err)
		}

		if err := InstallSettings(root); err != nil {
			t.Fatalf("InstallSettings: %v", err)
		}

		got, err := os.ReadFile(filepath.Join(root, ".claude", "settings.json"))
		if err != nil {
			t.Fatalf("read settings: %v", err)
		}

		var settings map[string]any
		if err := json.Unmarshal(got, &settings); err != nil {
			t.Fatalf("parse settings: %v", err)
		}

		// Original key preserved
		if _, ok := settings["permissions"]; !ok {
			t.Fatal("existing permissions key was removed")
		}
		// Hooks added
		if _, ok := settings["hooks"]; !ok {
			t.Fatal("hooks key was not added")
		}
	})

	t.Run("merges into existing hooks", func(t *testing.T) {
		root := t.TempDir()
		if err := os.MkdirAll(filepath.Join(root, ".claude"), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		existing := `{"hooks": {"PreToolUse": [{"matcher": "", "hooks": [{"type": "command", "command": "echo pre"}]}]}}`
		if err := os.WriteFile(filepath.Join(root, ".claude", "settings.json"), []byte(existing), 0o644); err != nil {
			t.Fatalf("write existing: %v", err)
		}

		if err := InstallSettings(root); err != nil {
			t.Fatalf("InstallSettings: %v", err)
		}

		got, err := os.ReadFile(filepath.Join(root, ".claude", "settings.json"))
		if err != nil {
			t.Fatalf("read settings: %v", err)
		}

		var settings map[string]any
		if err := json.Unmarshal(got, &settings); err != nil {
			t.Fatalf("parse settings: %v", err)
		}

		hooks := settings["hooks"].(map[string]any)
		// Existing hook preserved
		if _, ok := hooks["PreToolUse"]; !ok {
			t.Fatal("existing PreToolUse hook was removed")
		}
		// SessionStart added
		if _, ok := hooks["SessionStart"]; !ok {
			t.Fatal("SessionStart was not added")
		}
	})

	t.Run("preserves existing SessionStart hooks", func(t *testing.T) {
		root := t.TempDir()
		if err := os.MkdirAll(filepath.Join(root, ".claude"), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		existing := `{"hooks": {"SessionStart": [{"matcher": "", "hooks": [{"type": "command", "command": "echo existing-hook"}]}]}}`
		if err := os.WriteFile(filepath.Join(root, ".claude", "settings.json"), []byte(existing), 0o644); err != nil {
			t.Fatalf("write existing: %v", err)
		}

		if err := InstallSettings(root); err != nil {
			t.Fatalf("InstallSettings: %v", err)
		}

		got, err := os.ReadFile(filepath.Join(root, ".claude", "settings.json"))
		if err != nil {
			t.Fatalf("read settings: %v", err)
		}

		var settings map[string]any
		if err := json.Unmarshal(got, &settings); err != nil {
			t.Fatalf("parse settings: %v", err)
		}

		hooks := settings["hooks"].(map[string]any)
		sessionStart := hooks["SessionStart"].([]any)
		if len(sessionStart) != 2 {
			t.Fatalf("expected 2 SessionStart entries (existing + recon), got %d", len(sessionStart))
		}

		// Verify the existing hook is still there
		first := sessionStart[0].(map[string]any)
		firstHooks := first["hooks"].([]any)
		firstHook := firstHooks[0].(map[string]any)
		if firstHook["command"] != "echo existing-hook" {
			t.Fatal("existing SessionStart hook was not preserved")
		}

		// Verify recon hook was appended
		second := sessionStart[1].(map[string]any)
		secondHooks := second["hooks"].([]any)
		secondHook := secondHooks[0].(map[string]any)
		if secondHook["command"] != ".claude/hooks/recon-orient.sh" {
			t.Fatal("recon hook was not appended")
		}
	})

	t.Run("skips if recon hook already present", func(t *testing.T) {
		root := t.TempDir()
		if err := os.MkdirAll(filepath.Join(root, ".claude"), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		existing := `{"hooks": {"SessionStart": [{"matcher": "", "hooks": [{"type": "command", "command": ".claude/hooks/recon-orient.sh"}]}]}}`
		if err := os.WriteFile(filepath.Join(root, ".claude", "settings.json"), []byte(existing), 0o644); err != nil {
			t.Fatalf("write existing: %v", err)
		}

		if err := InstallSettings(root); err != nil {
			t.Fatalf("InstallSettings: %v", err)
		}

		got, err := os.ReadFile(filepath.Join(root, ".claude", "settings.json"))
		if err != nil {
			t.Fatalf("read settings: %v", err)
		}

		var settings map[string]any
		if err := json.Unmarshal(got, &settings); err != nil {
			t.Fatalf("parse settings: %v", err)
		}

		hooks := settings["hooks"].(map[string]any)
		sessionStart := hooks["SessionStart"].([]any)
		if len(sessionStart) != 1 {
			t.Fatalf("expected 1 SessionStart entry (no duplicate), got %d", len(sessionStart))
		}
	})

	t.Run("error on invalid existing JSON", func(t *testing.T) {
		root := t.TempDir()
		if err := os.MkdirAll(filepath.Join(root, ".claude"), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(root, ".claude", "settings.json"), []byte("NOT JSON"), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}

		err := InstallSettings(root)
		if err == nil {
			t.Fatal("expected error for invalid JSON")
		}
		if !strings.Contains(err.Error(), "parse existing settings") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("error on unreadable settings file", func(t *testing.T) {
		root := t.TempDir()
		dir := filepath.Join(root, ".claude")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		// Create a directory where a file is expected — ReadFile will fail with non-ENOENT.
		if err := os.Mkdir(filepath.Join(dir, "settings.json"), 0o755); err != nil {
			t.Fatalf("mkdir settings.json: %v", err)
		}

		err := InstallSettings(root)
		if err == nil {
			t.Fatal("expected error for unreadable settings")
		}
		if !strings.Contains(err.Error(), "read settings") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("error on read-only root", func(t *testing.T) {
		root, cleanup := readOnlyRoot(t)
		defer cleanup()

		err := InstallSettings(root)
		if err == nil {
			t.Fatal("expected error for read-only root")
		}
		if !strings.Contains(err.Error(), "create .claude dir") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestInstallHookErrors(t *testing.T) {
	t.Run("error on read-only root", func(t *testing.T) {
		root, cleanup := readOnlyRoot(t)
		defer cleanup()

		err := InstallHook(root)
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "create hooks dir") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("error on unwritable hooks dir", func(t *testing.T) {
		root := t.TempDir()
		dir := filepath.Join(root, ".claude", "hooks")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		// Make hooks dir read-only so WriteFile fails.
		if err := os.Chmod(dir, 0o444); err != nil {
			t.Fatalf("chmod: %v", err)
		}
		defer os.Chmod(dir, 0o755)

		err := InstallHook(root)
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "write hook") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestInstallSkillErrors(t *testing.T) {
	t.Run("error on read-only root", func(t *testing.T) {
		root, cleanup := readOnlyRoot(t)
		defer cleanup()

		err := InstallSkill(root)
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "create skills dir") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("error on unwritable skills dir", func(t *testing.T) {
		root := t.TempDir()
		dir := filepath.Join(root, ".claude", "skills", "recon")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.Chmod(dir, 0o444); err != nil {
			t.Fatalf("chmod: %v", err)
		}
		defer os.Chmod(dir, 0o755)

		err := InstallSkill(root)
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "write skill") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

// saveReadAsset saves and restores the readAsset var around a test.
func saveReadAsset(t *testing.T) {
	t.Helper()
	orig := readAsset
	t.Cleanup(func() { readAsset = orig })
}

// saveMarshalJSON saves and restores the marshalJSON var around a test.
func saveMarshalJSON(t *testing.T) {
	t.Helper()
	orig := marshalJSON
	t.Cleanup(func() { marshalJSON = orig })
}

func TestInstallHookReadAssetError(t *testing.T) {
	saveReadAsset(t)
	readAsset = func(string) ([]byte, error) {
		return nil, fmt.Errorf("injected read error")
	}
	err := InstallHook(t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "read embedded hook") {
		t.Fatalf("expected read error, got: %v", err)
	}
}

func TestInstallSkillReadAssetError(t *testing.T) {
	saveReadAsset(t)
	readAsset = func(string) ([]byte, error) {
		return nil, fmt.Errorf("injected read error")
	}
	err := InstallSkill(t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "read embedded skill") {
		t.Fatalf("expected read error, got: %v", err)
	}
}

func TestInstallClaudeSectionReadAssetError(t *testing.T) {
	saveReadAsset(t)
	readAsset = func(string) ([]byte, error) {
		return nil, fmt.Errorf("injected read error")
	}
	err := InstallClaudeSection(t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "read embedded claude section") {
		t.Fatalf("expected read error, got: %v", err)
	}
}

func TestInstallSettingsMarshalError(t *testing.T) {
	saveMarshalJSON(t)
	marshalJSON = func(v any, prefix, indent string) ([]byte, error) {
		return nil, fmt.Errorf("injected marshal error")
	}
	err := InstallSettings(t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "marshal settings") {
		t.Fatalf("expected marshal error, got: %v", err)
	}
}

func TestInstallClaudeSectionErrors(t *testing.T) {
	t.Run("error on unreadable CLAUDE.md", func(t *testing.T) {
		root := t.TempDir()
		// Create a directory where CLAUDE.md should be — ReadFile fails with non-ENOENT.
		if err := os.Mkdir(filepath.Join(root, "CLAUDE.md"), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		err := InstallClaudeSection(root)
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "read CLAUDE.md") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("replaces Recon section with trailing sections", func(t *testing.T) {
		root := t.TempDir()
		existing := "# My Project\n\n## Recon (Code Intelligence)\n\nOld stuff.\n\n## Other Section\n\nKept.\n"
		if err := os.WriteFile(filepath.Join(root, "CLAUDE.md"), []byte(existing), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}

		if err := InstallClaudeSection(root); err != nil {
			t.Fatalf("InstallClaudeSection: %v", err)
		}

		got, err := os.ReadFile(filepath.Join(root, "CLAUDE.md"))
		if err != nil {
			t.Fatalf("read: %v", err)
		}

		content := string(got)
		if strings.Contains(content, "Old stuff.") {
			t.Fatal("old recon content not replaced")
		}
		if !strings.Contains(content, "## Recon (Code Intelligence)") {
			t.Fatal("new recon section missing")
		}
		if !strings.Contains(content, "## Other Section") {
			t.Fatal("trailing section was removed")
		}
		if !strings.Contains(content, "Kept.") {
			t.Fatal("trailing section content was removed")
		}
	})

	t.Run("appends to content without trailing newline", func(t *testing.T) {
		root := t.TempDir()
		existing := "# My Project\nNo trailing newline"
		if err := os.WriteFile(filepath.Join(root, "CLAUDE.md"), []byte(existing), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}

		if err := InstallClaudeSection(root); err != nil {
			t.Fatalf("InstallClaudeSection: %v", err)
		}

		got, err := os.ReadFile(filepath.Join(root, "CLAUDE.md"))
		if err != nil {
			t.Fatalf("read: %v", err)
		}

		content := string(got)
		if !strings.HasPrefix(content, existing+"\n") {
			t.Fatal("newline not added before recon section")
		}
		if !strings.Contains(content, "## Recon (Code Intelligence)") {
			t.Fatal("recon section not appended")
		}
	})
}
