# SQLite Knowledge Graph for Recon

## What Is This

Designing a graph-like knowledge layer for recon using only SQLite — an `edges`
table, traversal patterns, auto-linking, and command integration that gives
recon relationship awareness without adding any new infrastructure dependencies.

## Version History

| Version | Date       | Summary                                                                      |
| ------- | ---------- | ---------------------------------------------------------------------------- |
| v1      | 2026-02-16 | Full design: edges table, relation vocabulary, auto-linking, command surface |

## Major Decisions

1. Single generic `edges` table (not typed junction tables)
2. Knowledge edges only — code→code stays in imports/symbol_deps
3. 6 relation types: affects, evidenced_by, supersedes, contradicts, related,
   reinforces
4. Bidirectional relations stored as two directed rows
5. Conservative auto-linking (exact package paths, distinctive symbols, evidence
   refs)
6. Manual linking via CLI flags with high confidence
7. `to_ref` as text reference for code targets (survives sync)
8. Migrate `pattern_files` into edges table, drop old table

## Status

**Active** — Design complete, ready for implementation planning
