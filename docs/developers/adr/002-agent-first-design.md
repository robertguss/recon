# ADR-002: Agent-First Design

## Status

Accepted (2026-02-13)

## Context

Recon could be designed for human developers, AI coding agents, or both. The
primary consumers needed to be identified to guide CLI design, output formats,
and integration patterns.

## Decision

Design for **AI coding agents as the primary consumer**, with human developers
as a secondary audience.

## Rationale

AI coding agents (like Claude Code) are the primary users of code intelligence
tools:

- Agents need structured context at session start
- Agents benefit from machine-readable output (`--json`)
- Agents need non-interactive mode (`--no-prompt`)
- Agents can record decisions and patterns as they work, building a knowledge
  base over time

Human developers benefit from the same tool but interact differently:

- Text output is the default for readability
- Interactive prompts help with exploration
- The same knowledge base is useful for onboarding and reference

### Design implications

1. **Every command supports `--json`** — Agents always use JSON mode
2. **`--no-prompt` flag** — Disables all interactive prompts globally
3. **`--json-strict`** — Suppresses warnings that could break JSON parsing
4. **SessionStart hook** — Automatically injects project context at session
   start
5. **Skill definition** — Teaches the agent how to use Recon effectively
6. **Evidence verification** — Agents can record decisions with automated checks
   instead of manual verification

### Agent proposes, human approves

The knowledge growth model is:

- Agents discover patterns and make decisions during coding sessions
- Evidence checks provide automated verification
- Humans review accumulated knowledge via `recall` and `decide --list`
- This creates a virtuous cycle of growing project knowledge

## Consequences

- CLI output must work for both humans (text) and agents (JSON)
- Error messages must be structured (JSON error envelopes with codes)
- The orient payload must be comprehensive enough to bootstrap an agent
- Integration files (hook, skill, settings) are installed alongside the database
- Non-interactive mode must be fully functional (no prompts, no TTY required)
