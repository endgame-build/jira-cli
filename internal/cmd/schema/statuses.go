package schema

import (
	"context"
	"fmt"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"

	"github.com/endgameio/jira-cli/internal/factory"
	"github.com/endgameio/jira-cli/internal/output"
)

// StatusesOptions holds all resolved inputs for the schema statuses command.
type StatusesOptions struct {
	Factory *factory.Factory

	Project string // --project (forward-compat no-op)
	Type    string // --type (forward-compat no-op)
}

// NewCmdStatuses creates the "schema statuses" command.
func NewCmdStatuses(f *factory.Factory) *cobra.Command {
	opts := &StatusesOptions{
		Factory: f,
	}

	cmd := &cobra.Command{
		Use:   "statuses",
		Short: "List workflow statuses",
		Long:  "List all available workflow statuses. Use 'jira issue transitions <key>' for issue-specific transitions.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSchemaStatuses(opts)
		},
	}

	cmd.Flags().StringVar(&opts.Project, "project", "", "Filter by project (not yet implemented — shows all statuses)")
	cmd.Flags().StringVar(&opts.Type, "type", "", "Filter by issue type (not yet implemented — shows all statuses)")

	return cmd
}

// runSchemaStatuses executes the schema statuses workflow.
func runSchemaStatuses(opts *StatusesOptions) error {
	f := opts.Factory
	ctx := context.Background()

	// Emit warnings for forward-compat no-op flags.
	if opts.Project != "" {
		fmt.Fprintln(f.IOStreams.Err, "Warning: --project filtering is not yet implemented — showing all statuses. Use 'jira issue transitions <key>' for issue-specific transitions.")
	}
	if opts.Type != "" {
		fmt.Fprintln(f.IOStreams.Err, "Warning: --type filtering is not yet implemented — showing all statuses. Use 'jira issue transitions <key>' for issue-specific transitions.")
	}

	client, err := f.APIClient()
	if err != nil {
		return err
	}

	statuses, err := client.ListStatuses(ctx)
	if err != nil {
		return err
	}

	formatter := output.NewFormatter(f.IOStreams, f.OutputJSON, f.JQExpr)

	if f.Quiet {
		return nil
	}

	// JSON mode: unpaginated list envelope (pagination: null).
	if formatter.IsJSON() {
		return formatter.OutputList(statuses, nil, nil)
	}

	// Text mode: table output.
	if len(statuses) == 0 {
		fmt.Fprintln(f.IOStreams.Out, "No statuses found")
		return nil
	}

	return formatter.OutputList(statuses, nil, func(tw table.Writer) {
		tw.AppendHeader(table.Row{"NAME", "CATEGORY", "ID"})

		for _, s := range statuses {
			category := ""
			if s.StatusCategory != nil {
				category = s.StatusCategory.Name
			}

			tw.AppendRow(table.Row{s.Name, category, s.ID})
		}
	})
}
