package project

import (
	"context"
	"fmt"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"

	"github.com/endgameio/jira-cli/internal/api"
	"github.com/endgameio/jira-cli/internal/cmd/shared"
	"github.com/endgameio/jira-cli/internal/factory"
	"github.com/endgameio/jira-cli/internal/output"
)

// ProjectListOptions holds all resolved inputs for the project list command.
type ProjectListOptions struct {
	Factory *factory.Factory

	Limit   int  // --limit
	Offset  int  // --offset
	NoPager bool // --no-pager
}

// NewCmdList creates the "project list" command.
func NewCmdList(f *factory.Factory) *cobra.Command {
	opts := &ProjectListOptions{
		Factory: f,
	}

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List Jira projects",
		Long:  "List Jira projects with pagination.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.NoPager {
				f.IOStreams.NoPager = true
			}
			return runProjectList(opts)
		},
	}

	cmd.Flags().IntVar(&opts.Limit, "limit", 50, "Maximum number of projects to return")
	cmd.Flags().IntVar(&opts.Offset, "offset", 0, "Number of projects to skip")
	cmd.Flags().BoolVar(&opts.NoPager, "no-pager", false, "Do not pipe output through a pager")

	return cmd
}

// runProjectList executes the project list workflow.
func runProjectList(opts *ProjectListOptions) error {
	f := opts.Factory
	ctx := context.Background()

	client, err := f.APIClient()
	if err != nil {
		return err
	}

	items, meta, err := api.FetchOffsetPage(
		ctx,
		opts.Offset,
		opts.Limit,
		func(ctx context.Context, startAt, maxResults int) (*api.OffsetPageResult[api.ProjectDetail], error) {
			page, err := client.ListProjects(ctx, api.OffsetPaginationOptions{
				StartAt:    startAt,
				MaxResults: maxResults,
			})
			if err != nil {
				return nil, err
			}
			return &api.OffsetPageResult[api.ProjectDetail]{
				Items:   page.Values,
				StartAt: page.StartAt,
				Total:   page.Total,
			}, nil
		},
	)
	if err != nil {
		return err
	}

	// Jira returns HTTP 200 with empty results for unauthenticated requests.
	if len(items) == 0 {
		if err := shared.CheckEmptyResultsAuth(ctx, client, f.IOStreams.Err); err != nil {
			return err
		}
	}

	formatter := output.NewFormatter(f.IOStreams, f.OutputJSON, f.JQExpr)

	if f.Quiet {
		return nil
	}

	// JSON mode: output with offset-paginated envelope.
	if formatter.IsJSON() {
		return formatter.OutputList(items, meta, nil)
	}

	// Text mode: table output.
	if len(items) == 0 {
		fmt.Fprintln(f.IOStreams.Out, "No projects found")
		return nil
	}

	ios := f.IOStreams
	ios.StartPager()
	defer ios.StopPager()

	return formatter.OutputList(items, meta, func(tw table.Writer) {
		tw.AppendHeader(table.Row{"KEY", "NAME", "LEAD", "TYPE"})

		for _, p := range items {
			lead := ""
			if p.Lead != nil {
				lead = p.Lead.DisplayName
			}
			tw.AppendRow(table.Row{p.Key, p.Name, lead, p.ProjectTypeKey})
		}
	})
}
