# E2E Dogfood Phase 2: Bugs & Improvements

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to
> implement this plan task-by-task.

**Goal:** Fix all bugs and implement all UX improvements found during E2E
dogfood testing of the recon CLI against three repos: recon (31 files),
cortex_code (173 files), and hugo (502 files, 7882 symbols).

**Architecture:** Bug fixes are isolated to their respective CLI/service files.
UX improvements follow existing patterns (Cobra flags, JSON/text output modes).
The orient truncation change only affects the text renderer. The
`edges --create` subcommand extends the existing edges CLI.

**Tech Stack:** Go, SQLite (modernc.org/sqlite), Cobra CLI, go-sqlmock for error
path testing

---

### Task 1: Fix --affects flag not creating edges in JSON mode (decide + pattern)

**Priority: P0 — This breaks the primary use case (Claude Code hooks use
--json)**

In both `decide.go` and `pattern.go`, when `jsonOut` is true, the code returns
`writeJSON(result)` _before_ reaching the edge creation block. This means
`--affects` silently does nothing in JSON mode — which is the mode Claude Code
hooks always use.

**Files:**

- Modify: `internal/cli/decide.go`
- Modify: `internal/cli/pattern.go`
- Test: `internal/cli/commands_test.go` or new test file

**Step 1: Write failing tests for both commands**

Add tests that verify edges are created when using `--json --affects`:

```go
func TestDecide_AffectsWorksInJSONMode(t *testing.T) {
	// Set up test DB with packages indexed
	// Run: decide "test" --reasoning "r" --evidence-summary "e"
	//   --check-type file_exists --check-path go.mod --affects internal/cli --json
	// Query edges table: should have a row with from_type='decision', to_type='package',
	//   to_ref='internal/cli', relation='affects', source='manual'
}

func TestPattern_AffectsWorksInJSONMode(t *testing.T) {
	// Same pattern for pattern command
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/cli/... -run TestDecide_AffectsWorksInJSONMode -v`

Expected: FAIL — no edges created

**Step 3: Fix decide.go — move edge creation before JSON return**

In `internal/cli/decide.go`, move the edge creation block (lines 249-278) to
execute **before** the JSON output block (lines 236-247). The fix is to
restructure so both JSON and text paths execute edge creation:

```go
// Create edges after successful promotion (both JSON and text paths)
if result.Promoted {
	edgeSvc := edge.NewService(conn)
	// Manual edges from --affects flag
	for _, ref := range affectsRefs {
		refType := inferRefType(ref)
		_, err := edgeSvc.Create(cmd.Context(), edge.CreateInput{
			FromType:   "decision",
			FromID:     result.DecisionID,
			ToType:     refType,
			ToRef:      ref,
			Relation:   "affects",
			Source:     "manual",
			Confidence: "high",
		})
		if err != nil {
			// Log warning but don't fail the command
			_ = err
		}
	}
	// Auto-link from title + reasoning
	linker := edge.NewAutoLinker(conn)
	detected := linker.Detect(cmd.Context(), "decision", result.DecisionID, title, reasoning)
	for _, d := range detected {
		edgeSvc.Create(cmd.Context(), edge.CreateInput{
			FromType: "decision", FromID: result.DecisionID,
			ToType: d.ToType, ToRef: d.ToRef, Relation: d.Relation,
			Source: "auto", Confidence: "medium",
		})
	}
}

// THEN handle output (JSON or text)
if jsonOut {
	if !result.VerificationPassed {
		// ... existing error handling ...
	}
	return writeJSON(result)
}
// ... existing text output ...
```

**Step 4: Apply the same fix to pattern.go**

Same restructuring: move the edge creation block (lines 158-187) before the JSON
output block (lines 146-156).

**Step 5: Run tests to verify they pass**

Run: `go test ./internal/cli/... -v`

Expected: PASS

**Step 6: Run full test suite**

Run: `just test`

Expected: PASS

**Step 7: Commit**

```bash
git add internal/cli/decide.go internal/cli/pattern.go
git commit -m "fix(cli): create edges before JSON output in decide and pattern commands"
```

---

### Task 2: Fix auto-linker false positive on root package "."

**Priority: P1**

The auto-linker scans text for known package paths. The root package path `.` is
a substring of literally everything, causing spurious `affects` edges to
`package:.`. Observed on Hugo where `decision:1 -> package:. (source=auto)` was
created.

**Files:**

- Modify: `internal/edge/autolink.go`
- Modify: `internal/edge/autolink_test.go`

**Step 1: Write failing test**

Add to `internal/edge/autolink_test.go`:

```go
func TestAutoLink_SkipsRootPackagePath(t *testing.T) {
	conn, cleanup := edgeTestDB(t)
	defer cleanup()
	now := "2024-01-01T00:00:00Z"
	// Insert root package "."
	conn.ExecContext(context.Background(),
		`INSERT INTO packages (path, name, import_path, created_at, updated_at)
		 VALUES ('.', 'main', 'example.com/test', ?, ?)`, now, now)
	conn.ExecContext(context.Background(),
		`INSERT INTO packages (path, name, import_path, created_at, updated_at)
		 VALUES ('internal/cli', 'cli', 'example.com/test/internal/cli', ?, ?)`, now, now)

	linker := NewAutoLinker(conn)
	edges := linker.Detect(context.Background(), "decision", 1,
		"Some decision", "This is about internal/cli package")

	for _, e := range edges {
		if e.ToRef == "." {
			t.Fatal("should not auto-link root package path '.'")
		}
	}
	// Should still find internal/cli
	found := false
	for _, e := range edges {
		if e.ToRef == "internal/cli" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected internal/cli in auto-linked edges")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/edge/... -run TestAutoLink_SkipsRootPackagePath -v`

Expected: FAIL — root package "." is matched

**Step 3: Fix loadPackagePaths to skip short paths**

In `internal/edge/autolink.go`, update `loadPackagePaths` to skip package paths
shorter than 3 characters:

```go
func (a *AutoLinker) loadPackagePaths(ctx context.Context) []string {
	rows, err := a.db.QueryContext(ctx,
		`SELECT path FROM packages WHERE length(path) >= 3 ORDER BY length(path) DESC;`)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var paths []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			continue
		}
		paths = append(paths, p)
	}
	return paths
}
```

**Step 4: Run tests**

Run: `go test ./internal/edge/... -v`

Expected: PASS

**Step 5: Commit**

```bash
git add internal/edge/autolink.go internal/edge/autolink_test.go
git commit -m "fix(edge): skip root package path in auto-linker to prevent false positives"
```

---

### Task 3: Investigate and fix symbol count non-determinism across syncs

**Priority: P1**

On Hugo, consecutive syncs with no file changes reported different symbol
counts: first sync = 7882, `status` = 7770, second sync =
`symbols_before: 7770, symbols_after: 7882`. This suggests the indexer is not
deterministically counting or upserting symbols.

**Files:**

- Investigate: `internal/index/service.go` — the Sync method
- Test: `internal/index/service_test.go`

**Step 1: Write a test that reproduces the issue**

```go
func TestSync_DeterministicSymbolCount(t *testing.T) {
	conn := setupTestDB(t)
	svc := NewService(conn)
	tmpDir := createTestModuleWithMultipleFiles(t)

	// First sync
	result1, err := svc.Sync(context.Background(), tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	// Second sync with no changes
	result2, err := svc.Sync(context.Background(), tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	if result1.IndexedSymbols != result2.IndexedSymbols {
		t.Fatalf("symbol count changed between syncs: %d -> %d",
			result1.IndexedSymbols, result2.IndexedSymbols)
	}

	// Also verify status count matches
	var statusCount int
	conn.QueryRowContext(context.Background(),
		"SELECT COUNT(*) FROM symbols").Scan(&statusCount)
	if statusCount != result2.IndexedSymbols {
		t.Fatalf("status count %d != sync count %d", statusCount, result2.IndexedSymbols)
	}
}
```

**Step 2: Run the test**

Run: `go test ./internal/index/... -run TestSync_DeterministicSymbolCount -v`

**Step 3: Investigate the root cause**

Likely candidates:

- The `status` command queries `SELECT COUNT(*) FROM symbols` while `sync`
  reports the count of symbols it processed (which may differ from what's in DB)
- The sync may be deleting and re-inserting, and the "before" count query runs
  at a different point than the "after" count
- Test files or generated files may be included inconsistently

Debug by comparing `result.IndexedSymbols` vs `SELECT COUNT(*) FROM symbols`
after sync.

**Step 4: Fix the root cause**

The fix depends on what's found. Likely: ensure the `IndexedSymbols` count in
`SyncResult` matches the actual DB count after sync completes, not the count of
symbols processed during parsing.

**Step 5: Run full test suite**

Run: `just test`

Expected: PASS

**Step 6: Commit**

```bash
git add internal/index/service.go internal/index/service_test.go
git commit -m "fix(index): ensure deterministic symbol count across syncs"
```

---

### Task 4: Unify --reasoning/--description flags across decide and pattern

**Priority: P2**

`decide` uses `--reasoning` while `pattern` uses `--description` for the same
concept (explaining the "why"). This inconsistency confuses users. Unify to
`--reasoning` on both commands.

**Files:**

- Modify: `internal/cli/pattern.go` — rename `--description` to `--reasoning`

**Step 1: Rename the flag in pattern.go**

Change the variable name and flag registration:

```go
// Change variable name
var reasoning string  // was: description

// Update flag registration
cmd.Flags().StringVar(&reasoning, "reasoning", "", "Pattern reasoning")

// Update usage in ProposePatternInput
Description: reasoning,  // field name stays the same in the struct

// Update auto-linker call
detected := linker.Detect(cmd.Context(), "pattern", result.PatternID, title, reasoning)
```

Keep the `--description` flag as a deprecated hidden alias if you want, but
since Robert said he doesn't care about backwards compatibility, just rename it.

**Step 2: Run tests**

Run: `just test`

Expected: PASS (or fix any tests that reference `--description`)

**Step 3: Commit**

```bash
git add internal/cli/pattern.go
git commit -m "refactor(cli): rename pattern --description to --reasoning for consistency with decide"
```

---

### Task 5: Separate reasoning and evidence_summary in recall output

**Priority: P2**

When recalling a decision, the `reasoning` field contains both the reasoning AND
the evidence_summary concatenated with a newline. They should be separate
fields.

**Files:**

- Investigate: `internal/recall/service.go` — the query that builds Items
- Modify if needed: the SQL query or the Item struct

**Step 1: Investigate the current behavior**

Read the recall service to understand where reasoning and evidence_summary are
being concatenated. Check if it's in the SQL query or in Go code.

**Step 2: Fix the separation**

Ensure `Item.Reasoning` contains only the reasoning text and
`Item.EvidenceSummary` contains only the evidence summary. The recall query
likely joins proposals/decisions and may be using a COALESCE or GROUP_CONCAT
that merges them.

**Step 3: Run tests**

Run: `go test ./internal/recall/... -v`

Expected: PASS

**Step 4: Commit**

```bash
git add internal/recall/service.go
git commit -m "fix(recall): separate reasoning and evidence_summary in recall output"
```

---

### Task 6: Add `edges --create` for direct edge creation

**Priority: P2**

Currently there's no way to create edges directly from the CLI — you must go
through `decide --affects` or `pattern --affects`. This makes bidirectional
relations (`related`, `contradicts`, `supersedes`) unreachable from the CLI.

**Files:**

- Modify: `internal/cli/edges.go` — add create mode with `--from`, `--to`,
  `--relation` flags

**Step 1: Write the test**

```go
func TestEdgesCreate(t *testing.T) {
	// Set up test DB
	// Run: edges --create --from decision:1 --to decision:2 --relation related --json
	// Verify edge was created
	// For bidirectional relation, verify reverse edge also exists
}
```

**Step 2: Add create mode to edges command**

Add new flags to `internal/cli/edges.go`:

```go
var (
	createFlag bool
	relation   string
	source     string
	confidence string
)

cmd.Flags().BoolVar(&createFlag, "create", false, "Create a new edge")
cmd.Flags().StringVar(&relation, "relation", "affects",
	"Edge relation: affects, evidenced_by, supersedes, contradicts, related, reinforces")
cmd.Flags().StringVar(&source, "source", "manual", "Edge source: manual, auto")
cmd.Flags().StringVar(&confidence, "confidence", "high",
	"Edge confidence: low, medium, high")
```

In the RunE, add create mode before the existing from/to/list modes:

```go
if createFlag {
	if fromRef == "" || toRef == "" {
		msg := "edges --create requires --from and --to"
		// ... error handling ...
	}
	fromType, fromID, err := parseEntityRef(fromRef)
	// ... validation ...
	parts := strings.SplitN(toRef, ":", 2)
	// ... create edge via edgeSvc.Create ...
	// ... output result ...
}
```

**Step 3: Run tests**

Run: `just test`

Expected: PASS

**Step 4: Commit**

```bash
git add internal/cli/edges.go
git commit -m "feat(edges): add --create mode for direct edge creation"
```

---

### Task 7: Truncate orient dependency flow for large repos

**Priority: P2**

On Hugo (191 packages), the orient dependency flow is a massive wall of text
that dominates the output and provides diminishing returns. For the Claude Code
session-start use case, only the most important dependency relationships matter.

**Files:**

- Modify: `internal/orient/render.go` — truncate text output
- Modify: `internal/orient/service.go` — add stats for summary

The JSON output should remain complete (Claude can parse it). The text output
should show only inter-module dependencies (dependencies between the modules
listed in the Modules section), plus a count of how many total edges exist.

**Step 1: Update render.go text output**

In the dependency flow text rendering, filter to only show edges where both
`from` and `to` are in the top modules list:

```go
// Collect top module paths
topModules := map[string]bool{}
for _, m := range payload.Modules {
	topModules[m.Path] = true
}

// Filter dependency edges to only inter-module deps
var interModuleDeps []string
for _, edge := range payload.Architecture.DependencyFlow {
	relevantTos := []string{}
	for _, to := range edge.To {
		if topModules[to] {
			relevantTos = append(relevantTos, to)
		}
	}
	if topModules[edge.From] && len(relevantTos) > 0 {
		if len(relevantTos) == 1 {
			interModuleDeps = append(interModuleDeps, edge.From+" → "+relevantTos[0])
		} else {
			interModuleDeps = append(interModuleDeps,
				edge.From+" → {"+strings.Join(relevantTos, ", ")+"}")
		}
	}
}

totalEdges := len(payload.Architecture.DependencyFlow)
fmt.Fprintf(&b, "Dependency flow: %s", strings.Join(interModuleDeps, "; "))
if totalEdges > len(interModuleDeps) {
	fmt.Fprintf(&b, " (+%d more)", totalEdges-len(interModuleDeps))
}
fmt.Fprintln(&b)
```

**Step 2: Run tests**

Run: `go test ./internal/orient/... -v`

Expected: PASS (update render_test.go assertions as needed)

**Step 3: Verify on Hugo**

Run: `cd /Users/robertguss/Projects/experiments/hugo && recon orient`

Expected: Dependency flow shows only inter-module deps (manageable length)

**Step 4: Commit**

```bash
git add internal/orient/render.go internal/orient/render_test.go
git commit -m "feat(orient): truncate dependency flow to inter-module deps in text output"
```

---

### Task 8: Enrich --list-packages with heat and commit count

**Priority: P3**

`find --list-packages --json` returns only `path`, `name`, `file_count`,
`line_count`. Adding `heat` and `recent_commits` would make it match the orient
module view and be more useful for programmatic consumers.

**Files:**

- Modify: `internal/find/service.go` — add heat/commits to package list results
- Modify: `internal/cli/find.go` — pass data through

**Step 1: Update the package list query**

In the find service, the list-packages query should join with git commit data
(same approach as orient). Add `heat` and `recent_commits` to the package list
item struct.

**Step 2: Run tests**

Run: `just test`

Expected: PASS

**Step 3: Commit**

```bash
git add internal/find/service.go internal/cli/find.go
git commit -m "feat(find): add heat and recent commits to --list-packages output"
```

---

### Task 9: Add file path detection to auto-linker

**Priority: P3**

The auto-linker currently detects package paths and symbol names but not file
paths. When reasoning text mentions "defined in exit_error.go" or
"internal/cli/exit_error.go", it should create `file:...` edges.

**Files:**

- Modify: `internal/edge/autolink.go` — add file path detection
- Modify: `internal/edge/autolink_test.go`

**Step 1: Write failing test**

```go
func TestAutoLink_FindsFilePaths(t *testing.T) {
	conn, cleanup := edgeTestDB(t)
	defer cleanup()
	now := "2024-01-01T00:00:00Z"
	conn.ExecContext(context.Background(),
		`INSERT INTO packages (path, name, import_path, created_at, updated_at)
		 VALUES ('internal/cli', 'cli', 'example.com/test/internal/cli', ?, ?)`, now, now)
	var pkgID int64
	conn.QueryRowContext(context.Background(),
		`SELECT id FROM packages WHERE path = 'internal/cli'`).Scan(&pkgID)
	conn.ExecContext(context.Background(),
		`INSERT INTO files (package_id, path, language, lines, hash, created_at, updated_at)
		 VALUES (?, 'internal/cli/exit_error.go', 'go', 20, 'abc', ?, ?)`, pkgID, now, now)

	linker := NewAutoLinker(conn)
	edges := linker.Detect(context.Background(), "decision", 1,
		"ExitError convention",
		"Defined in internal/cli/exit_error.go, used everywhere")

	fileRefs := map[string]bool{}
	for _, e := range edges {
		if e.ToType == "file" {
			fileRefs[e.ToRef] = true
		}
	}
	if !fileRefs["internal/cli/exit_error.go"] {
		t.Fatal("expected internal/cli/exit_error.go in auto-linked edges")
	}
}
```

**Step 2: Implement file path detection**

Add a `loadFilePaths` method and a file matching section in `Detect`:

```go
// Match file paths (paths containing .go)
files := a.loadFilePaths(ctx)
for _, fp := range files {
	if strings.Contains(text, fp) {
		key := "file:" + fp
		if !seen[key] {
			seen[key] = true
			edges = append(edges, DetectedEdge{
				ToType: "file", ToRef: fp, Relation: "affects",
			})
		}
	}
}
```

**Step 3: Run tests**

Run: `go test ./internal/edge/... -v`

Expected: PASS

**Step 4: Commit**

```bash
git add internal/edge/autolink.go internal/edge/autolink_test.go
git commit -m "feat(edge): auto-link file paths mentioned in title and reasoning"
```

---

### Task 10: Fix orient knowledge confidence source

**Priority: P3**

The orient JSON shows the decision/pattern's confidence for knowledge items, but
users might expect the edge's confidence. Clarify by showing both or picking the
right one.

**Files:**

- Modify: `internal/orient/service.go` — update `loadModuleEdges` query

**Step 1: Update the query to include edge confidence**

In `loadModuleEdges`, the current query uses
`COALESCE(d.confidence, p.confidence, 'medium')`. Update `ModuleKnowledge` to
distinguish:

```go
type ModuleKnowledge struct {
	ID             int64  `json:"id"`
	Type           string `json:"type"`
	Title          string `json:"title"`
	Confidence     string `json:"confidence"`      // decision/pattern confidence
	EdgeConfidence string `json:"edge_confidence"`  // edge confidence
}
```

Update the query to also select `e.confidence` and scan it into
`EdgeConfidence`.

**Step 2: Run tests**

Run: `go test ./internal/orient/... -v`

Expected: PASS

**Step 3: Commit**

```bash
git add internal/orient/service.go internal/orient/service_test.go
git commit -m "feat(orient): include edge confidence in module knowledge output"
```

---

### Task 11: Add edges --create support for bidirectional relations

**Priority: P3**

This is a follow-on to Task 6. When creating edges with `contradicts` or
`related` relation via `edges --create`, the bidirectional behavior already
exists in the edge service — but verify it works end-to-end via the CLI and add
a test.

**Files:**

- Test: Add E2E test for bidirectional creation via CLI

**Step 1: Write the test**

```go
func TestEdgesCreate_Bidirectional(t *testing.T) {
	// Create edge: decision:1 -> decision:2 with relation "related"
	// Verify both forward and reverse edges exist
}
```

**Step 2: Run tests and verify**

Run: `just test`

Expected: PASS

**Step 3: Commit**

```bash
git add internal/cli/edges_test.go
git commit -m "test(edges): verify bidirectional edge creation via CLI"
```

---

## Summary of Tasks

| Task | Priority | Description                                        | Depends On |
| ---- | -------- | -------------------------------------------------- | ---------- |
| 1    | P0       | Fix --affects in JSON mode (decide + pattern)      | —          |
| 2    | P1       | Fix auto-linker false positive on root package "." | —          |
| 3    | P1       | Fix symbol count non-determinism across syncs      | —          |
| 4    | P2       | Unify --reasoning/--description flags              | —          |
| 5    | P2       | Separate reasoning and evidence_summary in recall  | —          |
| 6    | P2       | Add edges --create for direct edge creation        | —          |
| 7    | P2       | Truncate orient dependency flow for large repos    | —          |
| 8    | P3       | Enrich --list-packages with heat and commit count  | —          |
| 9    | P3       | Add file path detection to auto-linker             | 2          |
| 10   | P3       | Fix orient knowledge confidence source             | —          |
| 11   | P3       | Test bidirectional edge creation via CLI           | 6          |

**Dependencies:** Tasks 1-8 are independent and can be done in any order. Task 9
depends on Task 2 (both modify autolink.go). Task 11 depends on Task 6 (needs
edges --create to exist).
