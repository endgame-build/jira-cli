package alias

import (
	"fmt"
	"regexp"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"

	cliErrors "github.com/endgameio/jira-cli/internal/errors"
	"github.com/endgameio/jira-cli/internal/factory"
	"github.com/endgameio/jira-cli/internal/output"
)

// aliasNameRe allows alphanumeric characters and hyphens.
var aliasNameRe = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?$`)

// aliasManager is the subset of config methods needed for alias operations.
type aliasManager interface {
	Aliases() map[string]string
	SetAlias(name, command string)
	Save() error
}

// SetOptions holds all resolved inputs for the alias set command.
type SetOptions struct {
	Name    string
	Command string

	Factory *factory.Factory

	// rootCmd is used for shadow detection against built-in commands.
	rootCmd *cobra.Command
}

// NewCmdAliasSet creates the "alias set" command.
func NewCmdAliasSet(f *factory.Factory) *cobra.Command {
	opts := &SetOptions{Factory: f}

	cmd := &cobra.Command{
		Use:   "set <name> <command>",
		Short: "Create or update a command alias",
		Long:  "Create or update a command alias. Alias names must be alphanumeric (hyphens allowed) and cannot shadow built-in commands.",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Name = args[0]
			opts.Command = args[1]
			opts.rootCmd = cmd.Root()
			return runAliasSet(opts)
		},
	}

	return cmd
}

func runAliasSet(opts *SetOptions) error {
	// Validate alias name format.
	if !aliasNameRe.MatchString(opts.Name) {
		return cliErrors.NewValidationError(
			fmt.Sprintf("Invalid alias name %q: must be alphanumeric with optional hyphens", opts.Name),
		).WithSuggestion("Alias names must match [a-zA-Z0-9-] and cannot start or end with a hyphen.")
	}

	// Shadow detection: check against built-in command names.
	if opts.rootCmd != nil {
		for _, sub := range opts.rootCmd.Commands() {
			if sub.Name() == opts.Name {
				return cliErrors.NewValidationError(
					fmt.Sprintf("Cannot alias %q: conflicts with built-in command", opts.Name),
				).WithSuggestion("Choose a different alias name that does not match an existing command.")
			}
		}
	}

	cfg, err := opts.Factory.Config()
	if err != nil {
		return err
	}

	mgr, ok := cfg.(aliasManager)
	if !ok {
		return fmt.Errorf("config does not support aliases")
	}

	mgr.SetAlias(opts.Name, opts.Command)

	if err := mgr.Save(); err != nil {
		return err
	}

	ios := opts.Factory.IOStreams
	f := output.NewFormatter(ios, opts.Factory.OutputJSON, opts.Factory.JQExpr)

	if opts.Factory.Quiet {
		return nil
	}

	return f.OutputMutation(
		map[string]interface{}{
			"name":    opts.Name,
			"command": opts.Command,
		},
		func(t table.Writer) {
			ios.Out.Write([]byte(fmt.Sprintf("Alias %s = %s\n", opts.Name, opts.Command)))
		},
	)
}
