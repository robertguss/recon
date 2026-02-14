# Build Recon Milestone 1 CLI (Init, Sync, Orient, Find, Decide, Recall)

This ExecPlan is a living document. The sections `Progress`, `Surprises & Discoveries`, `Decision Log`, and `Outcomes & Retrospective` must be kept up to date as work proceeds.

This plan must be maintained in accordance with `.agent/PLANS.md`.

## Purpose / Big Picture

After this change, a developer or coding agent can initialize `recon` inside a Go repository, index project code into a local SQLite database, get a startup context summary, extract an exact symbol with direct dependencies, store verified design decisions, and query promoted knowledge. This creates a working hookless loop for agent collaboration: run `recon sync`, run `recon orient`, do work, run `recon decide`, and run `recon recall`. The behavior is observable from CLI output and JSON responses in the same repository where work is happening.

## Progress

- [x] (2026-02-14 00:00Z) Reviewed `brainstorms/project-rethink/_index.md`, `brainstorms/project-rethink/project-rethink-v1.md`, `brainstorms/project-rethink/project-rethink-v2.md`, and `brainstorms/project-rethink/project-rethink-v3.md`; captured unresolved decisions and converted them into explicit product constraints for milestone 1.
- [x] (2026-02-14 00:00Z) Finalized milestone 1 scope with user: project name `recon`, command set (`init`, `sync`, `orient`, `find`, `decide`, `recall`), auto-promote after verification, local `.recon/recon.db`, Go 1.26, Cobra, `golang-migrate`, `modernc.org/sqlite`, and hookless stale detection in `orient`.
- [x] (2026-02-14 00:20Z) Scaffolded Go module `github.com/robertguss/recon` with Cobra root command and six milestone 1 subcommands.
- [x] (2026-02-14 00:23Z) Implemented SQLite connection layer, migration bootstrap, and migration files for code index entities, knowledge entities, proposals, evidence, search index, and sync state.
- [x] (2026-02-14 00:24Z) Implemented `recon init` to create `.recon/`, create/open `.recon/recon.db`, run migrations, and ensure `.recon/recon.db` is ignored by git.
- [x] (2026-02-14 00:28Z) Implemented `recon sync` for single-threaded Go indexing of module `.go` files with default exclusions (`vendor/`, `testdata/`, hidden directories, generated files, and `_test.go` files).
- [x] (2026-02-14 00:30Z) Implemented canonical `orient` data assembler with stale detection and dual renderers (`text` and `--json`) and stale warning behavior for non-interactive usage.
- [x] (2026-02-14 00:33Z) Implemented exact-match `find` with deterministic ambiguity handling and direct dependency extraction.
- [x] (2026-02-14 00:35Z) Implemented `decide` with immediate verification checks (`grep_pattern`, `symbol_exists`, `file_exists`), auto-promotion on pass, and pending proposals on failure.
- [x] (2026-02-14 00:36Z) Implemented knowledge-only `recall` using SQLite FTS with SQL `LIKE` fallback and structured JSON output.
- [x] (2026-02-14 00:37Z) Added module dependency lock via `go mod tidy` and generated `go.sum`.
- [ ] Add tests for each command and core storage/indexing paths, then validate milestone 1 behavior end-to-end from a clean repository checkout.
- [ ] Update this plan during implementation with timestamps, discoveries, decisions, and outcomes after each milestone stop.

## Surprises & Discoveries

- Observation: The source brainstorms contain a core governance contradiction that would have caused architecture churn during implementation.
  Evidence: `project-rethink-v2.md` states a human approval gate, while `project-rethink-v3.md` states auto-promotion after verification. The user confirmed milestone 1 must follow the v3 model.
- Observation: Hook-based automation was treated as central in brainstorms, but this environment does not guarantee hook execution.
  Evidence: User explicitly raised hook availability and requested a hookless-safe flow with stale checks inside `orient`.
- Observation: A strict FTS-only query path can fail for malformed user search syntax, which would make `recall` brittle for agents.
  Evidence: Implementation includes fallback query behavior (`LIKE`) so result retrieval still works when FTS parsing rejects a query string.

## Decision Log

- Decision: Milestone 1 command set is fixed to `init`, `sync`, `orient`, `find`, `decide`, `recall`.
  Rationale: This is the smallest vertical slice that proves recon’s differentiator end-to-end.
  Date/Author: 2026-02-14 / Robert + Codex
- Decision: Knowledge lifecycle is auto-promote after verification; no human approval gate in milestone 1.
  Rationale: This is the final product direction from v3 and avoids interactive friction.
  Date/Author: 2026-02-14 / Robert
- Decision: Recon runs hookless by default, with `orient` stale detection and optional prompt behavior.
  Rationale: Hooks are not guaranteed across agent environments; core reliability must not depend on hooks.
  Date/Author: 2026-02-14 / Robert + Codex
- Decision: `orient` in non-interactive mode warns on stale index and still returns output.
  Rationale: Agents need usable output and structured status instead of hard failure.
  Date/Author: 2026-02-14 / Robert
- Decision: Evidence requires at least one runnable check to promote; summary-only entries cannot auto-promote.
  Rationale: Auto-promotion without verification would quickly erode trust in the knowledge graph.
  Date/Author: 2026-02-14 / Robert + Codex
- Decision: MVP index scope is Go files in current module, excluding `vendor/`, `testdata/`, hidden directories, generated files, and `_test.go`.
  Rationale: Keeps indexing deterministic, fast, and focused on production code.
  Date/Author: 2026-02-14 / Robert
- Decision: Agent-friendly output in milestone 1 is `text` plus `--json`; `ndjson` deferred.
  Rationale: Stable JSON contracts are required now; streaming formats add scope without core value yet.
  Date/Author: 2026-02-14 / Robert + Codex
- Decision: `orient --json` is canonical; text output is a renderer of canonical data.
  Rationale: Prevents divergence between human and agent modes.
  Date/Author: 2026-02-14 / Robert
- Decision: CLI framework is Cobra.
  Rationale: Multi-command ergonomics and growth path justify dependency cost.
  Date/Author: 2026-02-14 / Robert
- Decision: Database migration tool is `golang-migrate`.
  Rationale: Mature, existing tool preferred over custom migration runner.
  Date/Author: 2026-02-14 / Robert
- Decision: SQLite driver is `modernc.org/sqlite`.
  Rationale: Pure-Go portability without CGO friction.
  Date/Author: 2026-02-14 / Robert + Codex
- Decision: Milestone 1 durability is SQLite-only; snapshot export/import deferred to milestone 2.
  Rationale: Reduces initial complexity and accelerates first working loop.
  Date/Author: 2026-02-14 / Robert + Codex
- Decision: `decide` uses explicit flags for evidence input, not interactive prompts.
  Rationale: Agents and scripts require deterministic non-interactive invocation.
  Date/Author: 2026-02-14 / Robert
- Decision: `find` default match mode is exact symbol match with ambiguity suggestions.
  Rationale: Deterministic behavior is safer for automation than fuzzy matching.
  Date/Author: 2026-02-14 / Robert + Codex
- Decision: `recall` in milestone 1 searches promoted knowledge only.
  Rationale: Keeps separation of concerns between memory search (`recall`) and code extraction (`find`).
  Date/Author: 2026-02-14 / Robert + Codex
- Decision: `orient` must enforce a hard output budget in milestone 1 (about 1200 tokens in text mode, deterministic field caps in JSON).
  Rationale: Protects downstream context windows and keeps behavior predictable.
  Date/Author: 2026-02-14 / Robert + Codex
- Decision: Module path is `github.com/robertguss/recon`; repository-local implementation in this repo root with Go best practices.
  Rationale: Aligns with user naming and deployment intent.
  Date/Author: 2026-02-14 / Robert
- Decision: `recon init` updates `.gitignore` to ignore `.recon/recon.db`.
  Rationale: Keep runtime state local and untracked while preserving future tracked metadata options under `.recon/`.
  Date/Author: 2026-02-14 / Robert
- Decision: `recon sync` performs full index-table rebuild per run in milestone 1 instead of incremental per-file diff updates.
  Rationale: Full rebuild is simpler and less error-prone for first release; incremental sync can be added in milestone 2.
  Date/Author: 2026-02-14 / Codex
- Decision: `recall` uses FTS primary query with a `LIKE` fallback for resilience.
  Rationale: Preserves utility when FTS query parsing rejects agent/user input formatting.
  Date/Author: 2026-02-14 / Codex

## Outcomes & Retrospective

Current implementation outcome: milestone 1 command and service scaffolding is in place with functional storage, indexing, context generation, symbol extraction, decision verification/promotion, and recall paths. Remaining gaps are validation depth (tests and command acceptance runs), output budget hardening to precise token limits, and potential schema/index adjustments discovered during live dogfooding.

## Context and Orientation

This repository currently contains strategy documents and does not yet contain a Go implementation. The source direction lives under `brainstorms/project-rethink/`. The new implementation should be added in this repository root using standard Go project layout:

- `cmd/recon/` for CLI entrypoint.
- `internal/cli/` for Cobra command construction and output rendering.
- `internal/db/` for connection handling and migrations integration.
- `internal/db/migrations/` for SQL migration files used by `golang-migrate`.
- `internal/index/` for Go AST parsing and index persistence.
- `internal/orient/` for canonical orient payload assembly and text rendering.
- `internal/find/` for exact symbol resolution and direct dependency extraction.
- `internal/knowledge/` for decisions, proposals, evidence verification, and promotion logic.
- `internal/recall/` for FTS-backed knowledge query behavior.

Important plain-language definitions used in this plan:

- Canonical orient payload: a single in-memory data structure that represents `orient` results. JSON output serializes this structure directly. Text output is formatted from this structure.
- Direct dependency extraction: for one resolved symbol, return only the project symbols it directly references, not a recursive transitive graph.
- Evidence check: a runnable validation rule stored with a proposal. In milestone 1, checks are `grep_pattern`, `symbol_exists`, and `file_exists`.
- Pending proposal: a stored proposal that failed verification and therefore was not promoted into permanent knowledge.
- Promoted knowledge: verified decision records that can be returned by `recall`.

## Plan of Work

Milestone 1 begins by scaffolding the module and command shell so each command has a stable invocation path. The first concrete code should establish `go.mod`, Cobra root wiring, and subcommand stubs. Once command entrypoints exist, implement database bootstrap and migrations so all commands share the same storage contract.

After storage is stable, implement `recon init` and `recon sync`. `init` must create `.recon/recon.db`, run migrations, and ensure git ignore handling for the database file. `sync` must parse eligible Go files using standard library parsing packages and upsert packages, files, symbols, imports, and sync metadata in one consistent run. Keep the implementation single-threaded in milestone 1 for easier correctness.

With index data available, implement `orient` as a canonical JSON payload builder plus a text renderer. Before payload assembly, `orient` must run a fast stale probe by comparing current git state and current file fingerprints to the stored sync state. In interactive terminals, prompt to run `sync` when stale; in non-interactive mode return status plus warning while still emitting payload.

Next, implement `find` and `recall`. `find` resolves exact symbol names from indexed symbols, returns function/type content with source location and direct in-project dependencies, and reports ambiguity suggestions deterministically. `recall` queries only promoted decision knowledge using FTS and structured filters.

Finally, implement `decide` with immediate verification and auto-promotion. `decide` should write a proposal, execute selected checks, record evidence summary and check result, and either promote on success or keep pending on failure with failure detail. Ensure promoted records become searchable by `recall`.

## Concrete Steps

Run each command from repository root: `/Users/robertguss/Projects/startups/recon`.

1. Scaffold module and dependencies.

   go mod init github.com/robertguss/recon
   go get github.com/spf13/cobra@latest
   go get github.com/golang-migrate/migrate/v4@latest
   go get modernc.org/sqlite@latest

   Expected: `go.mod` created with Go 1.26 and required dependencies.

2. Create command entrypoint and command packages.

   mkdir -p cmd/recon internal/cli internal/db internal/db/migrations internal/index internal/orient internal/find internal/knowledge internal/recall

   Implement root Cobra command and subcommands:
   - `init`
   - `sync`
   - `orient`
   - `find`
   - `decide`
   - `recall`

3. Add SQL migrations and migration bootstrap.

   Create migration files in `internal/db/migrations` for:
   - packages/files/symbols/imports
   - decisions/evidence/proposals
   - sessions/session_files
   - search_index (fts5)
   - sync_state

   Implement migration runner in `internal/db` invoked by `recon init`.

4. Implement `recon init`.

   Behavior:
   - Create `.recon/` if missing.
   - Create/open `.recon/recon.db`.
   - Run migrations to latest.
   - Ensure `.gitignore` contains `.recon/recon.db` (append only if missing).

5. Implement `recon sync`.

   Behavior:
   - Resolve module root from `go.mod`.
   - Enumerate eligible `.go` files with exclusions.
   - Parse files with `go/parser` and `go/ast`.
   - Persist files, symbols, imports, packages.
   - Update `sync_state` with commit hash, dirty flag, file count, timestamp, and fingerprint.

6. Implement canonical `orient`.

   Behavior:
   - Compute staleness quickly.
   - For interactive stale case, prompt to run `sync`.
   - Build capped canonical payload.
   - Render JSON for `--json`.
   - Render plain text for default mode from canonical payload.

7. Implement exact `find`.

   Behavior:
   - Exact symbol lookup.
   - If zero matches, return not-found with close suggestions.
   - If multiple exact hits by package context, return deterministic ambiguity list.
   - Return symbol body/signature/location and direct project dependencies.

8. Implement `decide`.

   Behavior:
   - Accept title, reasoning, confidence, evidence summary, check type, and check spec via flags.
   - Insert proposal.
   - Execute check.
   - On success: promote to decisions and persist evidence with status `ok`.
   - On failure: keep proposal `pending`, store failure detail.

9. Implement `recall`.

   Behavior:
   - Query promoted decisions and evidence summaries with FTS.
   - Support text and JSON output.
   - Exclude pending or failed proposals.

10. Add tests and run acceptance commands.

    go test ./...
    recon init
    recon sync
    recon orient --json
    recon find <known_symbol>
    recon decide "<title>" --reasoning "<reason>" --evidence-summary "<summary>" --check-type file_exists --check-spec '{"path":"go.mod"}'
    recon recall "<keyword>"

    Expected:
    - Tests pass.
    - `orient` returns canonical JSON with stale state metadata.
    - `decide` successful run promotes entry and `recall` returns it.

## Validation and Acceptance

Acceptance is behavioral and command-visible.

- A new repository user can run `recon init` once and get `.recon/recon.db` plus migrated schema.
- After `recon sync`, the database contains indexed Go files and symbols from the module, excluding configured paths.
- `recon orient --json` returns canonical structured output including freshness metadata and capped result sizes.
- `recon orient` returns plain text generated from the same canonical object.
- `recon find <symbol>` returns exact symbol content and direct dependencies; ambiguity and not-found behavior are deterministic.
- `recon decide ...` with a passing check produces promoted knowledge; with a failing check produces pending proposal.
- `recon recall <query>` returns promoted knowledge and excludes pending items.
- Non-interactive stale orient calls do not hard-fail; they emit warning/status and payload.

## Idempotence and Recovery

`recon init` must be safely re-runnable. Re-running should not destroy existing data and should only apply missing migrations. `recon sync` should be repeatable and converge on current source state without duplicating indexed entities. `.gitignore` modification must be append-if-missing only. If a migration fails mid-run, fix migration state and rerun `recon init` to converge. If `.recon/recon.db` is deleted during milestone 1, rerun `recon init` and `recon sync` to rebuild index and schema; milestone 1 does not include snapshot restore.

## Artifacts and Notes

Expected JSON shape for `orient --json` (illustrative, not exhaustive):

    {
      "project": {
        "name": "recon",
        "language": "go"
      },
      "freshness": {
        "is_stale": false,
        "last_sync_at": "2026-02-14T00:00:00Z",
        "reason": ""
      },
      "architecture": {
        "entrypoints": ["cmd/recon/main.go"],
        "module_summary": []
      },
      "active_knowledge": {
        "decisions": []
      }
    }

Expected stale warning behavior in non-interactive mode:

    $ recon orient --json
    {
      "freshness": {
        "is_stale": true,
        "reason": "git_head_changed_since_last_sync"
      },
      ...
    }

## Interfaces and Dependencies

Use these dependencies:

- `github.com/spf13/cobra` for CLI command graph and flags.
- `github.com/golang-migrate/migrate/v4` for SQL migrations.
- `modernc.org/sqlite` as database/sql driver.

Define stable internal interfaces so command handlers remain thin:

- In `internal/db/store.go`, define:

  type Store interface {
  BeginTx(ctx context.Context) (*sql.Tx, error)
  ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
  QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
  QueryRowContext(ctx context.Context, query string, args ...any) \*sql.Row
  Close() error
  }

- In `internal/index/service.go`, define:

  type Service interface {
  Sync(ctx context.Context, moduleRoot string) (SyncResult, error)
  }

- In `internal/orient/service.go`, define:

  type Service interface {
  Build(ctx context.Context, opts BuildOptions) (Payload, error)
  }

- In `internal/find/service.go`, define:

  type Service interface {
  FindExact(ctx context.Context, symbol string) (Result, error)
  }

- In `internal/knowledge/service.go`, define:

  type Service interface {
  ProposeAndVerifyDecision(ctx context.Context, in ProposeDecisionInput) (ProposeDecisionResult, error)
  }

- In `internal/recall/service.go`, define:

  type Service interface {
  Recall(ctx context.Context, query string, opts RecallOptions) (RecallResult, error)
  }

Plan revision note: Created this initial ExecPlan to convert brainstorm direction into a self-contained, implementation-ready milestone document under `docs/plans/`, as requested, using `.agent/PLANS.md` structure and constraints.

Plan revision note: Updated the living sections after implementing milestone 1 code scaffolding and core command/service behavior so the plan accurately reflects completed work, current risks, and implementation decisions.
