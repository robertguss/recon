# Documentation Changelog

## 2026-02-16 — Initial Documentation

Created comprehensive documentation for the Recon project.

### Created

**User documentation:**

- `README.md` — Project overview with progressive disclosure
- `docs/users/getting-started.md` — Installation, setup, first workflow
- `docs/users/commands.md` — Complete CLI reference (all commands, flags,
  options)
- `docs/users/workflows.md` — Decision lifecycle, patterns, recall, agent
  workflows
- `docs/users/claude-code-integration.md` — Hook, skill, settings, customization
- `docs/users/troubleshooting.md` — Common errors and solutions

**Developer documentation:**

- `docs/developers/architecture.md` — System design with Mermaid diagrams
- `docs/developers/schema.md` — Database tables, relationships, FTS5, migrations
- `docs/developers/services.md` — Domain service APIs and patterns
- `docs/developers/testing.md` — Test strategy, sqlmock vs real SQLite
- `docs/developers/contributing.md` — Dev setup, conventions, PR workflow

**Architecture Decision Records:**

- `docs/developers/adr/001-go-and-sqlite.md` — Language and storage choice
- `docs/developers/adr/002-agent-first-design.md` — Primary audience decision
- `docs/developers/adr/003-function-var-injection.md` — Testability pattern
- `docs/developers/adr/004-fts5-search-strategy.md` — Full-text search approach

### Updated

- `AGENTS.md` — Rewritten to reflect actual project state (was outdated)

### Documentation Health

- Files created: 15
- Total estimated word count: ~9,500
- Coverage: All 8 CLI commands, all 6 services, full database schema, 4 ADRs
