package install

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
}
