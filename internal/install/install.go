package install

import (
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

//go:embed assets/*
var assetsFS embed.FS

// readAsset is a package-level var for testability.
var readAsset = func(name string) ([]byte, error) {
	return assetsFS.ReadFile(name)
}

// marshalJSON is a package-level var for testability.
var marshalJSON = json.MarshalIndent

func InstallHook(root string) error {
	data, err := readAsset("assets/hook.sh")
	if err != nil {
		return fmt.Errorf("read embedded hook: %w", err)
	}

	dir := filepath.Join(root, ".claude", "hooks")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create hooks dir: %w", err)
	}

	path := filepath.Join(dir, "recon-orient.sh")
	if err := os.WriteFile(path, data, 0o755); err != nil {
		return fmt.Errorf("write hook: %w", err)
	}
	return nil
}

func InstallSkill(root string) error {
	data, err := readAsset("assets/SKILL.md")
	if err != nil {
		return fmt.Errorf("read embedded skill: %w", err)
	}

	dir := filepath.Join(root, ".claude", "skills", "recon")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create skills dir: %w", err)
	}

	path := filepath.Join(dir, "SKILL.md")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write skill: %w", err)
	}
	return nil
}

func InstallClaudeSection(root string) error {
	section, err := readAsset("assets/CLAUDE_SECTION.md")
	if err != nil {
		return fmt.Errorf("read embedded claude section: %w", err)
	}

	claudePath := filepath.Join(root, "CLAUDE.md")
	existing, err := os.ReadFile(claudePath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read CLAUDE.md: %w", err)
	}

	content := string(existing)
	sectionStr := string(section)

	if content == "" {
		// No existing file — write section directly.
		return os.WriteFile(claudePath, section, 0o644)
	}

	// Check if Recon section already exists.
	const marker = "## Recon (Code Intelligence)"
	idx := strings.Index(content, marker)
	if idx >= 0 {
		// Find the end of the Recon section (next ## heading or EOF).
		rest := content[idx+len(marker):]
		endIdx := strings.Index(rest, "\n## ")
		if endIdx >= 0 {
			// Replace just the Recon section.
			content = content[:idx] + sectionStr + rest[endIdx+1:]
		} else {
			// Recon section goes to EOF — replace it.
			content = content[:idx] + sectionStr
		}
	} else {
		// Append with a leading newline.
		if !strings.HasSuffix(content, "\n") {
			content += "\n"
		}
		content += "\n" + sectionStr
	}

	return os.WriteFile(claudePath, []byte(content), 0o644)
}

func InstallSettings(root string) error {
	dir := filepath.Join(root, ".claude")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create .claude dir: %w", err)
	}

	settingsPath := filepath.Join(dir, "settings.json")
	settings := make(map[string]any)

	existing, err := os.ReadFile(settingsPath)
	if err == nil {
		if err := json.Unmarshal(existing, &settings); err != nil {
			return fmt.Errorf("parse existing settings: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("read settings: %w", err)
	}

	// Build the hook entry.
	hookEntry := map[string]any{
		"type":    "command",
		"command": ".claude/hooks/recon-orient.sh",
		"timeout": 10000,
	}
	sessionStartEntry := map[string]any{
		"matcher": "",
		"hooks":   []any{hookEntry},
	}

	// Get or create hooks map.
	hooks, _ := settings["hooks"].(map[string]any)
	if hooks == nil {
		hooks = make(map[string]any)
	}

	hooks["SessionStart"] = []any{sessionStartEntry}
	settings["hooks"] = hooks

	data, err := marshalJSON(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal settings: %w", err)
	}

	return os.WriteFile(settingsPath, append(data, '\n'), 0o644)
}
