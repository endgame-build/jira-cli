// Package sprint provides the "jira sprint" command group.
package sprint

import (
	"github.com/endgame-build/jira-cli/internal/factory"
	"github.com/spf13/cobra"
)

// NewCmdSprint creates the "sprint" command group.
func NewCmdSprint(f *factory.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sprint <command>",
		Short: "Manage Jira sprints",
		Long:  "List sprints and inspect the active sprint for a project.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	cmd.AddCommand(NewCmdList(f))
	cmd.AddCommand(NewCmdActive(f))

	return cmd
}
