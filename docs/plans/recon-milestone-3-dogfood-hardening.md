# Deliver Recon Milestone 3 dogfood hardening for large-repo reliability

This ExecPlan is a living document. The sections `Progress`,
`Surprises & Discoveries`, `Decision Log`, and `Outcomes & Retrospective` must
be kept up to date as work proceeds.

This plan must be maintained in accordance with `.agent/PLANS.md`.

This plan builds on `docs/plans/recon-milestone-1.md` and
`docs/plans/recon-milestone-2-cli-ergonomics.md`. Milestones 1 and 2 delivered
an end-to-end CLI with machine-friendly ergonomics. Milestone 3 hardens
contracts and result quality based on real dogfooding in both
`/Users/robertguss/Projects/startups/recon` and
`/Users/robertguss/Projects/startups/cortex_code`.

## Purpose / Big Picture

After this change, `recon` will behave more predictably for agents and humans
on real repositories, especially larger codebases. A user will get consistent
JSON error envelopes across commands, reliable validation for numeric and typed
flags, more precise `find` dependency output, and cleaner decision semantics for
invalid input.

The result is observable by running `recon` against real repositories and
verifying that error shapes, exit codes, and result relevance remain stable in
both happy and failure paths. Implementation must follow strict TDD (red,
green, refactor) and finish with 100% test coverage across all packages.

## Progress

- [x] (2026-02-14 15:45Z) Captured dogfood findings from both `recon` and
      `cortex_code` and defined Milestone 3 hardening scope.
- [x] (2026-02-14 16:33Z) Implemented JSON-mode error-envelope consistency for
      all DB-backed commands (`orient`, `find`, `decide`, `recall`, `sync`),
      including pre-init failures.
- [x] (2026-02-14 16:33Z) Normalized remaining list-like JSON fields:
      `find` not-found `suggestions` now serialize as `[]`.
- [x] (2026-02-14 16:33Z) Added `find --max-body-lines` validation with stable
      `invalid_input` envelopes in JSON mode and exit-2 text errors.
- [x] (2026-02-14 16:33Z) Improved dependency precision by indexing dependency
      package/kind context and excluding external-selector false positives.
- [x] (2026-02-14 16:33Z) Added `find` disambiguation filters:
      `--package`, `--file`, and `--kind` with deterministic filtered errors.
- [x] (2026-02-14 16:33Z) Prevented `decide` persistence on unsupported
      `check_type` invalid input (`proposal`/`evidence` rows remain unchanged).
- [x] (2026-02-14 16:33Z) Added failing tests first, then green/refactor tests
      for each milestone behavior.
- [x] (2026-02-14 16:33Z) Ran `go test ./...` and
      `go test ./... -coverprofile=coverage.out`; confirmed total coverage
      `100.0%`.
- [x] (2026-02-14 16:33Z) Updated living sections (`Progress`,
      `Surprises & Discoveries`, `Decision Log`, `Outcomes`).

## Surprises & Discoveries

- Observation: JSON-mode failures are inconsistent before initialization.
  Evidence: On a fresh module, `find --json`, `recall --json`, `orient --json`,
  and `sync --json` emitted plain-text errors with exit `1`, while
  `decide --json` emitted an envelope with exit `2`.
- Observation: `find` not-found JSON still emits `"suggestions": null`.
  Evidence: Dogfood command `find DefinitelyMissingSymbol --json` on
  `cortex_code` returned `null` suggestions.
- Observation: `find` dependency output uses name-only resolution and can return
  unrelated symbols in larger repos.
  Evidence: `find GenerateSessionID` in `cortex_code` returned dependencies for
  project-local `Format` methods even though the call site used
  `time.Now().Format`.
- Observation: invalid `--max-body-lines` values are accepted silently.
  Evidence: `find GenerateSessionID --max-body-lines -1` returned full output
  with exit `0` instead of input validation failure.
- Observation: `decide` invalid input currently persists proposal records.
  Evidence: `decide --check-type nope --json` returned `invalid_input` with a
  `proposal_id`, showing state mutation on user input errors.
- Observation: common symbol names are frequently ambiguous in large repos.
  Evidence: `find NewService` in `cortex_code` returned 9 candidates, creating
  manual triage overhead without narrowing flags.
- Observation: selector calls to external imports (`time.Now`) can be
  misidentified as in-project deps unless import aliases are tracked.
  Evidence: while implementing precision tests, `Now` appeared as a false dep
  until import aliases were recorded (external aliases map to empty package and
  are ignored for dependency materialization).

## Decision Log

- Decision: Milestone 3 prioritizes correctness and contract consistency over
  new command surface area. Rationale: dogfood findings are mostly reliability
  and predictability gaps; fixing these improves trust in automation and human
  workflows immediately. Date/Author: 2026-02-14 / Codex
- Decision: JSON error envelope behavior should be centralized and reused by all
  DB-backed commands (`orient`, `find`, `decide`, `recall`, `sync`) when
  `--json` is active. Rationale: prevents per-command drift in error shape and
  exit semantics. Date/Author: 2026-02-14 / Codex
- Decision: `invalid_input` failures must be non-persistent in `decide`.
  Rationale: invalid user input should fail fast and avoid writing proposal or
  evidence records that add noise to repository knowledge state.
  Date/Author: 2026-02-14 / Codex
- Decision: `find` must gain deterministic narrowing controls for ambiguous
  symbols in large repos. Rationale: ambiguity is normal at scale; the CLI must
  offer first-class narrowing rather than forcing manual retries.
  Date/Author: 2026-02-14 / Codex
- Decision: encode DB pre-init failures as `not_initialized` (not
  `internal_error`) in the shared JSON envelope helper. Rationale: gives
  deterministic machine semantics for recoverable setup issues.
  Date/Author: 2026-02-14 / Codex
- Decision: persist dependency context (`dep_package`, `dep_kind`) in
  `symbol_deps` instead of only `dep_name`. Rationale: enables precise
  dependency joins and prevents routine cross-package false positives.
  Date/Author: 2026-02-14 / Codex

## Outcomes & Retrospective

Milestone 3 is implemented and validated.

Before/after evidence:

- Pre-init JSON envelope consistency now holds across DB-backed commands:

```shell
$ /tmp/recon-dogfood find Missing --json
{
  "error": {
    "code": "not_initialized",
    "details": { "path": ".../.recon/recon.db" }
  }
}
```

- `find` not-found JSON list normalization:

```shell
$ (cd cortex_code && /tmp/recon-dogfood find DefinitelyMissingSymbol --json)
{
  "error": {
    "code": "not_found",
    "details": {
      "suggestions": []
    }
  }
}
```

- `find` dependency precision on real repo (`cortex_code`):

```shell
$ (cd cortex_code && /tmp/recon-dogfood find GenerateSessionID --json)
# dependencies length = 0 (no unrelated local Format methods)
```

- `find` disambiguation filters:

```shell
$ (cd cortex_code && /tmp/recon-dogfood find NewService --json)
# ambiguous (9 candidates)
```

```shell
$ (cd cortex_code && /tmp/recon-dogfood find NewService --package internal/session --json)
# single symbol result (resolved)
```

- `decide` invalid-input non-persistence in real repo (`cortex_code`):

```sql
$ sqlite3 .recon/recon.db 'select count(*) from proposals;'
0
```

```shell
$ /tmp/recon-dogfood decide "invalid" --reasoning r --evidence-summary e --check-type nope --check-spec '{}' --json
# invalid_input
```

```sql
$ sqlite3 .recon/recon.db 'select count(*) from proposals;'
0
```

Test/coverage validation:

```shell
go test ./...
go test ./... -coverprofile=coverage.out
go tool cover -func=coverage.out
total: (statements) 100.0%
```

Milestone outcomes match the purpose: predictable JSON contracts, stricter input
validation, improved find result quality at scale, and no invalid-input state
mutation regressions.

## Context and Orientation

Implementation remains in Go under these paths:

- `internal/cli/root.go` defines root command setup and shared app state.
- `internal/cli/output.go` contains JSON writers and prompt helpers.
- `internal/cli/orient.go`, `internal/cli/find.go`, `internal/cli/decide.go`,
  `internal/cli/recall.go`, `internal/cli/sync.go` contain command handlers.
- `internal/cli/exit_error.go` defines structured process-exit behavior.
- `internal/index/service.go` currently extracts symbol dependencies.
- `internal/find/service.go` returns symbol and dependency data used by `find`.
- `internal/knowledge/service.go` executes checks and persists decisions.
- `internal/cli/commands_test.go` and new focused test files under
  `internal/cli/` hold integration-style command tests.

Terms used in this plan:

- JSON error envelope: stable JSON object of the form
  `{ "error": { "code": "...", "message": "...", "details": ... } }`.
- DB-backed command: any command requiring `.recon/recon.db` to be present
  (`sync`, `orient`, `find`, `decide`, `recall`).
- Dependency precision: ability of `find` to return symbols actually referenced
  by the matched symbol body, not unrelated same-name symbols.
- Non-persistent invalid input: input validation failure that does not mutate
  proposals, decisions, evidence, or search index tables.

## Plan of Work

Start by introducing a small shared CLI error classification layer used by all
DB-backed commands when `--json` is set. This layer should map common failures
(such as missing DB initialization and invalid flags) to stable codes and
standard envelope output. Keep text-mode behavior unchanged unless explicitly
covered by acceptance criteria.

Next, close remaining JSON null-list gaps by ensuring `find` not-found
suggestions serialize as `[]` when empty. This should mirror the Milestone 2
orient list normalization strategy.

Then harden `find` text flag validation: reject negative `--max-body-lines` as
`invalid_input` (JSON envelope in JSON mode; clear text error otherwise).

After contract hardening, improve dependency precision in indexing and/or find
resolution. Replace name-only dependency expansion with a representation that can
preserve receiver/package context from call sites and resolve dependencies more
accurately. This change must include regression tests showing elimination of
cross-package false positives found in dogfood.

Add disambiguation filters to `find` for common-scale ambiguity, at minimum
narrowing by package path, file path, and symbol kind. These filters should
apply consistently to text and JSON outputs and have deterministic error
messages when filters eliminate all candidates.

Finally, update `decide` flow so `invalid_input` returns before proposal/evidence
writes. Keep verification failures persistent (pending proposal) as-is, but
separate validation failures from verification outcomes.

Implement all features with strict TDD: write failing tests first, add minimal
code to pass, then refactor.

## Concrete Steps

Run all commands from repository root:
`/Users/robertguss/Projects/startups/recon`

1. Add failing tests for each hardening behavior.

   go test ./... -run TestJSONModeDBErrorsAreEnveloped
   go test ./... -run TestFindJSONNotFoundSuggestionsArray
   go test ./... -run TestFindRejectsNegativeMaxBodyLines
   go test ./... -run TestFindDependencyPrecision
   go test ./... -run TestFindDisambiguationFilters
   go test ./... -run TestDecideInvalidInputDoesNotPersist

   Expected before implementation: each targeted test fails for the intended
   gap.

2. Implement shared JSON error-envelope handling for DB-backed pre-init and
   execution failures.

   go test ./... -run TestJSONModeDBErrorsAreEnveloped

3. Normalize `find` suggestions arrays and validate `--max-body-lines`.

   go test ./... -run 'TestFindJSONNotFoundSuggestionsArray|TestFindRejectsNegativeMaxBodyLines'

4. Implement dependency precision and disambiguation filters in `find`.

   go test ./... -run 'TestFindDependencyPrecision|TestFindDisambiguationFilters'

5. Implement fail-fast non-persistent `decide` invalid input.

   go test ./... -run TestDecideInvalidInputDoesNotPersist

6. Run full validation and coverage.

   go test ./...
   go test ./... -coverprofile=coverage.out
   go tool cover -func=coverage.out

   Expected after implementation:
   - all tests pass
   - total coverage is exactly `100.0%`

7. Run real-repo dogfood verification in both repositories.

   (cd /Users/robertguss/Projects/startups/recon && /tmp/recon-dogfood init && /tmp/recon-dogfood sync)
   (cd /Users/robertguss/Projects/startups/cortex_code && /tmp/recon-dogfood init && /tmp/recon-dogfood sync)

   Then verify:
   - consistent JSON envelopes on DB pre-init failures
   - no `null` list fields in JSON contracts
   - improved `find` dependency relevance and ambiguity narrowing
   - invalid `decide` input produces no persisted proposal row

## Validation and Acceptance

Acceptance is complete when all statements below are true and demonstrated by
tests plus CLI output snapshots:

All DB-backed commands in JSON mode return the standard envelope for execution
failures, including pre-init missing DB cases.

`find --json` not-found responses always encode `suggestions` as `[]` when
empty.

`find --max-body-lines` rejects invalid negative values with `invalid_input` and
non-zero exit.

`find` dependency output for known selector-call scenarios no longer includes
unrelated same-name symbols from other packages.

`find` ambiguity can be narrowed deterministically via filter flags and returns
clear not-found/ambiguous semantics after filtering.

`decide` invalid input does not persist proposal/evidence rows, while legitimate
verification failures still produce pending proposals.

`go test ./...` passes and coverage remains `100.0%`.

## Idempotence and Recovery

All changes are code and tests only. No destructive schema migration is
expected. If partial work causes failures, revert only incomplete feature
commits and re-apply in smaller slices.

Dogfood runs may create `.recon/recon.db` and modify `.gitignore` in target
repositories on first init. When validating in external repos, clean up by
removing `.recon/recon.db` (and optional empty `.recon/` directory) and
restoring `.gitignore` if needed.

## Artifacts and Notes

Representative dogfood evidence that motivates this milestone:

    $ recon find DefinitelyMissingSymbol --json
    {
      "error": {
        "code": "not_found",
        "details": {
          "suggestions": null
        }
      }
    }

    $ recon find GenerateSessionID
    Direct dependencies:
    - method Format (internal/formatter/table.go)
    - method Format (internal/formatter/tsv.go)

    $ recon decide "invalid" --check-type nope --check-spec '{}' --json
    {
      "error": {
        "code": "invalid_input",
        "details": { "proposal_id": 4 }
      }
    }

## Interfaces and Dependencies

Use existing repository dependencies. No new third-party libraries are expected
for this milestone.

At completion, these interfaces/behaviors must exist:

- DB-backed commands share JSON failure envelope semantics in JSON mode.
- `internal/find` dependency resolution includes enough call-site context to
  prevent routine name-only false positives.
- `internal/cli/find.go` supports deterministic disambiguation flags for common
  symbols in large repositories.
- `internal/cli/decide.go` and/or `internal/knowledge/service.go` enforce
  fail-fast invalid input with no proposal persistence.
- All new logic is fully covered, preserving `100.0%` total coverage.

Revision note (2026-02-14): Created this plan from direct dogfooding findings in
`recon` and `cortex_code` to guide the next reliability-focused milestone.

Revision note (2026-02-14): Updated after implementation completion with
executed red/green/refactor test evidence, real-repo (`cortex_code`) dogfood
results, final coverage confirmation (`100.0%`), and finalized decisions.
