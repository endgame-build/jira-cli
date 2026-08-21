package agent

import (
	"context"
	"fmt"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"

	"github.com/endgame-build/jira-cli/internal/api"
	"github.com/endgame-build/jira-cli/internal/cmd/shared"
	"github.com/endgame-build/jira-cli/internal/factory"
	"github.com/endgame-build/jira-cli/internal/output"
)

// StatusOptions holds all resolved inputs for the agent status command.
type StatusOptions struct {
	Factory *factory.Factory
	Project string
}

// NewCmdStatus creates the "agent status" command.
func NewCmdStatus(f *factory.Factory) *cobra.Command {
	opts := &StatusOptions{
		Factory: f,
	}

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show current agent work status",
		Long:  "Show a summary of what's ready, in progress, blocked, and done today.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStatus(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Project, "project", "p", "", "Jira project key (falls back to default.project config)")

	return cmd
}

// statusResult holds the aggregated status data.
type statusResult struct {
	Project         string       `json:"project"`
	Sprint          *sprintInfo  `json:"sprint,omitempty"`
	ReadyCount      int          `json:"ready_count"`
	InProgressCount int          `json:"in_progress_count"`
	BlockedCount    int          `json:"blocked_count"`
	DoneToday       int          `json:"done_today"`
	MyWork          []myWorkItem `json:"my_work"`
}

type myWorkItem struct {
	Key      string `json:"key"`
	Summary  string `json:"summary"`
	Status   string `json:"status"`
	Priority string `json:"priority"`
}

// runStatus fetches and displays the agent status summary.
func runStatus(opts *StatusOptions) error {
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

	result := statusResult{Project: project}

	// My in-progress work.
	myWorkResults, err := client.SearchIssues(ctx, &api.SearchOptions{
		JQL:        fmt.Sprintf("project = %q AND assignee = currentUser() AND statusCategory = \"In Progress\"", project),
		Fields:     []string{"summary", "status", "priority"},
		MaxResults: 50,
	})
	if err != nil {
		return err
	}
	// Bad credentials make search/jql return HTTP 200 with no issues rather
	// than 401, so without this probe an expired token produces a clean
	// all-zeros summary at exit 0 — see the note in ready.go.
	if len(myWorkResults.Issues) == 0 {
		if err := shared.CheckEmptyResultsAuth(ctx, client, f.IOStreams.Err); err != nil {
			return err
		}
	}

	result.InProgressCount = len(myWorkResults.Issues)
	for _, issue := range myWorkResults.Issues {
		item := myWorkItem{
			Key:     issue.Key,
			Summary: issue.Fields.Summary,
		}
		if issue.Fields.Status != nil {
			item.Status = issue.Fields.Status.Name
		}
		if issue.Fields.Priority != nil {
			item.Priority = issue.Fields.Priority.Name
		}
		result.MyWork = append(result.MyWork, item)
	}

	// Ready count.
	readyResults, err := client.SearchIssues(ctx, &api.SearchOptions{
		JQL:        fmt.Sprintf("project = %q AND statusCategory = \"To Do\"", project),
		Fields:     []string{"summary"},
		MaxResults: 100,
	})
	if err != nil {
		fmt.Fprintf(f.IOStreams.Err, "Warning: could not fetch ready count: %v\n", err)
	} else {
		result.ReadyCount = len(readyResults.Issues)
	}

	// Done today.
	doneResults, err := client.SearchIssues(ctx, &api.SearchOptions{
		JQL:        fmt.Sprintf("project = %q AND statusCategory = Done AND status CHANGED TO Done AFTER startOfDay()", project),
		Fields:     []string{"summary"},
		MaxResults: 100,
	})
	if err != nil {
		fmt.Fprintf(f.IOStreams.Err, "Warning: could not fetch done-today count: %v\n", err)
	} else {
		result.DoneToday = len(doneResults.Issues)
	}

	// Blocked count — uses same fetch+filter approach as the blocked command
	// (standard JQL, no ScriptRunner required).
	blockedResults, err := client.SearchIssues(ctx, &api.SearchOptions{
		JQL:        fmt.Sprintf("project = %q AND statusCategory != Done", project),
		Fields:     AgentReadyFields(),
		MaxResults: 100,
	})
	if err != nil {
		fmt.Fprintf(f.IOStreams.Err, "Warning: could not fetch blocked count: %v\n", err)
	} else {
		for i := range blockedResults.Issues {
			if IsBlocked(&blockedResults.Issues[i]) {
				result.BlockedCount++
			}
		}
	}

	// Active sprint metadata (non-fatal).
	if sprint, err := client.GetActiveSprint(ctx, project); err != nil {
		fmt.Fprintf(f.IOStreams.Err, "Warning: could not fetch sprint info: %v\n", err)
	} else {
		result.Sprint = toSprintInfo(sprint)
	}

	if f.Quiet {
		return nil
	}

	formatter := output.NewFormatter(f.IOStreams, f.OutputJSON, f.JQExpr)

	if formatter.IsJSON() {
		return formatter.OutputData(result, nil)
	}

	// Text output.
	fmt.Fprintf(f.IOStreams.Out, "Project: %s\n", project)
	if result.Sprint != nil {
		fmt.Fprintf(f.IOStreams.Out, "Sprint:  %s (ends %s, %d days left)\n", result.Sprint.Name, result.Sprint.EndDate, result.Sprint.RemainingDays)
	}
	fmt.Fprintln(f.IOStreams.Out)
	fmt.Fprintf(f.IOStreams.Out, "  Ready:       %d\n", result.ReadyCount)
	fmt.Fprintf(f.IOStreams.Out, "  In Progress: %d\n", result.InProgressCount)
	fmt.Fprintf(f.IOStreams.Out, "  Blocked:     %d\n", result.BlockedCount)
	fmt.Fprintf(f.IOStreams.Out, "  Done Today:  %d\n", result.DoneToday)

	if len(result.MyWork) > 0 {
		fmt.Fprintf(f.IOStreams.Out, "\nMy Work:\n")
		tw := table.NewWriter()
		tw.SetOutputMirror(f.IOStreams.Out)
		tw.SetStyle(table.StyleLight)
		tw.Style().Options.DrawBorder = false
		tw.Style().Options.SeparateHeader = true
		tw.AppendHeader(table.Row{"KEY", "SUMMARY", "STATUS", "PRIORITY"})
		for _, item := range result.MyWork {
			summary := item.Summary
			if len(summary) > 50 {
				summary = summary[:47] + "..."
			}
			tw.AppendRow(table.Row{item.Key, summary, item.Status, item.Priority})
		}
		tw.Render()
	}

	return nil
}
