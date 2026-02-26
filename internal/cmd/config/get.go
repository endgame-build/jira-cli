package config

import (
	"github.com/spf13/cobra"

	"github.com/endgameio/jira-cli/internal/factory"
)

// GetOptions holds all resolved inputs for the config get command.
type GetOptions struct {
	Key string

	Factory *factory.Factory
}

// NewCmdConfigGet creates the "config get" command.
func NewCmdConfigGet(f *factory.Factory) *cobra.Command {
	opts := &GetOptions{Factory: f}

	cmd := &cobra.Command{
		Use:   "get <key>",
		Short: "Get a configuration value",
		Long:  "Print the current value of a configuration key. Unset keys print an empty string.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Key = args[0]
			return runConfigGet(opts)
		},
	}

	return cmd
}

func runConfigGet(opts *GetOptions) error {
	cfg, err := opts.Factory.Config()
	if err != nil {
		return err
	}

	value := cfg.Get(opts.Key)

	ios := opts.Factory.IOStreams
	ios.Out.Write([]byte(value + "\n"))

	return nil
}
