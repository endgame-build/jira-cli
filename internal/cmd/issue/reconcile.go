package issue

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"

	"github.com/endgame-build/jira-cli/internal/api"
	clierrors "github.com/endgame-build/jira-cli/internal/errors"
	"github.com/endgame-build/jira-cli/internal/factory"
	"github.com/endgame-build/jira-cli/internal/markdown"
	"github.com/endgame-build/jira-cli/internal/output"
)

// ReconcileOptions holds all resolved inputs for the issue reconcile command.
type ReconcileOptions struct {
	Factory *factory.Factory

	Dir          string // --dir: markdown directory
	Epic         string // --epic: scope to children of this epic
	Project      string // --project: scope to all issues in project
	Action       string // --action: list, close, delete
	TargetStatus string // --target-status: status for close action
	Yes          bool   // --yes: skip confirmation
}

// orphanInfo holds information about an orphaned Jira issue.
type orphanInfo struct {
	Key     string `json:"key"`
	Summary string `json:"summary"`
	Status  string `json:"status"`
	Action  string `json:"action,omitempty"`
}

// NewCmdReconcile creates the "issue reconcile" command.
func NewCmdReconcile(f *factory.Factory) *cobra.Command {
	opts := &ReconcileOptions{
		Factory: f,
	}

	cmd := &cobra.Command{
		Use:   "reconcile",
		Short: "Detect orphaned Jira issues not in markdown directory",
		Long:  "Compare markdown files against Jira to find issues that exist in Jira but have no corresponding markdown file. Optionally close or delete orphans.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.Dir == "" {
				return clierrors.NewValidationError("--dir is required").
					WithSuggestion("Specify the markdown directory with --dir")
			}
			if opts.Epic == "" && opts.Project == "" {
				return clierrors.NewValidationError("specify --epic or --project to scope the Jira query").
					WithSuggestion("Use --epic PROJ-123 or --project PROJ")
			}
			if opts.Epic != "" && opts.Project != "" {
				return clierrors.NewValidationError("--epic and --project are mutually exclusive").
					WithSuggestion("Use one or the other to scope the Jira query")
			}
			if opts.Epic != "" {
				key, err := ValidateIssueKeyOrID(opts.Epic)
				if err != nil {
					return err
				}
				opts.Epic = key
			}

			switch opts.Action {
			case "list", "close", "delete":
				// valid
			default:
				return clierrors.NewValidationError(fmt.Sprintf("invalid --action %q", opts.Action)).
					WithSuggestion("Use one of: list, close, delete")
			}

			if opts.Action == "delete" && !opts.Yes && !f.DryRun {
				return clierrors.NewValidationError("--yes is required for --action delete").
					WithSuggestion("Add --yes to confirm deletion, or use --dry-run to preview")
			}

			return runReconcile(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Dir, "dir", "d", "", "Markdown directory (required)")
	cmd.Flags().StringVar(&opts.Epic, "epic", "", "Scope to children of this epic")
	cmd.Flags().StringVarP(&opts.Project, "project", "p", "", "Scope to all issues in project")
	cmd.Flags().StringVar(&opts.Action, "action", "list", "Action for orphans: list, close, delete")
	cmd.Flags().StringVar(&opts.TargetStatus, "target-status", "Done", "Target status for --action close")
	cmd.Flags().BoolVarP(&opts.Yes, "yes", "y", false, "Skip confirmation for close/delete")

	return cmd
}

// runReconcile executes the reconciliation workflow.
func runReconcile(opts *ReconcileOptions) error {
	f := opts.Factory
	ctx := context.Background()

	// Step 1: Parse markdown files → set of real issue keys.
	issueFiles, err := markdown.ParseDir(opts.Dir)
	if err != nil {
		return err
	}

	localKeys := make(map[string]bool, len(issueFiles))
	for _, issueFile := range issueFiles {
		key := issueFile.Frontmatter.Key
		if markdown.IsTempKey(key) {
			continue
		}
		localKeys[key] = true
	}

	// Step 2: Build JQL.
	jql := buildReconcileJQL(opts)

	// Step 3: Search Jira and collect orphans.
	client, err := f.APIClient()
	if err != nil {
		return err
	}

	orphans, err := findOrphans(ctx, client, jql, localKeys, opts.Epic)
	if err != nil {
		return err
	}

	// Step 4: Act on orphans.
	for i := range orphans {
		orphans[i].Action = opts.Action
	}

	if opts.Action == "close" && !f.DryRun {
		if err := closeOrphans(ctx, client, orphans, opts.TargetStatus, f); err != nil {
			return err
		}
	}

	if opts.Action == "delete" && !f.DryRun {
		if err := deleteOrphans(ctx, client, orphans); err != nil {
			return err
		}
	}

	return renderReconcileResults(opts, orphans)
}

// buildReconcileJQL constructs the JQL query for reconciliation.
func buildReconcileJQL(opts *ReconcileOptions) string {
	if opts.Epic != "" {
		return fmt.Sprintf("parent = %s ORDER BY key ASC", opts.Epic)
	}
	return fmt.Sprintf("project = '%s' ORDER BY key ASC", opts.Project)
}

// findOrphans paginates through Jira search results and returns issues
// whose key is not in the local set. The epic key itself is excluded.
func findOrphans(ctx context.Context, client *api.Client, jql string, localKeys map[string]bool, epicKey string) ([]orphanInfo, error) {
	var orphans []orphanInfo
	token := ""

	for {
		results, err := client.SearchIssues(ctx, &api.SearchOptions{
			JQL:           jql,
			Fields:        []string{"summary", "status"},
			MaxResults:    50,
			NextPageToken: token,
		})
		if err != nil {
			return nil, err
		}

		for _, issue := range results.Issues {
			// Skip the epic itself.
			if issue.Key == epicKey {
				continue
			}
			if localKeys[issue.Key] {
				continue
			}

			status := ""
			if issue.Fields.Status != nil {
				status = issue.Fields.Status.Name
			}

			orphans = append(orphans, orphanInfo{
				Key:     issue.Key,
				Summary: issue.Fields.Summary,
				Status:  status,
			})
		}

		if results.IsLast || len(results.Issues) == 0 {
			break
		}
		token = results.NextPageToken
	}

	return orphans, nil
}

// closeOrphans transitions each orphan to the target status.
func closeOrphans(ctx context.Context, client *api.Client, orphans []orphanInfo, targetStatus string, f *factory.Factory) error {
	for i, orphan := range orphans {
		transitions, err := client.GetTransitions(ctx, orphan.Key)
		if err != nil {
			return err
		}

		sort.Slice(transitions, func(a, b int) bool {
			return transitions[a].ID < transitions[b].ID
		})

		matched, err := matchTransition(transitions, targetStatus)
		if err != nil {
			fmt.Fprintf(f.IOStreams.Err, "warning: %s: cannot transition to %q, skipping: %v\n", orphan.Key, targetStatus, err)
			orphans[i].Action = "skipped"
			continue
		}

		input := &api.DoTransitionInput{
			Transition: api.TransitionRef{ID: matched.ID},
		}
		if err := client.DoTransition(ctx, orphan.Key, input); err != nil {
			return err
		}
		orphans[i].Action = "closed"
	}
	return nil
}

// deleteOrphans deletes each orphan issue (with subtasks).
func deleteOrphans(ctx context.Context, client *api.Client, orphans []orphanInfo) error {
	for i, orphan := range orphans {
		if err := client.DeleteIssue(ctx, orphan.Key, true); err != nil {
			return err
		}
		orphans[i].Action = "deleted"
	}
	return nil
}

// renderReconcileResults outputs the reconciliation results.
func renderReconcileResults(opts *ReconcileOptions, orphans []orphanInfo) error {
	f := opts.Factory
	formatter := output.NewFormatter(f.IOStreams, f.OutputJSON, f.JQExpr)

	if f.Quiet {
		return nil
	}

	resultData := map[string]interface{}{
		"orphans": orphans,
		"count":   len(orphans),
		"action":  opts.Action,
	}

	if f.DryRun {
		if formatter.IsJSON() {
			return formatter.OutputDryRun(resultData, "passed", nil)
		}
		return formatter.OutputDryRun(nil, "", func(tw table.Writer) {
			fmt.Fprintf(f.IOStreams.Out, "DRY RUN — reconcile preview\n\n")
			if len(orphans) == 0 {
				fmt.Fprintf(f.IOStreams.Out, "No orphaned issues found.\n")
				return
			}
			renderOrphanTable(f, orphans, opts.Action)
			fmt.Fprintf(f.IOStreams.Out, "\nWould %s %d orphaned issues\n", opts.Action, len(orphans))
		})
	}

	if formatter.IsJSON() {
		return formatter.OutputMutation(resultData, nil)
	}

	// Text output.
	return formatter.OutputMutation(nil, func(tw table.Writer) {
		if len(orphans) == 0 {
			fmt.Fprintf(f.IOStreams.Out, "No orphaned issues found.\n")
			return
		}
		renderOrphanTable(f, orphans, opts.Action)

		actionCounts := map[string]int{}
		for _, o := range orphans {
			actionCounts[o.Action]++
		}
		parts := []string{}
		for action, count := range actionCounts {
			parts = append(parts, fmt.Sprintf("%d %s", count, action))
		}
		sort.Strings(parts)
		fmt.Fprintf(f.IOStreams.Out, "\n%s\n", strings.Join(parts, ", "))
	})
}

// renderOrphanTable prints orphans as a table.
func renderOrphanTable(f *factory.Factory, orphans []orphanInfo, defaultAction string) {
	tw := table.NewWriter()
	tw.SetOutputMirror(f.IOStreams.Out)
	tw.SetStyle(table.StyleLight)
	tw.Style().Options.DrawBorder = false
	tw.Style().Options.SeparateHeader = true
	tw.Style().Options.SeparateRows = false

	tw.AppendHeader(table.Row{"KEY", "SUMMARY", "STATUS", "ACTION"})
	for _, o := range orphans {
		action := o.Action
		if action == "" {
			action = defaultAction
		}
		tw.AppendRow(table.Row{o.Key, o.Summary, o.Status, action})
	}
	tw.Render()
}
