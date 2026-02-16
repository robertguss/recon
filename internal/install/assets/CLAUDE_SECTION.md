## Recon (Code Intelligence)

This project uses [recon](https://github.com/robertguss/recon) for code
intelligence. Recon indexes Go source code into a local database and maintains a
knowledge layer — decisions, patterns, and relationships — that persists across
sessions. Each session builds on what previous sessions learned, rather than
starting from scratch.

A recon orient payload is automatically injected at session start via hook,
giving you project structure, hot modules, and active decisions upfront.

Recon is a two-way knowledge cycle: you **consume** knowledge (orient, find,
recall) and **produce** knowledge (decide, pattern). Recording what you discover
is as important as querying what's already known.

### When to use recon

- **When exploring or understanding the codebase** — `recon find` gives
  structured symbol lookups with dependencies, `--list-packages` shows package
  structure with activity heat, and `recon orient` gives a full project overview
  — all faster and richer than manual file exploration
- **When following established patterns** — `recon pattern --list` shows
  recorded conventions and `recon recall` surfaces how similar problems were
  solved before, so your code matches the project's style
- **When writing tests** — `recon find` shows a symbol's dependencies so you
  know what to mock, and `recon recall` surfaces existing testing patterns and
  conventions
- **Before modifying existing code** — `recon recall` surfaces decisions
  explaining why code is structured the way it is, preventing you from undoing
  intentional design
- **After discovering something significant** — record it with `recon decide` or
  `recon pattern` so future sessions benefit
- **After major code changes** — `recon sync` re-indexes the codebase

### Command reference

Run `recon <command> --help` for flags and usage. Use the `/recon` skill for the
full reference. All commands support `--json` for structured output.

Commands: `init`, `sync`, `orient`, `find`, `decide`, `pattern`, `recall`,
`status`, `edges`, `version`
