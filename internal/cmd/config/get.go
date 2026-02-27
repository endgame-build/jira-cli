package config

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/endgame-build/jira-cli/internal/factory"
	"github.com/endgame-build/jira-cli/internal/output"
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

	if opts.Factory.Quiet {
		return nil
	}

	ios := opts.Factory.IOStreams
	formatter := output.NewFormatter(ios, opts.Factory.OutputJSON, opts.Factory.JQExpr)

	if formatter.IsJSON() {
		return formatter.RawJSON(map[string]string{
			"key":   opts.Key,
			"value": value,
		})
	}

	fmt.Fprintln(ios.Out, value)
	return nil
}
