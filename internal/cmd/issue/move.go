package issue

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"

	"github.com/endgameio/jira-cli/internal/adf"
	"github.com/endgameio/jira-cli/internal/api"
	clierrors "github.com/endgameio/jira-cli/internal/errors"
	"github.com/endgameio/jira-cli/internal/factory"
	"github.com/endgameio/jira-cli/internal/output"
)

// MoveOptions holds all resolved inputs for the issue move command.
type MoveOptions struct {
	Factory      *factory.Factory
	KeyOrID      string // positional arg: issue key
	TargetStatus string // positional arg: target status name
	Resolution   string // --resolution (e.g., "Fixed", "Won't Fix")
	Comment      string // --comment (Markdown → ADF)
}

// NewCmdMove creates the "issue move" command.
func NewCmdMove(f *factory.Factory) *cobra.Command {
	opts := &MoveOptions{
		Factory: f,
	}

	cmd := &cobra.Command{
		Use:   "move <key-or-id> <status>",
		Short: "Transition a Jira issue to a new status",
		Long:  "Move a Jira issue through its workflow by specifying the target status. Supports case-insensitive exact and substring matching.",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			key, err := ValidateIssueKeyOrID(args[0])
			if err != nil {
				return err
			}
			opts.KeyOrID = key
			opts.TargetStatus = args[1]

			return runMove(opts)
		},
	}

	cmd.Flags().StringVar(&opts.Resolution, "resolution", "", "Resolution to set (e.g., Fixed, Won't Fix)")
	cmd.Flags().StringVar(&opts.Comment, "comment", "", "Comment to add during transition (Markdown)")

	return cmd
}

// runMove fetches the current status, finds a matching transition, and executes it.
func runMove(opts *MoveOptions) error {
	f := opts.Factory
	ctx := context.Background()

	client, err := f.APIClient()
	if err != nil {
		return err
	}

	// Fetch current issue to get "from" status.
	issue, err := client.GetIssue(ctx, opts.KeyOrID, nil)
	if err != nil {
		return err
	}

	fromStatus := ""
	if issue.Fields.Status != nil {
		fromStatus = issue.Fields.Status.Name
	}

	// Fetch available transitions.
	transitions, err := client.GetTransitions(ctx, opts.KeyOrID)
	if err != nil {
		return err
	}

	// Sort by ID for determinism when multiple transitions lead to the same target.
	sort.Slice(transitions, func(i, j int) bool {
		return transitions[i].ID < transitions[j].ID
	})

	// Match transition by target status name.
	matched, err := matchTransition(transitions, opts.TargetStatus)
	if err != nil {
		return err
	}

	toStatus := ""
	if matched.To != nil {
		toStatus = matched.To.Name
	}

	// Build transition input with optional resolution and comment.
	input := &api.DoTransitionInput{
		Transition: api.TransitionRef{ID: matched.ID},
	}

	if opts.Resolution != "" {
		input.Fields = map[string]interface{}{
			"resolution": map[string]string{"name": opts.Resolution},
		}
	}

	if opts.Comment != "" {
		commentADF, err := adf.Convert(opts.Comment)
		if err != nil {
			return clierrors.NewValidationError(
				fmt.Sprintf("Failed to convert comment to ADF: %v", err),
			)
		}
		commentOp := []map[string]interface{}{
			{"add": map[string]interface{}{"body": commentADF}},
		}
		commentJSON, err := json.Marshal(commentOp)
		if err != nil {
			return clierrors.NewValidationError(
				fmt.Sprintf("Failed to marshal comment: %v", err),
			)
		}
		input.Update = map[string]json.RawMessage{
			"comment": commentJSON,
		}
	}

	formatter := output.NewFormatter(f.IOStreams, f.OutputJSON, f.JQExpr)

	// Dry-run: show preview without executing transition.
	if f.DryRun {
		return renderMoveDryRun(formatter, f, opts, matched, fromStatus, toStatus)
	}

	// Execute transition.
	if err := client.DoTransition(ctx, opts.KeyOrID, input); err != nil {
		return err
	}

	if f.Quiet {
		return nil
	}

	if formatter.IsJSON() {
		extras := map[string]interface{}{
			"key":        opts.KeyOrID,
			"action":     "moved",
			"from":       fromStatus,
			"to":         toStatus,
			"transition": matched.Name,
		}
		if opts.Resolution != "" {
			extras["resolution"] = opts.Resolution
		}
		return formatter.OutputMutation(extras, nil)
	}

	return formatter.OutputMutation(nil, func(t table.Writer) {
		fmt.Fprintf(f.IOStreams.Out, "Moved %s to %s\n", opts.KeyOrID, toStatus)
	})
}

// renderMoveDryRun outputs a dry-run preview of the move operation.
func renderMoveDryRun(formatter *output.Formatter, f *factory.Factory, opts *MoveOptions, matched *api.Transition, fromStatus, toStatus string) error {
	payload := map[string]interface{}{
		"key":        opts.KeyOrID,
		"from":       fromStatus,
		"to":         toStatus,
		"transition": matched.Name,
	}
	if opts.Resolution != "" {
		payload["resolution"] = opts.Resolution
	}
	if opts.Comment != "" {
		payload["comment"] = opts.Comment
	}

	return formatter.OutputDryRun(payload, "passed", func(tw table.Writer) {
		tw.AppendHeader(table.Row{"FIELD", "VALUE"})
		tw.AppendRow(table.Row{"Issue", opts.KeyOrID})
		tw.AppendRow(table.Row{"From", fromStatus})
		tw.AppendRow(table.Row{"To", toStatus})
		tw.AppendRow(table.Row{"Transition", matched.Name})
		if opts.Resolution != "" {
			tw.AppendRow(table.Row{"Resolution", opts.Resolution})
		}
		if opts.Comment != "" {
			tw.AppendRow(table.Row{"Comment", opts.Comment})
		}
	})
}

// matchTransition finds a single transition matching the target status.
// Matching order: (1) case-insensitive exact match on transition.to.name,
// (2) case-insensitive substring match (single match only; 2+ = ambiguous).
func matchTransition(transitions []api.Transition, target string) (*api.Transition, error) {
	targetLower := strings.ToLower(target)

	// Phase 1: case-insensitive exact match on to.name.
	for i := range transitions {
		if transitions[i].To != nil && strings.EqualFold(transitions[i].To.Name, target) {
			return &transitions[i], nil
		}
	}

	// Phase 2: case-insensitive substring match.
	var matches []api.Transition
	for _, t := range transitions {
		if t.To != nil && strings.Contains(strings.ToLower(t.To.Name), targetLower) {
			matches = append(matches, t)
		}
	}

	if len(matches) == 1 {
		return &matches[0], nil
	}

	// Build available transitions for error context.
	available := transitionContext(transitions)

	if len(matches) > 1 {
		return nil, clierrors.NewTransitionError(
			fmt.Sprintf("Ambiguous status '%s': matches multiple transitions", target),
			available,
		).WithSuggestion("Use a more specific status name to match a single transition")
	}

	// No match.
	return nil, clierrors.NewTransitionError(
		fmt.Sprintf("No transition found matching '%s'", target),
		available,
	).WithSuggestion("Run 'jira issue transitions <key>' to see available transitions")
}

// transitionContext builds the context slice for INVALID_TRANSITION errors.
func transitionContext(transitions []api.Transition) []map[string]interface{} {
	available := make([]map[string]interface{}, 0, len(transitions))
	for _, t := range transitions {
		entry := map[string]interface{}{
			"id":   t.ID,
			"name": t.Name,
		}
		if t.To != nil {
			entry["toStatus"] = t.To.Name
		}
		available = append(available, entry)
	}
	return available
}

// renderTransitionsTable renders available transitions as a table (reusable by transitions command).
func renderTransitionsTable(w interface {
	Write(p []byte) (n int, err error)
}, transitions []api.Transition) table.Writer {
	tw := table.NewWriter()
	tw.SetOutputMirror(w)
	tw.SetStyle(table.StyleLight)
	tw.Style().Options.DrawBorder = false
	tw.Style().Options.SeparateHeader = true
	tw.Style().Options.SeparateRows = false

	tw.AppendHeader(table.Row{"NAME", "TARGET STATUS", "ID"})
	for _, t := range transitions {
		toName := ""
		if t.To != nil {
			toName = t.To.Name
		}
		tw.AppendRow(table.Row{t.Name, toName, t.ID})
	}
	tw.Render()
	return tw
}
