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

// BlockedOptions holds all resolved inputs for the agent blocked command.
type BlockedOptions struct {
	Factory *factory.Factory
	Project string
	Limit   int
}

// NewCmdBlocked creates the "agent blocked" command.
func NewCmdBlocked(f *factory.Factory) *cobra.Command {
	opts := &BlockedOptions{
		Factory: f,
	}

	cmd := &cobra.Command{
		Use:   "blocked",
		Short: "Show issues blocked by unresolved dependencies",
		Long:  "List issues that cannot proceed because they have unresolved 'is blocked by' links.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBlocked(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Project, "project", "p", "", "Jira project key (falls back to default.project config)")
	cmd.Flags().IntVar(&opts.Limit, "limit", 50, "Maximum issues to return")

	return cmd
}

// blockedItem is the JSON representation of a blocked issue.
type blockedItem struct {
	Key       string        `json:"key"`
	Summary   string        `json:"summary"`
	Status    string        `json:"status"`
	BlockedBy []blockerInfo `json:"blocked_by"`
}

type blockerInfo struct {
	Key     string `json:"key"`
	Summary string `json:"summary"`
	Status  string `json:"status"`
}

// runBlocked finds and displays blocked issues.
func runBlocked(opts *BlockedOptions) error {
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

	// Fetch open issues with their links.
	results, err := client.SearchIssues(ctx, &api.SearchOptions{
		JQL:        fmt.Sprintf("project = %q AND statusCategory != Done", project),
		Fields:     AgentReadyFields(),
		MaxResults: opts.Limit * 2,
	})
	if err != nil {
		return err
	}

	// Filter to only blocked issues and collect blocker details.
	var items []blockedItem
	for i := range results.Issues {
		issue := &results.Issues[i]
		blockers := FindBlockers(issue)
		if len(blockers) == 0 {
			continue
		}

		item := blockedItem{
			Key:     issue.Key,
			Summary: issue.Fields.Summary,
		}
		if issue.Fields.Status != nil {
			item.Status = issue.Fields.Status.Name
		}
		for _, b := range blockers {
			bi := blockerInfo{Key: b.Key}
			if b.Fields != nil {
				bi.Summary = b.Fields.Summary
				if b.Fields.Status != nil {
					bi.Status = b.Fields.Status.Name
				}
			}
			item.BlockedBy = append(item.BlockedBy, bi)
		}
		items = append(items, item)
	}

	if len(items) > opts.Limit {
		items = items[:opts.Limit]
	}

	if f.Quiet {
		return nil
	}

	formatter := output.NewFormatter(f.IOStreams, f.OutputJSON, f.JQExpr)

	total := len(items)
	meta := &output.PaginationMeta{
		Limit: opts.Limit,
		Total: &total,
	}

	if formatter.IsJSON() {
		return formatter.OutputList(items, meta, nil)
	}

	if len(items) == 0 {
		fmt.Fprintln(f.IOStreams.Out, "No blocked issues found")
		return nil
	}

	return formatter.OutputList(items, meta, func(tw table.Writer) {
		tw.AppendHeader(table.Row{"KEY", "SUMMARY", "STATUS", "BLOCKED BY"})
		for _, item := range items {
			summary := item.Summary
			if len(summary) > 40 {
				summary = summary[:37] + "..."
			}
			var blockerKeys []string
			for _, b := range item.BlockedBy {
				blockerKeys = append(blockerKeys, b.Key)
			}
			tw.AppendRow(table.Row{
				item.Key, summary, item.Status, fmt.Sprintf("%v", blockerKeys),
			})
		}
	})
}
