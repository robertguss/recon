# M5 Dogfood Findings

**Date:** 2026-02-15 **Tested by:** Claude (AI agent) via Claude Code **Repos
tested:**

- `recon` (self) — 26 files, 172 symbols, 9 packages
- `cortex_code` — 173 files, 1698 symbols, 23 packages (~69K lines Go)

---

## Test Matrix

| Command                 | Recon (self) | Cortex Code  | Notes                         |
| ----------------------- | :----------: | :----------: | ----------------------------- |
| `init`                  |     PASS     |     PASS     | Idempotent, doesn't wipe data |
| `init --json`           |     PASS     |      —       | Clean JSON output             |
| `sync`                  | PASS (36ms)  | PASS (128ms) | Fast at both scales           |
| `sync` (re-run)         | PASS (36ms)  | PASS (137ms) | No fingerprint skip           |
| `status`                |     PASS     |     PASS     |                               |
| `status --json`         |     PASS     |     PASS     |                               |
| `orient`                |     PASS     |  **ISSUE**   | Modules=(none) when all cold  |
| `orient --json`         |     PASS     |     PASS     | JSON shows modules correctly  |
| `find <exact>`          |     PASS     |     PASS     |                               |
| `find <ambiguous>`      |     PASS     |     PASS     | Good candidate listing        |
| `find --package`        |  **ISSUE**   |     PASS     | Short names don't match       |
| `find --kind`           |     PASS     |     PASS     |                               |
| `find --no-body`        |     PASS     |     PASS     |                               |
| `find --max-body-lines` |     PASS     |     PASS     | Truncation works              |
| `find <not-found>`      |     PASS     |     PASS     | Suggestions are helpful       |
| `find --json`           |     PASS     |     PASS     |                               |
| `decide` (create)       |     PASS     |     PASS     |                               |
| `decide --list`         |     PASS     |     PASS     |                               |
| `decide --update`       |      —       |     PASS     |                               |
| `decide --delete`       |      —       |     PASS     | Soft-delete works             |
| `decide --dry-run`      |      —       |     PASS     |                               |
| `pattern` (create)      |     PASS     |     PASS     |                               |
| `recall`                |     PASS     |     PASS     |                               |
| `recall --json`         |     PASS     |     PASS     |                               |
| `recall` (no match)     |     PASS     |     PASS     |                               |

---

## Performance

| Operation            | Recon (26 files) | Cortex (173 files) | Scaling                          |
| -------------------- | :--------------: | :----------------: | -------------------------------- |
| `sync`               |       36ms       |       128ms        | ~3.6x for 6.6x files — sublinear |
| `orient`             |     instant      |      instant       |                                  |
| `find` (exact)       |     instant      |      instant       |                                  |
| `find --kind` (list) |     instant      |      instant       |                                  |
| `orient --json` size |      ~1.5KB      |       ~3.3KB       | Compact payloads                 |

Performance is excellent. Not a concern at current or foreseeable scale.

---

## Bugs Found

### BUG-1: `orient` text output hides all modules when heat is cold

**Severity:** Medium **Reproduction:**

```bash
cd cortex_code && recon orient
# Output: Modules: (none)
# But: orient --json shows 8 modules correctly
```

**Cause:** The text renderer filters out modules with `heat=cold`. When all
modules are cold (no commits in 2-week window), the output says "(none)".

**Impact:** An agent or human reading text output gets zero architectural
information. The JSON output works fine — this is a text rendering bug.

**Fix:** Always show modules in text output. Annotate with `[COLD]` but never
suppress them entirely.

### BUG-2: `--package` flag requires full path, no short name matching

**Severity:** Medium **Reproduction:**

```bash
recon find NewService --package index
# → "not found with provided filters"

recon find NewService --package internal/index
# → works
```

**Impact:** An agent that knows the package name but not the full path will
waste a tool call. The ambiguous error shows `pkg internal/find` (short name)
but the `--package` flag requires `internal/find` (full path). Inconsistent.

**Fix options:**

1. Support last-segment matching (e.g. `--package index` matches
   `internal/index`)
2. Add "did you mean internal/index?" to the error message
3. Both

### BUG-3: Heat calculation window too narrow (2 weeks)

**Severity:** Low-Medium **Reproduction:**

```bash
cd cortex_code && recon orient --json | jq '.modules[0].recent_commits'
# → 0  (despite 282 commits in last 30 days)
```

**Cause:** `loadModuleHeat()` uses `--since=2 weeks ago`. Cortex's last commit
was 18 days ago. Everything shows as cold.

**Impact:** Heat becomes useless for any repo with even a brief development
pause. A 2-week vacation makes the entire codebase appear cold.

**Fix:** Widen to 30 days, or make configurable, or use a multi-bucket approach
(7d/30d/90d).

### BUG-4: `init` doesn't distinguish fresh vs already-initialized

**Severity:** Low **Reproduction:**

```bash
recon init  # "Initialized recon at .../recon.db"
recon init  # "Initialized recon at .../recon.db" (same message)
```

**Impact:** Minor confusion. Not destructive (migrations are idempotent).

**Fix:** Check if `.recon/recon.db` exists. If so, print "recon already
initialized" or "recon re-initialized (existing data preserved)".

### BUG-5: grep_pattern check quoting/escaping unclear

**Severity:** Low **Reproduction:**

```bash
recon decide "test" --check-type grep_pattern --check-pattern '"--json"' ...
# → "grep pattern matched 0 of 13 files"
# Expected: matches in all CLI command files
```

**Impact:** Agent trying to verify conventions via grep_pattern may get false
negatives. The interaction between shell quoting and the internal grep
implementation isn't obvious.

**Fix:** Document escaping rules. Consider adding a `decide --check-test` mode
that shows what the grep actually matched without creating a proposal.

---

## Missing Features (Prioritized for Agent Workflows)

### P0 — Immediately impactful

#### MISS-1: Fuzzy / partial symbol search

**Current:** `find` requires exact name match. `find "Build*"` → not found.

**Need:** An agent often knows _part_ of a symbol name. "Show me everything with
'Error' in the name" or "all symbols starting with 'New'".

**Suggested:** `find --contains Error` or `find --prefix New` or
`find --glob "Build*"`.

**Why P0:** This is the single most common friction point. An agent exploring a
new codebase doesn't know exact names. Every other code intelligence tool
supports this.

#### MISS-2: Package listing command

**Current:** No way to list packages. Must hack it:
`find --kind type --limit 500 | grep pkg= | sort -u`

**Need:** `recon packages` or `recon find --list-packages`

**Suggested output:**

```
internal/build     13 files  4500 lines  [HOT]
internal/conductor 12 files  4054 lines  [COLD]
...
```

**Why P0:** Package list is the first thing an agent needs for orientation.
Currently requires workaround that's fragile and wasteful.

### P1 — Significantly improves capability

#### MISS-3: Reverse dependency lookup (callers / "who uses this?")

**Current:** `find` shows what a symbol _depends on_ (outbound edges) but not
what _depends on it_ (inbound edges).

**Need:** "Who calls `NewOrchestrator`?" / "What depends on the `database`
package?"

**Suggested:** `find NewOrchestrator --callers` or `recon refs NewOrchestrator`

**Why P1:** Forward + backward dependency = complete understanding. Without
reverse deps, an agent can trace implementation but can't assess impact of
changes.

#### MISS-4: Package-level dependency query

**Current:** `orient` shows a dependency flow string but it's not queryable.

**Need:** `recon deps internal/conductor` → shows what it imports and what
imports it.

**Why P1:** Package-level deps are the quickest way to understand architecture
boundaries. The data is already in the DB (imports table).

#### MISS-5: Orient verbosity levels

**Current:** Orient has one output level.

**Need:**

- `orient --brief` — just freshness + summary counts (for quick health checks)
- `orient` — current default
- `orient --full` — include top symbols per module, decision details, recent git
  activity with context

**Why P1:** Different agent tasks need different depth. A "fix this bug" task
needs brief. A "refactor this system" task needs full.

### P2 — Good improvements

#### MISS-6: Incremental sync with fingerprint skip

The fingerprint is computed but sync always does a full re-index. At current
scale (128ms for 173 files) this doesn't matter, but it will at 1000+ files.

#### MISS-7: Interface / type API grouping

Can't ask "what methods does this type have?" or "what implements this
interface?" The data exists (methods have receivers) but there's no query for
it.

**Suggested:** `find --receiver Orchestrator` to list all methods on a type.

#### MISS-8: Export visibility filter

No `--exported` or `--unexported` flag on find. Useful for understanding a
package's public API vs internals.

#### MISS-9: File-level info

No `recon file <path>` command. An agent often starts from a file path (e.g.
from an error stack trace) and wants to know: what symbols are in this file,
what package does it belong to, what does it import.

#### MISS-10: Test file awareness

Test files don't appear to be indexed. An agent understanding test coverage
would benefit from `--include-tests` or a separate test index.

---

## Agent Workflow Observations

### What an agent actually does with recon (observed during this session)

1. **Orient** — First call. Gets project overview, module layout, active
   decisions. This worked well.

2. **Find types in a package** — `find --kind type --package internal/build`.
   Second most common operation. Works well.

3. **Find specific symbol** — `find Execute --package internal/conductor`. Works
   well, deps are the killer feature.

4. **Explore a concept** — "What handles builds?" This is where recon falls
   short. No fuzzy search, no concept search. Agent falls back to grep.

5. **Understand impact** — "If I change this, what breaks?" Requires reverse
   deps, which don't exist yet.

6. **Record learned knowledge** — decide/pattern. Works but is manual. Agent
   rarely does this unprompted.

### The orient payload as CLAUDE.md supplement

The orient output is compact enough (~3KB JSON) to inject into an agent's
context at session start. It effectively replaces the "Architecture" section of
CLAUDE.md with live, verified data. This is a strong use case.

**Gap:** Orient doesn't include "how to build/test" info. A
`recon orient --include-dev-commands` that reads the justfile/Makefile and
includes build commands would make orient a complete session-start payload.

### Token efficiency

All outputs are concise. The `--no-body` flag on find is essential for agent use
— without it, large function bodies eat context window. The `--max-body-lines`
flag is a good compromise.

One concern: `find --kind type --limit 500` on cortex_code returns 231 results.
That's a lot of tokens for listing. A `--compact` mode that just shows
`name (file:line)` without pkg repetition would help.

---

## Recommendations for Next Milestone

### Quick wins (can ship in a day):

1. Fix orient text to always show modules (BUG-1)
2. Widen heat window to 30 days (BUG-3)
3. Add `--list-packages` to find (MISS-2)
4. Add short package name matching (BUG-2)

### Medium effort (2-3 days each):

5. Fuzzy/partial symbol search (MISS-1)
6. `--receiver` filter for method grouping (MISS-7)
7. Incremental sync with fingerprint check (MISS-6)

### Larger features (1+ week):

8. Reverse dependency lookup (MISS-3)
9. Package dependency query (MISS-4)
10. Orient verbosity levels (MISS-5)
