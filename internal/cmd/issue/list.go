package issue

import (
	"context"
	"fmt"
	"strings"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"

	"github.com/endgameio/jira-cli/internal/api"
	"github.com/endgameio/jira-cli/internal/cmd/shared"
	clierrors "github.com/endgameio/jira-cli/internal/errors"
	"github.com/endgameio/jira-cli/internal/factory"
	"github.com/endgameio/jira-cli/internal/output"
)

// ListOptions holds all resolved inputs for the issue list command.
type ListOptions struct {
	Factory *factory.Factory

	Project  string   // --project (falls back to default.project)
	Assignee string   // --assignee (@me supported)
	Status   string   // --status
	Type     string   // --type
	Label    string   // --label
	JQL      string   // --jql (overrides all filter flags)
	Sort     string   // --sort (field name for ORDER BY)
	Order    string   // --order (asc/desc, default: desc)
	Fields   []string // --fields (controls returned/displayed fields)
	Limit    int      // --limit
	Offset   int      // --offset
	NoPager  bool     // --no-pager
}

// defaultDisplayFields are the fields shown in text table output when --fields is not set.
var defaultDisplayFields = []string{"summary", "status", "assignee", "priority", "issuetype"}

// defaultAPIFields are the fields requested from Jira when --fields is not set.
// Includes "key" implicitly (always returned by Jira), plus display fields.
var defaultAPIFields = []string{"summary", "status", "assignee", "priority", "issuetype"}

// NewCmdList creates the "issue list" command.
func NewCmdList(f *factory.Factory) *cobra.Command {
	opts := &ListOptions{
		Factory: f,
	}

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List Jira issues",
		Long:  "List Jira issues with optional filters. Without flags, defaults to your open issues.",
		RunE: func(cmd *cobra.Command, args []string) error {
			// --order without --sort is an error.
			if cmd.Flags().Changed("order") && opts.Sort == "" {
				return clierrors.NewValidationError("--order requires --sort").
					WithSuggestion("Specify a field to sort by with --sort")
			}
			// Validate --order value.
			if opts.Sort != "" {
				order := strings.ToLower(opts.Order)
				if order != "asc" && order != "desc" {
					return clierrors.NewValidationError("--order must be 'asc' or 'desc'").
						WithSuggestion("Use --order asc or --order desc")
				}
			}
			if opts.NoPager {
				f.IOStreams.NoPager = true
			}
			return runList(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Project, "project", "p", "", "Filter by project key (falls back to default.project config)")
	cmd.Flags().StringVarP(&opts.Assignee, "assignee", "a", "", "Filter by assignee (display name, @me, or account ID)")
	cmd.Flags().StringVar(&opts.Status, "status", "", "Filter by status name")
	cmd.Flags().StringVarP(&opts.Type, "type", "t", "", "Filter by issue type")
	cmd.Flags().StringVarP(&opts.Label, "label", "l", "", "Filter by label")
	cmd.Flags().StringVar(&opts.JQL, "jql", "", "Raw JQL query (overrides all filter flags)")
	cmd.Flags().StringVarP(&opts.Sort, "sort", "s", "", "Sort results by field name (appends ORDER BY to JQL)")
	cmd.Flags().StringVar(&opts.Order, "order", "desc", "Sort order: asc or desc (requires --sort)")
	cmd.Flags().StringSliceVar(&opts.Fields, "fields", nil, "Comma-separated list of fields to display")
	cmd.Flags().IntVar(&opts.Limit, "limit", 50, "Maximum number of results")
	cmd.Flags().IntVar(&opts.Offset, "offset", 0, "Number of results to skip")
	cmd.Flags().BoolVar(&opts.NoPager, "no-pager", false, "Do not pipe output through a pager")

	return cmd
}

// runList executes the issue list workflow.
func runList(opts *ListOptions) error {
	f := opts.Factory
	ctx := context.Background()

	client, err := f.APIClient()
	if err != nil {
		return err
	}

	// Build JQL query.
	jql, err := buildJQL(ctx, client, opts)
	if err != nil {
		return err
	}

	// Determine API fields to request.
	apiFields := defaultAPIFields
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

	// Jira returns HTTP 200 with empty results for unauthenticated search
	// requests (instead of 401). When the first page is empty, verify
	// credentials so we can surface auth errors instead of silently showing
	// "No issues found".
	if len(items) == 0 {
		if err := shared.CheckEmptyResultsAuth(ctx, client, f.IOStreams.Err); err != nil {
			return err
		}
	}

	formatter := output.NewFormatter(f.IOStreams, f.OutputJSON, f.JQExpr)

	if f.Quiet {
		return nil
	}

	// JSON mode: output with pagination envelope.
	// When --fields is specified, filter the JSON to only include requested fields.
	if formatter.IsJSON() {
		wantFields := shared.FieldSet(opts.Fields)
		if wantFields != nil {
			return formatter.OutputList(shared.FilterIssueFields(items, wantFields), meta, nil)
		}
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
		if shared.ShowField(wantFields, "issuetype") || shared.ShowField(wantFields, "type") {
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
			if shared.ShowField(wantFields, "issuetype") || shared.ShowField(wantFields, "type") {
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

// buildJQL constructs a JQL query from filter flags or returns the raw --jql value.
// If --jql is set, it overrides all filter flags.
// If no filters and no --jql, defaults to: assignee = currentUser() AND resolution = Unresolved
func buildJQL(ctx context.Context, client *api.Client, opts *ListOptions) (string, error) {
	// --jql overrides everything.
	if opts.JQL != "" {
		return opts.JQL, nil
	}

	var clauses []string

	// --project (optional for list, falls back to default.project but not required).
	project := opts.Project
	if project == "" {
		project = configGet(opts.Factory, "default.project")
	}
	if project != "" {
		clauses = append(clauses, fmt.Sprintf("project = %q", project))
	}

	// --assignee with @me support.
	if opts.Assignee != "" {
		if opts.Assignee == "@me" {
			clauses = append(clauses, "assignee = currentUser()")
		} else {
			accountID, err := api.ResolveUser(ctx, client, opts.Assignee)
			if err != nil {
				return "", err
			}
			clauses = append(clauses, fmt.Sprintf("assignee = %q", accountID))
		}
	}

	// --status
	if opts.Status != "" {
		clauses = append(clauses, fmt.Sprintf("status = %q", opts.Status))
	}

	// --type
	if opts.Type != "" {
		clauses = append(clauses, fmt.Sprintf("issuetype = %q", opts.Type))
	}

	// --label
	if opts.Label != "" {
		clauses = append(clauses, fmt.Sprintf("labels = %q", opts.Label))
	}

	// No filters at all: default to "my open issues".
	if len(clauses) == 0 {
		return appendOrderBy("assignee = currentUser() AND resolution = Unresolved", opts.Sort, opts.Order), nil
	}

	return appendOrderBy(strings.Join(clauses, " AND "), opts.Sort, opts.Order), nil
}

// appendOrderBy appends ORDER BY to a JQL string if sort is non-empty.
func appendOrderBy(jql, sort, order string) string {
	if sort == "" {
		return jql
	}
	return fmt.Sprintf("%s ORDER BY %s %s", jql, sort, strings.ToUpper(order))
}
