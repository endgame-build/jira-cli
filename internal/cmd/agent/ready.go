package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"

	"github.com/endgame-build/jira-cli/internal/api"
	"github.com/endgame-build/jira-cli/internal/cmd/shared"
	clierrors "github.com/endgame-build/jira-cli/internal/errors"
	"github.com/endgame-build/jira-cli/internal/factory"
	"github.com/endgame-build/jira-cli/internal/output"
)

// ReadyOptions holds all resolved inputs for the agent ready command.
type ReadyOptions struct {
	Factory    *factory.Factory
	Project    string
	Limit      int
	Assignee   string
	Unassigned bool
	Type       string
	Labels     []string
	Priority   string
	Component  string
	Sort       string
	Sprint     string
}

// NewCmdReady creates the "agent ready" command.
func NewCmdReady(f *factory.Factory) *cobra.Command {
	opts := &ReadyOptions{
		Factory: f,
	}

	cmd := &cobra.Command{
		Use:   "ready",
		Short: "Show issues ready for work (no unresolved blockers)",
		Long:  "Search for issues that are actionable now — not done, not in progress, and not blocked by unresolved issues.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.Assignee != "" && opts.Unassigned {
				return clierrors.NewValidationError("Cannot use --assignee and --unassigned together").
					WithSuggestion("Use either --assignee or --unassigned, not both")
			}
			return runReady(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Project, "project", "p", "", "Jira project key (falls back to default.project config)")
	cmd.Flags().IntVar(&opts.Limit, "limit", 10, "Maximum issues to return")
	cmd.Flags().StringVarP(&opts.Assignee, "assignee", "a", "", "Filter by assignee (@me supported)")
	cmd.Flags().BoolVar(&opts.Unassigned, "unassigned", false, "Only show unassigned issues")
	cmd.Flags().StringVarP(&opts.Type, "type", "t", "", "Filter by issue type")
	cmd.Flags().StringSliceVarP(&opts.Labels, "label", "l", nil, "Filter by labels (AND)")
	cmd.Flags().StringVar(&opts.Priority, "priority", "", "Filter by priority name")
	cmd.Flags().StringVar(&opts.Component, "component", "", "Filter by component")
	cmd.Flags().StringVar(&opts.Sort, "sort", "priority", "Sort: priority, created, updated")
	cmd.Flags().StringVar(&opts.Sprint, "sprint", "", "Filter by sprint: active, future, or sprint name")

	return cmd
}

// runReady executes the agent ready workflow.
func runReady(opts *ReadyOptions) error {
	f := opts.Factory
	ctx := context.Background()

	project, err := shared.ResolveProject(f, opts.Project)
	if err != nil {
		return err
	}

	client, err := f.APIClient()
	if err != nil {
		return err
	}

	jql, err := buildReadyJQL(ctx, client, project, opts)
	if err != nil {
		return err
	}

	// Fetch more than limit to account for blocker filtering.
	fetchLimit := opts.Limit * 3
	if fetchLimit < 50 {
		fetchLimit = 50
	}

	results, err := client.SearchIssues(ctx, &api.SearchOptions{
		JQL:        jql,
		Fields:     AgentReadyFields(),
		MaxResults: fetchLimit,
	})
	if err != nil {
		return err
	}

	// Post-filter: exclude blocked issues.
	var ready []api.Issue
	for i := range results.Issues {
		if !IsBlocked(&results.Issues[i]) {
			ready = append(ready, results.Issues[i])
		}
	}

	// Re-sort after filtering.
	SortByPriorityThenCreated(ready)

	// Truncate to limit.
	if len(ready) > opts.Limit {
		ready = ready[:opts.Limit]
	}

	formatter := output.NewFormatter(f.IOStreams, f.OutputJSON, f.JQExpr)

	if f.Quiet {
		return nil
	}

	total := len(ready)
	meta := &output.PaginationMeta{
		Limit: opts.Limit,
		Total: &total,
	}

	if formatter.IsJSON() {
		items := make([]readyItem, len(ready))
		for i, issue := range ready {
			items[i] = toReadyItem(issue)
		}
		return formatter.OutputList(items, meta, nil)
	}

	if len(ready) == 0 {
		fmt.Fprintln(f.IOStreams.Out, "No ready issues found")
		return nil
	}

	return formatter.OutputList(ready, meta, func(tw table.Writer) {
		tw.AppendHeader(table.Row{"KEY", "SUMMARY", "STATUS", "PRIORITY", "TYPE"})
		for _, issue := range ready {
			summary := issue.Fields.Summary
			if len(summary) > 50 {
				summary = summary[:47] + "..."
			}
			status := ""
			if issue.Fields.Status != nil {
				status = issue.Fields.Status.Name
			}
			priority := ""
			if issue.Fields.Priority != nil {
				priority = issue.Fields.Priority.Name
			}
			typeName := ""
			if issue.Fields.IssueType != nil {
				typeName = issue.Fields.IssueType.Name
			}
			tw.AppendRow(table.Row{issue.Key, summary, status, priority, typeName})
		}
	})
}

// readyItem is the JSON representation of a ready queue issue.
type readyItem struct {
	Key      string        `json:"key"`
	Summary  string        `json:"summary"`
	Status   readyStatus   `json:"status"`
	Priority readyPriority `json:"priority"`
	Type     string        `json:"type"`
	Labels   []string      `json:"labels"`
	Created  string        `json:"created"`
	Updated  string        `json:"updated"`
	Parent   string        `json:"parent,omitempty"`
}

type readyStatus struct {
	Name     string `json:"name"`
	Category string `json:"category"`
}

type readyPriority struct {
	Name string `json:"name"`
	Rank int    `json:"rank"`
}

func toReadyItem(issue api.Issue) readyItem {
	item := readyItem{
		Key:     issue.Key,
		Summary: issue.Fields.Summary,
		Labels:  issue.Fields.Labels,
		Created: issue.Fields.Created,
		Updated: issue.Fields.Updated,
	}
	if item.Labels == nil {
		item.Labels = []string{}
	}
	if issue.Fields.Status != nil {
		item.Status.Name = issue.Fields.Status.Name
		if issue.Fields.Status.StatusCategory != nil {
			item.Status.Category = issue.Fields.Status.StatusCategory.Key
		}
	}
	if issue.Fields.Priority != nil {
		item.Priority.Name = issue.Fields.Priority.Name
		item.Priority.Rank = MapPriorityRank(issue.Fields.Priority)
	}
	if issue.Fields.IssueType != nil {
		item.Type = issue.Fields.IssueType.Name
	}
	if issue.Fields.Parent != nil {
		item.Parent = issue.Fields.Parent.Key
	}
	return item
}

// buildReadyJQL constructs JQL for the ready queue.
func buildReadyJQL(ctx context.Context, client *api.Client, project string, opts *ReadyOptions) (string, error) {
	clauses := []string{
		fmt.Sprintf("project = %q", project),
		"statusCategory != Done",
		"statusCategory != \"In Progress\"",
	}

	if opts.Unassigned {
		clauses = append(clauses, "assignee IS EMPTY")
	} else if opts.Assignee != "" {
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

	if opts.Type != "" {
		clauses = append(clauses, fmt.Sprintf("issuetype = %q", opts.Type))
	}

	for _, label := range opts.Labels {
		clauses = append(clauses, fmt.Sprintf("labels = %q", label))
	}

	if opts.Priority != "" {
		clauses = append(clauses, fmt.Sprintf("priority = %q", opts.Priority))
	}

	if opts.Component != "" {
		clauses = append(clauses, fmt.Sprintf("component = %q", opts.Component))
	}

	if clause := shared.SprintJQLClause(opts.Sprint); clause != "" {
		clauses = append(clauses, clause)
	}

	jql := strings.Join(clauses, " AND ")

	switch opts.Sort {
	case "created":
		jql += " ORDER BY created ASC"
	case "updated":
		jql += " ORDER BY updated DESC"
	default:
		jql += " ORDER BY priority ASC, created ASC"
	}

	return jql, nil
}
