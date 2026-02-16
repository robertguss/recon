# ADR-004: FTS5 Search Strategy for Recall

## Status

Accepted (2026-02-14)

## Context

The `recall` command needs to search across decisions and patterns by free-text
query. The search must be fast, relevant, and handle fuzzy matches.

## Decision

Use **SQLite FTS5** with Porter stemming as the primary search engine, with a
**LIKE fallback** for cases where FTS tokenization misses matches.

## Rationale

### FTS5 as primary

The `search_index` virtual table uses FTS5 with Porter stemming:

```sql
CREATE VIRTUAL TABLE search_index USING fts5 (
    title,
    content,
    entity_type UNINDEXED,
    entity_id UNINDEXED,
    tokenize='porter'
);
```

- **Porter stemming** — "testing" matches "test", "tests", "tested"
- **Ranked results** — FTS5's `rank` function orders by relevance
- **Fast** — FTS5 uses an inverted index, much faster than LIKE for text search
- **Built-in** — No additional dependencies, comes with SQLite

### LIKE fallback

FTS5 tokenization can miss matches that a substring search would find. For
example, a query for a package path like "internal/orient" might not match via
FTS5 tokens but would match via LIKE.

The recall service implements a two-stage strategy:

1. Try FTS5 MATCH query
2. If FTS5 fails (error) or returns no results, fall back to LIKE query
3. Both stages have a legacy path for databases without the patterns table

### UNINDEXED columns

`entity_type` and `entity_id` are stored in the FTS table but not tokenized.
This allows joining back to the source tables without adding them to the search
index.

### Alternatives considered

- **FTS5 only** — Simpler but misses substring matches for paths and technical
  terms
- **LIKE only** — No relevance ranking, slow on large datasets
- **External search engine (Bleve, Tantivy)** — Overkill for a local CLI tool
  with thousands (not millions) of documents
- **trigram index** — Better for fuzzy matching but more complex setup

## Consequences

- The FTS5 index must be updated whenever decisions or patterns are
  created/modified
- Porter stemming provides natural language matching but may over-stem technical
  terms
- The LIKE fallback ensures no query returns zero results when matches exist
- Legacy query paths handle databases that haven't run migration 003 (patterns
  table)
- Search performance is excellent for the expected scale (hundreds of decisions,
  not millions)
