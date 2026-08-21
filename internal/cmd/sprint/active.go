package sprint

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/endgame-build/jira-cli/internal/cmd/shared"
	clierrors "github.com/endgame-build/jira-cli/internal/errors"
	"github.com/endgame-build/jira-cli/internal/factory"
	"github.com/endgame-build/jira-cli/internal/output"
)

// ActiveOptions holds all resolved inputs for the sprint active command.
type ActiveOptions struct {
	Factory *factory.Factory
	Project string
}

// NewCmdActive creates the "sprint active" command.
func NewCmdActive(f *factory.Factory) *cobra.Command {
	opts := &ActiveOptions{
		Factory: f,
	}

	cmd := &cobra.Command{
		Use:   "active",
		Short: "Show the active sprint for a project",
		Long:  "Display details of the active sprint including name, goal, dates, and remaining days.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runActive(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Project, "project", "p", "", "Jira project key (falls back to default.project config)")

	return cmd
}

// activeResult is the JSON representation of the active sprint.
type activeResult struct {
	Name          string `json:"name"`
	Goal          string `json:"goal,omitempty"`
	State         string `json:"state"`
	StartDate     string `json:"start_date,omitempty"`
	EndDate       string `json:"end_date,omitempty"`
	RemainingDays int    `json:"remaining_days"`
	BoardID       int    `json:"board_id"`
}

// runActive executes the sprint active workflow.
func runActive(opts *ActiveOptions) error {
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

	sprint, err := client.GetActiveSprint(ctx, project)
	if err != nil {
		return err
	}
	if sprint == nil {
		return clierrors.NewNotFoundError(
			fmt.Sprintf("No active sprint found for project %s", project), project,
		).WithSuggestion("Ensure the project has a board with an active sprint")
	}

	if f.Quiet {
		return nil
	}

	result := activeResult{
		Name:          sprint.Name,
		Goal:          sprint.Goal,
		State:         sprint.State,
		StartDate:     sprint.StartDate,
		EndDate:       sprint.EndDate,
		RemainingDays: shared.SprintRemainingDays(sprint.EndDate),
		BoardID:       sprint.OriginBoardID,
	}

	formatter := output.NewFormatter(f.IOStreams, f.OutputJSON, f.JQExpr)

	if formatter.IsJSON() {
		return formatter.OutputData(result, nil)
	}

	// Text output.
	out := f.IOStreams.Out
	fmt.Fprintf(out, "Name:      %s\n", result.Name)
	if result.Goal != "" {
		fmt.Fprintf(out, "Goal:      %s\n", result.Goal)
	}
	fmt.Fprintf(out, "State:     %s\n", result.State)
	if result.StartDate != "" {
		fmt.Fprintf(out, "Start:     %s\n", shared.TruncateDate(result.StartDate))
	}
	if result.EndDate != "" {
		fmt.Fprintf(out, "End:       %s (%d days remaining)\n", shared.TruncateDate(result.EndDate), result.RemainingDays)
	}

	return nil
}
