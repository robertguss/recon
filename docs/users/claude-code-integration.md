# Claude Code Integration

Recon integrates with [Claude Code](https://claude.com/claude-code) to serve as
a knowledge layer for AI coding agents. This page explains what gets installed,
how it works, and how to customize it.

## What Gets Installed

Running `recon init` installs four Claude Code integration files:

### 1. SessionStart Hook

**File:** `.claude/hooks/recon-orient.sh`

A shell script that runs `recon orient --json-strict --auto-sync` at the start
of every Claude Code session. This gives the agent immediate context about:

- Project structure (packages, files, symbols)
- Architecture (entry points, dependency flow)
- Active decisions and drift status
- Active patterns
- Recent file activity
- Module heat map (which packages have recent commits)

The hook auto-syncs the index if it's stale, so the agent always gets fresh
context.

### 2. Skill Definition

**File:** `.claude/skills/recon/SKILL.md`

Registers the `/recon` skill in Claude Code. When invoked, the agent receives
documentation for all Recon commands with examples and workflow guidance. The
skill teaches the agent to:

- Use `recon find` before grep for Go symbols
- Check `recon recall` before making new decisions
- Record decisions and patterns during work
- Re-sync after major code changes

### 3. Settings

**File:** `.claude/settings.json`

Configures the SessionStart hook in Claude Code's settings. The hook is
registered with a 10-second timeout to prevent slow startups from blocking the
agent.

### 4. CLAUDE.md Section

**File:** `CLAUDE.md` (appended section)

Appends a "Recon (Code Intelligence)" section to the project's `CLAUDE.md` file.
This section tells Claude Code that Recon is available and how to use it. If the
section already exists, it's updated in place.

## How the Agent Uses Recon

### At Session Start

1. Claude Code starts a new session
2. The SessionStart hook fires, running `recon orient --json-strict --auto-sync`
3. The orient payload is injected into the agent's context
4. The agent now knows the project structure, active decisions, and patterns

### During Work

The agent can invoke Recon commands directly:

```bash
# Look up a symbol before modifying it
recon find HandleRequest --json

# Check existing knowledge before making changes
recon recall "error handling" --json

# Record a decision made during the session
recon decide "Refactor auth to use middleware" \
  --reasoning "Centralizes auth logic, reduces duplication" \
  --evidence-summary "middleware.go exists" \
  --check-type file_exists --check-path internal/middleware.go \
  --json

# Re-sync after a big refactor
recon sync --json
```

All commands use `--json` for structured output that agents can parse reliably.

## Customization

### Disabling the Hook

Remove or rename the hook file:

```bash
rm .claude/hooks/recon-orient.sh
```

Or remove the `SessionStart` entry from `.claude/settings.json`.

### Modifying the Hook

Edit `.claude/hooks/recon-orient.sh` to change the orient flags. For example, to
skip auto-sync:

```bash
#!/bin/sh
exec recon orient --json-strict
```

### Reinstalling

If you've modified integration files and want to restore defaults:

```bash
recon init --force
```

This re-writes all integration files to their default state.

## Troubleshooting

### Hook doesn't fire

- Check that `.claude/settings.json` contains a `SessionStart` hook entry
- Verify the hook script is executable: `chmod +x .claude/hooks/recon-orient.sh`
- Check the hook timeout (default 10s) — if sync takes too long, increase it

### Agent doesn't use Recon commands

- Verify the skill file exists at `.claude/skills/recon/SKILL.md`
- Check that `CLAUDE.md` contains the Recon section
- Invoke the skill explicitly with `/recon` in Claude Code

### Stale context

- The hook uses `--auto-sync` by default, which re-indexes when stale
- If context is still stale, run `recon sync` manually
- Check `recon status` to verify the database is healthy
