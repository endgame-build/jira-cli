package auth

import (
	"fmt"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"

	"github.com/endgameio/jira-cli/internal/config"
	cliErrors "github.com/endgameio/jira-cli/internal/errors"
	"github.com/endgameio/jira-cli/internal/factory"
	"github.com/endgameio/jira-cli/internal/output"
)

// LogoutOptions holds all resolved inputs for the logout command.
type LogoutOptions struct {
	Yes     bool
	Profile string

	Factory *factory.Factory
}

// NewCmdLogout creates the "auth logout" command.
func NewCmdLogout(f *factory.Factory) *cobra.Command {
	opts := &LogoutOptions{Factory: f}

	cmd := &cobra.Command{
		Use:   "logout",
		Short: "Remove stored credentials",
		Long:  "Remove the stored API token and profile configuration for a Jira profile.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if !opts.Yes {
				return cliErrors.NewValidationError("Use --yes to confirm logout").
					WithSuggestion("Run: jira auth logout --yes")
			}

			// Use --profile from global flag if set, else default to active profile.
			if f.Profile != "" {
				opts.Profile = f.Profile
			}
			if opts.Profile == "" {
				cfg, err := f.Config()
				if err != nil {
					return err
				}
				if cfg != nil {
					type activeProfiler interface {
						ActiveProfile() string
					}
					if ap, ok := cfg.(activeProfiler); ok {
						opts.Profile = ap.ActiveProfile()
					}
				}
			}
			if opts.Profile == "" {
				opts.Profile = "default"
			}

			return runLogout(opts)
		},
	}

	cmd.Flags().BoolVarP(&opts.Yes, "yes", "y", false, "Confirm logout")

	return cmd
}

// runLogout removes the token and profile from storage.
func runLogout(opts *LogoutOptions) error {
	f := opts.Factory
	ios := f.IOStreams

	// Load config and verify profile exists.
	cfg, err := f.Config()
	if err != nil {
		return err
	}

	type profileManager interface {
		GetProfile(name string) *config.Profile
		DeleteProfile(name string) error
		config.Config
	}
	pm, ok := cfg.(profileManager)
	if !ok {
		return fmt.Errorf("config does not support profile management")
	}

	if pm.GetProfile(opts.Profile) == nil {
		return cliErrors.NewNotFoundError("profile", opts.Profile).
			WithSuggestion("Run 'jira auth login' to create a profile")
	}

	// Delete token from keyring/fallback.
	tokenStore := f.TokenStore()
	_ = tokenStore.DeleteToken(opts.Profile) // best-effort; token may not exist

	// Delete profile from config.
	if err := pm.DeleteProfile(opts.Profile); err != nil {
		return err
	}
	if err := pm.Save(); err != nil {
		return err
	}

	// Output results.
	formatter := output.NewFormatter(ios, f.OutputJSON, f.JQExpr)

	if f.Quiet {
		return nil
	}

	return formatter.OutputMutation(
		map[string]interface{}{
			"profile": opts.Profile,
			"action":  "logout",
		},
		func(t table.Writer) {
			fmt.Fprintf(ios.Out, "Logged out from profile %q\n", opts.Profile)
		},
	)
}
