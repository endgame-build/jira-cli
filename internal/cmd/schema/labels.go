package schema

import (
	"context"
	"fmt"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"

	"github.com/endgameio/jira-cli/internal/api"
	"github.com/endgameio/jira-cli/internal/factory"
	"github.com/endgameio/jira-cli/internal/output"
)

// LabelsOptions holds all resolved inputs for the schema labels command.
type LabelsOptions struct {
	Factory *factory.Factory

	Project string // --project (forward-compat no-op)
	Limit   int    // --limit
	Offset  int    // --offset
}

// NewCmdLabels creates the "schema labels" command.
func NewCmdLabels(f *factory.Factory) *cobra.Command {
	opts := &LabelsOptions{
		Factory: f,
	}

	cmd := &cobra.Command{
		Use:   "labels",
		Short: "List labels in use",
		Long:  "List all labels currently in use across Jira issues with pagination.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSchemaLabels(opts)
		},
	}

	cmd.Flags().StringVar(&opts.Project, "project", "", "Filter by project (not supported by Jira API — shows all labels)")
	cmd.Flags().IntVar(&opts.Limit, "limit", 50, "Maximum number of labels to return")
	cmd.Flags().IntVar(&opts.Offset, "offset", 0, "Number of labels to skip")

	return cmd
}

// runSchemaLabels executes the schema labels workflow.
func runSchemaLabels(opts *LabelsOptions) error {
	f := opts.Factory
	ctx := context.Background()

	// Emit warning for forward-compat no-op flag.
	if opts.Project != "" {
		fmt.Fprintln(f.IOStreams.Err, "Warning: --project filtering is not supported by Jira API — showing all labels")
	}

	client, err := f.APIClient()
	if err != nil {
		return err
	}

	items, meta, err := api.FetchOffsetPage(
		ctx,
		opts.Offset,
		opts.Limit,
		func(ctx context.Context, startAt, maxResults int) (*api.OffsetPageResult[string], error) {
			page, err := client.ListLabels(ctx, api.OffsetPaginationOptions{
				StartAt:    startAt,
				MaxResults: maxResults,
			})
			if err != nil {
				return nil, err
			}
			return &api.OffsetPageResult[string]{
				Items:   page.Values,
				StartAt: page.StartAt,
				Total:   page.Total,
			}, nil
		},
	)
	if err != nil {
		return err
	}

	formatter := output.NewFormatter(f.IOStreams, f.OutputJSON, f.JQExpr)

	if f.Quiet {
		return nil
	}

	// JSON mode: offset-paginated list envelope.
	if formatter.IsJSON() {
		return formatter.OutputList(items, meta, nil)
	}

	// Text mode: table output.
	if len(items) == 0 {
		fmt.Fprintln(f.IOStreams.Out, "No labels found")
		return nil
	}

	return formatter.OutputList(items, meta, func(tw table.Writer) {
		tw.AppendHeader(table.Row{"LABEL"})

		for _, label := range items {
			tw.AppendRow(table.Row{label})
		}
	})
}
