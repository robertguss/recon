package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/robertguss/recon/internal/edge"
	"github.com/robertguss/recon/internal/knowledge"
	"github.com/spf13/cobra"
)

func newDecideCommand(app *App) *cobra.Command {
	var (
		reasoning       string
		confidence      string
		evidenceSummary string
		checkType       string
		checkSpec       string
		checkPath       string
		checkSymbol     string
		checkPattern    string
		checkScope      string
		jsonOut         bool
		listFlag        bool
		deleteID        int64
		updateID        int64
		dryRun          bool
		affectsRefs     []string
	)

	cmd := &cobra.Command{
		Use:   "decide [<title>]",
		Short: "Propose a decision, verify evidence, and auto-promote when checks pass",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// List mode
			if listFlag {
				conn, err := openExistingDB(app)
				if err != nil {
					if jsonOut {
						return exitJSONCommandError(err)
					}
					return err
				}
				defer conn.Close()

				items, err := knowledge.NewService(conn).ListDecisions(cmd.Context())
				if err != nil {
					if jsonOut {
						_ = writeJSONError("internal_error", err.Error(), nil)
						return ExitError{Code: 2}
					}
					return err
				}

				if jsonOut {
					return writeJSON(items)
				}
				if len(items) == 0 {
					fmt.Println("No active decisions.")
					return nil
				}
				for _, item := range items {
					fmt.Printf("#%d %s (confidence=%s, drift=%s)\n", item.ID, item.Title, item.Confidence, item.Drift)
				}
				return nil
			}

			// Delete mode
			if deleteID > 0 {
				conn, err := openExistingDB(app)
				if err != nil {
					if jsonOut {
						return exitJSONCommandError(err)
					}
					return err
				}
				defer conn.Close()

				err = knowledge.NewService(conn).ArchiveDecision(cmd.Context(), deleteID)
				if err != nil {
					if jsonOut {
						code := "internal_error"
						if errors.Is(err, knowledge.ErrNotFound) {
							code = "not_found"
						}
						_ = writeJSONError(code, err.Error(), map[string]any{"id": deleteID})
						return ExitError{Code: 2}
					}
					return err
				}
				if jsonOut {
					return writeJSON(map[string]any{"archived": true, "id": deleteID})
				}
				fmt.Printf("Decision %d archived.\n", deleteID)
				return nil
			}

			// Update mode
			if updateID > 0 {
				if !cmd.Flags().Changed("confidence") {
					msg := "--confidence is required when using --update"
					if jsonOut {
						_ = writeJSONError("missing_argument", msg, map[string]any{"id": updateID})
						return ExitError{Code: 2}
					}
					return ExitError{Code: 2, Message: msg}
				}

				conn, err := openExistingDB(app)
				if err != nil {
					if jsonOut {
						return exitJSONCommandError(err)
					}
					return err
				}
				defer conn.Close()

				err = knowledge.NewService(conn).UpdateConfidence(cmd.Context(), updateID, confidence)
				if err != nil {
					if jsonOut {
						code := "internal_error"
						switch {
						case errors.Is(err, knowledge.ErrNotFound):
							code = "not_found"
						case strings.Contains(err.Error(), "confidence must be"):
							code = "invalid_input"
						}
						_ = writeJSONError(code, err.Error(), map[string]any{"id": updateID})
						return ExitError{Code: 2}
					}
					return err
				}
				if jsonOut {
					return writeJSON(map[string]any{"updated": true, "id": updateID, "confidence": confidence})
				}
				fmt.Printf("Decision %d confidence updated to %s.\n", updateID, confidence)
				return nil
			}

			// Dry-run mode
			if dryRun {
				resolvedSpec, err := buildCheckSpec(checkType, checkSpec, checkPath, checkSymbol, checkPattern, checkScope)
				if err != nil {
					if jsonOut {
						details := map[string]any{"check_type": checkType}
						_ = writeJSONError("invalid_input", err.Error(), details)
						return ExitError{Code: 2}
					}
					return err
				}

				conn, err := openExistingDB(app)
				if err != nil {
					if jsonOut {
						return exitJSONCommandError(err)
					}
					return err
				}
				defer conn.Close()

				outcome := knowledge.NewService(conn).RunCheckPublic(cmd.Context(), checkType, resolvedSpec, app.ModuleRoot)

				type dryRunResult struct {
					Passed  bool   `json:"passed"`
					Details string `json:"details"`
				}
				result := dryRunResult{Passed: outcome.Passed, Details: outcome.Details}

				if jsonOut {
					if !result.Passed {
						_ = writeJSONError("verification_failed", result.Details, map[string]any{"passed": false})
						return ExitError{Code: 2}
					}
					return writeJSON(result)
				}

				if result.Passed {
					fmt.Printf("Dry run: passed — %s\n", result.Details)
					return nil
				}
				fmt.Printf("Dry run: failed — %s\n", result.Details)
				return ExitError{Code: 2}
			}

			// Propose mode (original flow)
			if len(args) == 0 {
				msg := "decide requires a <title> argument"
				if jsonOut {
					_ = writeJSONError("missing_argument", msg, map[string]any{"command": "decide"})
					return ExitError{Code: 2}
				}
				return ExitError{Code: 2, Message: msg}
			}
			title := args[0]

			resolvedSpec, err := buildCheckSpec(checkType, checkSpec, checkPath, checkSymbol, checkPattern, checkScope)
			if err != nil {
				if jsonOut {
					details := map[string]any{"check_type": checkType}
					_ = writeJSONError("invalid_input", err.Error(), details)
					return ExitError{Code: 2}
				}
				return err
			}

			conn, err := openExistingDB(app)
			if err != nil {
				if jsonOut {
					return exitJSONCommandError(err)
				}
				return err
			}
			defer conn.Close()

			result, err := knowledge.NewService(conn).ProposeAndVerifyDecision(cmd.Context(), knowledge.ProposeDecisionInput{
				Title:           title,
				Reasoning:       reasoning,
				Confidence:      confidence,
				EvidenceSummary: evidenceSummary,
				CheckType:       checkType,
				CheckSpec:       resolvedSpec,
				ModuleRoot:      app.ModuleRoot,
			})
			if err != nil {
				if jsonOut {
					code, details := classifyDecideError(checkType, err)
					_ = writeJSONError(code, err.Error(), details)
					return ExitError{Code: 2}
				}
				return err
			}

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
					if err != nil && !jsonOut {
						fmt.Printf("  edge warning: %v\n", err)
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

			if jsonOut {
				if !result.VerificationPassed {
					errorCode := classifyDecideMessage(result.VerificationDetails)
					details := map[string]any{
						"proposal_id": result.ProposalID,
						"check_type":  checkType,
					}
					_ = writeJSONError(errorCode, result.VerificationDetails, details)
					return ExitError{Code: 2}
				}
				return writeJSON(result)
			}

			if result.Promoted {
				fmt.Printf("Decision promoted: proposal=%d decision=%d\n", result.ProposalID, result.DecisionID)
			} else {
				fmt.Printf("Decision pending: proposal=%d\n", result.ProposalID)
			}
			fmt.Printf("Verification: passed=%v details=%s\n", result.VerificationPassed, result.VerificationDetails)
			if !result.VerificationPassed {
				return ExitError{Code: 2}
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&reasoning, "reasoning", "", "Decision reasoning")
	cmd.Flags().StringVar(&confidence, "confidence", "medium", "Confidence: low, medium, high")
	cmd.Flags().StringVar(&evidenceSummary, "evidence-summary", "", "Evidence summary")
	cmd.Flags().StringVar(&checkType, "check-type", "", "Verification check type: grep_pattern, symbol_exists, file_exists")
	cmd.Flags().StringVar(&checkSpec, "check-spec", "", "Verification check spec JSON")
	cmd.Flags().StringVar(&checkPath, "check-path", "", "Typed check field for file_exists: path")
	cmd.Flags().StringVar(&checkSymbol, "check-symbol", "", "Typed check field for symbol_exists: symbol name")
	cmd.Flags().StringVar(&checkPattern, "check-pattern", "", "Typed check field for grep_pattern: regex pattern")
	cmd.Flags().StringVar(&checkScope, "check-scope", "", "Typed check field for grep_pattern: optional file glob scope")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output JSON")
	cmd.Flags().BoolVar(&listFlag, "list", false, "List active decisions")
	cmd.Flags().Int64Var(&deleteID, "delete", 0, "Archive (soft-delete) a decision by ID")
	cmd.Flags().Int64Var(&updateID, "update", 0, "Update a decision by ID (use with --confidence)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Run verification check only, without creating any state")
	cmd.Flags().StringSliceVar(&affectsRefs, "affects", nil, "Package/file/symbol this decision affects (creates edges)")

	return cmd
}

func buildCheckSpec(checkType string, checkSpec string, checkPath string, checkSymbol string, checkPattern string, checkScope string) (string, error) {
	checkType = strings.TrimSpace(checkType)
	checkSpec = strings.TrimSpace(checkSpec)
	checkPath = strings.TrimSpace(checkPath)
	checkSymbol = strings.TrimSpace(checkSymbol)
	checkPattern = strings.TrimSpace(checkPattern)
	checkScope = strings.TrimSpace(checkScope)

	typedProvided := checkPath != "" || checkSymbol != "" || checkPattern != "" || checkScope != ""
	if checkSpec != "" && typedProvided {
		return "", fmt.Errorf("cannot combine --check-spec with typed check flags")
	}
	if checkType != "" && !supportedCheckType(checkType) {
		return "", fmt.Errorf("unsupported check type %q; must be one of: file_exists, symbol_exists, grep_pattern", checkType)
	}
	if checkSpec != "" {
		return checkSpec, nil
	}
	if !typedProvided {
		return "", fmt.Errorf("either --check-spec or typed check flags are required")
	}

	switch checkType {
	case "file_exists":
		if checkPath == "" {
			return "", fmt.Errorf("--check-path is required for check-type file_exists")
		}
		if checkSymbol != "" || checkPattern != "" || checkScope != "" {
			return "", fmt.Errorf("file_exists only supports --check-path")
		}
		return marshalCheckSpec(struct {
			Path string `json:"path"`
		}{Path: checkPath})
	case "symbol_exists":
		if checkSymbol == "" {
			return "", fmt.Errorf("--check-symbol is required for check-type symbol_exists")
		}
		if checkPath != "" || checkPattern != "" || checkScope != "" {
			return "", fmt.Errorf("symbol_exists only supports --check-symbol")
		}
		return marshalCheckSpec(struct {
			Name string `json:"name"`
		}{Name: checkSymbol})
	case "grep_pattern":
		if checkPattern == "" {
			return "", fmt.Errorf("--check-pattern is required for check-type grep_pattern")
		}
		if checkPath != "" || checkSymbol != "" {
			return "", fmt.Errorf("grep_pattern supports --check-pattern and optional --check-scope only")
		}
		return marshalCheckSpec(struct {
			Pattern string `json:"pattern"`
			Scope   string `json:"scope,omitempty"`
		}{Pattern: checkPattern, Scope: checkScope})
	default:
		return "", fmt.Errorf("unsupported check type %q; must be one of: file_exists, symbol_exists, grep_pattern", checkType)
	}
}

func supportedCheckType(checkType string) bool {
	switch checkType {
	case "file_exists", "symbol_exists", "grep_pattern":
		return true
	default:
		return false
	}
}

func marshalCheckSpec(v any) (string, error) {
	spec, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("marshal check spec: %w", err)
	}
	return string(spec), nil
}

func classifyDecideError(checkType string, err error) (string, any) {
	code := classifyDecideMessage(err.Error())
	if code == "invalid_input" {
		return code, map[string]any{"check_type": checkType}
	}
	return "internal_error", nil
}

func inferRefType(ref string) string {
	if strings.Contains(ref, ".go") {
		return "file"
	}
	if strings.Contains(ref, ".") && !strings.Contains(ref, "/") {
		return "symbol"
	}
	return "package"
}

func classifyDecideMessage(msg string) string {
	switch {
	case strings.Contains(msg, "unsupported check type"):
		return "invalid_input"
	case strings.Contains(msg, "check spec"):
		return "invalid_input"
	case strings.Contains(msg, "requires spec."):
		return "invalid_input"
	case strings.Contains(msg, "compile regex pattern"):
		return "invalid_input"
	case strings.Contains(msg, "requires spec"):
		return "invalid_input"
	default:
		return "verification_failed"
	}
}
