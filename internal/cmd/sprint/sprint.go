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
		Long:  "List sprints, inspect the active sprint, and move issues into a sprint.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	cmd.AddCommand(NewCmdList(f))
	cmd.AddCommand(NewCmdActive(f))
	cmd.AddCommand(NewCmdAdd(f))

	return cmd
}
