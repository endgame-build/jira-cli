package issue

import (
	"context"
	"fmt"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"

	"github.com/endgameio/jira-cli/internal/factory"
	"github.com/endgameio/jira-cli/internal/output"
)

// TransitionsOptions holds all resolved inputs for the issue transitions command.
type TransitionsOptions struct {
	Factory *factory.Factory
	KeyOrID string // positional arg: issue key
}

// NewCmdTransitions creates the "issue transitions" command.
func NewCmdTransitions(f *factory.Factory) *cobra.Command {
	opts := &TransitionsOptions{
		Factory: f,
	}

	cmd := &cobra.Command{
		Use:   "transitions <key-or-id>",
		Short: "List available transitions for a Jira issue",
		Long:  "Show the available workflow transitions for a Jira issue, including target status and transition ID.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			key, err := ValidateIssueKeyOrID(args[0])
			if err != nil {
				return err
			}
			opts.KeyOrID = key

			return runTransitions(opts)
		},
	}

	return cmd
}

// runTransitions fetches and displays available transitions for an issue.
func runTransitions(opts *TransitionsOptions) error {
	f := opts.Factory
	ctx := context.Background()

	client, err := f.APIClient()
	if err != nil {
		return err
	}

	transitions, err := client.GetTransitions(ctx, opts.KeyOrID)
	if err != nil {
		return err
	}

	formatter := output.NewFormatter(f.IOStreams, f.OutputJSON, f.JQExpr)

	if formatter.IsJSON() {
		// JSON list envelope with null pagination (transitions aren't paginated).
		return formatter.OutputList(transitions, nil, nil)
	}

	// Text mode: empty results message.
	if len(transitions) == 0 {
		fmt.Fprintf(f.IOStreams.Out, "No transitions available for %s\n", opts.KeyOrID)
		return nil
	}

	// Table output.
	return formatter.OutputList(nil, nil, func(tw table.Writer) {
		tw.AppendHeader(table.Row{"NAME", "TARGET STATUS", "ID"})
		for _, t := range transitions {
			targetStatus := ""
			if t.To != nil {
				targetStatus = t.To.Name
			}
			tw.AppendRow(table.Row{t.Name, targetStatus, t.ID})
		}
	})
}
