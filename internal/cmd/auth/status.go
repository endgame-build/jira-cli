package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"

	"github.com/endgameio/jira-cli/internal/api"
	cliErrors "github.com/endgameio/jira-cli/internal/errors"
	"github.com/endgameio/jira-cli/internal/factory"
	"github.com/endgameio/jira-cli/internal/output"
)

// StatusOptions holds all resolved inputs for the status command.
type StatusOptions struct {
	Factory    *factory.Factory
	clientOpts []api.ClientOption // for test injection of WithBaseURL
}

// NewCmdStatus creates the "auth status" command.
func NewCmdStatus(f *factory.Factory) *cobra.Command {
	opts := &StatusOptions{Factory: f}

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show authentication status",
		Long:  "Display the current profile, instance, user, and token validity.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStatus(opts)
		},
	}

	return cmd
}

// runStatus resolves stored credentials and checks token validity via GET /myself.
func runStatus(opts *StatusOptions) error {
	f := opts.Factory
	ios := f.IOStreams

	// Resolve credentials through the standard chain (flags > env > profile).
	creds, err := f.AuthCredentials()
	if err != nil {
		return err
	}

	profileName := f.Profile
	if profileName == "" {
		// Determine active profile name for display.
		cfg, cfgErr := f.Config()
		if cfgErr == nil && cfg != nil {
			type activeProfiler interface {
				ActiveProfile() string
			}
			if ap, ok := cfg.(activeProfiler); ok {
				profileName = ap.ActiveProfile()
			}
		}
		if profileName == "" {
			profileName = "default"
		}
	}

	// Build a client and check token validity via GET /myself.
	client := api.NewClient(creds, opts.clientOpts...)

	var user api.User
	apiErr := client.Do(context.Background(), "GET", "myself", nil, &user)

	tokenValid := true
	if apiErr != nil {
		var cliErr *cliErrors.CLIError
		if errors.As(apiErr, &cliErr) && cliErr.Code == cliErrors.AUTH_ERROR {
			// 401 — token is invalid, but that's a reportable state, not a command error.
			tokenValid = false
		} else {
			// Other errors (network, 5xx) are real command failures.
			return apiErr
		}
	}

	formatter := output.NewFormatter(ios, f.OutputJSON, f.JQExpr)

	if f.Quiet {
		return nil
	}

	email := "(email hidden)"
	if tokenValid && user.EmailAddress != nil {
		email = *user.EmailAddress
	}

	data := map[string]interface{}{
		"profile":     profileName,
		"instance":    creds.Instance,
		"user":        creds.User,
		"token_valid": tokenValid,
	}

	return formatter.OutputData(data, func(t table.Writer) {
		fmt.Fprintf(ios.Out, "Profile:  %s\n", profileName)
		fmt.Fprintf(ios.Out, "Instance: %s\n", creds.Instance)
		fmt.Fprintf(ios.Out, "User:     %s\n", creds.User)
		if email != creds.User {
			fmt.Fprintf(ios.Out, "Email:    %s\n", email)
		}
		if tokenValid {
			fmt.Fprintf(ios.Out, "Token:    %s\n", ios.Green("valid"))
		} else {
			fmt.Fprintf(ios.Out, "Token:    %s\n", ios.Red("invalid"))
		}
	})
}
