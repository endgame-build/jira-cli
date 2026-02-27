// Package schema provides the "jira schema" command group for schema introspection.
package schema

import (
	"github.com/endgameio/jira-cli/internal/factory"
	"github.com/spf13/cobra"
)

// NewCmdSchema creates the "schema" command group.
func NewCmdSchema(f *factory.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "schema <command>",
		Short: "Discover Jira schema metadata",
		Long:  "List fields, issue types, statuses, priorities, and labels available in your Jira instance.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	cmd.AddCommand(NewCmdFields(f))
	cmd.AddCommand(NewCmdTypes(f))
	cmd.AddCommand(NewCmdStatuses(f))
	cmd.AddCommand(NewCmdPriorities(f))
	cmd.AddCommand(NewCmdLabels(f))

	return cmd
}
