// Package root provides the top-level "jira" command with global flags.
package root

import (
	"github.com/spf13/cobra"

	"github.com/endgameio/jira-cli/internal/cmd/alias"
	authcmd "github.com/endgameio/jira-cli/internal/cmd/auth"
	configcmd "github.com/endgameio/jira-cli/internal/cmd/config"
	"github.com/endgameio/jira-cli/internal/cmd/issue"
	"github.com/endgameio/jira-cli/internal/cmd/search"
	clierrors "github.com/endgameio/jira-cli/internal/errors"
	"github.com/endgameio/jira-cli/internal/factory"
)

// Version is set via ldflags at build time.
var Version = "dev"

// NewCmdRoot creates the root "jira" command with global persistent flags.
func NewCmdRoot(f *factory.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "jira <command> [flags]",
		Short: "Jira CLI — interact with Jira Cloud from the terminal",
		Long:  "A command-line interface for Jira Cloud. Manage issues, search with JQL, and configure profiles.",

		SilenceErrors: true,
		SilenceUsage:  true,

		Version: Version,
	}

	// Enable typo suggestions (e.g., "jira isue" → "Did you mean issue?").
	cmd.SuggestionsMinimumDistance = 2

	// --- Global persistent flags ---
	cmd.PersistentFlags().StringVar(&f.Profile, "profile", "", "Use a named authentication profile")
	cmd.PersistentFlags().BoolVar(&f.OutputJSON, "json", false, "Output in JSON format")
	cmd.PersistentFlags().BoolVar(&f.NoColor, "no-color", false, "Disable color output")
	cmd.PersistentFlags().BoolVar(&f.Verbose, "verbose", false, "Enable verbose output (no-op)")
	cmd.PersistentFlags().BoolVar(&f.DryRun, "dry-run", false, "Preview changes without executing")
	cmd.PersistentFlags().BoolVarP(&f.Quiet, "quiet", "q", false, "Suppress non-essential output")
	cmd.PersistentFlags().StringVar(&f.JQExpr, "jq", "", "Filter JSON output with a jq expression")
	cmd.PersistentFlags().StringVar(&f.FlagInstance, "instance", "", "Jira instance URL")
	cmd.PersistentFlags().StringVar(&f.FlagUser, "user", "", "Jira user email")
	cmd.PersistentFlags().StringVar(&f.FlagToken, "token", "", "Jira API token")
	cmd.PersistentFlags().BoolVar(&f.Text, "text", false, "Force text output (overrides config)")

	// PersistentPreRunE runs before every command — reads flags into Factory,
	// resolves output format from config, configures color. Does NOT resolve auth.
	cmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		return preRun(f, cmd)
	}

	// --- Subcommand groups ---
	cmd.AddCommand(authcmd.NewCmdAuth(f))
	cmd.AddCommand(issue.NewCmdIssue(f))
	cmd.AddCommand(search.NewCmdSearch(f))
	cmd.AddCommand(configcmd.NewCmdConfig(f))
	cmd.AddCommand(alias.NewCmdAlias(f))

	return cmd
}

// preRun handles global flag propagation and validation.
func preRun(f *factory.Factory, cmd *cobra.Command) error {
	// --- Flag conflict validation (US-011b) ---
	if err := validateFlagConflicts(f); err != nil {
		return err
	}

	// --jq implies --json.
	if f.JQExpr != "" {
		f.OutputJSON = true
	}

	// Resolve JSON mode from config if not set via flags.
	if !f.OutputJSON && !f.Text {
		cfg, err := f.Config()
		if err == nil && cfg != nil && cfg.Get("output.format") == "json" {
			f.OutputJSON = true
		}
	}

	// --text overrides config-level JSON back to text.
	if f.Text {
		f.OutputJSON = false
	}

	// Configure color.
	if f.NoColor {
		f.IOStreams.SetColorEnabled(false)
	}

	// Sync IsJSON on IOStreams for pager gating.
	f.IOStreams.IsJSON = f.OutputJSON

	return nil
}

// validateFlagConflicts checks for mutually exclusive global flag combinations.
func validateFlagConflicts(f *factory.Factory) error {
	if f.OutputJSON && f.Text {
		return clierrors.NewValidationError("Cannot use --json and --text together").
			WithSuggestion("Use --json for machine-readable output or --text for human-readable output, not both.")
	}

	if f.JQExpr != "" && f.Text {
		return clierrors.NewValidationError("Cannot use --jq and --text together").
			WithSuggestion("--jq implies JSON output, which conflicts with --text.")
	}

	if f.Quiet && (f.OutputJSON || f.JQExpr != "") {
		return clierrors.NewValidationError("Cannot use --quiet with --json or --jq").
			WithSuggestion("--quiet suppresses output; --json/--jq requires output. Choose one.")
	}

	return nil
}
