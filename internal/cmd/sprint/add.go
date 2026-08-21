package sprint

import (
	"context"
	"fmt"
	"strconv"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"

	"github.com/endgame-build/jira-cli/internal/cmd/shared"
	clierrors "github.com/endgame-build/jira-cli/internal/errors"
	"github.com/endgame-build/jira-cli/internal/factory"
	"github.com/endgame-build/jira-cli/internal/output"
)

// maxSprintIssues is Jira's per-request cap for sprint issue moves.
const maxSprintIssues = 50

// AddOptions holds all resolved inputs for the sprint add command.
type AddOptions struct {
	Factory   *factory.Factory
	SprintID  int
	Project   string
	IssueKeys []string
}

// NewCmdAdd creates the "sprint add" command.
func NewCmdAdd(f *factory.Factory) *cobra.Command {
	opts := &AddOptions{Factory: f}

	cmd := &cobra.Command{
		Use:   "add <issue-key>...",
		Short: "Add issues to a sprint",
		Long: "Move one or more issues into a sprint.\n\n" +
			"Without --sprint the issues go to the project's active sprint.",
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			keys := make([]string, 0, len(args))
			for _, arg := range args {
				key, err := shared.ValidateIssueKeyOrID(arg)
				if err != nil {
					return err
				}
				keys = append(keys, key)
			}
			if len(keys) > maxSprintIssues {
				return clierrors.NewValidationError(
					fmt.Sprintf("Too many issues: %d (Jira accepts at most %d per request)", len(keys), maxSprintIssues),
				).WithSuggestion("Split the issues across several invocations")
			}
			opts.IssueKeys = keys

			return runAdd(opts)
		},
	}

	cmd.Flags().IntVar(&opts.SprintID, "sprint", 0, "Sprint ID (defaults to the project's active sprint)")
	cmd.Flags().StringVarP(&opts.Project, "project", "p", "", "Jira project key (falls back to default.project config)")

	return cmd
}

// runAdd moves the issues into the target sprint.
func runAdd(opts *AddOptions) error {
	f := opts.Factory
	ctx := context.Background()

	client, err := f.APIClient()
	if err != nil {
		return err
	}

	sprintID := opts.SprintID
	sprintName := ""

	// Without an explicit --sprint, resolve the project's active sprint.
	if sprintID == 0 {
		project, err := shared.ResolveProject(f, opts.Project)
		if err != nil {
			return err
		}

		active, err := client.GetActiveSprint(ctx, project)
		if err != nil {
			return err
		}
		if active == nil {
			return clierrors.NewNotFoundError(
				fmt.Sprintf("No active sprint found for project %s", project), project,
			).WithSuggestion("Specify --sprint <id>, or start a sprint on the project's board")
		}
		sprintID = active.ID
		sprintName = active.Name
	}

	formatter := output.NewFormatter(f.IOStreams, f.OutputJSON, f.JQExpr)

	if f.DryRun {
		payload := map[string]interface{}{
			"action": "sprint add",
			"sprint": sprintID,
			"issues": opts.IssueKeys,
		}
		if sprintName != "" {
			payload["sprint_name"] = sprintName
		}
		return formatter.OutputDryRun(payload, "passed", func(tw table.Writer) {
			tw.AppendHeader(table.Row{"FIELD", "VALUE"})
			tw.AppendRow(table.Row{"Sprint", sprintLabel(sprintID, sprintName)})
			tw.AppendRow(table.Row{"Issues", fmt.Sprintf("%v", opts.IssueKeys)})
		})
	}

	if err := client.AddIssuesToSprint(ctx, sprintID, opts.IssueKeys); err != nil {
		return err
	}

	if f.Quiet {
		return nil
	}

	if formatter.IsJSON() {
		extras := map[string]interface{}{
			"sprint": sprintID,
			"issues": opts.IssueKeys,
			"count":  len(opts.IssueKeys),
		}
		if sprintName != "" {
			extras["sprint_name"] = sprintName
		}
		return formatter.OutputMutation(extras, nil)
	}

	fmt.Fprintf(f.IOStreams.Out, "Added %d issue(s) to sprint %s\n",
		len(opts.IssueKeys), sprintLabel(sprintID, sprintName))
	return nil
}

// sprintLabel renders a sprint for display, preferring its name when known.
func sprintLabel(id int, name string) string {
	if name == "" {
		return strconv.Itoa(id)
	}
	return fmt.Sprintf("%s (%d)", name, id)
}
