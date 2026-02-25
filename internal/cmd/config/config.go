// Package config provides the "jira config" command group for configuration management.
package config

import (
	"github.com/endgameio/jira-cli/internal/factory"
	"github.com/spf13/cobra"
)

// NewCmdConfig creates the "config" command group.
// Subcommands (get, set, list) are registered in their own stories.
func NewCmdConfig(f *factory.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config <command>",
		Short: "Manage CLI configuration",
		Long:  "Get, set, and list CLI configuration values.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	return cmd
}
