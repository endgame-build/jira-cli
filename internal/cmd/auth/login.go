package auth

import (
	"context"
	"fmt"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"

	"github.com/endgame-build/jira-cli/internal/api"
	internalAuth "github.com/endgame-build/jira-cli/internal/auth"
	"github.com/endgame-build/jira-cli/internal/cmd/meta"
	"github.com/endgame-build/jira-cli/internal/config"
	cliErrors "github.com/endgame-build/jira-cli/internal/errors"
	"github.com/endgame-build/jira-cli/internal/factory"
	"github.com/endgame-build/jira-cli/internal/output"
)

// LoginOptions holds all resolved inputs for the login command.
type LoginOptions struct {
	Instance string
	User     string
	Token    string
	Profile  string

	Factory    *factory.Factory
	clientOpts []api.ClientOption // for test injection of WithBaseURL
}

// NewCmdLogin creates the "auth login" command.
func NewCmdLogin(f *factory.Factory) *cobra.Command {
	opts := &LoginOptions{Factory: f}

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate with a Jira Cloud instance",
		Long:  "Validate credentials against the Jira API and store them for future use.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.Instance == "" || opts.User == "" || opts.Token == "" {
				return cliErrors.NewValidationError("All three flags required: --instance, --user, --token").
					WithSuggestion("Provide --instance, --user, and --token together")
			}

			opts.Instance = internalAuth.NormalizeInstanceURL(opts.Instance)

			// Use --profile from global flag if not empty, else default.
			if f.Profile != "" {
				opts.Profile = f.Profile
			}
			if opts.Profile == "" {
				opts.Profile = "default"
			}

			return runLogin(opts)
		},
	}

	cmd.Flags().StringVar(&opts.Instance, "instance", "", "Jira instance URL (e.g. mysite.atlassian.net)")
	cmd.Flags().StringVar(&opts.User, "user", "", "Jira user email for authentication")
	cmd.Flags().StringVar(&opts.Token, "token", "", "Jira API token")

	meta.MarkRequired(cmd, "instance", "user", "token")

	return cmd
}

// runLogin validates credentials via GET /myself and stores them.
func runLogin(opts *LoginOptions) error {
	ios := opts.Factory.IOStreams

	// Create a temporary API client from the provided flags (bypass normal Factory auth chain).
	creds := &internalAuth.Credentials{
		Instance: opts.Instance,
		User:     opts.User,
		Token:    opts.Token,
	}
	client := api.NewClient(creds, opts.clientOpts...)

	// Validate credentials via GET /myself.
	var user api.User
	if err := client.Do(context.Background(), "GET", "myself", nil, &user); err != nil {
		return err
	}

	// Store profile in config (instance + user).
	cfg, err := opts.Factory.Config()
	if err != nil {
		return err
	}

	// Type-assert to access profile methods on *fileConfig.
	type profileSetter interface {
		SetProfile(name, instance, user string)
		SetActiveProfile(name string) error
		config.Config
	}
	pc, ok := cfg.(profileSetter)
	if !ok {
		return fmt.Errorf("config does not support profile management")
	}

	pc.SetProfile(opts.Profile, opts.Instance, opts.User)
	if err := pc.SetActiveProfile(opts.Profile); err != nil {
		return err
	}
	if err := pc.Save(); err != nil {
		return err
	}

	// Store token in keyring (or fallback).
	tokenStore := opts.Factory.TokenStore()
	if err := tokenStore.StoreToken(opts.Profile, opts.Token); err != nil {
		return fmt.Errorf("storing token: %w", err)
	}

	// Output results.
	f := output.NewFormatter(ios, opts.Factory.OutputJSON, opts.Factory.JQExpr)

	if opts.Factory.Quiet {
		return nil
	}

	email := ""
	if user.EmailAddress != nil {
		email = *user.EmailAddress
	}

	return f.OutputMutation(
		map[string]interface{}{
			"profile":      opts.Profile,
			"instance":     opts.Instance,
			"email":        nilIfEmpty(user.EmailAddress),
			"display_name": user.DisplayName,
		},
		func(t table.Writer) {
			if email != "" {
				fmt.Fprintf(ios.Out, "Logged in as %s (%s) on %s\n", user.DisplayName, email, opts.Instance)
			} else {
				fmt.Fprintf(ios.Out, "Logged in as %s on %s\n", user.DisplayName, opts.Instance)
			}
		},
	)
}

// nilIfEmpty returns nil for nil string pointers and the string value otherwise.
// This ensures JSON output has null for missing email, not empty string.
func nilIfEmpty(s *string) interface{} {
	if s == nil {
		return nil
	}
	return *s
}
