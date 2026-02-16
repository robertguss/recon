# Commands Reference

Complete reference for all Recon CLI commands, flags, and options.

## Global Flags

| Flag          | Default | Description                          |
| ------------- | ------- | ------------------------------------ |
| `--no-prompt` | `false` | Disable interactive prompts globally |

## recon init

Initialize Recon storage in the current Go module.

```bash
recon init
recon init --force
recon init --json
```

Creates the `.recon/` directory, runs database migrations, adds `.recon/` to
`.gitignore`, and installs Claude Code integration files (hook, skill, settings,
CLAUDE.md section).

If Recon is already initialized, prompts before reinstalling unless `--force` is
set.

**Requires:** A `go.mod` file in the project root.

| Flag      | Default | Description                       |
| --------- | ------- | --------------------------------- |
| `--json`  | `false` | Output JSON result                |
| `--force` | `false` | Force reinstall without prompting |

## recon sync

Index Go source code into the Recon database.

```bash
recon sync
recon sync --json
```

Parses all Go files in the module and indexes packages, files, symbols, imports,
and symbol dependencies. Records a fingerprint and git commit hash for staleness
detection.

| Flag     | Default | Description        |
| -------- | ------- | ------------------ |
| `--json` | `false` | Output JSON result |

**Text output example:**

```
Synced 26 files, 312 symbols across 11 packages
Fingerprint: a3f2b1c
Git commit: bb32546 dirty=false
Synced at: 2026-02-16T10:30:00Z
```

## recon orient

Serve startup context for the repository.

```bash
recon orient
recon orient --json
recon orient --json-strict
recon orient --sync
recon orient --auto-sync
```

Builds a structured context payload including project info, architecture (entry
points, dependency flow), summary counts, module heat map, active decisions,
active patterns, and recent file activity.

If the index is stale, orient will prompt to re-sync (in interactive mode),
auto-sync (with `--auto-sync`), or emit a warning.

| Flag            | Default | Description                                            |
| --------------- | ------- | ------------------------------------------------------ |
| `--json`        | `false` | Output JSON result                                     |
| `--json-strict` | `false` | Output JSON only, suppress warnings (implies `--json`) |
| `--sync`        | `false` | Run sync before building context                       |
| `--auto-sync`   | `false` | Automatically sync when stale instead of prompting     |

## recon find

Find exact symbol or list symbols by filter.

```bash
# Exact symbol lookup
recon find HandleRequest
recon find Service.Build              # dot syntax for methods

# Filtered lookup
recon find HandleRequest --package ./internal/api/
recon find HandleRequest --kind func
recon find HandleRequest --file handler.go

# List mode (no symbol argument, uses filters)
recon find --package ./internal/orient/ --limit 20
recon find --kind type

# List all packages
recon find --list-packages
```

### Modes

**Exact mode** — Provide a symbol name as the argument. Returns the symbol's
kind, signature, body, file location, line numbers, and direct dependencies.

**List mode** — Omit the symbol argument and provide filter flags. Returns a
list of matching symbols with their locations.

**Package list mode** — Use `--list-packages` to list all indexed packages with
file and line counts.

### Dot Syntax

Use `Receiver.Method` to find methods on a specific type:

```bash
recon find Service.Build        # finds the Build method on Service
recon find orient.Service.Build # also works with package prefix
```

| Flag               | Default | Description                                                     |
| ------------------ | ------- | --------------------------------------------------------------- |
| `--json`           | `false` | Output JSON result                                              |
| `--no-body`        | `false` | Omit symbol body in text output                                 |
| `--max-body-lines` | `0`     | Maximum body lines in text output (0 = no limit)                |
| `--package`        | `""`    | Filter by package path                                          |
| `--file`           | `""`    | Filter by file path (suffix match)                              |
| `--kind`           | `""`    | Filter by symbol kind: `func`, `method`, `type`, `var`, `const` |
| `--limit`          | `50`    | Maximum symbols in list mode                                    |
| `--list-packages`  | `false` | List all indexed packages                                       |

### Error Responses

**Not found** — Symbol doesn't exist. May include suggestions for similar names.

**Ambiguous** — Multiple symbols match. Lists candidates with their file paths,
packages, and receivers. Use `--package`, `--file`, or `--kind` to disambiguate.

## recon decide

Propose a decision, verify evidence, and auto-promote when checks pass.

```bash
# Record a new decision
recon decide "Use Cobra for CLI" \
  --reasoning "Industry standard CLI framework" \
  --evidence-summary "go.mod contains spf13/cobra" \
  --check-type file_exists --check-path go.mod

# List active decisions
recon decide --list

# Archive a decision
recon decide --delete 3

# Update confidence
recon decide --update 3 --confidence high

# Dry-run a check without recording
recon decide --dry-run --check-type symbol_exists --check-symbol NewService
```

### Decision Lifecycle

1. **Propose** — Title, reasoning, confidence, and evidence check
2. **Verify** — Evidence check runs automatically
3. **Promote** — If verification passes, decision becomes active
4. **Monitor** — Drift detection on subsequent syncs
5. **Update** — Change confidence as understanding evolves
6. **Archive** — Soft-delete when no longer relevant

### Evidence Check Types

| Check Type      | Required Flag     | Description                                    |
| --------------- | ----------------- | ---------------------------------------------- |
| `file_exists`   | `--check-path`    | Verify a file exists at the given path         |
| `symbol_exists` | `--check-symbol`  | Verify a Go symbol exists in the index         |
| `grep_pattern`  | `--check-pattern` | Verify a regex pattern matches in the codebase |

For `grep_pattern`, optionally use `--check-scope` to limit the search to files
matching a glob pattern.

Alternatively, use `--check-spec` with a raw JSON string instead of the typed
flags. You cannot combine `--check-spec` with typed flags.

| Flag                 | Default  | Description                                                |
| -------------------- | -------- | ---------------------------------------------------------- |
| `--reasoning`        | `""`     | Decision reasoning text                                    |
| `--confidence`       | `medium` | Confidence level: `low`, `medium`, `high`                  |
| `--evidence-summary` | `""`     | Evidence summary text                                      |
| `--check-type`       | `""`     | Check type: `file_exists`, `symbol_exists`, `grep_pattern` |
| `--check-spec`       | `""`     | Raw JSON check spec (alternative to typed flags)           |
| `--check-path`       | `""`     | Path for `file_exists` check                               |
| `--check-symbol`     | `""`     | Symbol name for `symbol_exists` check                      |
| `--check-pattern`    | `""`     | Regex pattern for `grep_pattern` check                     |
| `--check-scope`      | `""`     | File glob scope for `grep_pattern` check                   |
| `--json`             | `false`  | Output JSON result                                         |
| `--list`             | `false`  | List active decisions                                      |
| `--delete`           | `0`      | Archive a decision by ID                                   |
| `--update`           | `0`      | Update a decision by ID (requires `--confidence`)          |
| `--dry-run`          | `false`  | Run check only, don't create state                         |

## recon pattern

Propose a code pattern, verify evidence, and auto-promote when checks pass.

```bash
recon pattern "Error wrapping with %%w" \
  --description "All errors wrapped with fmt.Errorf and %%w" \
  --evidence-summary "grep finds consistent %%w usage" \
  --check-type grep_pattern --check-pattern "Errorf.*%%w"
```

Patterns follow the same propose/verify/promote lifecycle as decisions. The
`--evidence-summary` and `--check-type` flags are required.

| Flag                 | Default      | Description                                                |
| -------------------- | ------------ | ---------------------------------------------------------- |
| `--description`      | `""`         | Pattern description text                                   |
| `--example`          | `""`         | Code example demonstrating the pattern                     |
| `--confidence`       | `medium`     | Confidence level: `low`, `medium`, `high`                  |
| `--evidence-summary` | **required** | Evidence summary text                                      |
| `--check-type`       | **required** | Check type: `file_exists`, `symbol_exists`, `grep_pattern` |
| `--check-spec`       | `""`         | Raw JSON check spec                                        |
| `--check-path`       | `""`         | Path for `file_exists` check                               |
| `--check-symbol`     | `""`         | Symbol name for `symbol_exists` check                      |
| `--check-pattern`    | `""`         | Regex for `grep_pattern` check                             |
| `--check-scope`      | `""`         | File glob scope for `grep_pattern` check                   |
| `--json`             | `false`      | Output JSON result                                         |

## recon recall

Search promoted knowledge (decisions and patterns).

```bash
recon recall "error handling"
recon recall "CLI framework" --limit 5
recon recall "testing" --json
```

Uses FTS5 full-text search with Porter stemming, falling back to LIKE queries
when FTS produces no results. Searches across decision titles, reasoning,
evidence summaries, and pattern titles and descriptions.

| Flag      | Default | Description        |
| --------- | ------- | ------------------ |
| `--json`  | `false` | Output JSON result |
| `--limit` | `10`    | Maximum results    |

**Text output example:**

```
- [decision] #1 Use Cobra for CLI [high] drift=ok
  go.mod contains spf13/cobra
- [pattern] #2 Error wrapping with %w [medium] drift=ok
  grep finds consistent %w usage
```

## recon status

Quick health check for Recon state.

```bash
recon status
recon status --json
```

Shows initialization state, last sync time, and counts for files, symbols,
packages, decisions (with drifting count), and patterns.

| Flag     | Default | Description        |
| -------- | ------- | ------------------ |
| `--json` | `false` | Output JSON result |

**Text output example:**

```
Initialized: yes
Last sync: 2026-02-16T10:30:00Z
Files: 26 | Symbols: 312 | Packages: 11
Decisions: 3 (0 drifting) | Patterns: 2
```

## JSON Output

All commands support `--json` for machine-readable output. Successful responses
return the relevant data structure. Errors return a structured envelope:

```json
{
  "error": {
    "code": "not_found",
    "message": "symbol \"Foo\" not found",
    "details": {
      "symbol": "Foo",
      "suggestions": ["FooBar", "FooBaz"]
    }
  }
}
```

### Error Codes

| Code                  | Meaning                                     |
| --------------------- | ------------------------------------------- |
| `not_initialized`     | Database not initialized (run `recon init`) |
| `not_found`           | Symbol, decision, or entity not found       |
| `ambiguous`           | Symbol matches multiple candidates          |
| `invalid_input`       | Invalid flag value or argument              |
| `missing_argument`    | Required argument not provided              |
| `verification_failed` | Evidence check did not pass                 |
| `internal_error`      | Unexpected error                            |

## Exit Codes

| Code | Meaning                                              |
| ---- | ---------------------------------------------------- |
| `0`  | Success                                              |
| `1`  | General error                                        |
| `2`  | Validation error, not found, or verification failure |
