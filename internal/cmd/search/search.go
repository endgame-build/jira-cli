// Package search provides the "jira search" command for raw JQL queries.
package search

import (
	"context"
	"fmt"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"

	"github.com/endgameio/jira-cli/internal/api"
	"github.com/endgameio/jira-cli/internal/cmd/shared"
	clierrors "github.com/endgameio/jira-cli/internal/errors"
	"github.com/endgameio/jira-cli/internal/factory"
	"github.com/endgameio/jira-cli/internal/output"
)

// SearchOptions holds all resolved inputs for the search command.
type SearchOptions struct {
	Factory *factory.Factory

	JQL     string   // positional arg: raw JQL query
	Mine    bool     // --mine shortcut
	Status  string   // --status (only with --mine)
	Fields  []string // --fields
	Limit   int      // --limit
	Offset  int      // --offset
	NoPager bool     // --no-pager
}

// defaultSearchAPIFields are the fields requested from Jira when --fields is not set.
var defaultSearchAPIFields = []string{"summary", "status", "assignee", "priority", "issuetype"}

// NewCmdSearch creates the "search" command.
func NewCmdSearch(f *factory.Factory) *cobra.Command {
	opts := &SearchOptions{
		Factory: f,
	}

	cmd := &cobra.Command{
		Use:   "search [<jql>]",
		Short: "Search issues with JQL",
		Long:  "Search for Jira issues using a raw JQL query string, or use --mine for your open issues.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Capture positional JQL arg if provided.
			if len(args) > 0 {
				opts.JQL = args[0]
			}

			// Validate: --status without --mine and without JQL.
			if opts.Status != "" && !opts.Mine && opts.JQL == "" {
				return clierrors.NewValidationError("--status requires --mine or a JQL query").
					WithSuggestion("Use --mine --status 'In Progress' or provide a JQL query")
			}

			// Validate: neither JQL nor --mine.
			if opts.JQL == "" && !opts.Mine {
				return clierrors.NewValidationError("Provide a JQL query or use --mine").
					WithSuggestion("Example: jira search 'project = PROJ' or jira search --mine")
			}

			if opts.NoPager {
				f.IOStreams.NoPager = true
			}

			return runSearch(opts)
		},
	}

	cmd.Flags().BoolVar(&opts.Mine, "mine", false, "Show my open issues (assignee=currentUser() AND resolution=Unresolved)")
	cmd.Flags().StringVar(&opts.Status, "status", "", "Filter by status (requires --mine or JQL)")
	cmd.Flags().StringSliceVar(&opts.Fields, "fields", nil, "Comma-separated list of fields to display")
	cmd.Flags().IntVar(&opts.Limit, "limit", 50, "Maximum number of results")
	cmd.Flags().IntVar(&opts.Offset, "offset", 0, "Number of results to skip")
	cmd.Flags().BoolVar(&opts.NoPager, "no-pager", false, "Do not pipe output through a pager")

	return cmd
}

// runSearch executes the search workflow.
func runSearch(opts *SearchOptions) error {
	f := opts.Factory
	ctx := context.Background()

	client, err := f.APIClient()
	if err != nil {
		return err
	}

	// Build JQL query.
	jql := buildSearchJQL(opts)

	// Determine API fields to request.
	apiFields := defaultSearchAPIFields
	if len(opts.Fields) > 0 {
		apiFields = opts.Fields
	}

	// Fetch via token-based pagination.
	items, meta, err := api.FetchTokenPage(
		ctx,
		opts.Offset,
		opts.Limit,
		func(ctx context.Context, token string, maxResults int) ([]api.Issue, string, bool, error) {
			results, err := client.SearchIssues(ctx, &api.SearchOptions{
				JQL:           jql,
				Fields:        apiFields,
				MaxResults:    maxResults,
				NextPageToken: token,
			})
			if err != nil {
				return nil, "", false, err
			}
			return results.Issues, results.NextPageToken, results.IsLast, nil
		},
		f.IOStreams.Err,
	)
	if err != nil {
		return err
	}

	formatter := output.NewFormatter(f.IOStreams, f.OutputJSON, f.JQExpr)

	if f.Quiet {
		return nil
	}

	// JSON mode: output with pagination envelope.
	if formatter.IsJSON() {
		return formatter.OutputList(items, meta, nil)
	}

	// Text mode: table output.
	if len(items) == 0 {
		fmt.Fprintln(f.IOStreams.Out, "No issues found")
		return nil
	}

	ios := f.IOStreams
	wantFields := shared.FieldSet(opts.Fields)

	ios.StartPager()
	defer ios.StopPager()

	return formatter.OutputList(items, meta, func(tw table.Writer) {
		// Build header row.
		header := table.Row{"KEY"}
		if shared.ShowField(wantFields, "summary") {
			header = append(header, "SUMMARY")
		}
		if shared.ShowField(wantFields, "status") {
			header = append(header, "STATUS")
		}
		if shared.ShowField(wantFields, "assignee") {
			header = append(header, "ASSIGNEE")
		}
		if shared.ShowField(wantFields, "priority") {
			header = append(header, "PRIORITY")
		}
		if shared.ShowField(wantFields, "issuetype") {
			header = append(header, "TYPE")
		}
		tw.AppendHeader(header)

		for _, issue := range items {
			row := table.Row{issue.Key}

			if shared.ShowField(wantFields, "summary") {
				summary := issue.Fields.Summary
				if len(summary) > 60 {
					summary = summary[:57] + "..."
				}
				row = append(row, summary)
			}
			if shared.ShowField(wantFields, "status") {
				row = append(row, shared.StatusWithColor(ios, issue.Fields.Status))
			}
			if shared.ShowField(wantFields, "assignee") {
				assignee := "Unassigned"
				if issue.Fields.Assignee != nil {
					assignee = issue.Fields.Assignee.DisplayName
				}
				row = append(row, assignee)
			}
			if shared.ShowField(wantFields, "priority") {
				priority := ""
				if issue.Fields.Priority != nil {
					priority = issue.Fields.Priority.Name
				}
				row = append(row, priority)
			}
			if shared.ShowField(wantFields, "issuetype") {
				typeName := ""
				if issue.Fields.IssueType != nil {
					typeName = issue.Fields.IssueType.Name
				}
				row = append(row, typeName)
			}

			tw.AppendRow(row)
		}
	})
}

// buildSearchJQL constructs the JQL query for the search command.
// If a positional JQL arg is provided, it overrides --mine and --status.
// If --mine is set, it generates assignee=currentUser() AND resolution=Unresolved,
// optionally appending AND status=... if --status is provided.
func buildSearchJQL(opts *SearchOptions) string {
	// Positional JQL overrides everything.
	if opts.JQL != "" {
		return opts.JQL
	}

	// --mine mode.
	jql := "assignee = currentUser() AND resolution = Unresolved"
	if opts.Status != "" {
		jql += fmt.Sprintf(" AND status = %q", opts.Status)
	}
	return jql
}
