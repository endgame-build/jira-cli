package sprint

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

// ListOptions holds all resolved inputs for the sprint list command.
type ListOptions struct {
	Factory *factory.Factory
	Project string
	State   string // "active", "future", "closed", or "" for all
	Board   int    // --board (overrides auto-detect)
	NoPager bool
}

// NewCmdList creates the "sprint list" command.
func NewCmdList(f *factory.Factory) *cobra.Command {
	opts := &ListOptions{
		Factory: f,
	}

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List sprints for a project",
		Long:  "List sprints across boards for a project, optionally filtered by state.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.NoPager {
				f.IOStreams.NoPager = true
			}
			return runList(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Project, "project", "p", "", "Jira project key (falls back to default.project config)")
	cmd.Flags().StringVar(&opts.State, "state", "", "Filter by state: active, future, closed")
	cmd.Flags().IntVar(&opts.Board, "board", 0, "Board ID (auto-detected from project if omitted)")
	cmd.Flags().BoolVar(&opts.NoPager, "no-pager", false, "Do not pipe output through a pager")

	return cmd
}

// runList executes the sprint list workflow.
func runList(opts *ListOptions) error {
	f := opts.Factory
	ctx := context.Background()

	client, err := f.APIClient()
	if err != nil {
		return err
	}

	var boardIDs []int

	if opts.Board != 0 {
		boardIDs = []int{opts.Board}
	} else {
		project, err := shared.ResolveProject(f, opts.Project)
		if err != nil {
			return err
		}

		boards, err := client.GetBoardsForProject(ctx, project)
		if err != nil {
			return err
		}

		for _, b := range boards {
			if api.IsSprintBoardType(b.Type) {
				boardIDs = append(boardIDs, b.ID)
			}
		}
	}

	if f.Quiet {
		return nil
	}

	formatter := output.NewFormatter(f.IOStreams, f.OutputJSON, f.JQExpr)

	if len(boardIDs) == 0 {
		if formatter.IsJSON() {
			total := 0
			return formatter.OutputList([]interface{}{}, &output.PaginationMeta{Total: &total}, nil)
		}
		fmt.Fprintln(f.IOStreams.Out, "No sprints found")
		return nil
	}

	// Collect sprints from all scrum boards.
	var allSprints []sprintListItem
	for _, boardID := range boardIDs {
		sprints, err := client.GetSprintsForBoard(ctx, boardID, opts.State)
		if err != nil {
			return err
		}
		for _, s := range sprints {
			allSprints = append(allSprints, sprintListItem{
				ID:        s.ID,
				Name:      s.Name,
				State:     s.State,
				StartDate: s.StartDate,
				EndDate:   s.EndDate,
				Goal:      s.Goal,
				BoardID:   boardID,
			})
		}
	}

	total := len(allSprints)
	meta := &output.PaginationMeta{
		Total: &total,
	}

	if formatter.IsJSON() {
		return formatter.OutputList(allSprints, meta, nil)
	}

	if len(allSprints) == 0 {
		fmt.Fprintln(f.IOStreams.Out, "No sprints found")
		return nil
	}

	ios := f.IOStreams
	ios.StartPager()
	defer ios.StopPager()

	return formatter.OutputList(allSprints, meta, func(tw table.Writer) {
		tw.AppendHeader(table.Row{"NAME", "STATE", "START", "END", "GOAL"})
		for _, s := range allSprints {
			goal := s.Goal
			if len(goal) > 40 {
				goal = goal[:37] + "..."
			}
			tw.AppendRow(table.Row{
				s.Name,
				s.State,
				shared.TruncateDate(s.StartDate),
				shared.TruncateDate(s.EndDate),
				goal,
			})
		}
	})
}

// sprintListItem is the JSON representation of a sprint in list output.
type sprintListItem struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	State     string `json:"state"`
	StartDate string `json:"start_date,omitempty"`
	EndDate   string `json:"end_date,omitempty"`
	Goal      string `json:"goal,omitempty"`
	BoardID   int    `json:"board_id"`
}
