# Project Rethink v2

## Quick Context

Cortex is a code intelligence engine with a knowledge graph, built for AI coding agents. It
understands codebases at the AST level, accumulates knowledge over time, and serves focused context
packets so agents start sessions informed instead of blind.

## Session Log

- **Date**: 2026-02-13 (continued from v1)
- **Energy**: Deep exploration
- **Mode**: Connected
- **Methods**: Problem decomposition, convergence analysis, competitive landscape review

## The Product (Refined)

### What Cortex IS

A code intelligence engine with a knowledge graph, built for AI coding agents. It understands your
codebase at the AST level and accumulates knowledge about it over time — decisions, patterns,
issues, relationships. When an agent starts a session, Cortex serves a focused context packet.

### What Cortex IS NOT

- Not a session capture tool (Entire does that)
- Not a task tracker (Beads does that)
- Not a multi-agent orchestrator
- Not an SDLC methodology framework

### Unique Value

The thing nobody else is building: **code intelligence connected to accumulated knowledge.** The
knowledge graph that links files → symbols → decisions → patterns → issues. Entire captures
sessions. Beads tracks tasks. Cortex understands the code itself and what humans know about it.

## Four Problems This Solves

| #   | Problem          | How Cortex Solves It                                    |
| --- | ---------------- | ------------------------------------------------------- |
| 1   | Blank Slate      | `orient` serves a context packet on session start       |
| 2   | Blunt Instrument | `find` extracts exact functions + deps, not whole files |
| 3   | Amnesia          | Knowledge graph accumulates decisions, patterns, issues |
| 4   | Markdown Sprawl  | All state in SQLite, not scattered markdown files       |

## All Decisions Made

| Decision                             | Reasoning                                                   |
| ------------------------------------ | ----------------------------------------------------------- |
| **Go, not Rust**                     | Better DX, Robert's preference                              |
| **Go-only language support**         | Nail one language first, dogfood on itself, expand later    |
| **Agent-first**                      | Primary consumer is coding agents, not humans               |
| **Agent proposes, human approves**   | Knowledge graph grows through collaboration, not automation |
| **Conductor/contracts are separate** | Not core to code intelligence + knowledge graph             |
| **Engine + methodology inseparable** | But methodology is lightweight, not a full SDLC framework   |
| **Structured storage, not markdown** | SQLite for all state                                        |
| **Focus on unique value**            | Don't compete with Entire (sessions) or Beads (tasks)       |

## Command Set (9 commands)

```
cortex init                         → Index codebase, create .cortex/ with SQLite
cortex sync                         → Re-index (incremental, content-hash change detection)
cortex tree [--depth] [--focus]     → Annotated tree: structure + deps + knowledge
cortex find <symbol>                → Surgical extraction: function body + deps
cortex orient [--for "task desc"]   → Context packet for agent session start
cortex decide <title>               → Log a decision (agent proposes, human approves)
cortex pattern <name>               → Record a codebase pattern/convention
cortex issue <title>                → Track a known problem
cortex recall <query>               → Query the knowledge graph (FTS)
```

Each command either populates the graph (init, sync, decide, pattern, issue) or queries it (tree,
find, orient, recall).

## Orient Output Design

### Baseline Orient (task-independent, session start hook)

```
# Project: cortex
Language: Go | Module: github.com/user/cortex | 42 files | 8,200 lines

## Architecture
cmd/main.go → internal/commands/ → internal/graph/ → internal/database/
                                 → internal/analyzer/
Dependency flow is top-down. Nothing in database/ calls up to commands/.
Entry point: cmd/main.go | CLI framework: Cobra

## Modules
internal/commands/   9 files  24 symbols  HOT
internal/graph/      3 files  12 symbols  WARM  decisions(2)
internal/database/   6 files  18 symbols  COLD  patterns(1)
internal/analyzer/   4 files  10 symbols  COLD
internal/types/      3 files  15 symbols  COLD
internal/storage/    2 files   6 symbols  COLD

## Patterns & Conventions
- All commands go through service layer, never call DB directly
- Errors use fmt.Errorf with %w wrapping, never panic
- Tests use testify assertions, one TestXxx per exported function

## Active Knowledge
- DECISION: "Hybrid storage - JSON source of truth, SQLite cache" (high confidence)
- DECISION: "Content hash for incremental sync" (high confidence)
- ISSUE: "graph sync fails on circular imports" (open, high, analyzer/parser.go:84)

## Recent Activity
- Hot: internal/commands/ (4 sessions)
- Warm: internal/graph/ (1 session)
- Last session: Working on graph sync performance
```

Key design choices (from agent feedback):

- **Flat tabular module list** instead of nested tree (faster for agent to parse)
- **Architecture as dependency flow** with explicit boundary rules
- **Patterns surfaced upfront** so agent knows conventions before writing code
- **Dense, scannable format** — no visual decoration

### Task-Focused Orient (--for "task description")

Adds to baseline:

- Matching issues with file references
- **Actual code snippets** (function bodies, not just signatures) within a token budget
- Relevant decisions and patterns for the affected code
- Dependency chain showing call flow through affected functions

## Knowledge Graph Model

### Entities

| Entity    | Purpose                                    | Created By                     |
| --------- | ------------------------------------------ | ------------------------------ |
| Files     | Source files with metadata                 | `sync` (automated)             |
| Symbols   | Functions, types, consts, vars with bodies | `sync` (automated)             |
| Imports   | Dependency relationships between files     | `sync` (automated)             |
| Packages  | Go packages as directories                 | `sync` (automated)             |
| Decisions | Why something was built a certain way      | Agent proposes, human approves |
| Patterns  | How code should be written here            | Agent proposes, human approves |
| Issues    | Known problems                             | Agent proposes, human approves |

### Relationships

- Files → Symbols (contains)
- Files → Imports → Packages (depends on)
- Files ← Dependents (used by)
- Files ↔ Decisions (constrained by / affects)
- Files ↔ Patterns (applies to)
- Files ↔ Issues (has problem in)

### Storage

SQLite only. No JSON source-of-truth files. No markdown for state. The database IS the source of
truth. Git tracks the schema and migrations, not the data.

## Competitive Landscape

| Tool                     | Focus                               | Overlap with Cortex                                      |
| ------------------------ | ----------------------------------- | -------------------------------------------------------- |
| Entire (entireio/cli)    | Session capture & replay            | Amnesia (sessions only)                                  |
| Beads (steveyegge/beads) | Graph-based task tracking           | State management (tasks only)                            |
| Cortex                   | Code intelligence + knowledge graph | Unique: connects code structure to accumulated knowledge |

## Open Questions

1. What does the `orient` token budget look like? How do we cap output size for context windows?
2. How does `orient --for` match tasks to relevant code? (FTS + issue linking + dep walking?)
3. Hook integration specifics — what does the Claude Code hook config look like?
4. How does the approval flow work? Agent outputs a proposal, human confirms in terminal?

## Next Steps

- Design the SQLite schema for the knowledge graph
- Prototype `cortex init` + `cortex sync` (Go AST indexing)
- Prototype `cortex orient` baseline output
- Define the agent proposal → human approval UX flow
- Set up the new Go project (retire repo-scout Rust, fresh start or fork Cortex)
