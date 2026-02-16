# Getting Started

This guide walks you through installing Recon, initializing it in a Go project,
and running your first workflow.

## Prerequisites

- **Go 1.26+** installed and on your PATH
- A **Go module** — Recon requires a `go.mod` file in the project root

## Installation

### From source

```bash
go install github.com/robertguss/recon/cmd/recon@latest
```

### Build locally

```bash
git clone https://github.com/robertguss/recon.git
cd recon
just build        # builds to ./bin/recon
just install      # installs to $GOPATH/bin
```

Verify the installation:

```bash
recon --help
```

## Initialize a Project

Navigate to your Go project and run:

```bash
cd your-go-project
recon init
```

This creates:

- `.recon/recon.db` — SQLite database for indexed code and knowledge
- `.recon/` is added to `.gitignore` automatically
- Claude Code integration files (hook, skill, settings) if applicable

If Recon is already initialized, it will prompt before reinstalling. Use
`--force` to skip the prompt, or `--no-prompt` to exit with an error instead.

## Index Your Codebase

```bash
recon sync
```

Recon parses all Go files in the module and indexes:

- **Packages** — path, name, file count, line count
- **Files** — path, language, line count, content hash
- **Symbols** — functions, methods, types, variables, constants with full
  signatures and bodies
- **Imports** — file-to-package import relationships
- **Symbol dependencies** — which symbols reference which other symbols

Example output:

```
Synced 26 files, 312 symbols across 11 packages
Fingerprint: a3f2b1c
Git commit: bb32546 dirty=false
Synced at: 2026-02-16T10:30:00Z
```

Re-run `recon sync` any time you make significant code changes. The `orient`
command will tell you when the index is stale.

## Get Project Context

```bash
recon orient
```

This produces a structured summary of your project:

- **Project info** — module name, language
- **Architecture** — entry points, dependency flow
- **Summary** — file count, symbol count, package count, decision count
- **Module heat map** — which packages have recent activity
- **Active decisions** — recorded architectural decisions and their drift status
- **Active patterns** — recorded code patterns
- **Recent activity** — recently modified files

Use `--json` for machine-readable output (useful for agents):

```bash
recon orient --json
```

Use `--sync` to re-index before generating context:

```bash
recon orient --sync
```

## Find Symbols

```bash
recon find HandleRequest
```

Returns the symbol's kind, signature, body, file location, line numbers, and
direct dependencies. This is more precise than grep — it understands Go syntax.

Filter when multiple matches exist:

```bash
recon find HandleRequest --package ./internal/api/
recon find HandleRequest --kind func
recon find HandleRequest --file handler.go
```

Use dot syntax for methods:

```bash
recon find Service.Build
```

List symbols in a package:

```bash
recon find --package ./internal/orient/ --limit 20
```

List all indexed packages:

```bash
recon find --list-packages
```

## Record a Decision

Before recording a new decision, check if a related one already exists:

```bash
recon recall "CLI framework"
```

Then record:

```bash
recon decide "Use Cobra for CLI framework" \
  --reasoning "Industry standard, good subcommand support, flag parsing" \
  --evidence-summary "go.mod contains spf13/cobra" \
  --check-type file_exists --check-path go.mod
```

Recon verifies the evidence check and, if it passes, promotes the decision to
active status automatically.

List active decisions:

```bash
recon decide --list
```

## Record a Pattern

```bash
recon pattern "Function-var injection for testing" \
  --description "Package-level var funcs replaced in tests for isolation" \
  --evidence-summary "grep finds var declarations with function signatures" \
  --check-type grep_pattern --check-pattern "var.*=.*func"
```

## Recall Knowledge

Search across all decisions and patterns:

```bash
recon recall "error handling"
recon recall "testing pattern"
```

## Quick Health Check

```bash
recon status
```

Shows the current state of the Recon database — whether it's initialized,
synced, and healthy.

## What's Next

- [Commands Reference](commands.md) — Full details on every command and flag
- [Workflows](workflows.md) — Decision lifecycle, pattern detection, advanced
  usage
- [Claude Code Integration](claude-code-integration.md) — Set up the AI agent
  workflow
- [Troubleshooting](troubleshooting.md) — Common errors and how to fix them
