# Project Rethink: repo-scout + Cortex

## What Is This

Rethinking two overlapping projects — repo-scout (Rust, code navigation/AST parsing) and Cortex (Go,
session memory/knowledge graphs/orchestration) — with fresh eyes. Exploring what they should become,
whether they merge, split differently, or transform entirely.

## Version History

| Version | Date       | Summary                                                                 |
| ------- | ---------- | ----------------------------------------------------------------------- |
| v1      | 2026-02-13 | Initial exploration — identified 4 problems, Index/Remember/Serve core  |
| v2      | 2026-02-13 | Refined product definition, orient output design, competitive landscape |
| v3      | 2026-02-13 | Knowledge lifecycle, evidence system, storage architecture, hooks       |

## Major Decisions

1. Go, not Rust
2. Go-only language support (dogfood first, expand later)
3. Agent-first (coding agents are primary consumer)
4. Agent proposes, human approves (knowledge growth model)
5. Focus on unique value (don't compete with Entire or Beads)
6. SQLite only, no markdown for state
7. 9-command CLI surface

## Status

**Active** — Product definition solidifying, ready for schema design and prototyping
