# Claude Code Integration — `recon init` Installs Hook, Skill, and CLAUDE.md

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to
> implement this plan task-by-task.

**Goal:** Expand `recon init` to install Claude Code integration files (hook,
skill, CLAUDE.md section) so the agent automatically orients at session start
and knows how to use recon throughout a session.

**Architecture:** The init command checks for existing `.recon/` directory and
prompts for reinstall confirmation. It then installs the database (existing
behavior) plus three new Claude Code integration pieces: a SessionStart hook
script, a skill SKILL.md, and a CLAUDE.md instruction block. The
hook/skill/CLAUDE.md content lives as plain files in `internal/install/` and is
embedded via `//go:embed`. A new `internal/install` package owns the
file-writing logic.

**Tech Stack:** Go, embed, Cobra CLI, Claude Code hooks/skills conventions

---

## Task 1: Create the install asset files

These are plain text files that `recon init` will copy into the target project.
They live in `internal/install/assets/` and are embedded at compile time.

**Files:**

- Create: `internal/install/assets/hook.sh`
- Create: `internal/install/assets/SKILL.md`
- Create: `internal/install/assets/CLAUDE_SECTION.md`

**Step 1: Create the hook script**

Create `internal/install/assets/hook.sh`:

```bash
#!/usr/bin/env bash
# Recon SessionStart hook — injects orient payload into agent context.
# Installed by: recon init

set -euo pipefail

# Only run if recon is initialized in this repo.
if [ ! -f ".recon/recon.db" ]; then
  exit 0
fi

echo "## Recon Orient Context"
echo ""
echo "The following is live code intelligence data for this repository."
echo "Use it to understand the project structure, recent activity, and existing decisions."
echo ""
recon orient --json
```

**Step 2: Create the skill SKILL.md**

Create `internal/install/assets/SKILL.md`:

```yaml
---
name: recon
description:
  Code intelligence and knowledge CLI for Go repositories. Use when exploring Go
  code, finding symbols, recording architectural decisions, detecting patterns,
  or recalling prior knowledge about this codebase.
user-invocable: true
---
```

Then the markdown body (after the frontmatter) teaches the agent:

- What recon is and when to use each command
- `recon find <symbol>` — structured symbol lookup with dependency info, use
  before grep for Go symbols
- `recon decide "<text>" --confidence <level>` — record architectural decisions
  with evidence verification
- `recon pattern "<title>" --description "<text>"` — record recurring code
  patterns
- `recon recall "<query>"` — search existing decisions/knowledge before making
  changes
- `recon orient` — get project context payload (already injected by hook, but
  can re-run)
- `recon sync` — re-index after major code changes
- Workflow guidance: check recall before deciding, use find for symbol deps,
  record significant discoveries

**Step 3: Create the CLAUDE.md section**

Create `internal/install/assets/CLAUDE_SECTION.md`:

```markdown
## Recon (Code Intelligence)

This project uses [recon](https://github.com/robertguss/recon) for code
intelligence. A recon orient payload is injected at session start via hook. Use
the `/recon` skill for detailed command reference.

- Use `recon find` for Go symbol lookup (gives dependency info that grep cannot)
- Use `recon recall` to check existing decisions before making architectural
  changes
- Use `recon decide` to record significant architectural discoveries or
  decisions
- Use `recon pattern` to record recurring code patterns you observe
- Use `recon sync` to re-index after major code changes
```

**Step 4: Verify the files exist and are well-formed**

Run: `ls -la internal/install/assets/` Expected: `hook.sh`, `SKILL.md`,
`CLAUDE_SECTION.md` all present

---

## Task 2: Create the `internal/install` package

This package embeds the asset files and provides functions to write them into a
target project directory.

**Files:**

- Create: `internal/install/install.go`
- Create: `internal/install/install_test.go`

**Step 1: Write the failing test for InstallHook**

Create `internal/install/install_test.go` with a test that calls
`InstallHook(root)` and asserts:

- `.claude/hooks/recon-orient.sh` exists at the expected path
- File is executable (mode `0o755`)
- Content matches the embedded `hook.sh`

Run: `go test ./internal/install/... -run TestInstallHook -v` Expected: FAIL —
package doesn't exist yet

**Step 2: Write the failing test for InstallSkill**

Add test that calls `InstallSkill(root)` and asserts:

- `.claude/skills/recon/SKILL.md` exists
- Content matches the embedded `SKILL.md`

Run: `go test ./internal/install/... -run TestInstallSkill -v` Expected: FAIL

**Step 3: Write the failing test for InstallClaudeSection**

Add test that calls `InstallClaudeSection(root)` and asserts:

- When `CLAUDE.md` doesn't exist: creates it with the section content
- When `CLAUDE.md` exists without recon section: appends the section
- When `CLAUDE.md` already has `## Recon` section: replaces just that section
  (so reinstall updates it)

Run: `go test ./internal/install/... -run TestInstallClaudeSection -v` Expected:
FAIL

**Step 4: Write the failing test for InstallSettings**

Add test that calls `InstallSettings(root)` and asserts:

- When `.claude/settings.json` doesn't exist: creates it with the hook config
- When `.claude/settings.json` exists without hooks: adds the hooks key
- When `.claude/settings.json` already has hooks: merges/adds the SessionStart
  hook entry

The settings.json content should be:

```json
{
  "hooks": {
    "SessionStart": [
      {
        "matcher": "",
        "hooks": [
          {
            "type": "command",
            "command": ".claude/hooks/recon-orient.sh",
            "timeout": 10000
          }
        ]
      }
    ]
  }
}
```

Run: `go test ./internal/install/... -run TestInstallSettings -v` Expected: FAIL

**Step 5: Implement `internal/install/install.go`**

```go
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

func InstallHook(root string) error {
    // Read embedded hook.sh
    // Create .claude/hooks/ directory
    // Write recon-orient.sh with 0o755
}

func InstallSkill(root string) error {
    // Read embedded SKILL.md
    // Create .claude/skills/recon/ directory
    // Write SKILL.md with 0o644
}

func InstallClaudeSection(root string) error {
    // Read embedded CLAUDE_SECTION.md
    // Read existing CLAUDE.md (or empty)
    // If "## Recon" section exists, replace it
    // Otherwise append with a leading newline
    // Write CLAUDE.md
}

func InstallSettings(root string) error {
    // Read existing .claude/settings.json (or empty object)
    // Parse as map[string]any
    // Merge hooks.SessionStart entry
    // Write back with indentation
}
```

**Step 6: Run tests to verify they pass**

Run: `go test ./internal/install/... -v` Expected: All PASS

**Step 7: Commit**

```bash
git add internal/install/
git commit -m "feat(install): add install package with hook, skill, settings, and CLAUDE.md section writers"
```

---

## Task 3: Add reinstall detection and `--force` flag to init command

Modify the init command to check for existing `.recon/` directory and prompt
before overwriting.

**Files:**

- Modify: `internal/cli/init.go`

**Step 1: Write the failing test for reinstall prompt**

In `internal/cli/commands_test.go`, add a test:

- Set up a module root with an existing `.recon/` directory
- Run init command
- Assert it prompts "recon is already initialized. Reinstall? [y/N]:"
- When answer is no: exits without error, does not overwrite
- When answer is yes: proceeds with full init

Also test:

- `--force` flag bypasses the prompt
- `--no-prompt` with existing `.recon/` exits with error (can't prompt in
  non-interactive mode without --force)

Run: `go test ./internal/cli/... -run TestInitReinstall -v` Expected: FAIL

**Step 2: Implement the reinstall check in `init.go`**

Add to `newInitCommand`:

- `--force` bool flag
- Before `EnsureReconDir`: check if `.recon/` exists via `os.Stat`
- If exists and not `--force`: prompt with
  `askYesNo("recon is already initialized. Reinstall? [y/N]: ", false)`
- If `--no-prompt` and not `--force`: return error "recon already initialized;
  use --force to reinstall"
- If user says no: print "Cancelled." and return nil

**Step 3: Run tests to verify they pass**

Run: `go test ./internal/cli/... -run TestInitReinstall -v` Expected: PASS

**Step 4: Commit**

```bash
git add internal/cli/init.go internal/cli/commands_test.go
git commit -m "feat(init): add reinstall detection with --force flag"
```

---

## Task 4: Wire install package into init command

Make init call the install functions after database setup.

**Files:**

- Modify: `internal/cli/init.go`

**Step 1: Write the failing test for full init with Claude Code files**

Add test in `commands_test.go`:

- Run `init --json` on a fresh module root
- Assert `.claude/hooks/recon-orient.sh` exists and is executable
- Assert `.claude/skills/recon/SKILL.md` exists
- Assert `.claude/settings.json` exists and contains the hook config
- Assert `CLAUDE.md` contains `## Recon` section

Run: `go test ./internal/cli/... -run TestInitInstallsClaudeCodeFiles -v`
Expected: FAIL

**Step 2: Import install package and call functions**

In `init.go`, after the existing `EnsureGitIgnore` call, add:

```go
if err := install.InstallHook(app.ModuleRoot); err != nil {
    return fmt.Errorf("install hook: %w", err)
}
if err := install.InstallSkill(app.ModuleRoot); err != nil {
    return fmt.Errorf("install skill: %w", err)
}
if err := install.InstallSettings(app.ModuleRoot); err != nil {
    return fmt.Errorf("install settings: %w", err)
}
if err := install.InstallClaudeSection(app.ModuleRoot); err != nil {
    return fmt.Errorf("install claude section: %w", err)
}
```

Update the success output to mention Claude Code integration:

- Text mode:
  `"Initialized recon at %s\nClaude Code integration installed (.claude/hooks, skills, settings)\n"`
- JSON mode: add `"claude_code": true` to the JSON response

**Step 3: Run tests to verify they pass**

Run: `go test ./internal/cli/... -run TestInitInstallsClaudeCodeFiles -v`
Expected: PASS

**Step 4: Run the full test suite**

Run: `go test ./... -count=1` Expected: All PASS. Some existing init tests may
need updates if they now assert on the number of files created or output text.

**Step 5: Commit**

```bash
git add internal/cli/init.go internal/cli/commands_test.go
git commit -m "feat(init): wire Claude Code hook, skill, settings, and CLAUDE.md into init command"
```

---

## Task 5: Make install functions injectable for test isolation

Follow the project's function-var injection pattern so tests can mock install
functions to isolate init logic from install logic.

**Files:**

- Modify: `internal/cli/init.go`
- Modify: `internal/cli/commands_test.go`

**Step 1: Extract install calls into function vars**

In `init.go`, add at package level:

```go
var (
    installHook          = install.InstallHook
    installSkill         = install.InstallSkill
    installSettings      = install.InstallSettings
    installClaudeSection = install.InstallClaudeSection
)
```

Replace direct calls with these vars.

**Step 2: Update existing init tests to mock install functions**

In tests that test init but don't care about Claude Code file installation (like
`TestInitCommandErrorBranches`), save/restore the install function vars and
replace with no-ops. This prevents those tests from writing Claude Code files
into temp dirs and keeps them focused on their original assertions.

**Step 3: Add error path tests for install failures**

Add tests where each install function returns an error, asserting that init
surfaces the error correctly (e.g., "install hook: permission denied").

**Step 4: Run full test suite**

Run: `go test ./... -count=1` Expected: All PASS

**Step 5: Commit**

```bash
git add internal/cli/init.go internal/cli/commands_test.go
git commit -m "refactor(init): make install functions injectable for test isolation"
```

---

## Task 6: End-to-end verification

Build the binary and test the full flow manually.

**Files:**

- No new files

**Step 1: Build recon**

Run: `just build` Expected: `./bin/recon` built successfully

**Step 2: Test fresh init on recon itself**

```bash
# Clean slate
rm -rf .recon .claude/hooks/recon-orient.sh .claude/skills/recon .claude/settings.json
# Remove recon section from CLAUDE.md if present

./bin/recon init
```

Expected output mentions both database and Claude Code integration.

**Step 3: Verify installed files**

- `.claude/hooks/recon-orient.sh` exists, is executable, contains
  `recon orient --json`
- `.claude/skills/recon/SKILL.md` exists with frontmatter and command reference
- `.claude/settings.json` has `SessionStart` hook entry
- `CLAUDE.md` has `## Recon (Code Intelligence)` section appended

**Step 4: Test the hook works**

Run: `bash .claude/hooks/recon-orient.sh` Expected: Outputs the orient JSON
payload (requires `recon` in PATH or adjust script)

**Step 5: Test reinstall flow**

Run: `./bin/recon init` Expected: "recon is already initialized. Reinstall?
[y/N]:" Answer y: reinstalls successfully Answer n: "Cancelled."

Run: `./bin/recon init --force` Expected: Reinstalls without prompting

**Step 6: Run full test suite one final time**

Run: `just test` Expected: All PASS

**Step 7: Commit any fixups, then final commit**

```bash
git add -A
git commit -m "feat(init): complete Claude Code integration — hook, skill, settings, CLAUDE.md"
```

---

## Notes

- The hook script uses a relative path (`.recon/recon.db`) which works because
  Claude Code hooks run from the project root.
- The hook script assumes `recon` is in PATH. If the user installed via
  `just install`, it's in GOPATH/bin. If not, the hook will silently exit (set
  -e + command not found). We could add a PATH check later but keeping it simple
  for now.
- `InstallClaudeSection` uses `## Recon` as a section marker for replacement on
  reinstall. If the user renames the heading, reinstall will append a duplicate.
  Acceptable tradeoff for simplicity.
- `InstallSettings` needs to handle merging into existing settings.json
  carefully — it should not clobber other hook entries the user has configured.
- The skill description is the key for Claude discovering it. It should mention
  "Go", "symbols", "decisions", "patterns", "codebase" so Claude's description
  matching picks it up.
