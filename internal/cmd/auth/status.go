package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"

	"github.com/endgame-build/jira-cli/internal/api"
	cliErrors "github.com/endgame-build/jira-cli/internal/errors"
	"github.com/endgame-build/jira-cli/internal/factory"
	"github.com/endgame-build/jira-cli/internal/output"
)

// StatusOptions holds all resolved inputs for the status command.
type StatusOptions struct {
	Factory *factory.Factory

	// Check makes an invalid token an error rather than a reportable state, so
	// the command can gate a script or hook. Off by default: the exit code is
	// part of the existing contract.
	Check bool

	clientOpts []api.ClientOption // for test injection of WithBaseURL
}

// NewCmdStatus creates the "auth status" command.
func NewCmdStatus(f *factory.Factory) *cobra.Command {
	opts := &StatusOptions{Factory: f}

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show authentication status",
		Long: "Display the current profile, instance, user, and token validity.\n\n" +
			"By default an invalid token is reported and the command still exits 0. " +
			"Pass --check to exit with an authentication error instead, so this can " +
			"gate a script or a hook.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStatus(opts)
		},
	}

	cmd.Flags().BoolVar(&opts.Check, "check", false, "Exit with an error if the token is invalid")

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
	var authErr error
	if apiErr != nil {
		var cliErr *cliErrors.CLIError
		if errors.As(apiErr, &cliErr) && cliErr.Code == cliErrors.AUTH_ERROR {
			// 401 — token is invalid. Reportable state by default; an error
			// under --check.
			tokenValid = false
			authErr = apiErr
		} else {
			// Other errors (network, 5xx) are real command failures.
			return apiErr
		}
	}

	formatter := output.NewFormatter(ios, f.OutputJSON, f.JQExpr)

	if f.Quiet {
		if opts.Check && !tokenValid {
			return authErr
		}
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

	// Report first, then fail: the caller should see why the check failed.
	if err := formatter.OutputData(data, func(t table.Writer) {
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
	}); err != nil {
		return err
	}

	if opts.Check && !tokenValid {
		return authErr
	}
	return nil
}
