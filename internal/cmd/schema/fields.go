package schema

import (
	"context"
	"fmt"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"

	"github.com/endgameio/jira-cli/internal/factory"
	"github.com/endgameio/jira-cli/internal/output"
)

// FieldsOptions holds all resolved inputs for the schema fields command.
type FieldsOptions struct {
	Factory *factory.Factory

	Project string // --project (forward-compat no-op)
	Type    string // --type (forward-compat no-op)
}

// NewCmdFields creates the "schema fields" command.
func NewCmdFields(f *factory.Factory) *cobra.Command {
	opts := &FieldsOptions{
		Factory: f,
	}

	cmd := &cobra.Command{
		Use:   "fields",
		Short: "List all Jira fields",
		Long:  "List all available Jira field definitions including custom fields.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSchemaFields(opts)
		},
	}

	cmd.Flags().StringVar(&opts.Project, "project", "", "Filter by project (not yet implemented — shows all fields)")
	cmd.Flags().StringVar(&opts.Type, "type", "", "Filter by issue type (not yet implemented — shows all fields)")

	return cmd
}

// runSchemaFields executes the schema fields workflow.
func runSchemaFields(opts *FieldsOptions) error {
	f := opts.Factory
	ctx := context.Background()

	// Emit warnings for forward-compat no-op flags.
	if opts.Project != "" {
		fmt.Fprintln(f.IOStreams.Err, "Warning: --project filtering is not yet implemented — showing all fields")
	}
	if opts.Type != "" {
		fmt.Fprintln(f.IOStreams.Err, "Warning: --type filtering is not yet implemented — showing all fields")
	}

	client, err := f.APIClient()
	if err != nil {
		return err
	}

	fields, err := client.ListFields(ctx)
	if err != nil {
		return err
	}

	formatter := output.NewFormatter(f.IOStreams, f.OutputJSON, f.JQExpr)

	if f.Quiet {
		return nil
	}

	// JSON mode: unpaginated list envelope (pagination: null).
	if formatter.IsJSON() {
		return formatter.OutputList(fields, nil, nil)
	}

	// Text mode: table output.
	if len(fields) == 0 {
		fmt.Fprintln(f.IOStreams.Out, "No fields found")
		return nil
	}

	return formatter.OutputList(fields, nil, func(tw table.Writer) {
		tw.AppendHeader(table.Row{"ID", "NAME", "TYPE", "CUSTOM"})

		for _, field := range fields {
			custom := "no"
			if field.Custom {
				custom = "yes"
			}

			tw.AppendRow(table.Row{field.ID, field.Name, field.Schema.Type, custom})
		}
	})
}
