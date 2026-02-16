# Troubleshooting

Common errors and how to fix them.

## "recon: not initialized"

**Error:** `recon: database not found; run "recon init" first`

**Cause:** The `.recon/recon.db` file doesn't exist. Recon hasn't been
initialized in this project.

**Fix:**

```bash
recon init
```

## "go.mod not found"

**Error:** `go.mod not found at /path; run recon from a Go module`

**Cause:** Recon requires a Go module. There's no `go.mod` in the project root
or any parent directory.

**Fix:** Make sure you're running recon from within a Go module:

```bash
cd your-go-project    # must contain go.mod
recon init
```

## "recon already initialized"

**Error:** `recon already initialized; use --force to reinstall`

**Cause:** Recon has already been initialized. Running `init` again without
`--force` exits to prevent accidental re-initialization.

**Fix:**

```bash
recon init --force    # re-runs migrations and reinstalls integration files
```

Or in non-interactive mode:

```bash
recon init --force --no-prompt
```

## "symbol not found"

**Error:** `symbol "Foo" not found`

**Possible causes:**

- The symbol doesn't exist in the indexed codebase
- The index is stale (code changed since last sync)
- The symbol is in a file that wasn't indexed

**Fix:**

1. Re-sync the index: `recon sync`
2. Check suggestions in the error output
3. Verify the symbol exists: `recon find --package ./path/ --limit 50`

## "symbol is ambiguous"

**Error:** `symbol "Service" is ambiguous (4 candidates)`

**Cause:** Multiple symbols share the same name across different packages.

**Fix:** Use filter flags to disambiguate:

```bash
recon find Service --package ./internal/orient/
recon find Service --kind type
recon find Service --file service.go
```

Or use dot syntax for methods:

```bash
recon find orient.Service.Build
```

## "stale context"

**Warning:** `warning: stale context (commit changed since last sync)`

**Cause:** The git commit has changed since the last `recon sync`. The orient
context may not reflect the current code.

**Fix:**

```bash
recon sync                  # re-index manually
recon orient --sync         # or sync as part of orient
recon orient --auto-sync    # auto-sync when stale
```

## "verification failed"

**Error:** `Dry run: failed — file not found: path/to/file`

**Cause:** An evidence check didn't pass. The file, symbol, or pattern specified
in the check doesn't exist or doesn't match.

**Fix:**

- Verify the check spec is correct with `--dry-run`
- Check that the file/symbol/pattern actually exists
- Update the check if the codebase has changed

```bash
# Test a check before recording
recon decide --dry-run --check-type file_exists --check-path go.mod

# Test a grep pattern
recon decide --dry-run --check-type grep_pattern --check-pattern "Errorf"
```

## "cannot combine --check-spec with typed check flags"

**Error:** Self-explanatory — you used both `--check-spec` (raw JSON) and typed
flags like `--check-path`.

**Fix:** Use one or the other:

```bash
# Typed flags (recommended)
recon decide "title" --check-type file_exists --check-path go.mod

# OR raw JSON spec
recon decide "title" --check-type file_exists --check-spec '{"path":"go.mod"}'
```

## "unsupported check type"

**Error:**
`unsupported check type "foo"; must be one of: file_exists, symbol_exists, grep_pattern`

**Fix:** Use a valid check type:

- `file_exists` with `--check-path`
- `symbol_exists` with `--check-symbol`
- `grep_pattern` with `--check-pattern` (and optionally `--check-scope`)

## Database Issues

### Reset the database

If the database is corrupted or you want to start fresh:

```bash
rm .recon/recon.db
recon init
recon sync
```

Or using just:

```bash
just db-reset
just init
just sync
```

### Database locked

If you see "database is locked", another process may be using it. Recon opens
SQLite with `MaxOpenConns(1)`, so only one connection is allowed at a time.

**Fix:** Wait for the other process to finish, or check for zombie processes.

## Claude Code Integration Issues

### Hook doesn't fire at session start

1. Check the hook file exists: `ls .claude/hooks/recon-orient.sh`
2. Check it's executable: `chmod +x .claude/hooks/recon-orient.sh`
3. Check settings: `cat .claude/settings.json` — look for `SessionStart` entry
4. Re-install: `recon init --force`

### Agent doesn't know about Recon

1. Check the skill file: `ls .claude/skills/recon/SKILL.md`
2. Check CLAUDE.md has the Recon section
3. Try invoking manually: `/recon` in Claude Code
4. Re-install: `recon init --force`

## Exit Codes

| Code | Meaning                                                      |
| ---- | ------------------------------------------------------------ |
| `0`  | Success                                                      |
| `1`  | General error (database issues, system errors)               |
| `2`  | Validation error (not found, ambiguous, verification failed) |

## Getting Help

- Check `recon --help` or `recon <command> --help` for flag documentation
- See the [Commands Reference](commands.md) for complete details
- File issues at the project repository
