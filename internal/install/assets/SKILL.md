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

## Commands

### `recon find <symbol>`

Structured symbol lookup with dependency info. **Use before grep for Go
symbols** — gives you type, receiver, file, line, dependencies, and body.

```bash
recon find HandleRequest          # exact symbol lookup
recon find --package ./internal/  # list symbols in a package
```

### `recon decide "<text>" --reasoning "<why>" --evidence-summary "<what>" --check-type <type> --check-path <path>`

Record architectural decisions with evidence verification. Decisions are
automatically verified against the codebase.

```bash
recon decide "Use Cobra for CLI" \
  --reasoning "because it's the Go standard" \
  --evidence-summary "go.mod contains cobra" \
  --check-type file_exists --check-path go.mod
```

Check types: `file_exists`, `symbol_exists`, `grep_pattern`

### `recon pattern "<title>" --description "<text>" --evidence-summary "<what>" --check-type <type>`

Record recurring code patterns you observe in the codebase.

```bash
recon pattern "Error wrapping" \
  --description "Use fmt.Errorf with %w for error wrapping" \
  --evidence-summary "grep finds %w usage" \
  --check-type grep_pattern --check-pattern "Errorf"
```

### `recon recall "<query>"`

Search existing decisions and knowledge before making changes. Always check
recall before creating new decisions to avoid duplicates.

```bash
recon recall "error handling"
recon recall "CLI framework"
```

### `recon orient`

Get project context payload (structure, recent activity, decisions). Already
injected by the SessionStart hook, but can be re-run manually.

```bash
recon orient          # text output
recon orient --json   # structured JSON
```

### `recon sync`

Re-index the codebase after major code changes. Run this after large refactors,
adding new packages, or when orient reports stale context.

```bash
recon sync
```

## Workflow Guidance

1. **Check recall before deciding** — search existing knowledge before recording
   new decisions to avoid duplicates
2. **Use find for symbol deps** — `recon find` gives dependency information that
   grep cannot provide
3. **Record significant discoveries** — when you discover an important
   architectural pattern or make a decision, record it with `recon decide` or
   `recon pattern`
4. **Re-sync after major changes** — run `recon sync` after large refactors or
   when adding new packages
