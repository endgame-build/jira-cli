package comment

import (
	"context"
	"fmt"
	"strings"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"

	"github.com/endgame-build/jira-cli/internal/adf"
	"github.com/endgame-build/jira-cli/internal/api"
	"github.com/endgame-build/jira-cli/internal/cmd/shared"
	"github.com/endgame-build/jira-cli/internal/factory"
	"github.com/endgame-build/jira-cli/internal/output"
)

// CommentListOptions holds all resolved inputs for the comment list command.
type CommentListOptions struct {
	Factory *factory.Factory

	IssueKey string // positional arg (required)
	Limit    int    // --limit
	Offset   int    // --offset
	NoPager  bool   // --no-pager
}

// NewCmdList creates the "comment list" command.
func NewCmdList(f *factory.Factory) *cobra.Command {
	opts := &CommentListOptions{
		Factory: f,
	}

	cmd := &cobra.Command{
		Use:   "list <issue-key>",
		Short: "List comments on a Jira issue",
		Long:  "List comments on a Jira issue with pagination and relative timestamps.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			key, err := shared.ValidateIssueKeyOrID(args[0])
			if err != nil {
				return err
			}
			opts.IssueKey = key

			if opts.NoPager {
				f.IOStreams.NoPager = true
			}
			return runCommentList(opts)
		},
	}

	cmd.Flags().IntVar(&opts.Limit, "limit", 25, "Maximum number of comments to return")
	cmd.Flags().IntVar(&opts.Offset, "offset", 0, "Number of comments to skip")
	cmd.Flags().BoolVar(&opts.NoPager, "no-pager", false, "Do not pipe output through a pager")

	return cmd
}

// runCommentList executes the comment list workflow.
func runCommentList(opts *CommentListOptions) error {
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
		func(ctx context.Context, startAt, maxResults int) (*api.OffsetPageResult[api.Comment], error) {
			page, err := client.ListComments(ctx, opts.IssueKey, api.OffsetPaginationOptions{
				StartAt:    startAt,
				MaxResults: maxResults,
			})
			if err != nil {
				return nil, err
			}
			return &api.OffsetPageResult[api.Comment]{
				Items:   page.Comments,
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

	// JSON mode: output with offset-paginated envelope.
	if formatter.IsJSON() {
		return formatter.OutputList(items, meta, nil)
	}

	// Text mode: table output.
	if len(items) == 0 {
		fmt.Fprintf(f.IOStreams.Out, "No comments on %s\n", opts.IssueKey)
		return nil
	}

	ios := f.IOStreams
	ios.StartPager()
	defer ios.StopPager()

	return formatter.OutputList(items, meta, func(tw table.Writer) {
		tw.AppendHeader(table.Row{"ID", "AUTHOR", "CREATED", "BODY"})

		for _, c := range items {
			author := "Unknown"
			if c.Author != nil {
				author = c.Author.DisplayName
			}

			created := output.RelativeTime(c.Created)

			body := truncateBody(adf.ToPlaintext(c.Body), 3)

			tw.AppendRow(table.Row{c.ID, author, created, body})
		}
	})
}

// truncateBody returns the first n lines of text. If there are more lines,
// appends "..." to indicate truncation.
func truncateBody(text string, maxLines int) string {
	lines := strings.SplitN(text, "\n", maxLines+1)
	if len(lines) > maxLines {
		lines = lines[:maxLines]
		return strings.Join(lines, "\n") + "..."
	}
	return strings.Join(lines, "\n")
}
