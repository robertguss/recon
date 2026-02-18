package cli

import (
	"errors"
	"fmt"

	"github.com/robertguss/recon/internal/edge"
	"github.com/robertguss/recon/internal/pattern"
	"github.com/spf13/cobra"
)

func newPatternCommand(app *App) *cobra.Command {
	var (
		reasoning       string
		updateTitle     string
		example         string
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
		affectsRefs     []string
	)

	cmd := &cobra.Command{
		Use:   "pattern [<title>]",
		Short: "Propose a code pattern, verify evidence, and auto-promote when checks pass",
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

				items, err := pattern.NewService(conn).ListPatterns(cmd.Context())
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
					fmt.Println("No active patterns.")
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

				err = pattern.NewService(conn).ArchivePattern(cmd.Context(), deleteID)
				if err != nil {
					if jsonOut {
						code := "internal_error"
						if errors.Is(err, pattern.ErrNotFound) {
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
				fmt.Printf("Pattern %d archived.\n", deleteID)
				return nil
			}

			// Update mode
			if updateID > 0 {
				titleChanged := cmd.Flags().Changed("title")
				reasoningChanged := cmd.Flags().Changed("reasoning")

				if !titleChanged && !reasoningChanged {
					msg := "--update requires at least one of --reasoning or --title"
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

				svc := pattern.NewService(conn)

				if titleChanged || reasoningChanged {
					if err := svc.UpdatePattern(cmd.Context(), updateID, pattern.UpdatePatternInput{
						Title:       updateTitle,
						Description: reasoning,
					}); err != nil {
						if jsonOut {
							code := "internal_error"
							if errors.Is(err, pattern.ErrNotFound) {
								code = "not_found"
							}
							_ = writeJSONError(code, err.Error(), map[string]any{"id": updateID})
							return ExitError{Code: 2}
						}
						return err
					}
				}

				if jsonOut {
					fields := map[string]any{"updated": true, "id": updateID}
					if titleChanged {
						fields["title"] = updateTitle
					}
					if reasoningChanged {
						fields["reasoning"] = reasoning
					}
					return writeJSON(fields)
				}
				fmt.Printf("Pattern %d updated.\n", updateID)
				return nil
			}

			// Propose mode
			if len(args) == 0 {
				msg := "pattern requires a <title> argument"
				if jsonOut {
					_ = writeJSONError("missing_argument", msg, map[string]any{"command": "pattern"})
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

			result, err := pattern.NewService(conn).ProposeAndVerifyPattern(cmd.Context(), pattern.ProposePatternInput{
				Title:           title,
				Description:     reasoning,
				Example:         example,
				Confidence:      confidence,
				EvidenceSummary: evidenceSummary,
				CheckType:       checkType,
				CheckSpec:       resolvedSpec,
				ModuleRoot:      app.ModuleRoot,
			})
			if err != nil {
				if jsonOut {
					_ = writeJSONError("internal_error", err.Error(), nil)
					return ExitError{Code: 2}
				}
				return err
			}

			// Create edges after successful promotion (both JSON and text paths)
			if result.Promoted {
				edgeSvc := edge.NewService(conn)
				// Manual edges from --affects flag
				var edgeErrors []string
				for _, ref := range affectsRefs {
					refType := inferRefType(ref)
					_, err := edgeSvc.Create(cmd.Context(), edge.CreateInput{
						FromType:   "pattern",
						FromID:     result.PatternID,
						ToType:     refType,
						ToRef:      ref,
						Relation:   "affects",
						Source:     "manual",
						Confidence: "high",
					})
					if err != nil {
						if jsonOut {
							edgeErrors = append(edgeErrors, fmt.Sprintf("ref=%s: %v", ref, err))
						} else {
							fmt.Printf("  edge warning: %v\n", err)
						}
					}
				}
				if jsonOut && len(edgeErrors) > 0 {
					details := map[string]any{
						"pattern_id": result.PatternID,
						"errors":     edgeErrors,
					}
					_ = writeJSONError("edge_creation_failed", "one or more edges could not be created", details)
					return ExitError{Code: 2}
				}
				// Auto-link from title + reasoning
				linker := edge.NewAutoLinker(conn)
				detected := linker.Detect(cmd.Context(), "pattern", result.PatternID, title, reasoning)
				for _, d := range detected {
					edgeSvc.Create(cmd.Context(), edge.CreateInput{
						FromType: "pattern", FromID: result.PatternID,
						ToType: d.ToType, ToRef: d.ToRef, Relation: d.Relation,
						Source: "auto", Confidence: "medium",
					})
				}
			}

			if jsonOut {
				if !result.VerificationPassed {
					details := map[string]any{
						"proposal_id": result.ProposalID,
						"check_type":  checkType,
					}
					_ = writeJSONError("verification_failed", result.VerificationDetails, details)
					return ExitError{Code: 2}
				}
				return writeJSON(result)
			}

			if result.Promoted {
				fmt.Printf("Pattern promoted: proposal=%d pattern=%d\n", result.ProposalID, result.PatternID)
			} else {
				fmt.Printf("Pattern pending: proposal=%d\n", result.ProposalID)
			}
			fmt.Printf("Verification: passed=%v details=%s\n", result.VerificationPassed, result.VerificationDetails)
			if !result.VerificationPassed {
				return ExitError{Code: 2}
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&reasoning, "reasoning", "", "Pattern reasoning")
	cmd.Flags().StringVar(&example, "example", "", "Code example demonstrating the pattern")
	cmd.Flags().StringVar(&confidence, "confidence", "medium", "Confidence: low, medium, high")
	cmd.Flags().StringVar(&evidenceSummary, "evidence-summary", "", "Evidence summary")
	cmd.Flags().StringVar(&checkType, "check-type", "", "Verification check type: grep_pattern, symbol_exists, file_exists")
	cmd.Flags().StringVar(&checkSpec, "check-spec", "", "Verification check spec JSON")
	cmd.Flags().StringVar(&checkPath, "check-path", "", "Typed check field for file_exists: path")
	cmd.Flags().StringVar(&checkSymbol, "check-symbol", "", "Typed check field for symbol_exists: symbol name")
	cmd.Flags().StringVar(&checkPattern, "check-pattern", "", "Typed check field for grep_pattern: regex pattern")
	cmd.Flags().StringVar(&checkScope, "check-scope", "", "Typed check field for grep_pattern: optional file glob scope")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output JSON")
	cmd.Flags().BoolVar(&listFlag, "list", false, "List active patterns")
	cmd.Flags().Int64Var(&deleteID, "archive", 0, "Archive (soft-delete) a pattern by ID")
	// --delete kept as a hidden alias for backward compatibility
	cmd.Flags().Int64Var(&deleteID, "delete", 0, "")
	_ = cmd.Flags().MarkHidden("delete")
	cmd.Flags().Int64Var(&updateID, "update", 0, "Update a pattern by ID (use with --reasoning or --title)")
	cmd.Flags().StringVar(&updateTitle, "title", "", "New title (for --update mode)")
	cmd.Flags().StringSliceVar(&affectsRefs, "affects", nil, "Package/file/symbol this pattern affects (creates edges)")

	return cmd
}
