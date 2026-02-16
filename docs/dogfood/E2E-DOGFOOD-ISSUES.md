# E2E Dogfood Issues

Discovered during comprehensive E2E testing on 2026-02-16 across three repos:

- **recon** (self) — 31 files, 236 symbols, 11 packages
- **cortex_code** — 295 files (173 indexed), 1,698 symbols, 23 packages
- **hugo** — 885 files (502 indexed), 7,770 symbols, 191 packages

Zero command failures. All issues below are ergonomics, documentation, or
missing features — not correctness bugs.

---

## P0 — Documentation Drift

### 1. CLAUDE.md references flags that don't exist

The project CLAUDE.md and the `/recon` skill reference CLI flags that differ
from the actual implementation. An AI agent reading CLAUDE.md will construct
incorrect commands.

| Documented                    | Actual                                | Command        |
| ----------------------------- | ------------------------------------- | -------------- |
| `find --type func`            | `find --kind func`                    | `recon find`   |
| `find --import X`             | Flag does not exist                   | `recon find`   |
| `find --dep X`                | Flag does not exist (use `--package`) | `recon find`   |
| `decide --title "X"`          | Positional: `decide "X"`              | `recon decide` |
| `decide --evidence-type grep` | `decide --check-type grep`            | `recon decide` |
| `decide --evidence-query "X"` | `decide --check-pattern "X"`          | `recon decide` |
| `recall --type decision`      | Flag does not exist                   | `recon recall` |

**Impact:** High. The primary consumer of CLAUDE.md is Claude Code itself. If
the docs are wrong, the AI will fail on first attempt and waste turns
discovering the correct flags.

**Fix:** Audit CLAUDE.md and the `/recon` skill against `--help` output for
every command. Ensure all documented examples actually work.

### 2. `/recon` skill likely has same drift

The skill file that Claude Code loads when `/recon` is invoked probably
references the same incorrect flags. Needs the same audit.

---

## P1 — Ergonomics Issues

### 3. `recon init` mutates `.claude/` directory in target repo

Running `recon init` on cortex_code and hugo installed `.claude/hooks`,
`.claude/skills`, and `.claude/settings.json` into those repos. This is
opinionated and potentially destructive if the target repo already has its own
Claude Code setup.

**Current behavior:** `recon init` always installs Claude Code integration.

**Expected behavior:** `recon init` should only create `.recon/`. The Claude
Code integration should be a separate command (`recon install-hooks`) or an
opt-in flag (`recon init --with-claude`).

**Workaround:** Manually delete `.claude/` artifacts after init.

### 4. `find --file` requires exact filename, not substring

`recon find --file template` returns 0 results on hugo, but
`recon find --file template.go` returns 76 symbols. The flag matches the exact
filename, not a substring or glob of the path.

**Expected behavior:** `--file template` should match any file containing
"template" in its name (e.g., `template.go`, `template_funcs.go`,
`shortcode_template.go`).

**Alternative:** If exact match is intentional, document it clearly and consider
adding `--file-pattern` or glob support.

### 5. `find` output lacks code context

`recon find` shows package, name, kind, file, and line number but no code
snippet or function signature. Compare to `rg` which shows surrounding lines.

Example output today:

```
Service | type | internal/find/service.go:42
```

More useful output:

```
Service | type | internal/find/service.go:42
  type Service struct { db *sql.DB }
```

For functions, showing the signature (`func NewService(db *sql.DB) *Service`)
would make find results immediately actionable without needing to open the file.

### 6. Edge entity refs are ID-only

Creating edges requires knowing numeric IDs:

```
recon edges --create --from "decision:1" --to "pattern:1" --relation related
```

There's no way to reference entities by title or fuzzy match. Something like
`--from "decision:ExitError"` would be more ergonomic, especially for human
users who don't memorize IDs.

### 7. `decide --delete` says "archived" not "deleted"

Running `recon decide --delete 1` outputs "Decision 1 archived." This is
semantically correct (decisions are archived, not hard-deleted) but the flag
name `--delete` is misleading. Consider renaming to `--archive` or updating the
output to clarify: "Decision 1 archived (soft-deleted)."

Same applies to `pattern --delete`.

---

## P2 — Missing Features

### 8. `recall` has no type filtering

There's no way to filter recall results by entity type (decisions vs patterns).
If you have 50 decisions and 20 patterns, `recon recall "service"` returns a
mixed list. As the knowledge base grows, this will get noisy.

**Suggestion:** Add `--kind decision|pattern` flag to `recon recall`.

### 9. No `recon check` command for CI drift detection

Drift detection exists inside `decide --list` and `orient`, but there's no
dedicated command to check whether evidence still holds. This would be the
killer CI integration:

```sh
# In CI pipeline
recon check || echo "Decisions have drifted — review evidence"
```

**Suggestion:** `recon check` that exits non-zero if any promoted
decision/pattern has drifted, with a summary of what changed.

### 10. No incremental sync

Every `recon sync` re-indexes the entire repository from scratch. The
fingerprint mechanism already tracks state, but there's no delta-based
re-indexing.

**Current scale:** Hugo (885 files) syncs in 0.41s, so this isn't urgent. But
for monorepos with 10K+ files, full re-indexing on every sync will become a
bottleneck.

**Suggestion:** Compare file mtimes or content hashes against the previous sync
and only re-index changed files.

### 11. No import/dependency search in `find`

There's no way to search by import path or find reverse dependencies. Questions
like "what packages import internal/db?" or "show me all imports of
modernc.org/sqlite" can't be answered.

The data is in the database (imports and symbol_deps tables) but there's no CLI
surface for it.

**Suggestion:** Add `recon find --imports-of <package>` and
`recon find --imported-by <package>`.

### 12. `decide` and `pattern` lack an update/edit workflow

You can create and archive, but you can't update the reasoning or title of an
existing decision. If understanding evolves, you must archive and recreate,
losing the original ID and breaking any edges that reference it.

**Suggestion:** `recon decide --update 1 --reasoning "Updated reasoning"` that
preserves the ID and edges.

### 13. `orient` text output doesn't show decision reasoning

The text-mode orient output shows decision titles and confidence but not the
reasoning or evidence summary. An LLM getting this at session start wants to
know _why_ a decision was made, not just that it exists.

The `--json` output has more detail, but the human-readable text format should
surface reasoning too (even if truncated).

---

## P3 — Minor / Nice-to-Have

### 14. `find` with ambiguous results could suggest disambiguation

When `recon find Service` returns 9 candidates, it lists them but doesn't
suggest the next command. Adding a hint like "Try: recon find Service --package
internal/find" would help new users.

**Note:** The self-test agent found that recon already shows suggestions for
near-misses (e.g., when a symbol isn't found exactly). The ambiguous case could
benefit from similar guidance.

### 15. `find --kind func` shows limited results by default

On hugo, `find --kind func` found 1,464 functions but only displayed 50. The
default limit is reasonable, but there's no obvious way to paginate or increase
it.

**Suggestion:** Show the total count and a hint like "Showing 50 of 1,464. Use
--limit N to see more."

### 16. `edges --list` could show entity titles, not just refs

Current output:

```
#1 decision:1 -[related]-> pattern:1 (source=manual, confidence=high)
```

More useful:

```
#1 decision:1 "ExitError is the standard error type" -[related]-> pattern:1 "Service pattern" (source=manual, confidence=high)
```

### 17. No `recon reset` or `recon clean` command

To start fresh, you have to `rm -rf .recon`. A `recon reset` command (or
`recon init --force`) would be cleaner, especially for scripted workflows.

`just db-reset` exists in the justfile but only works in the recon repo itself.

### 18. Binary size is 12MB

The recon binary is 12MB due to the pure-Go SQLite driver. Not a blocker, but
worth noting. If distribution size matters later, `upx` compression or trimming
debug symbols (`-ldflags "-s -w"`) could help.

---

## Test Infrastructure Notes

### What was tested

Every command was run on all three repos with full output captured:

- `init`, `sync`, `status`, `orient`, `orient --json`
- `find` with symbol search, `--kind`, `--file`, `--list-packages`, `--package`
- `decide` create, `--list`, `--delete`
- `pattern` create, `--list`, `--delete`
- `recall` with queries, `--json`
- `edges --create`, `--list`, `--delete`
- `version`
- Error cases: no args, wrong flags, uninitialized DB

### Performance benchmarks

| Operation     | recon (31 files) | cortex_code (173 files) | hugo (502 files) |
| ------------- | ---------------- | ----------------------- | ---------------- |
| `init`        | 0.217s           | ~0.2s                   | ~0.2s            |
| `sync`        | 0.039s           | ~0.1s                   | 0.41s            |
| `status`      | 0.008s           | ~0.01s                  | ~0.01s           |
| `orient`      | 0.062s           | ~0.06s                  | ~0.08s           |
| `find` (any)  | 0.006-0.009s     | 0.006-0.009s            | 0.006-0.009s     |
| `recall`      | 0.007s           | ~0.01s                  | ~0.01s           |
| `edges` (any) | 0.007-0.009s     | ~0.01s                  | ~0.01s           |

All read operations are sub-10ms regardless of repo size. Sync scales linearly
and stays under 1 second for repos up to ~900 files.
