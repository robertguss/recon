# Project Rethink v1

## Quick Context

Rethinking repo-scout (Rust, code intelligence) and Cortex (Go, cognitive infrastructure) as a
single unified tool. Both projects have accumulated scope and markdown sprawl. Starting fresh to
find the essential core.

## Session Log

- **Date**: 2026-02-13
- **Energy**: Deep exploration
- **Mode**: Connected (cross-project awareness)
- **Methods**: Problem decomposition, convergence analysis

## Four Problems Identified

| #   | Problem          | Description                                                       |
| --- | ---------------- | ----------------------------------------------------------------- |
| 1   | Blank Slate      | Agent wastes time rediscovering codebase every session            |
| 2   | Blunt Instrument | Context window filled with irrelevant code, no surgical precision |
| 3   | Amnesia          | Work from previous sessions is invisible to new sessions          |
| 4   | Markdown Sprawl  | State scattered across hundreds of MD files, impossible to manage |

Problem 4 is a symptom of solving Problem 3 with the wrong tool (markdown as poor man's database).

## Core Vision: Index / Remember / Serve

Three capabilities, tightly integrated:

- **Index** — Understand codebase structure (AST-level symbols, relationships, architecture)
- **Remember** — Store decisions, tasks, session state, learnings in queryable local DB
- **Serve** — Generate focused context packets for agent sessions (minimum viable context)

## Decisions Made

| Decision                                      | Reasoning                                                                        |
| --------------------------------------------- | -------------------------------------------------------------------------------- |
| **Go, not Rust**                              | DX of Rust is awful; Robert prefers Go. repo-scout ideas get ported.             |
| **Agent-first**                               | Primary consumer is coding agents (Claude Code, etc.), not humans                |
| **Conductor/contracts are separate concerns** | Not core to Index/Remember/Serve                                                 |
| **Engine + methodology are inseparable**      | Engine without methodology gets ignored; methodology without engine has no teeth |
| **Structured storage, not markdown**          | SQLite + git-tracked files; no more markdown sprawl for state                    |

## Methodology Insight

Studied 6 spec-driven development frameworks (spec-kit, GSD, Superpowers, Compound Engineering,
BMAD, Agents marketplace). All converge on:

**Specify → Plan → Execute → Verify → Remember**

The methodology IS the interface. The agent doesn't optionally use the tool — the tool defines how
the agent works. Hooks, slash commands, mandatory activation.

## Emerging CLI Shape (~10 commands)

```
cortex init          → Index codebase, create .cortex/ with SQLite store
cortex serve         → Generate context packet for session start (via hook)
cortex orient        → What is this project, what happened last, what's next?
cortex find <symbol> → Surgical code extraction (function + deps, not whole files)
cortex plan <task>   → Create structured plan with tasks (stored in DB)
cortex task next     → What should I work on right now?
cortex task done     → Mark complete, capture what was learned
cortex remember <x>  → Store a decision, pattern, or insight
cortex recall <q>    → Query accumulated knowledge
cortex capture       → End-of-session state snapshot (via hook)
```

## Open Questions

1. **How opinionated should the methodology be?** Mandatory phases (Superpowers-style) vs. flexible
   phases (GSD-style)? Opinionated = more reliable but less flexible.
2. **What does the context packet (serve) actually contain?** Architecture summary? Recent session
   context? Relevant tasks? All three? How do we size it for context window limits?
3. **AST parsing in Go** — repo-scout uses tree-sitter via Rust bindings. Go has tree-sitter
   bindings too (go-tree-sitter) but maturity needs evaluation.
4. **MCP server vs CLI vs both?** Agent integration via MCP tools could be lower friction than
   Bash-based CLI calls.
5. **What from existing Cortex is worth porting?** 669 commits of mature Go code — some of the
   session/decision/storage patterns are directly reusable.

## Ideas Inventory

| Idea                          | Maturity   | Notes                                   |
| ----------------------------- | ---------- | --------------------------------------- |
| Index/Remember/Serve core     | Developing | Validated as the right framing          |
| ~10 command CLI               | Raw        | Needs concrete design                   |
| Hook-based auto serve/capture | Developing | Claude Code hooks enable this           |
| MCP server integration        | Raw        | Could replace or supplement CLI         |
| Opinionated SDLC methodology  | Developing | Studied 6 frameworks, patterns clear    |
| SQLite for all state          | Developing | Cortex hybrid model is a starting point |

## The Overnight Question

How opinionated should the methodology be? If the agent must follow a strict workflow, adoption is
reliable but flexibility suffers. If the workflow is optional, the agent drifts back to defaults.

Is there a middle ground — a minimal mandatory structure (orient → execute → capture) with optional
depth (plan, verify, recall) that activates based on task complexity?

## Next Steps

- Decide on methodology strictness
- Evaluate what's directly portable from Cortex's Go codebase
- Evaluate Go tree-sitter bindings for AST indexing
- Design the context packet format (what does `cortex serve` actually output?)
- Decide on MCP server vs CLI vs hybrid approach
