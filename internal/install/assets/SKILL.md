---
name: recon
description:
  Code intelligence and knowledge CLI for Go repositories. Use when exploring Go
  code, finding symbols, recording architectural decisions, detecting patterns,
  or recalling prior knowledge about this codebase.
user-invocable: true
---

# Recon — Code Intelligence CLI

Recon indexes your Go codebase and provides structured lookup, decision
recording, and pattern detection. Use it whenever you need accurate symbol
information, want to record or recall architectural decisions, or need to
understand project structure.

Run `recon <command> --help` for the most up-to-date flags and usage for any
command. All commands support `--json` for structured output.

Global flag: `--no-prompt` disables interactive prompts.

## Commands

### `recon init`

Initialize recon storage in the current Go module. Creates `.recon/` directory,
runs schema migrations, and installs Claude Code integration (hook, skill,
CLAUDE.md section, settings).

```bash
recon init            # interactive — prompts if already initialized
recon init --force    # reinstall without prompting
```

Flags:

- `--json` — output JSON
- `--force` — force reinstall without prompting

### `recon sync`

Index Go source code into the recon database. Parses all `.go` files, extracts
packages, symbols, imports, and dependencies. Run after code changes to keep the
index current.

```bash
recon sync
```

Flags:

- `--json` — output JSON (includes file/symbol/package counts, diff,
  fingerprint)

### `recon orient`

Serve startup context for the repository — project structure, hot modules,
active decisions, freshness status. Already injected at session start via hook,
but can be re-run manually.

```bash
recon orient              # text output
recon orient --json       # structured JSON
recon orient --sync       # run sync first, then orient
recon orient --auto-sync  # auto-sync if stale instead of prompting
```

Flags:

- `--json` — output JSON
- `--json-strict` — JSON only, suppresses stderr warnings (implies `--json`)
- `--sync` — run sync before building orient context
- `--auto-sync` — automatically sync when stale instead of prompting

### `recon find [<symbol>]`

Structured symbol lookup with dependency info. Returns kind, receiver, file,
line range, body, and direct dependencies.

Two modes:

1. **Exact lookup** — pass a symbol name to get full details
2. **List mode** — use filter flags without a symbol to browse

```bash
# Exact symbol lookup (returns body + dependencies)
recon find HandleRequest
recon find HandleRequest --package internal/cli
recon find HandleRequest --no-body

# List mode (browse symbols by filter)
recon find --kind func                          # all functions
recon find --kind type --package internal/db    # types in a package
recon find --file service.go                    # symbols in a file
recon find --kind func --limit 100              # increase result limit

# Package exploration
recon find --list-packages                      # all packages with line counts and heat
```

Flags:

- `--json` — output JSON (includes knowledge links from edges)
- `--package <path>` — filter by package path
- `--file <filename>` — filter by filename (exact match)
- `--kind <kind>` — filter by symbol kind: `func`, `method`, `type`, `var`,
  `const`
- `--limit <n>` — max symbols in list mode (default: 50)
- `--list-packages` — list all indexed packages with file counts, line counts,
  and activity heat
- `--no-body` — omit symbol body in text output
- `--max-body-lines <n>` — truncate body to N lines (0 = no limit)

### `recon decide [<title>]`

Record architectural decisions with evidence verification. Decisions are
proposed, verified against the codebase, and auto-promoted when checks pass.

```bash
# Create a decision with evidence
recon decide "Use Cobra for CLI" \
  --reasoning "standard Go CLI framework with broad ecosystem support" \
  --evidence-summary "go.mod contains cobra dependency" \
  --check-type file_exists --check-path go.mod

# Using grep_pattern check
recon decide "ExitError is the standard error type" \
  --reasoning "all CLI commands return ExitError for controlled exit codes" \
  --evidence-summary "ExitError used across CLI package" \
  --check-type grep_pattern --check-pattern "ExitError" --check-scope "*.go"

# Using symbol_exists check
recon decide "Service pattern for domain logic" \
  --reasoning "each domain package exposes a Service struct" \
  --evidence-summary "Service type exists in find package" \
  --check-type symbol_exists --check-symbol "Service"

# With edge creation (links decision to affected code)
recon decide "Title" --reasoning "..." --evidence-summary "..." \
  --check-type file_exists --check-path go.mod \
  --affects internal/cli --affects internal/db

# List, update, delete
recon decide --list                              # list active decisions with drift status
recon decide --delete 3                          # archive (soft-delete) decision #3
recon decide --update 3 --confidence high        # update confidence level
recon decide --dry-run --check-type grep_pattern --check-pattern "ExitError"  # test a check without creating state
```

Flags:

- `--reasoning <text>` — why this decision was made
- `--confidence <level>` — `low`, `medium` (default), `high`
- `--evidence-summary <text>` — summary of supporting evidence
- `--check-type <type>` — verification type: `file_exists`, `symbol_exists`,
  `grep_pattern`
- `--check-path <path>` — for `file_exists`: the file path to check
- `--check-symbol <name>` — for `symbol_exists`: the symbol name to check
- `--check-pattern <regex>` — for `grep_pattern`: regex pattern to search for
- `--check-scope <glob>` — for `grep_pattern`: optional file glob scope
- `--check-spec <json>` — raw JSON check spec (alternative to typed flags)
- `--affects <ref>` — package/file/symbol this decision affects (creates edges,
  repeatable)
- `--list` — list active decisions
- `--delete <id>` — archive a decision by ID
- `--update <id>` — update a decision by ID (use with `--confidence`)
- `--dry-run` — run verification check only, without creating any state
- `--json` — output JSON

### `recon pattern [<title>]`

Record recurring code patterns observed in the codebase. Works like `decide` but
for patterns rather than one-off decisions.

```bash
# Create a pattern
recon pattern "Function-var injection for testability" \
  --reasoning "override package-level vars in tests for isolation" \
  --example "var osGetwd = os.Getwd  // overridden in tests" \
  --evidence-summary "pattern found across CLI package" \
  --check-type grep_pattern --check-pattern "var .+ = " --check-scope "*.go"

# With edge creation
recon pattern "Title" --reasoning "..." --evidence-summary "..." \
  --check-type file_exists --check-path go.mod \
  --affects internal/cli

# List and delete
recon pattern --list                             # list active patterns with drift status
recon pattern --delete 2                         # archive (soft-delete) pattern #2
```

Flags:

- `--reasoning <text>` — why this pattern matters
- `--example <text>` — code example demonstrating the pattern
- `--confidence <level>` — `low`, `medium` (default), `high`
- `--evidence-summary <text>` — summary of supporting evidence
- `--check-type <type>` — verification type: `file_exists`, `symbol_exists`,
  `grep_pattern`
- `--check-path <path>` — for `file_exists`: the file path to check
- `--check-symbol <name>` — for `symbol_exists`: the symbol name to check
- `--check-pattern <regex>` — for `grep_pattern`: regex pattern to search for
- `--check-scope <glob>` — for `grep_pattern`: optional file glob scope
- `--check-spec <json>` — raw JSON check spec (alternative to typed flags)
- `--affects <ref>` — package/file/symbol this pattern affects (creates edges,
  repeatable)
- `--list` — list active patterns
- `--delete <id>` — archive a pattern by ID
- `--json` — output JSON

### `recon recall <query>`

Search promoted decisions and patterns using full-text search. Always check
recall before creating new decisions to avoid duplicates.

```bash
recon recall "error handling"       # search for relevant knowledge
recon recall "testing" --limit 20   # increase result limit
recon recall "CLI" --json           # structured output with edges
```

Flags:

- `--json` — output JSON (includes connected edges)
- `--limit <n>` — max results (default: 10)

### `recon status`

Quick health check showing initialization state, last sync time, and counts for
files, symbols, packages, decisions, and patterns.

```bash
recon status
recon status --json
```

Flags:

- `--json` — output JSON

### `recon edges`

Manage knowledge graph edges that link decisions and patterns to code entities
(packages, files, symbols) and to each other.

```bash
# Create an edge
recon edges --create \
  --from "decision:1" --to "package:internal/cli" \
  --relation affects

# Query edges
recon edges --list                               # list all edges
recon edges --from "decision:1"                  # edges from a specific entity
recon edges --to "package:internal/cli"           # edges pointing to a target

# Delete an edge
recon edges --delete 5

# Custom relation and confidence
recon edges --create \
  --from "pattern:2" --to "decision:1" \
  --relation reinforces --confidence medium --source auto
```

Entity ref format: `type:id` for decisions/patterns (e.g., `decision:1`,
`pattern:3`), `type:ref` for code entities (e.g., `package:internal/cli`,
`file:service.go`, `symbol:HandleRequest`).

Relations: `affects`, `evidenced_by`, `supersedes`, `contradicts`, `related`,
`reinforces`

Flags:

- `--create` — create a new edge (requires `--from` and `--to`)
- `--from <ref>` — source entity ref
- `--to <ref>` — target entity ref
- `--relation <rel>` — edge relation (default: `affects`)
- `--source <src>` — edge source: `manual` (default), `auto`
- `--confidence <level>` — `low`, `medium`, `high` (default)
- `--list` — list all edges
- `--delete <id>` — delete an edge by ID
- `--json` — output JSON

### `recon version`

Print the recon version.

```bash
recon version
```

## Workflow Guidance

1. **Check recall before deciding** — search existing knowledge before recording
   new decisions to avoid duplicates
2. **Record significant discoveries** — when you discover an important
   architectural pattern or make a decision, record it with `recon decide` or
   `recon pattern`
3. **Use find for structured exploration** — `recon find` gives dependency
   information and symbol context that file reads alone cannot provide
4. **Follow existing patterns** — check `recon pattern --list` and
   `recon recall` before writing new code to match established conventions
5. **Re-sync after major changes** — run `recon sync` after large refactors or
   when adding new packages
6. **Use --help for discovery** — run `recon <command> --help` for the latest
   flags if you're unsure about a command's options
