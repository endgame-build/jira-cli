package auth

import (
	"fmt"
	"strings"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"

	"github.com/endgame-build/jira-cli/internal/config"
	cliErrors "github.com/endgame-build/jira-cli/internal/errors"
	"github.com/endgame-build/jira-cli/internal/factory"
	"github.com/endgame-build/jira-cli/internal/output"
)

// SwitchOptions holds all resolved inputs for the switch command.
type SwitchOptions struct {
	Profile string

	Factory *factory.Factory
}

// NewCmdSwitch creates the "auth switch" command.
func NewCmdSwitch(f *factory.Factory) *cobra.Command {
	opts := &SwitchOptions{Factory: f}

	cmd := &cobra.Command{
		Use:   "switch <profile>",
		Short: "Switch the active authentication profile",
		Long:  "Change the active profile used for Jira API authentication.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Profile = args[0]
			return runSwitch(opts)
		},
	}

	return cmd
}

// runSwitch sets the active profile.
func runSwitch(opts *SwitchOptions) error {
	f := opts.Factory
	ios := f.IOStreams

	cfg, err := f.Config()
	if err != nil {
		return err
	}

	type profileSwitcher interface {
		GetProfile(name string) *config.Profile
		SetActiveProfile(name string) error
		ListProfiles() []string
		config.Config
	}
	ps, ok := cfg.(profileSwitcher)
	if !ok {
		return fmt.Errorf("config does not support profile management")
	}

	// Verify profile exists.
	profile := ps.GetProfile(opts.Profile)
	if profile == nil {
		available := ps.ListProfiles()
		msg := fmt.Sprintf("Profile %q not found", opts.Profile)
		if len(available) > 0 {
			msg = fmt.Sprintf("Profile %q not found. Available: %s", opts.Profile, strings.Join(available, ", "))
		}
		return cliErrors.NewNotFoundError(msg, opts.Profile).
			WithSuggestion("Run 'jira auth login' to create a profile")
	}

	// Set active profile and save.
	if err := ps.SetActiveProfile(opts.Profile); err != nil {
		return err
	}
	if err := ps.Save(); err != nil {
		return err
	}

	// Output results.
	formatter := output.NewFormatter(ios, f.OutputJSON, f.JQExpr)

	if f.Quiet {
		return nil
	}

	return formatter.OutputMutation(
		map[string]interface{}{
			"profile":  opts.Profile,
			"instance": profile.Instance,
		},
		func(t table.Writer) {
			fmt.Fprintf(ios.Out, "Switched to profile %q (%s)\n", opts.Profile, profile.Instance)
		},
	)
}
