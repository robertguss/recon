# Deliver Recon Milestone 2 CLI ergonomics for agents and humans

This ExecPlan is a living document. The sections `Progress`,
`Surprises & Discoveries`, `Decision Log`, and `Outcomes & Retrospective` must
be kept up to date as work proceeds.

This plan must be maintained in accordance with `.agent/PLANS.md`.

This plan builds on `docs/plans/recon-milestone-1.md`. Milestone 1 delivered a
working CLI loop (`init`, `sync`, `orient`, `find`, `decide`, `recall`) with
auto-promotion and local SQLite storage. This plan refines operator ergonomics
and machine contracts without changing the core product model.

## Purpose / Big Picture

After this change, `recon` becomes safer and easier to automate in CI and agent
environments while still being usable by humans at the terminal. A user will be
able to run JSON commands without `null` list ambiguity, submit decision checks
without fragile JSON shell quoting, receive a consistent JSON error envelope for
command execution failures, control `find` output size, and disable all
interactive prompts globally.

The outcome is observable by running the CLI in a clean test module and
verifying exact command output and exit codes. The implementation must follow
strict TDD (red, green, refactor) and finish with 100% test coverage across all
packages.

## Progress

- [x] (2026-02-14 04:35Z) Created this Milestone 2 ExecPlan and captured scope,
      architecture edits, and acceptance criteria.
- [x] (2026-02-14 06:02Z) Implemented JSON output normalization so array-like
      fields serialize as `[]` instead of `null`.
- [x] (2026-02-14 06:05Z) Implemented typed `decide` check flags that generate
      `--check-spec` internally for supported checks.
- [x] (2026-02-14 06:07Z) Implemented a standard JSON error envelope for
      command execution failures in JSON mode.
- [x] (2026-02-14 06:08Z) Implemented `find` output controls for body verbosity
      (`--no-body`, `--max-body-lines`).
- [x] (2026-02-14 06:09Z) Implemented root-level `--no-prompt` behavior and
      wired it into `orient` prompt logic.
- [x] (2026-02-14 06:16Z) Added and updated tests in red/green/refactor order
      for every changed path, including new helper-path coverage tests.
- [x] (2026-02-14 06:19Z) Ran `go test ./...` and
      `go test ./... -coverprofile=coverage.out`, confirming total coverage
      remains `100.0%`.
- [x] (2026-02-14 06:20Z) Updated this document’s `Progress`,
      `Surprises & Discoveries`, `Decision Log`, and
      `Outcomes & Retrospective` sections.

## Surprises & Discoveries

- Observation: Current JSON responses can include `null` for list fields, which
  is valid JSON but causes extra branching in agents that expect arrays.
  Evidence: Recent `orient --json` output in a fresh repo returned
  `"modules": null` and `"active_decisions": null` before indexing.
- Observation: `decide --check-spec` currently requires shell-escaped JSON,
  which is error-prone in automation. Evidence: Manual invocation requires
  nested quoting like `--check-spec '{"path":"go.mod"}'`.
- Observation: New `--json-strict`, `--sync`, and `--auto-sync` behavior
  improved machine workflows and validated that explicit non-interactive flow
  controls are worth extending. Evidence: End-to-end run showed stale
  `orient --json-strict` emits JSON only and `orient --auto-sync --json`
  refreshed freshness state immediately.
- Observation: `symbol_exists` verification inside an open transaction can
  block indefinitely with SQLite when max open connections is `1`.
  Evidence: `TestDecideTypedCheckFlags` timed out with stack traces blocked in
  `runSymbolExists` while waiting on `database/sql.(*DB).conn`.
- Observation: Raising CLI ergonomic branches increased uncovered statements in
  `internal/cli` even though behavior was correct.
  Evidence: initial post-change coverage dropped to `99.6%` total and required
  targeted branch tests to restore `100.0%`.

## Decision Log

- Decision: Milestone 2 keeps the existing command set and adds ergonomics
  through flags and output contracts instead of introducing new top-level
  commands. Rationale: This preserves user muscle memory and minimizes migration
  overhead. Date/Author: 2026-02-14 / Codex
- Decision: JSON error envelope scope is command execution failures in JSON
  mode, not Cobra pre-execution usage text rendering. Rationale: Command
  failures are under repository control and can be normalized consistently;
  Cobra usage handling is framework-managed and should remain standard.
  Date/Author: 2026-02-14 / Codex
- Decision: `--check-spec` remains supported, but typed check flags are added as
  preferred input ergonomics. Rationale: Backward compatibility for scripts
  while reducing quoting friction for new usage. Date/Author: 2026-02-14 / Codex
- Decision: Root-level `--no-prompt` is global and must force non-interactive
  behavior even on a TTY. Rationale: CI and agent environments need
  deterministic behavior regardless of pseudo-terminal availability.
  Date/Author: 2026-02-14 / Codex
- Decision: JSON mode uses envelope codes `invalid_input` for unsupported or
  malformed checks and `verification_failed` for check failures that ran
  successfully. Rationale: callers can distinguish user-fixable input issues
  from true verification outcomes without parsing free-form text.
  Date/Author: 2026-02-14 / Codex
- Decision: Move decision verification execution before transaction creation in
  `knowledge.Service.ProposeAndVerifyDecision`. Rationale: avoids SQLite
  connection starvation/deadlock for checks that query using the same pool
  (`symbol_exists`) while preserving the existing persisted proposal/evidence
  outcomes. Date/Author: 2026-02-14 / Codex

## Outcomes & Retrospective

Milestone 2 is complete. The CLI now emits stable array fields for empty orient
lists, supports typed decision check flags that remove JSON shell quoting for
common checks, emits a standard JSON error envelope for JSON-mode command
failures, supports `find --no-body` and `find --max-body-lines`, and honors a
global `--no-prompt` flag that suppresses interactive prompts deterministically.

Validation was completed with new and existing tests, including targeted red and
green runs for each milestone behavior and full-suite verification. Final test
and coverage commands:

    go test ./...
    go test ./... -coverprofile=coverage.out
    go tool cover -func=coverage.out

All packages and total statements report `100.0%` coverage. No deferred items
remain for Milestone 2.

## Context and Orientation

The implementation lives in Go under these repository-relative paths:

- `cmd/recon/main.go` handles process exit code behavior.
- `internal/cli/root.go` constructs the root Cobra command and shared app state.
- `internal/cli/init.go`, `internal/cli/sync.go`, `internal/cli/orient.go`,
  `internal/cli/find.go`, `internal/cli/decide.go`, `internal/cli/recall.go`
  implement command handlers.
- `internal/cli/output.go` contains JSON writing and prompt helpers.
- `internal/cli/exit_error.go` defines structured exit errors used to return
  non-zero codes without noisy duplicate stderr output.
- `internal/orient/service.go` and `internal/orient/render.go` shape orient
  payloads and text rendering.
- `internal/find/service.go` defines typed ambiguity/not-found data used by
  `find`.
- Tests currently live in `cmd/recon/main_test.go` and
  `internal/cli/commands_test.go` with additional package tests across
  `internal/*`.

Key terms used in this plan:

- JSON error envelope: a stable JSON object that describes a failure in a
  machine-parsable way with fields for error code, message, and optional
  details.
- Typed check flags: dedicated flags like `--check-path` that build the check
  specification internally, so users do not hand-write JSON strings.
- Prompt suppression: behavior where the command never waits for terminal input
  and instead follows deterministic non-interactive logic.

## Plan of Work

Milestone 2 begins with output contract hardening. Update orient payload
assembly so list fields serialize as empty arrays when there are no results.
This requires changing payload construction defaults (not renderer-only changes)
so every JSON output path remains consistent. Add tests that assert exact JSON
fragments for empty list states.

Next, improve decision input ergonomics in `internal/cli/decide.go`. Keep
existing `--check-type` and `--check-spec`, but add typed alternatives for each
supported check and synthesize the correct check specification when typed flags
are provided. For `file_exists`, add `--check-path`. For `symbol_exists`, add
`--check-symbol`. For `grep_pattern`, add `--check-pattern` and optional
`--check-scope`. When both raw `--check-spec` and typed flags are supplied, fail
deterministically with a clear conflict error.

Then add a common JSON failure envelope helper in `internal/cli/output.go` (or a
small dedicated file in `internal/cli/`) and use it in JSON-mode command error
branches. Standardize shape as:

    {
      "error": {
        "code": "<stable_code>",
        "message": "<human-readable message>",
        "details": { ... optional typed object ... }
      }
    }

Use stable codes for common failure classes in command handlers, such as
`not_found`, `ambiguous`, `verification_failed`, `invalid_input`, and
`internal_error`. Preserve non-zero exit behavior through `ExitError`.

After error contract work, add body verbosity controls to `find`. Implement
`--no-body` to omit the symbol body section and `--max-body-lines <n>` to
truncate body output for text mode only. JSON output must remain full-fidelity
unless a later plan introduces explicit JSON truncation flags. Add deterministic
truncation text (for example, a final line marker such as `... (truncated)`),
and test exact output.

Finally, implement a root persistent flag `--no-prompt` in
`internal/cli/root.go` and store it on shared app state (for example,
`App.NoPrompt bool`). Update `orient` prompt logic to treat this flag as
non-interactive override. The command should never call `askYesNo` when
`--no-prompt` is enabled.

Every change is implemented with strict TDD. For each feature, start with
failing tests, then minimal code to pass, then small refactor while preserving
behavior. Keep the system additive and avoid changing unrelated contracts.

## Concrete Steps

Run all commands from repository root:
`/Users/robertguss/Projects/startups/recon`

1. Write failing tests for each feature before production edits.

   go test ./... -run TestOrientJSONEmptyLists go test ./... -run
   TestDecideTypedCheckFlags go test ./... -run TestJSONErrorEnvelope go test
   ./... -run TestFindBodyFlags go test ./... -run
   TestNoPromptDisablesOrientPrompt

   Expected before implementation: each new test fails for the intended reason.

2. Implement JSON list normalization in orient payload builders and update
   tests.

   go test ./... -run TestOrientJSONEmptyLists

   Expected after implementation: test passes and JSON has `[]` for empty lists.

3. Implement typed `decide` check flags and conflict validation logic.

   go test ./... -run TestDecideTypedCheckFlags

   Expected after implementation: typed flags produce correct verification
   behavior; conflicting inputs fail with stable error code and non-zero exit.

4. Implement shared JSON error envelope helper and wire command JSON error
   paths.

   go test ./... -run TestJSONErrorEnvelope

   Expected after implementation: known command failures return standardized
   envelope and non-zero exit codes.

5. Implement `find` text verbosity controls and output truncation tests.

   go test ./... -run TestFindBodyFlags

   Expected after implementation: `--no-body` suppresses body section;
   `--max-body-lines` truncates deterministically.

6. Implement root `--no-prompt` and orient integration.

   go test ./... -run TestNoPromptDisablesOrientPrompt

   Expected after implementation: orient never prompts when `--no-prompt` is
   present, regardless of TTY state.

7. Run full validation and coverage.

   go test ./... go test ./... -coverprofile=coverage.out go tool cover
   -func=coverage.out

   Expected after implementation:
   - all tests pass
   - total coverage is exactly `100.0%`

8. Run behavioral acceptance commands in a temp module.

   /tmp/recon init /tmp/recon orient --json /tmp/recon orient --json-strict
   /tmp/recon orient --no-prompt --auto-sync --json /tmp/recon find Clash
   /tmp/recon find Alpha --no-body /tmp/recon find Alpha --max-body-lines 3
   /tmp/recon decide "Typed file check" --reasoning r --evidence-summary e
   --check-type file_exists --check-path go.mod

   Expected after implementation:
   - machine-friendly JSON behavior with stable error envelopes
   - deterministic non-prompt behavior
   - improved text ergonomics in `find`

## Validation and Acceptance

Acceptance is complete when all statements below are true and demonstrated with
command output and tests:

A user in a new Go module can run `recon orient --json` before any sync and see
empty list fields as arrays (`[]`) instead of `null`.

A user can run `recon decide` with typed check flags (`--check-path`,
`--check-symbol`, `--check-pattern`, optional `--check-scope`) and get the same
verification behavior as raw `--check-spec` without needing JSON shell quoting.

In JSON mode, command execution failures return the standard error envelope
shape and exit non-zero. At minimum, `find` ambiguity/not-found and `decide`
verification failure are validated.

`recon find` supports `--no-body` and `--max-body-lines` in text mode with
deterministic output.

`recon --no-prompt orient` never prompts and remains deterministic on stale
context.

`go test ./...` passes and coverage remains 100% after all edits.

## Idempotence and Recovery

All milestone edits are source-code and test changes only; no destructive data
migration is required. `recon init` and `recon sync` remain safe to re-run. If a
partial implementation introduces failing tests, recover by reverting only the
incomplete feature commit and reapplying in smaller increments. If local state
interferes with acceptance runs, remove `.recon/recon.db`, re-run `recon init`,
then `recon sync`, and repeat acceptance commands.

When adding typed check flags, preserve backward compatibility: existing scripts
using raw `--check-spec` must continue to work. If a conflict is introduced by
supplying both typed flags and raw spec, fail explicitly rather than guessing
precedence.

## Artifacts and Notes

Illustrative expected outputs after Milestone 2:

    $ recon find Clash
    symbol "Clash" is ambiguous (2 candidates)
    - method A.Clash (main.go, pkg .)
    - method B.Clash (main.go, pkg .)

    [exit=2]

    $ recon decide "Bad check" --reasoning r --evidence-summary e --check-type nope --check-spec '{}' --json
    {
      "error": {
        "code": "invalid_input",
        "message": "unsupported check type \"nope\"",
        "details": {
          "check_type": "nope"
        }
      }
    }

    [exit=2]

    $ recon orient --json
    {
      "modules": [],
      "active_decisions": []
    }

## Interfaces and Dependencies

Use only existing repository dependencies unless a clear need appears. This
milestone should not require new third-party libraries.

At completion, these interfaces and behaviors must exist:

- In `internal/cli/root.go`, root command defines persistent flag `--no-prompt`
  and propagates it into shared app state.
- In `internal/cli/decide.go`, command supports typed check flags:
  - `--check-path string`
  - `--check-symbol string`
  - `--check-pattern string`
  - `--check-scope string` and resolves them into
    `knowledge.ProposeDecisionInput.CheckSpec` deterministically.
- In `internal/cli/find.go`, text mode supports:
  - `--no-body`
  - `--max-body-lines int`
- In `internal/cli/output.go` or `internal/cli/json_error.go`, a reusable JSON
  error writer exists with stable envelope shape.
- In orient payload assembly (`internal/orient/service.go` and related
  structures), list fields default to empty slices to ensure JSON arrays when
  empty.
- `cmd/recon/main.go` continues to honor `ExitError` codes for
  machine-meaningful exits.

This milestone must preserve strict TDD workflow and conclude with 100%
statement coverage.

Revision note (2026-02-14): Created this plan to close the five operator and
agent UX gaps discovered during post-feature CLI dogfooding after adding
`orient --sync`, `--auto-sync`, and `--json-strict`.
Revision note (2026-02-14): Updated after implementation to record completion
status, discovered SQLite verification deadlock risk, final command/error
contract decisions, and 100% coverage validation evidence.
