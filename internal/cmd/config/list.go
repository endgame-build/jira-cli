package config

import (
	"fmt"
	"sort"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"

	"github.com/endgame-build/jira-cli/internal/factory"
	"github.com/endgame-build/jira-cli/internal/output"
)

// ListOptions holds all resolved inputs for the config list command.
type ListOptions struct {
	Factory *factory.Factory
}

// NewCmdConfigList creates the "config list" command.
func NewCmdConfigList(f *factory.Factory) *cobra.Command {
	opts := &ListOptions{Factory: f}

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all configuration values",
		Long:  "List all set configuration key-value pairs.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfigList(opts)
		},
	}

	return cmd
}

func runConfigList(opts *ListOptions) error {
	cfg, err := opts.Factory.Config()
	if err != nil {
		return err
	}

	values := cfg.List()

	ios := opts.Factory.IOStreams
	f := output.NewFormatter(ios, opts.Factory.OutputJSON, opts.Factory.JQExpr)

	if f.IsJSON() {
		return f.OutputData(values, nil)
	}

	// Sort keys for deterministic output.
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	return f.OutputData(values, func(t table.Writer) {
		for _, k := range keys {
			fmt.Fprintf(ios.Out, "%s=%s\n", k, values[k])
		}
	})
}
