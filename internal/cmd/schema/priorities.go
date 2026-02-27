package schema

import (
	"context"
	"fmt"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"

	"github.com/endgame-build/jira-cli/internal/factory"
	"github.com/endgame-build/jira-cli/internal/output"
)

// PrioritiesOptions holds all resolved inputs for the schema priorities command.
type PrioritiesOptions struct {
	Factory *factory.Factory
}

// NewCmdPriorities creates the "schema priorities" command.
func NewCmdPriorities(f *factory.Factory) *cobra.Command {
	opts := &PrioritiesOptions{
		Factory: f,
	}

	cmd := &cobra.Command{
		Use:   "priorities",
		Short: "List issue priorities",
		Long:  "List all available issue priority levels.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSchemaPriorities(opts)
		},
	}

	return cmd
}

// runSchemaPriorities executes the schema priorities workflow.
func runSchemaPriorities(opts *PrioritiesOptions) error {
	f := opts.Factory
	ctx := context.Background()

	client, err := f.APIClient()
	if err != nil {
		return err
	}

	priorities, err := client.ListPriorities(ctx)
	if err != nil {
		return err
	}

	formatter := output.NewFormatter(f.IOStreams, f.OutputJSON, f.JQExpr)

	if f.Quiet {
		return nil
	}

	// JSON mode: unpaginated list envelope (pagination: null).
	if formatter.IsJSON() {
		return formatter.OutputList(priorities, nil, nil)
	}

	// Text mode: table output.
	if len(priorities) == 0 {
		fmt.Fprintln(f.IOStreams.Out, "No priorities found")
		return nil
	}

	return formatter.OutputList(priorities, nil, func(tw table.Writer) {
		tw.AppendHeader(table.Row{"NAME", "ID", "DESCRIPTION", "ICON URL"})

		for _, p := range priorities {
			desc := p.Description
			if len(desc) > 60 {
				desc = desc[:57] + "..."
			}

			tw.AppendRow(table.Row{p.Name, p.ID, desc, p.IconURL})
		}
	})
}
