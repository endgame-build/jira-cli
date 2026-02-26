package config

import (
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"

	cliErrors "github.com/endgameio/jira-cli/internal/errors"
	"github.com/endgameio/jira-cli/internal/factory"
	"github.com/endgameio/jira-cli/internal/output"
)

// SetOptions holds all resolved inputs for the config set command.
type SetOptions struct {
	Key   string
	Value string

	Factory *factory.Factory
}

// NewCmdConfigSet creates the "config set" command.
func NewCmdConfigSet(f *factory.Factory) *cobra.Command {
	opts := &SetOptions{Factory: f}

	cmd := &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Set a configuration value",
		Long:  "Set a persistent configuration value. Valid keys: default.project, default.assignee, output.format, output.color.",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Key = args[0]
			opts.Value = args[1]
			return runConfigSet(opts)
		},
	}

	return cmd
}

func runConfigSet(opts *SetOptions) error {
	cfg, err := opts.Factory.Config()
	if err != nil {
		return err
	}

	if err := cfg.Set(opts.Key, opts.Value); err != nil {
		return cliErrors.NewValidationError(err.Error())
	}

	if err := cfg.Save(); err != nil {
		return err
	}

	ios := opts.Factory.IOStreams
	f := output.NewFormatter(ios, opts.Factory.OutputJSON, opts.Factory.JQExpr)

	if opts.Factory.Quiet {
		return nil
	}

	return f.OutputMutation(
		map[string]interface{}{
			"key":   opts.Key,
			"value": opts.Value,
		},
		func(t table.Writer) {
			ios.Out.Write([]byte("Set " + opts.Key + " = " + opts.Value + "\n"))
		},
	)
}
