package cli

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/robertguss/recon/internal/edge"
	"github.com/spf13/cobra"
)

func newEdgesCommand(app *App) *cobra.Command {
	var (
		jsonOut    bool
		fromRef    string
		toRef      string
		deleteID   int64
		listAll    bool
		createFlag bool
		relation   string
		source     string
		confidence string
	)

	cmd := &cobra.Command{
		Use:   "edges",
		Short: "Manage knowledge graph edges",
		RunE: func(cmd *cobra.Command, args []string) error {
			conn, err := openExistingDB(app)
			if err != nil {
				if jsonOut {
					return exitJSONCommandError(err)
				}
				return err
			}
			defer conn.Close()

			svc := edge.NewService(conn)

			// Create mode
			if createFlag {
				if fromRef == "" || toRef == "" {
					msg := "edges --create requires --from and --to"
					if jsonOut {
						_ = writeJSONError("missing_argument", msg, nil)
						return ExitError{Code: 2}
					}
					return ExitError{Code: 2, Message: msg}
				}
				fromType, fromID, err := parseEntityRef(fromRef)
				if err != nil {
					if jsonOut {
						_ = writeJSONError("invalid_input", err.Error(), nil)
						return ExitError{Code: 2}
					}
					return err
				}
				parts := strings.SplitN(toRef, ":", 2)
				if len(parts) != 2 {
					msg := "invalid --to format; use type:ref (e.g., decision:2, package:internal/cli)"
					if jsonOut {
						_ = writeJSONError("invalid_input", msg, nil)
						return ExitError{Code: 2}
					}
					return ExitError{Code: 2, Message: msg}
				}
				created, err := svc.Create(cmd.Context(), edge.CreateInput{
					FromType:   fromType,
					FromID:     fromID,
					ToType:     parts[0],
					ToRef:      parts[1],
					Relation:   relation,
					Source:     source,
					Confidence: confidence,
				})
				if err != nil {
					if jsonOut {
						_ = writeJSONError("internal_error", err.Error(), nil)
						return ExitError{Code: 2}
					}
					return err
				}
				if jsonOut {
					return writeJSON(created)
				}
				fmt.Printf("Edge #%d created: %s:%d -[%s]-> %s:%s\n",
					created.ID, created.FromType, created.FromID, created.Relation, created.ToType, created.ToRef)
				return nil
			}

			// Delete mode
			if deleteID > 0 {
				err := svc.Delete(cmd.Context(), deleteID)
				if err != nil {
					if jsonOut {
						code := "internal_error"
						if errors.Is(err, edge.ErrNotFound) {
							code = "not_found"
						}
						_ = writeJSONError(code, err.Error(), map[string]any{"id": deleteID})
						return ExitError{Code: 2}
					}
					return err
				}
				if jsonOut {
					return writeJSON(map[string]any{"deleted": true, "id": deleteID})
				}
				fmt.Printf("Edge %d deleted.\n", deleteID)
				return nil
			}

			// From mode: edges --from decision:2
			if fromRef != "" {
				fromType, fromID, err := parseEntityRef(fromRef)
				if err != nil {
					if jsonOut {
						_ = writeJSONError("invalid_input", err.Error(), nil)
						return ExitError{Code: 2}
					}
					return err
				}
				edges, err := svc.ListFromWithTitles(cmd.Context(), fromType, fromID)
				if err != nil {
					if jsonOut {
						_ = writeJSONError("internal_error", err.Error(), nil)
						return ExitError{Code: 2}
					}
					return err
				}
				return renderEdges(edges, jsonOut)
			}

			// To mode: edges --to package:internal/cli
			if toRef != "" {
				parts := strings.SplitN(toRef, ":", 2)
				if len(parts) != 2 {
					msg := "invalid --to format; use type:ref (e.g., package:internal/cli)"
					if jsonOut {
						_ = writeJSONError("invalid_input", msg, nil)
						return ExitError{Code: 2}
					}
					return ExitError{Code: 2, Message: msg}
				}
				edges, err := svc.ListToWithTitles(cmd.Context(), parts[0], parts[1])
				if err != nil {
					if jsonOut {
						_ = writeJSONError("internal_error", err.Error(), nil)
						return ExitError{Code: 2}
					}
					return err
				}
				return renderEdges(edges, jsonOut)
			}

			// List all mode
			if listAll {
				edges, err := svc.ListAllWithTitles(cmd.Context())
				if err != nil {
					if jsonOut {
						_ = writeJSONError("internal_error", err.Error(), nil)
						return ExitError{Code: 2}
					}
					return err
				}
				return renderEdges(edges, jsonOut)
			}

			msg := "edges requires --create, --from, --to, --delete, or --list"
			if jsonOut {
				_ = writeJSONError("missing_argument", msg, nil)
				return ExitError{Code: 2}
			}
			return ExitError{Code: 2, Message: msg}
		},
	}

	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output JSON")
	cmd.Flags().BoolVar(&createFlag, "create", false, "Create a new edge")
	cmd.Flags().StringVar(&fromRef, "from", "", "Entity ref (e.g., decision:2)")
	cmd.Flags().StringVar(&toRef, "to", "", "Entity ref (e.g., package:internal/cli, decision:3)")
	cmd.Flags().StringVar(&relation, "relation", "affects", "Edge relation: affects, evidenced_by, supersedes, contradicts, related, reinforces")
	cmd.Flags().StringVar(&source, "source", "manual", "Edge source: manual, auto")
	cmd.Flags().StringVar(&confidence, "confidence", "high", "Edge confidence: low, medium, high")
	cmd.Flags().Int64Var(&deleteID, "delete", 0, "Delete an edge by ID")
	cmd.Flags().BoolVar(&listAll, "list", false, "List all edges")

	return cmd
}

func parseEntityRef(ref string) (string, int64, error) {
	parts := strings.SplitN(ref, ":", 2)
	if len(parts) != 2 {
		return "", 0, fmt.Errorf("invalid entity ref %q; use type:id (e.g., decision:2)", ref)
	}
	id, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return "", 0, fmt.Errorf("invalid entity ID %q; must be an integer", parts[1])
	}
	return parts[0], id, nil
}

func renderEdges(edges []edge.EdgeWithTitle, jsonOut bool) error {
	if jsonOut {
		return writeJSON(edges)
	}
	if len(edges) == 0 {
		fmt.Println("No edges found.")
		return nil
	}
	for _, e := range edges {
		title := ""
		if e.FromTitle != "" {
			title = fmt.Sprintf(" %q", e.FromTitle)
		}
		fmt.Printf("#%d %s:%d%s -[%s]-> %s:%s (source=%s, confidence=%s)\n",
			e.ID, e.FromType, e.FromID, title, e.Relation,
			e.ToType, e.ToRef, e.Source, e.Confidence)
	}
	return nil
}
