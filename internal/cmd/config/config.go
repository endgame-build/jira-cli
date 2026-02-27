// Package config provides the "jira config" command group for configuration management.
package config

import (
	"github.com/endgame-build/jira-cli/internal/factory"
	"github.com/spf13/cobra"
)

// NewCmdConfig creates the "config" command group.
func NewCmdConfig(f *factory.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config <command>",
		Short: "Manage CLI configuration",
		Long:  "Get, set, and list CLI configuration values.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	cmd.AddCommand(NewCmdConfigSet(f))
	cmd.AddCommand(NewCmdConfigGet(f))
	cmd.AddCommand(NewCmdConfigList(f))

	return cmd
}
