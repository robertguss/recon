# Cortex Ecosystem Analysis

**Date:** 2026-02-15 **Purpose:** Understand the full landscape of Robert's code
intelligence + knowledge graph work across three projects, and identify the path
forward.

---

## The Three Projects

### 1. Recon (current)

- **What:** Go CLI for code intelligence on Go repos
- **Storage:** SQLite (in-repo, `.recon/recon.db`)
- **Strengths:** Fast (128ms for 173 files), symbol indexing, dependency
  tracking, decision/pattern recording with evidence verification
- **Limitations:** Flat knowledge model, no relationships between knowledge
  entities, no fuzzy search, in-repo only
- **Delivery:** CLI binary, `--json` for agent consumption

### 2. Cortex Code (`cortex_code`)

- **What:** Go CLI, predecessor/sibling to recon
- **Storage:** SQLite
- **Scale:** ~69K lines Go, 23 packages
- **Notable:** Has conductor (orchestration engine), build system, session
  management — more ambitious scope than recon

### 3. Cortex Canvas (`cortex`)

- **What:** Visual shared thinking space for humans + AI
- **Storage:** ArangoDB (multi-model) + TypeDB (inference engine)
- **Delivery:** Python MCP server (FastMCP) + React UI (ReactFlow)
- **Status:** MVP complete (phases 1-7), phase 8 in progress

---

## What Cortex Canvas Already Built

### Dual-Database Architecture

```
ArangoDB (The Body)          TypeDB (The Mind)
├── nodes collection         ├── entities (idea, decision, question)
├── edges collection         ├── relations (influence, blocks, related)
├── cortex_graph             ├── recursive functions (transitive inference)
├── spatial MDI index        └── inferred relationships
└── explicit relationships
         │
         └──── batch sync ────►
```

**Key insight:** ArangoDB stores what users explicitly create. TypeDB discovers
what's _implied_ — transitive relationships, influence chains. "If A influenced
B and B influenced C, then A transitively influenced C."

### Node Types

- **idea** — raw concepts from exploration
- **decision** — crystallized choices with reasoning
- **question** — open, unresolved topics

### Edge Types (5, all directed except "related")

- **sparked** — source idea led to target idea
- **led_to** — decision led to outcome
- **related** — bidirectional association
- **blocks** — constraint/obstacle
- **implements** — code realizes idea/decision

### MCP Tools Already Implemented

- `create_node`, `update_node`, `delete_node`
- `create_edge`, `delete_edge`
- `query_nodes`, `query_edges`
- `nearby(node_id, radius)` — spatial proximity query
- `traverse(node_id, depth)` — BFS graph traversal
- `infer(query_type, node_id)` — TypeDB transitive inference
- `trace(node_id, direction)` — complete trace with explicit + inferred
- `sync_databases()` — ArangoDB → TypeDB sync
- `health_check()`

### The Vision Docs

Two key documents capture the north star:

**`dual-knowledge-graph.md`** — Links code graph to ideas graph via cross-graph
edges (`implements`, `addresses`, `violates`, `supersedes`). Enables queries:

- "Why does this code exist?" → traces to ideas
- "What ideas have no implementing code?" → gap detection
- "If I change this decision, what breaks?" → impact analysis
- "What decisions were made in January?" → decision archaeology

**`vision.md`** — The canvas IS the database. Position is semantic data. Human
spatial intuition (clustering) becomes queryable structure. Neither the visual
nor the graph representation is "primary" — they're the same thing, experienced
differently.

---

## The Gap Between Recon and Cortex Canvas

### What Recon Has That Canvas Doesn't

- **Code indexing** — AST parsing, symbol extraction, dependency tracking
- **Evidence verification** — decisions tied to verifiable code checks
- **Drift detection** — knowledge validity checked against actual code
- **CLI speed** — 36ms sync, instant queries
- **In-repo locality** — `.recon/` lives with the code

### What Canvas Has That Recon Doesn't

- **Graph relationships** — nodes connected by typed, directed edges
- **Inference** — transitive relationship discovery via TypeDB
- **Spatial reasoning** — position as semantic data
- **Visual interface** — React canvas for human interaction
- **MCP delivery** — tools accessible to any MCP-capable AI
- **Rich node types** — ideas, decisions, questions as first-class entities

### What Neither Has

- **Cross-graph links** — code entities → idea entities (the `implements`,
  `addresses`, `violates` edges from the vision doc)
- **Semantic search** — finding things by concept, not just keyword
- **Auto-discovery** — system proposes patterns/decisions from observed code
- **Multi-repo** — knowledge that spans repositories
- **Temporal reasoning** — how things evolved over time

---

## The Convergence Question

Robert's intuition: "I think this is actually an MCP server potentially. We need
more horsepower."

### Why In-Repo SQLite Hits a Ceiling

1. **No graph traversal** — SQLite can do recursive CTEs but it's not native.
   "What influenced what influenced what?" is awkward.
2. **No inference** — SQLite can store facts but can't derive new ones.
   Transitive relationships must be materialized manually.
3. **No cross-repo** — `.recon/recon.db` is per-repo. Decisions in repo A can't
   reference decisions in repo B.
4. **No real-time** — CLI is request/response. An MCP server can maintain state,
   push updates, serve multiple clients.
5. **No visual** — CLI output is text. The canvas needs a server.

### Why a Dedicated MCP Server Makes Sense

1. **Persistent process** — maintains database connections, caches, state
2. **Multi-client** — serves Claude Code, web UI, other tools simultaneously
3. **Graph DB access** — connects to ArangoDB/TypeDB/Neo4j without embedding
4. **Cross-repo** — single server indexes multiple repos
5. **Tool-native** — MCP tools are the natural interface for AI agents
6. **Inference** — can run background analysis, discover patterns, suggest links

### The Architecture That Emerges

```
┌─────────────────────────────────────────────────────────┐
│                    MCP Server                            │
│                 (persistent process)                     │
├──────────────────────┬──────────────────────────────────┤
│   CODE INTELLIGENCE  │      KNOWLEDGE GRAPH             │
│                      │                                   │
│   AST indexing       │   Ideas, decisions, questions     │
│   Symbol resolution  │   Typed relationships             │
│   Dependency graph   │   Transitive inference            │
│   Drift detection    │   Spatial reasoning               │
│   Evidence checks    │   Cross-project links             │
│                      │                                   │
│   (recon's engine)   │   (cortex canvas's engine)        │
├──────────────────────┴──────────────────────────────────┤
│                 CROSS-GRAPH LINKS                        │
│   implements | addresses | violates | supersedes         │
├─────────────────────────────────────────────────────────┤
│                    STORAGE                               │
│   SQLite (code index, fast, per-repo)                    │
│   + Graph DB (relationships, inference, global)          │
├─────────────────────────────────────────────────────────┤
│                   CLIENTS                                │
│   Claude Code (MCP)  |  Web Canvas (REST)  |  CLI        │
└─────────────────────────────────────────────────────────┘
```

---

## Open Questions for Discussion

1. **Graph DB choice:** ArangoDB + TypeDB (already built in canvas), Neo4j (most
   mature graph DB), or something lighter?

2. **Where does code indexing live?** Does the MCP server shell out to recon for
   AST parsing, or does the indexing move into the server?

3. **Language:** The MCP server is Python (cortex canvas). Recon's indexing is
   Go. Bridge them or rewrite?

4. **Scope:** Is this a per-machine tool (local MCP server) or could it be
   shared/hosted?

5. **Migration:** How do existing recon databases and cortex canvas data
   converge?

6. **What's the MVP?** What's the smallest thing that proves the cross-graph
   link concept works?
