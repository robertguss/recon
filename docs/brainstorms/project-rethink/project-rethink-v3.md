# Project Rethink v3

## Quick Context

Cortex is a code intelligence engine with a knowledge graph, built for AI coding agents. This
version adds the knowledge lifecycle (propose → verify → promote), evidence system with drift
detection, and storage/durability design.

## Session Log

- **Date**: 2026-02-13 (continued from v2)
- **Energy**: Deep exploration
- **Mode**: Connected
- **Topics**: Approval flow, evidence system, storage architecture, hooks integration

## Knowledge Lifecycle

### The Flow: Propose → Verify → Promote

The agent is the primary operator. The human is an occasional auditor, not a gatekeeper.

1. **Propose** — During a session, the agent writes freely to a staging area in SQLite. No approval
   needed, no friction. Captures patterns, decisions, issues as it encounters them.

2. **Verify** — Before promoting, the agent checks proposals against the actual codebase. "I said
   errors always use `%w` — let me verify." Runs verification, records evidence. Proposals must be
   backed by evidence from the code.

3. **Promote** — Verified proposals are promoted to the permanent knowledge graph automatically. No
   human in the loop. The evidence is what makes this safe.

4. **Audit (optional)** — Human can review accumulated knowledge via `cortex recall` or
   `cortex review` at any time. Can remove or correct entries. But this is optional.

### Design Principle

The agent proposes, reviews, and verifies. Minimum human interaction. The evidence system is what
prevents garbage from accumulating — every knowledge entry must be backed by verifiable claims about
the codebase.

## Evidence System

Every knowledge entry (decision, pattern, issue) has associated evidence.

### Evidence Has Two Parts

**Summary** — Human/agent-readable explanation: "Verified across 47 error handling sites. 46/47 use
`fmt.Errorf` with `%w` wrapping. Exception: `parser.go:23` uses bare `errors.New`."

**Check** (optional) — Structured, re-runnable verification:

```json
{
  "type": "grep_pattern",
  "pattern": "fmt\\.Errorf.*%w",
  "scope": "*.go",
  "expect": "majority",
  "baseline": {
    "matched": 46,
    "total": 47,
    "exceptions": ["internal/analyzer/parser.go:23"]
  }
}
```

### Drift Detection

On `cortex sync`, re-runnable checks are re-executed against the current codebase. If results
diverge from baseline, the entry gets flagged:

- **ok** — evidence still holds
- **drifting** — partially degraded (e.g., was 98%, now 73%)
- **broken** — evidence no longer supports the claim

`cortex orient` surfaces drift warnings so the agent knows which knowledge may be stale.

### Evidence Schema Addition

```sql
evidence (
    id                INTEGER PRIMARY KEY,
    entity_type       TEXT NOT NULL,        -- pattern, decision, issue
    entity_id         INTEGER NOT NULL,
    summary           TEXT NOT NULL,
    check_type        TEXT,                 -- grep_pattern, symbol_count, file_exists, null
    check_spec        TEXT,                 -- JSON for re-runnable checks
    baseline          TEXT,                 -- JSON baseline results
    last_verified_at  TEXT,
    last_result       TEXT,                 -- JSON most recent re-verification
    drift_status      TEXT DEFAULT 'ok'     -- ok, drifting, broken
)
```

## Storage Architecture

### Design: SQLite + Git-Tracked Snapshot

**SQLite is the source of truth.** Single data store, single write path, fast queries.

**A snapshot file is the safety net.** After every knowledge promotion, the knowledge tables
(decisions, patterns, issues, evidence — NOT the code index) are exported to a single JSON file
that's git-tracked.

```
.cortex/
  cortex.db                  (git-ignored — source of truth)
  knowledge-snapshot.json    (git-tracked — durability backup)
```

### Why This Design

| Concern                            | How It's Handled                                                      |
| ---------------------------------- | --------------------------------------------------------------------- |
| Data loss (DB corruption/deletion) | Restore from git-tracked snapshot                                     |
| Code index loss                    | `cortex sync` rebuilds from source code (re-derivable)                |
| Sprawl                             | One snapshot file, not hundreds of markdown/JSON files                |
| Sync bugs                          | One-directional: DB → snapshot. Never snapshot → DB during normal ops |
| Collaboration                      | Snapshot is diffable/mergeable in git                                 |

### Data Risk Tiers

| Data                                  | Re-derivable?                   | Protected By         |
| ------------------------------------- | ------------------------------- | -------------------- |
| Files, symbols, imports, packages     | Yes — rebuild via `cortex sync` | Source code itself   |
| Decisions, patterns, issues, evidence | No — accumulated knowledge      | Snapshot file in git |
| Sessions, activity tracking           | No — but historical only        | Snapshot file in git |

### Hook-Driven Durability

Claude Code hooks make durability automatic and invisible:

**session_start hook:**

1. Run `cortex sync --quick` — check DB and snapshot are in sync
2. If DB missing/corrupted, auto-restore from snapshot
3. Run `cortex orient` — serve context packet to agent

**session_end hook:**

1. Run `cortex capture` — write final snapshot, record session summary

**During session:**

- Each knowledge promotion writes to SQLite AND updates snapshot
- Worst case crash scenario: lose session summary, never lose knowledge

### Crash Safety

| Scenario              | Data Loss                                                                    |
| --------------------- | ---------------------------------------------------------------------------- |
| Normal session end    | None — hook writes snapshot                                                  |
| Ctrl-C                | None — snapshot updated on each promotion                                    |
| Hard crash (kill -9)  | At most the session summary                                                  |
| DB file deleted       | None — auto-restore from snapshot on next session start                      |
| Snapshot file deleted | None — DB is source of truth, snapshot regenerated                           |
| Both deleted          | Code index rebuilt from source. Knowledge lost. (git history is last resort) |

## Full Schema (Updated)

```sql
-- Code entities (re-derivable via cortex sync)

CREATE TABLE packages (
    id          INTEGER PRIMARY KEY,
    path        TEXT UNIQUE NOT NULL,
    name        TEXT NOT NULL,
    import_path TEXT,
    file_count  INTEGER DEFAULT 0,
    line_count  INTEGER DEFAULT 0,
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);

CREATE TABLE files (
    id          INTEGER PRIMARY KEY,
    package_id  INTEGER REFERENCES packages(id),
    path        TEXT UNIQUE NOT NULL,
    language    TEXT NOT NULL DEFAULT 'go',
    lines       INTEGER NOT NULL,
    hash        TEXT NOT NULL,
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);

CREATE TABLE symbols (
    id          INTEGER PRIMARY KEY,
    file_id     INTEGER REFERENCES files(id) ON DELETE CASCADE,
    kind        TEXT NOT NULL,
    name        TEXT NOT NULL,
    signature   TEXT,
    body        TEXT,
    line_start  INTEGER NOT NULL,
    line_end    INTEGER NOT NULL,
    exported    BOOLEAN NOT NULL,
    receiver    TEXT,
    UNIQUE(file_id, kind, name)
);

CREATE TABLE imports (
    id            INTEGER PRIMARY KEY,
    from_file_id  INTEGER REFERENCES files(id) ON DELETE CASCADE,
    to_path       TEXT NOT NULL,
    to_package_id INTEGER REFERENCES packages(id),
    alias         TEXT,
    import_type   TEXT NOT NULL,
    UNIQUE(from_file_id, to_path)
);

-- Knowledge entities (agent-verified, not re-derivable)

CREATE TABLE decisions (
    id          INTEGER PRIMARY KEY,
    title       TEXT NOT NULL,
    reasoning   TEXT NOT NULL,
    confidence  TEXT NOT NULL DEFAULT 'medium',
    status      TEXT NOT NULL DEFAULT 'active',
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);

CREATE TABLE decision_files (
    decision_id INTEGER REFERENCES decisions(id) ON DELETE CASCADE,
    file_id     INTEGER REFERENCES files(id) ON DELETE CASCADE,
    PRIMARY KEY (decision_id, file_id)
);

CREATE TABLE patterns (
    id           INTEGER PRIMARY KEY,
    name         TEXT NOT NULL,
    description  TEXT NOT NULL,
    example      TEXT,
    anti_example TEXT,
    created_at   TEXT NOT NULL,
    updated_at   TEXT NOT NULL
);

CREATE TABLE pattern_files (
    pattern_id INTEGER REFERENCES patterns(id) ON DELETE CASCADE,
    file_id    INTEGER REFERENCES files(id) ON DELETE CASCADE,
    PRIMARY KEY (pattern_id, file_id)
);

CREATE TABLE issues (
    id          INTEGER PRIMARY KEY,
    title       TEXT NOT NULL,
    description TEXT,
    severity    TEXT NOT NULL DEFAULT 'medium',
    status      TEXT NOT NULL DEFAULT 'open',
    file_id     INTEGER REFERENCES files(id),
    line_number INTEGER,
    created_at  TEXT NOT NULL,
    resolved_at TEXT
);

CREATE TABLE evidence (
    id               INTEGER PRIMARY KEY,
    entity_type      TEXT NOT NULL,
    entity_id        INTEGER NOT NULL,
    summary          TEXT NOT NULL,
    check_type       TEXT,
    check_spec       TEXT,
    baseline         TEXT,
    last_verified_at TEXT,
    last_result      TEXT,
    drift_status     TEXT DEFAULT 'ok'
);

-- Staging (proposals before verification/promotion)

CREATE TABLE proposals (
    id          INTEGER PRIMARY KEY,
    session_id  INTEGER REFERENCES sessions(id),
    entity_type TEXT NOT NULL,
    entity_data TEXT NOT NULL,       -- JSON blob matching target table schema
    status      TEXT NOT NULL DEFAULT 'pending',  -- pending, verified, promoted, rejected
    proposed_at TEXT NOT NULL,
    verified_at TEXT,
    promoted_at TEXT
);

-- Activity tracking

CREATE TABLE sessions (
    id         INTEGER PRIMARY KEY,
    started_at TEXT NOT NULL,
    ended_at   TEXT,
    summary    TEXT
);

CREATE TABLE session_files (
    session_id INTEGER REFERENCES sessions(id) ON DELETE CASCADE,
    file_id    INTEGER REFERENCES files(id) ON DELETE CASCADE,
    PRIMARY KEY (session_id, file_id)
);

-- Full-text search

CREATE VIRTUAL TABLE search_index USING fts5 (
    title,
    content,
    entity_type,
    entity_id,
    tokenize='porter'
);
```

## All Decisions Made (Cumulative)

| #   | Decision                                      | Reasoning                                                           |
| --- | --------------------------------------------- | ------------------------------------------------------------------- |
| 1   | Go, not Rust                                  | Better DX, Robert's preference                                      |
| 2   | Go-only language support                      | Nail one language, dogfood on itself                                |
| 3   | Agent-first                                   | Primary consumer is coding agents                                   |
| 4   | Agent proposes, verifies, promotes            | Minimum human interaction; evidence prevents garbage                |
| 5   | Conductor/contracts are separate              | Not core to code intelligence + knowledge graph                     |
| 6   | Engine + methodology inseparable              | Lightweight methodology, not full SDLC                              |
| 7   | Focus on unique value                         | Don't compete with Entire or Beads                                  |
| 8   | SQLite source of truth + git-tracked snapshot | One data store, one backup file, no sprawl                          |
| 9   | Hook-driven durability                        | session_start restores, session_end snapshots, automatic            |
| 10  | Evidence-backed knowledge                     | Every entry has evidence; re-runnable checks enable drift detection |
| 11  | Fresh project, not fork                       | Cortex has too much legacy; carry ideas, not code                   |

## Open Questions

1. What does the `orient` token budget look like? How do we cap output size?
2. How does `orient --for` match tasks to relevant code?
3. Specific Claude Code hook configuration format
4. How does `cortex review` present accumulated knowledge for human audit?
5. What check_types does the evidence system support beyond grep_pattern?
6. Project name — still "Cortex" or something new for the fresh start?

## Next Steps

1. Decide on project name and location
2. Initialize Go project with module, basic CLI skeleton (Cobra)
3. Implement schema + migrations
4. Implement `cortex init` + `cortex sync` (Go AST indexing)
5. Implement `cortex orient` baseline output
6. Implement proposal → verify → promote lifecycle
7. Implement snapshot backup/restore
8. Set up Claude Code hooks for session_start/session_end
9. Dogfood on itself
