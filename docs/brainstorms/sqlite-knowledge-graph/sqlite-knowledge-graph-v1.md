# SQLite Knowledge Graph v1

## Quick Context

Recon indexes Go code into SQLite and records decisions/patterns with evidence.
But knowledge entities float independently — no relationships between them, no
links to the code they describe. This design adds a lightweight graph layer
using a single `edges` table, keeping recon's zero-dependency architecture.

## Session Log

- **Date**: 2026-02-16
- **Energy**: Deep exploration
- **Mode**: Connected (cross-references to project-rethink, dogfood findings,
  cortex ecosystem analysis)

## The Problem

Recon has two implicit graph structures that don't connect:

1. **Code graph** — `imports` (file → package), `symbol_deps` (symbol → symbol),
   files → packages. Rich, queryable, already working.

2. **Knowledge entities** — decisions, patterns, evidence. Flat. No
   relationships to each other or to the code they describe.

The dogfood findings (M5) identified this gap: an agent can find symbols and
read decisions, but can't answer "what decisions affect this package?" or "what
patterns relate to this decision?" The cortex ecosystem analysis identified that
SQLite hits a ceiling — but that ceiling is higher than originally estimated.

## Design Decisions

### Decision 1: Single Generic Edges Table

**Rejected alternative:** Typed junction tables (`decision_packages`,
`pattern_symbols`, `decision_patterns`, etc.) + UNION ALL view.

**Chosen:** Single `edges` table with `(from_type, from_id, to_type, to_ref)`
polymorphic references.

**Reasoning:** The whole point is making relationships traversable as a graph.
Typed junction tables fragment the graph. A single table means every traversal
query uses the same pattern. The FK integrity loss is manageable — enforced in
Go code, consistent with the existing `evidence` table pattern.

### Decision 2: Knowledge Edges Only

**Rejected alternative:** Put code→code relationships (imports, symbol_deps)
into the edges table for unified traversal.

**Chosen:** Edges table is only for knowledge↔knowledge and knowledge→code.
Code→code stays in `imports` and `symbol_deps`.

**Reasoning:** Code→code edges are bulk-created during `recon sync` (hundreds of
rows). Duplicating them in edges creates a sync problem and bloats the table.
When unified traversal is needed, a Go function walks both tables — not a single
SQL query.

### Decision 3: Six Relation Types

| Relation       | Direction     | Example                              |
| -------------- | ------------- | ------------------------------------ |
| `affects`      | Directed      | decision #1 → package `internal/cli` |
| `evidenced_by` | Directed      | pattern #1 → file `root.go`          |
| `supersedes`   | Directed      | decision #3 → decision #1            |
| `contradicts`  | Bidirectional | decision #2 ↔ pattern #1             |
| `related`      | Bidirectional | decision #1 ↔ decision #2            |
| `reinforces`   | Directed      | pattern #1 → decision #2             |

Bidirectional relations stored as two directed rows. Storage cost is negligible
(dozens of edges, not millions). Uniform traversal — always query `from_*`
columns.

### Decision 4: Text References for Code Targets

**Rejected alternatives:**

- Integer FK to code entity IDs (breaks on every sync — IDs are volatile)
- Dual columns (to_id + to_ref with post-sync re-resolution)
- Stable IDs for code entities (requires sync engine changes)

**Chosen:** `to_ref` as text reference (package path, file path, or symbol
name). Resolved to IDs at query time via joins.

**Reasoning:** `from_type` is always a knowledge entity (decision, pattern) with
stable IDs. Only the `to_*` side points at code entities whose IDs change on
sync. Text references survive sync, DB rebuilds, snapshot imports. The edge
count is small enough that string joins are effectively free. Matches how agents
think — they say `--affects internal/cli`, not `--affects 7`.

### Decision 5: Conservative Auto-Linking

**Auto-link when:**

- Reasoning contains a literal package path that exists in the index
- Title contains an exact exported symbol name (6+ chars, not a common word)
- Evidence `check_spec` references specific files

**Don't auto-link when:**

- Short/common symbol names (Error, Run, New)
- Substring matches (avoid "cli" matching "clicking")
- Ambiguous short package names

**Source tracking:** `source='auto'` with `confidence='medium'`. Manual links
get `source='manual'` with `confidence='high'`. Agents and humans can prune bad
auto-links.

**Reasoning:** Trust is binary — if auto-links are noisy, the whole graph gets
ignored. Conservative now, eager later is easy (add low-confidence matchers).
Eager now, conservative later means you've already trained users not to trust
it.

### Decision 6: Migrate pattern_files to Edges

Current `pattern_files` stores `(pattern_id, file_path TEXT)` — no FK to
`files.id`, not traversable. Migration resolves `file_path` → `files.id` at
migration time, creates `affects` edges, drops the table.

## Schema

```sql
CREATE TABLE IF NOT EXISTS edges (
    id          INTEGER PRIMARY KEY,
    from_type   TEXT NOT NULL,     -- 'decision', 'pattern'
    from_id     INTEGER NOT NULL,
    to_type     TEXT NOT NULL,     -- 'decision', 'pattern', 'package', 'file', 'symbol'
    to_ref      TEXT NOT NULL,     -- stable reference: path for packages/files, name for symbols, id-as-string for knowledge
    relation    TEXT NOT NULL,     -- 'affects', 'evidenced_by', 'supersedes', 'contradicts', 'related', 'reinforces'
    source      TEXT NOT NULL DEFAULT 'manual',  -- 'manual', 'auto', 'inferred'
    confidence  TEXT NOT NULL DEFAULT 'medium',  -- 'low', 'medium', 'high'
    created_at  TEXT NOT NULL,
    UNIQUE(from_type, from_id, to_type, to_ref, relation)
);

CREATE INDEX idx_edges_from ON edges(from_type, from_id);
CREATE INDEX idx_edges_to ON edges(to_type, to_ref);
CREATE INDEX idx_edges_relation ON edges(relation);
```

## Command Integration

### orient — Nest Knowledge Under Modules

Decisions and patterns appear under the modules they affect:

```
Modules:
  internal/cli      13 files  1403 lines  [HOT]
    decisions: #2 ExitError is the standard error type
    patterns:  #1 Testability injection via package-level vars
  internal/orient    2 files   516 lines  [HOT]
    decisions: #1 Services use constructor injection via NewService
```

Query: join edges where `to_type='package'` and `relation='affects'` against the
module list. Only show `affects` edges — orient is a summary view.

### find — Show Knowledge Context

When finding a symbol, show decisions/patterns that affect it:

```
ExitError  type  internal/cli/exit_error.go:10-16  [exported]
  deps: fmt
  decision #2: ExitError is the standard error type (high)
```

Show knowledge edges in `--json` mode always (agents use JSON). Text mode could
be behind a `--context` flag to avoid clutter.

### decide / pattern — Creation with Linking

Manual linking via flags:

```bash
recon decide "ExitError is the standard error type" \
  --reasoning "All CLI commands return ExitError..." \
  --affects internal/cli \
  --affects-symbol ExitError \
  --related-pattern 1
```

Auto-linking runs after creation, reports what it found:

```
Created decision #3
  auto-linked: internal/cli (package path in reasoning)
```

Unresolved targets warn but don't fail.

### recall — Graph-Aware Search

FTS search + 1-hop edge walk from results:

```bash
recon recall "error handling"
# → Decision #2: ExitError is the standard error type (high)
#     affects: internal/cli
#     reinforced by: Pattern #1 Testability injection via package-level vars
```

Default: 1 hop. `--depth 2` for 2 hops. Never beyond 2.

### edges (new) — Direct Edge Management

```bash
recon edges --from decision:2         # Show edges from decision #2
recon edges --to package:internal/cli # Show edges pointing at this package
recon edges --delete 7                # Remove an edge by ID
recon edges --list                    # Show all edges
```

Escape hatch for maintenance and debugging.

## Key Query Patterns

### Edges for a module (orient)

```sql
SELECT e.from_type, e.from_id, e.relation,
       COALESCE(d.title, p.title) as title,
       COALESCE(d.confidence, p.confidence) as confidence
FROM edges e
LEFT JOIN decisions d ON e.from_type = 'decision' AND e.from_id = d.id
LEFT JOIN patterns p ON e.from_type = 'pattern' AND e.from_id = p.id
WHERE e.to_type = 'package' AND e.to_ref = ?
  AND e.relation = 'affects'
ORDER BY confidence DESC;
```

### Knowledge pointing at a symbol (find)

```sql
SELECT e.from_type, e.from_id, e.relation,
       COALESCE(d.title, p.title) as title
FROM edges e
LEFT JOIN decisions d ON e.from_type = 'decision' AND e.from_id = d.id
LEFT JOIN patterns p ON e.from_type = 'pattern' AND e.from_id = p.id
WHERE e.to_type = 'symbol' AND e.to_ref = ?;
```

### 1-hop walk from a knowledge entity (recall)

```sql
SELECT e.to_type, e.to_ref, e.relation, e.confidence,
       COALESCE(pkg.path, f.path, d.title, p.title) as target_name
FROM edges e
LEFT JOIN packages pkg ON e.to_type = 'package' AND e.to_ref = pkg.path
LEFT JOIN files f ON e.to_type = 'file' AND e.to_ref = f.path
LEFT JOIN decisions d ON e.to_type = 'decision' AND e.to_ref = CAST(d.id AS TEXT)
LEFT JOIN patterns p ON e.to_type = 'pattern' AND e.to_ref = CAST(p.id AS TEXT)
WHERE e.from_type = ? AND e.from_id = ?;
```

### Multi-hop traversal (future, recall --depth 2)

```sql
WITH RECURSIVE graph(entity_type, entity_ref, depth, path) AS (
    SELECT to_type, to_ref, 1, from_type || ':' || from_id
    FROM edges
    WHERE from_type = ? AND from_id = ?

    UNION

    SELECT e.to_type, e.to_ref, g.depth + 1,
           g.path || ' -> ' || e.from_type || ':' || e.from_id
    FROM graph g
    JOIN edges e ON e.from_type = g.entity_type
                AND e.from_id = CAST(g.entity_ref AS INTEGER)
    WHERE g.depth < ?
      AND g.entity_type IN ('decision', 'pattern')  -- only traverse knowledge nodes
)
SELECT DISTINCT entity_type, entity_ref, depth FROM graph
ORDER BY depth, entity_type;
```

## Implementation Order (Suggested)

| Phase | What                                           | Why First                                  |
| ----- | ---------------------------------------------- | ------------------------------------------ |
| 1     | Migration: edges table + pattern_files migrate | Foundation — everything depends on this    |
| 2     | Edge service: CRUD operations                  | Needed by all commands                     |
| 3     | decide/pattern: --affects and auto-linking     | Populates the graph                        |
| 4     | orient: nest knowledge under modules           | Biggest agent impact, validates the design |
| 5     | recall: 1-hop edge walk                        | Second biggest impact                      |
| 6     | find: knowledge context                        | Nice-to-have, lower priority               |
| 7     | edges command: direct management               | Escape hatch, can ship anytime             |

## Open Questions

1. **Symbol reference format in `to_ref`** — just the name (`ExitError`) or
   qualified (`internal/cli.ExitError`)? Qualified is unambiguous but verbose.
   Just the name risks collisions across packages.

2. **Edge limits per orient module** — if a module has 15 decisions pointing at
   it, orient becomes noisy. Cap at 3-5 per module? Or show count with "and 12
   more"?

3. **Snapshot inclusion** — edges are knowledge (not re-derivable). Should they
   be included in the git-tracked knowledge snapshot?

## The Overnight Question

Is the `to_ref` format for symbols right? `ExitError` is unique in recon today,
but in a larger codebase, multiple packages might have a `Service` type. Should
`to_ref` for symbols be `package_path:symbol_name` from day one?
