package alias

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"

	"github.com/endgameio/jira-cli/internal/factory"
	"github.com/endgameio/jira-cli/internal/output"
)

// ListOptions holds all resolved inputs for the alias list command.
type ListOptions struct {
	Factory *factory.Factory
}

// NewCmdAliasList creates the "alias list" command.
func NewCmdAliasList(f *factory.Factory) *cobra.Command {
	opts := &ListOptions{Factory: f}

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all command aliases",
		Long:  "List all command aliases defined in configuration.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAliasList(opts)
		},
	}

	return cmd
}

func runAliasList(opts *ListOptions) error {
	cfg, err := opts.Factory.Config()
	if err != nil {
		return err
	}

	mgr, ok := cfg.(aliasManager)
	if !ok {
		return fmt.Errorf("config does not support aliases")
	}

	aliases := mgr.Aliases()

	ios := opts.Factory.IOStreams
	f := output.NewFormatter(ios, opts.Factory.OutputJSON, opts.Factory.JQExpr)

	if f.IsJSON() {
		// JSON: flat object of name→command pairs.
		obj := make(map[string]interface{}, len(aliases))
		for k, v := range aliases {
			obj[k] = v
		}
		return f.OutputData(obj, nil)
	}

	// Text: sorted name=command lines.
	keys := make([]string, 0, len(aliases))
	for k := range aliases {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		fmt.Fprintf(ios.Out, "%s=%s\n", k, aliases[k])
	}

	return nil
}
