# Workflows

This guide covers end-to-end workflows for using Recon beyond individual
commands. It explains the decision lifecycle, pattern detection, knowledge
recall, and how these pieces fit together in daily development.

## The Knowledge Loop

Recon's core workflow follows a loop:

1. **Orient** — Understand the current state of the project
2. **Find** — Locate specific code elements
3. **Decide** — Record architectural decisions with evidence
4. **Detect** — Identify recurring patterns
5. **Recall** — Search accumulated knowledge before making changes

This loop is designed for both human developers and AI coding agents.

## Decision Lifecycle

Decisions in Recon have a lifecycle with automatic verification and drift
detection.

### Recording a Decision

Always check existing knowledge first:

```bash
recon recall "database choice"
```

If no relevant decisions exist, record a new one:

```bash
recon decide "Use SQLite for local storage" \
  --reasoning "Pure-Go driver, zero config, single-file database" \
  --confidence high \
  --evidence-summary "go.mod lists modernc.org/sqlite" \
  --check-type grep_pattern \
  --check-pattern "modernc.org/sqlite" \
  --check-scope "go.mod"
```

What happens:

1. Recon creates a **proposal** with the decision data
2. The evidence check runs (grep for "modernc.org/sqlite" in go.mod)
3. If the check passes, the decision is **promoted** to active status
4. If the check fails, the proposal stays pending

### Confidence Levels

| Level    | When to Use                                           |
| -------- | ----------------------------------------------------- |
| `low`    | Tentative, exploring options, might change            |
| `medium` | Reasonable belief, some evidence, default             |
| `high`   | Strong conviction, well-evidenced, unlikely to change |

Update confidence as understanding evolves:

```bash
recon decide --update 1 --confidence high
```

### Drift Detection

When you run `recon sync`, Recon re-verifies evidence checks. If a check that
previously passed now fails, the decision's drift status changes from `ok` to
`drifted`.

For example, if you recorded a decision that "all errors use `fmt.Errorf` with
`%w`" with a grep check, and someone introduces an error without wrapping, the
drift status will update.

View drifting decisions:

```bash
recon decide --list
# Shows: #1 Use SQLite for local storage (confidence=high, drift=ok)
```

Confidence decays automatically when drift is detected — a `high` confidence
decision that drifts will step down to `medium`.

### Archiving Decisions

When a decision is no longer relevant (e.g., you switched databases):

```bash
recon decide --delete 1
```

This soft-deletes the decision. It won't appear in `decide --list` or `recall`
results, but remains in the database for historical reference.

### Dry Runs

Test an evidence check without creating any state:

```bash
recon decide --dry-run \
  --check-type symbol_exists \
  --check-symbol NewService
```

This is useful for verifying your check spec before committing to a decision.

## Pattern Detection

Patterns are similar to decisions but represent recurring code structures rather
than one-time choices.

### Recording a Pattern

```bash
recon pattern "Function-var injection for testing" \
  --description "Package-level var funcs replaced in tests for isolation" \
  --example "var osGetwd = os.Getwd" \
  --confidence high \
  --evidence-summary "Multiple packages use var = func pattern" \
  --check-type grep_pattern \
  --check-pattern "var\s+\w+\s+=\s+(os\.|func)" \
  --check-scope "internal/**/*.go"
```

Patterns require `--evidence-summary` and `--check-type` flags.

### When to Use Decisions vs Patterns

| Use a Decision                      | Use a Pattern                       |
| ----------------------------------- | ----------------------------------- |
| One-time architectural choice       | Recurring code structure            |
| "We chose X because Y"              | "We always do X this way"           |
| Framework selection, storage choice | Error handling style, test patterns |
| Changes rarely                      | Applies to many files               |

## Recall Strategies

### Before Making Changes

Always search existing knowledge before introducing new patterns or making
architectural changes:

```bash
recon recall "authentication"
recon recall "error handling"
recon recall "testing"
```

This prevents:

- Duplicate decisions
- Contradictory patterns
- Reinventing existing conventions

### Search Syntax

Recall uses full-text search with Porter stemming. This means:

- `"testing"` matches "test", "tests", "testing", "tested"
- `"error handling"` matches documents containing both terms
- Search covers decision titles, reasoning, evidence summaries, pattern titles,
  and descriptions

### Filtering Results

```bash
recon recall "database" --limit 5    # Top 5 results
recon recall "database" --json       # Machine-readable for agents
```

## Keeping the Index Fresh

### When to Sync

Run `recon sync` when:

- You've added new packages or files
- You've done a significant refactor
- `recon orient` reports stale context
- You want drift detection to re-verify evidence

### Automatic Sync

The `orient` command can handle staleness automatically:

```bash
recon orient --auto-sync    # Syncs when stale, no prompt
recon orient --sync         # Always sync before generating context
```

### Freshness Detection

Recon compares the current git commit hash against the last sync commit. If they
differ, the index is considered stale. The `orient` command shows this:

```
warning: stale context (commit changed since last sync)
```

## Agent Workflows

Recon is designed as a knowledge layer for AI coding agents. Here's the typical
agent workflow:

### Session Start

The Claude Code hook automatically runs `recon orient --json-strict --auto-sync`
at session start, providing the agent with:

- Project structure (entry points, packages, symbols)
- Active decisions and their drift status
- Active patterns
- Recent file activity
- Module heat map

### During Work

Agents use:

1. `recon find <symbol>` — Precise symbol lookup instead of grep
2. `recon recall "<query>"` — Check existing knowledge before making changes
3. `recon decide` — Record significant decisions as they're made
4. `recon pattern` — Record patterns discovered during work

### End of Session

Agents should:

1. Record any significant decisions made during the session
2. Record any new patterns discovered
3. Run `recon sync` if major code changes were made

## Combining Commands

### Investigate then Decide

```bash
# Find how errors are currently handled
recon find --kind func --package ./internal/knowledge/ --limit 20
recon recall "error handling"

# Record what you find
recon decide "Knowledge service wraps all SQL errors" \
  --reasoning "Consistent error context for debugging" \
  --evidence-summary "All service methods use fmt.Errorf with %%w" \
  --check-type grep_pattern \
  --check-pattern "fmt.Errorf" \
  --check-scope "internal/knowledge/*.go"
```

### Orient then Focus

```bash
# Get the big picture
recon orient

# Drill into a hot module
recon find --package ./internal/orient/ --limit 50

# Check relevant decisions
recon recall "orient"
```
