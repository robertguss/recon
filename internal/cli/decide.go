package cli

import (
	"fmt"

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
		jsonOut         bool
	)

	cmd := &cobra.Command{
		Use:   "decide <title>",
		Short: "Propose a decision, verify evidence, and auto-promote when checks pass",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			title := args[0]

			conn, err := openExistingDB(app)
			if err != nil {
				return err
			}
			defer conn.Close()

			result, err := knowledge.NewService(conn).ProposeAndVerifyDecision(cmd.Context(), knowledge.ProposeDecisionInput{
				Title:           title,
				Reasoning:       reasoning,
				Confidence:      confidence,
				EvidenceSummary: evidenceSummary,
				CheckType:       checkType,
				CheckSpec:       checkSpec,
				ModuleRoot:      app.ModuleRoot,
			})
			if err != nil {
				return err
			}

			if jsonOut {
				return writeJSON(result)
			}

			if result.Promoted {
				fmt.Printf("Decision promoted: proposal=%d decision=%d\n", result.ProposalID, result.DecisionID)
			} else {
				fmt.Printf("Decision pending: proposal=%d\n", result.ProposalID)
			}
			fmt.Printf("Verification: passed=%v details=%s\n", result.VerificationPassed, result.VerificationDetails)
			return nil
		},
	}

	cmd.Flags().StringVar(&reasoning, "reasoning", "", "Decision reasoning")
	cmd.Flags().StringVar(&confidence, "confidence", "medium", "Confidence: low, medium, high")
	cmd.Flags().StringVar(&evidenceSummary, "evidence-summary", "", "Evidence summary")
	cmd.Flags().StringVar(&checkType, "check-type", "", "Verification check type: grep_pattern, symbol_exists, file_exists")
	cmd.Flags().StringVar(&checkSpec, "check-spec", "", "Verification check spec JSON")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output JSON")

	_ = cmd.MarkFlagRequired("reasoning")
	_ = cmd.MarkFlagRequired("evidence-summary")
	_ = cmd.MarkFlagRequired("check-type")
	_ = cmd.MarkFlagRequired("check-spec")

	return cmd
}
