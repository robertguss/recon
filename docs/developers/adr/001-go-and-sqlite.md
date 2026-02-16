# ADR-001: Go and SQLite for Implementation

## Status

Accepted (2026-02-13)

## Context

Recon needed a language and storage backend. The project started as two
overlapping efforts: repo-scout (Rust, AST parsing) and Cortex (Go, session
memory). A decision was needed on whether to consolidate and which stack to use.

Key requirements:

- Single binary distribution (no runtime dependencies)
- Local-only storage (no server, no network)
- Fast enough for interactive CLI use
- Must be dogfood-able (Recon indexes Go repos first)

## Decision

**Go** for the implementation language. **SQLite** (via `modernc.org/sqlite`,
pure-Go driver) for storage.

## Rationale

### Go over Rust

- Recon indexes Go code — writing it in Go means dogfooding the indexer on
  itself
- Go's `go/ast` and `go/parser` provide first-class AST support for Go code
- Faster iteration than Rust for a CLI tool where nanosecond performance isn't
  critical
- Cobra provides a mature CLI framework with subcommands, flags, and help
  generation

### SQLite over alternatives

- Single-file database — no server, no configuration, no network
- Portable — `.recon/recon.db` is easy to delete, back up, or inspect
- FTS5 — built-in full-text search with Porter stemming for the recall system
- `modernc.org/sqlite` — pure-Go driver, no CGO, builds anywhere Go builds
- SQL is well-understood — services can write queries directly without an ORM

### Alternatives considered

- **Rust + custom file format** — Higher performance but slower development,
  can't dogfood Go indexing
- **Go + Markdown files** — Simpler but no query capability, no full-text
  search, harder to maintain consistency
- **Go + embedded key-value store (bbolt, badger)** — No relational queries, no
  FTS, more code for basic lookups
- **Go + `mattn/go-sqlite3`** — Requires CGO, complicates cross-compilation

## Consequences

- All code is Go, all tools are Go ecosystem (go test, go build, go vet)
- Single binary with zero runtime dependencies
- Slightly slower SQLite than CGO version, but acceptable for local CLI
- Database schema changes require golang-migrate migrations
- No external API or web UI — purely local CLI
