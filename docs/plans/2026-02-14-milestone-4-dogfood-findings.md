# Milestone 4: Agent Dogfood Findings & Recommendations

> **Source:** End-to-end CLI dogfood session by Claude (the primary consumer)
> against the `recon` repository itself. Every command was run with JSON and
> text output, good inputs, bad inputs, edge cases, and out-of-repo contexts.

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to
> implement this plan task-by-task.

**Goal:** Transform recon from a working reference tool into essential agent
infrastructure by closing the gaps between what was brainstormed and what was
shipped, plus addressing new issues discovered during real usage.

**Architecture:** Builds on M1-M3 foundation. No new external dependencies
expected. Enriches existing commands (`orient`, `find`, `recall`, `decide`) and
adds two new commands (`pattern`, `status`). All changes maintain 100% test
coverage and strict TDD.

**Tech Stack:** Go 1.26, Cobra, SQLite (modernc.org/sqlite), FTS5

---

## What's Working Well (Keep These)

These are genuine strengths worth preserving as the codebase evolves:

- **JSON output contract** — Every command supports `--json` consistently.
  Structured error envelope with `error.code`, `error.message`, `error.details`
  is exactly right for programmatic consumption. Branch on `ambiguous` vs
  `not_found` vs `invalid_input` without string parsing.
- **`find` symbol lookup** — Fuzzy suggestions on not-found (`NewRoot` →
  `NewRootCommand`), ambiguity handling with candidate lists, package/file/kind
  filters. The dependency graph in responses is a great touch.
- **`orient` as session-start command** — Module structure, decisions,
  freshness, `--auto-sync` and `--sync` flags show good awareness of the
  staleness problem.
- **`decide` with verification checks** — Propose + verify + auto-promote in one
  shot. Three check types (`file_exists`, `symbol_exists`, `grep_pattern`) cover
  common evidence patterns.
- **Error handling consistency** — Exit code 2 for user errors, structured JSON
  errors, suggestions on not-found, candidate lists on ambiguous.
- **Speed** — Everything runs in milliseconds.
- **`--no-prompt` global flag** — Correct default for agent environments.
- **`--json-strict` on orient** — Suppresses stderr warnings in machine mode.

---

## Gap Analysis: Brainstormed vs Built vs New

### Already Shipped (M1-M3)

| Feature                                                   | Milestone | Status  |
| --------------------------------------------------------- | --------- | ------- |
| JSON error envelope consistency                           | M2 + M3   | Shipped |
| `find` disambiguation filters (--package, --file, --kind) | M3        | Shipped |
| `find` dependency false-positive reduction                | M3        | Shipped |
| Typed `decide` flags (--check-path, --check-symbol, etc.) | M2        | Shipped |
| `--no-prompt` global flag                                 | M2        | Shipped |
| Null → empty array normalization                          | M2 + M3   | Shipped |
| Invalid input non-persistence for `decide`                | M3        | Shipped |
| `find --no-body` and `--max-body-lines`                   | M2        | Shipped |

### Brainstormed But Not Built

| Feature                                                                | Brainstorm Source                   | Notes                                            |
| ---------------------------------------------------------------------- | ----------------------------------- | ------------------------------------------------ |
| Rich `orient` output (architecture flow, heat map, patterns, activity) | v2 brainstorm                       | Current orient only has module stats + decisions |
| `orient --for "task"` focused mode                                     | v2 brainstorm                       | Not started                                      |
| `pattern` command                                                      | v2 command set, v3 schema           | Tables may exist in schema, no CLI               |
| `issue` command                                                        | v2 command set, v3 schema           | Tables may exist in schema, no CLI               |
| Knowledge snapshot durability                                          | v3 brainstorm (detailed design)     | M1 deferred to M2, M2 didn't do it               |
| Session tracking                                                       | v3 schema (sessions, session_files) | Schema exists, no CLI surface                    |
| Hook integration (session_start/session_end)                           | v3 brainstorm                       | M1 chose hookless by design                      |
| `recall` across all knowledge fields                                   | v3 schema (FTS5 search_index)       | FTS exists but searches narrowly                 |

### New Gaps (Discovered in Dogfood)

| Finding                                            | Evidence                                                                                               |
| -------------------------------------------------- | ------------------------------------------------------------------------------------------------------ |
| No way to list/delete/manage decisions             | Created 3 test decisions with no cleanup path                                                          |
| `find --file` doesn't do suffix matching           | `--file service.go` returns "not found" even though all Service types live in files named `service.go` |
| No `find` browse/list mode                         | Can't list all types in a package or all symbols in a file without knowing a name first                |
| No `decide --dry-run`                              | Must create proposal to check if verification passes                                                   |
| No `recon status` quick health check               | Only `orient` gives init/stale info but it's heavyweight                                               |
| Missing-args error is generic Cobra text           | `recon find` → "accepts 1 arg(s), received 0". Not helpful, `--json` never fires                       |
| `--check-type invalid_type` gives misleading error | Says "either --check-spec or typed check flags are required" instead of "unknown check type"           |
| No method receiver syntax in `find`                | Can't search `Service.Find`, must search `Find` and wade through all packages                          |
| Confidence doesn't decay on drift                  | Static low/medium/high. Drift detection exists but doesn't affect confidence                           |

---

## Tier 1: High Impact

These make the agent (me) significantly more effective. These are the features
I'd reach for every session.

### 1.1 — Richer `orient` Output

**Problem:** Current `orient` gives me stats (file counts, symbol counts,
decision list). The v2 brainstorm designs an orient that gives me
_understanding_ — architecture flow, module heat, patterns, entry points, recent
activity.

**What to build:**

- **Architecture section** — Entry points (`cmd/*/main.go`), dependency flow
  direction between top-level packages. Not a full dep graph — a one-line
  summary like
  `cmd/recon → internal/cli → internal/{orient,find,knowledge,recall,index,db}`.
- **Module heat** — Use `git log --since="2 weeks ago" --name-only` to compute
  recent commit frequency per module directory. Label modules HOT (4+ recent
  sessions/commits), WARM (1-3), COLD (0).
- **Active patterns** — Surface promoted patterns (once `pattern` command
  exists) in orient output.
- **Recent activity** — Last 3-5 changed files from git log, giving the agent a
  sense of "what was being worked on."

**Brainstorm reference:** v2 `project-rethink-v2.md` lines 78-119 has the full
orient output design.

**Orient JSON shape addition (illustrative):**

```json
{
  "architecture": {
    "entry_points": ["cmd/recon/main.go"],
    "dependency_flow": "cmd/recon → internal/cli → internal/{db,index,orient,find,knowledge,recall}"
  },
  "modules": [
    {
      "path": "internal/cli",
      "name": "cli",
      "file_count": 11,
      "line_count": 901,
      "heat": "hot",
      "recent_commits": 12
    }
  ],
  "recent_activity": [
    {
      "file": "internal/cli/decide.go",
      "last_modified": "2026-02-14T15:34:39Z"
    },
    { "file": "internal/cli/find.go", "last_modified": "2026-02-14T15:34:39Z" }
  ]
}
```

### 1.2 — `find` Browse/List Mode

**Problem:** Every `find` query requires knowing a symbol name. I can't discover
what exists. If I'm new to a codebase (which I am every session), I need
exploration, not just lookup.

**What to build:**

- `recon find --kind type --package internal/db` → list all types in that
  package
- `recon find --package internal/cli` → list all symbols in that package
- `recon find --file internal/cli/root.go` → list all symbols in that file
- `recon find --kind func` → list all functions across the codebase (with limit)

When no `<symbol>` arg is provided but filter flags are present, switch to list
mode. Return name, kind, file, line range — no bodies in list mode.

**JSON shape (list mode):**

```json
{
  "symbols": [
    {
      "name": "NewRootCommand",
      "kind": "func",
      "file_path": "internal/cli/root.go",
      "line_start": 23,
      "line_end": 52,
      "package": "internal/cli"
    }
  ],
  "total": 45,
  "limit": 50
}
```

**Cobra change:** Make `<symbol>` arg optional (`cobra.MaximumNArgs(1)`) when
filter flags are provided.

### 1.3 — `find --file` Suffix Matching

**Problem:** `find Service --file service.go` returns "not found" because the
filter expects full relative path. But I think in filenames, not paths.

**What to build:**

- If `--file` value doesn't contain `/`, treat it as a suffix match (filename
  match)
- If `--file` value contains `/`, treat it as a substring/prefix match against
  the relative path
- `--file service.go` matches `internal/find/service.go`,
  `internal/recall/service.go`, etc.
- `--file internal/find/service.go` matches exactly

### 1.4 — `pattern` Command

**Problem:** Patterns are the most valuable knowledge for me. "Errors use %w
wrapping", "all commands go through service layer", "tests use testify
assertions." This is what I need to write code that _fits_ the codebase. The
brainstorms define this as a first-class entity with its own table, but it was
never built.

**What to build:**

```
recon pattern "Error wrapping with %w" \
  --description "All errors use fmt.Errorf with %w wrapping, never panic" \
  --example 'return fmt.Errorf("resolve cwd: %w", err)' \
  --check-type grep_pattern \
  --check-pattern 'fmt\.Errorf.*%w' \
  --check-scope '*.go' \
  --json
```

Same propose → verify → promote lifecycle as `decide`. Same evidence/drift
system. Patterns appear in `orient` output and are searchable via `recall`.

**Schema:** v3 brainstorm already defines `patterns` and `pattern_files` tables
(lines 227-241).

---

## Tier 2: Medium Impact

These remove friction and improve workflow quality.

### 2.1 — Decision Lifecycle Management

**Problem:** Knowledge accumulates without cleanup. I created 3 test decisions
during dogfood with no way to remove them. In real use, decisions get
superseded, become irrelevant, or are just wrong.

**What to build:**

- `recon decide --list` → list all promoted decisions (id, title, confidence,
  drift, updated_at)
- `recon decide --list --json` → same in JSON
- `recon decide --delete <id>` → soft-delete a decision (set status to
  'archived')
- `recon decide --update <id> --confidence high` → update confidence on existing
  decision

Keep it simple. No interactive editing. Just list, delete, update-confidence.

### 2.2 — `recon status`

**Problem:** No lightweight way to check repo health without running full
`orient`.

**What to build:**

```
$ recon status
Initialized: yes
Last sync: 2026-02-14T17:10:08Z (fresh)
Files: 23 | Symbols: 138 | Packages: 8
Decisions: 7 (0 drifting) | Patterns: 0

$ recon status --json
{
  "initialized": true,
  "stale": false,
  "last_sync_at": "2026-02-14T17:10:08Z",
  "counts": {
    "files": 23,
    "symbols": 138,
    "packages": 8,
    "decisions": 7,
    "decisions_drifting": 0,
    "patterns": 0
  }
}
```

Fast. No git operations. Just DB reads.

### 2.3 — `decide --dry-run`

**Problem:** Can't test if verification would pass without creating state.

**What to build:**

- `recon decide "test" --dry-run --check-type file_exists --check-path go.mod --json`
- Runs the verification check, returns pass/fail result
- Does NOT create proposal or evidence rows
- Same output shape minus `proposal_id` and `decision_id`

### 2.4 — Better Missing-Args Errors

**Problem:** `recon find` with no args → "accepts 1 arg(s), received 0". Generic
Cobra message. `--json` flag never fires because Cobra rejects before RunE.

**What to build:**

- Override Cobra's `Args` validator for `find`, `recall`, `decide` to emit
  structured errors
- In JSON mode:
  `{"error": {"code": "missing_argument", "message": "find requires a <symbol> argument", "details": {"command": "find"}}}`
- In text mode: show usage hint with examples
- Exit code 2 (consistent with other input errors)

---

## Tier 3: Nice to Have

Lower priority. Ship if time allows or defer to M5.

### 3.1 — `find` Receiver Syntax

**Problem:** Can't search for `Service.Find`. Must search `Find` and filter
through all packages.

**What to build:**

- Support `recon find Service.Find` syntax — split on `.`, use left side as
  receiver filter
- Alternatively/additionally: `--receiver Service` flag
- Only applies to methods (kind=method)

### 3.2 — `recall` Search Improvement

**Problem:** "CLI framework" doesn't find "Use Cobra for CLI". FTS searches too
narrowly.

**What to build:**

- Ensure FTS5 search_index is populated with title + reasoning +
  evidence_summary combined for each promoted decision
- Verify porter tokenizer is active (it's in the v3 schema design)
- Make `recall` query the FTS index as primary path, LIKE as fallback
- Test: `recall "CLI framework"` should find decision with title "Use Cobra for
  CLI"

### 3.3 — Confidence Decay on Drift

**Problem:** Confidence is static. Drift detection runs but doesn't affect
confidence.

**What to build:**

- On `sync`, after drift detection runs: if drift_status changes to `drifting`,
  auto-downgrade confidence one level (high→medium, medium→low)
- If drift_status changes to `broken`, set confidence to `low`
- If drift_status returns to `ok`, do NOT auto-upgrade (requires human/agent
  intent)
- Surface in `orient`: "Decision #3 confidence downgraded: evidence drifting"

### 3.4 — `issue` Command

**Problem:** Lower priority than `pattern` because `decide` can capture
issue-like things, but the brainstorm design is clean and completes the
knowledge entity set.

**What to build:**

```
recon issue "Graph sync fails on circular imports" \
  --description "Circular package imports cause infinite loop in dependency walker" \
  --severity high \
  --file internal/index/service.go \
  --line 84 \
  --json
```

Same lifecycle as `decide` and `pattern`. Appears in `orient` output.

**Schema:** v3 brainstorm already defines `issues` table (lines 247-254).

### 3.5 — `--check-type` Validation Fix

**Problem:** `decide --check-type invalid_type` gives "either --check-spec or
typed check flags are required" instead of "unknown check type: invalid_type".

**What to build:**

- Validate `--check-type` value against known types (`grep_pattern`,
  `symbol_exists`, `file_exists`) before validating typed flags
- Return
  `{"error": {"code": "invalid_input", "message": "unknown check type \"invalid_type\"; must be one of: grep_pattern, symbol_exists, file_exists", "details": {"check_type": "invalid_type"}}}`

---

## Implementation Order Recommendation

```
Phase A (orient + find enrichment):
  1.1  Richer orient output
  1.2  find browse/list mode
  1.3  find --file suffix matching

Phase B (knowledge expansion):
  1.4  pattern command
  2.1  Decision lifecycle management
  2.2  recon status command

Phase C (workflow polish):
  2.3  decide --dry-run
  2.4  Better missing-args errors
  3.5  --check-type validation fix

Phase D (nice to have):
  3.1  find receiver syntax
  3.2  recall search improvement
  3.3  Confidence decay on drift
  3.4  issue command
```

---

## Open Questions

1. **Orient heat source** — Should module heat come from `git log` (simple,
   available now) or session tracking (richer, requires new infrastructure)?
   Recommendation: git log for M4, session tracking for M5.
2. **Orient token budget** — M1 mentions ~1200 token budget for text mode. With
   richer orient, how do we cap output? Truncate modules list? Omit cold
   modules?
3. **Pattern approval flow** — Same as decide (auto-promote on verification
   pass)? Or should patterns require higher evidence bar?
4. **find list mode limit** — Default limit for browse results? 50? 100?
   Configurable via `--limit`?
5. **Snapshot durability** — v3 brainstorm has detailed snapshot design. Should
   M4 include this or defer? It's been deferred twice already.

---

_Generated: 2026-02-14 from end-to-end CLI dogfood session_
