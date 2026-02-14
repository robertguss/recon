# Repository Guidelines

## Project Structure & Module Organization

This repository is currently a strategy and design workspace for the Cortex direction, not an executable codebase. The relevant content is:

- `brainstorms/` — all working notes and planning docs.
- `brainstorms/project-rethink/_index.md` — current snapshot and decisions.
- `brainstorms/project-rethink/project-rethink-v1.md`
- `brainstorms/project-rethink/project-rethink-v2.md`
- `brainstorms/project-rethink/project-rethink-v3.md`

Store new design documents in `brainstorms/<topic>/` with descriptive, lowercase, hyphenated file names (for example: `project-rethink-v4.md`). Add references to major decisions in `_index.md` first.

## Build, Test, and Development Commands

No build/test/runtime pipeline is defined yet. Use these file-level workflows:

- `rg --files brainstorms/project-rethink` to inventory docs.
- `sed -n '1,200p' <file>` for focused review.
- When tooling is added, include exact commands here in the same section before merging.

If you introduce automation, document commands immediately with expected output and scope, for example `make test`, `npm run lint`, or `go test ./...`.

## Coding Style & Naming Conventions

- Keep Markdown concise and scannable: short headings, tight bullet lists, one concept per section.
- Use sentence case for headings unless quoting command names.
- Prefer imperative phrasing for proposed actions and decisions (e.g., “Index, verify, promote”).
- Use fenced code blocks for SQL, shell, and CLI examples.
- Use ASCII unless a file already contains non-ASCII text.

## Testing Guidelines

There are no automated tests in this repo today. Validation is currently manual:

- Confirm links and references are accurate.
- Check spelling and terminology consistency for terms like `orient`, `find`, `decide`, `sync`.
- Review diffs for duplicated claims or conflicting decisions.
- If executable components are added, add matching tests and mention the command here before finalizing the PR.

## Commit & Pull Request Guidelines

This folder is not currently a git repository in this environment, so no local commit history is available to infer conventions. For consistency, use Conventional Commits-style messages, for example `docs: add orient output requirements`.

For PRs, include:

- Purpose and summary.
- Files changed and what decisions they affect.
- Risks and follow-up work.
- Link to the relevant brainstorm section for context.

## Security & Configuration Tips

Do not place secrets or credentials in markdown notes. If secrets or keys become required for implementation, add them to a local `.env` (not tracked) and list required variables explicitly in a dedicated config note.

# ExecPlans

When writing complex features or significant refactors, use an ExecPlan (as described in .agent/PLANS.md) from design to implementation.
