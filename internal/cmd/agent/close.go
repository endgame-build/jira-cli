package agent

import (
	"context"
	"fmt"
	"io"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"

	"github.com/endgame-build/jira-cli/internal/adf"
	"github.com/endgame-build/jira-cli/internal/api"
	"github.com/endgame-build/jira-cli/internal/cmd/shared"
	"github.com/endgame-build/jira-cli/internal/factory"
	"github.com/endgame-build/jira-cli/internal/output"
)

// CloseOptions holds all resolved inputs for the agent close command.
type CloseOptions struct {
	Factory     *factory.Factory
	KeyOrID     string
	Reason      string
	SuggestNext bool
	ClaimNext   bool
}

// NewCmdClose creates the "agent close" command.
func NewCmdClose(f *factory.Factory) *cobra.Command {
	opts := &CloseOptions{
		Factory: f,
	}

	cmd := &cobra.Command{
		Use:   "close <key>",
		Short: "Transition an issue to Done",
		Long:  "Transition an issue to Done and optionally record a close reason as a comment. Can suggest or auto-claim newly unblocked work.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			key, err := shared.ValidateIssueKeyOrID(args[0])
			if err != nil {
				return err
			}
			opts.KeyOrID = key
			return runClose(opts)
		},
	}

	cmd.Flags().StringVar(&opts.Reason, "reason", "", "Close reason (added as comment)")
	cmd.Flags().BoolVar(&opts.SuggestNext, "suggest-next", false, "Show newly unblocked issues after closing")
	cmd.Flags().BoolVar(&opts.ClaimNext, "claim-next", false, "Auto-claim the top unblocked issue after closing")

	return cmd
}

// runClose transitions the issue to Done.
func runClose(opts *CloseOptions) error {
	f := opts.Factory
	ctx := context.Background()

	client, err := f.APIClient()
	if err != nil {
		return err
	}

	// Fetch issue to check current state and get links for unblocked detection.
	issue, err := client.GetIssue(ctx, opts.KeyOrID, &api.GetIssueOptions{
		Fields: []string{"status", "issuelinks"},
	})
	if err != nil {
		return err
	}

	prevStatus := ""
	if issue.Fields.Status != nil {
		prevStatus = issue.Fields.Status.Name
	}

	// Idempotent: already done.
	if issue.Fields.Status != nil && issue.Fields.Status.StatusCategory != nil &&
		issue.Fields.Status.StatusCategory.Key == CategoryDone {
		return renderCloseResult(f, opts.KeyOrID, prevStatus, prevStatus, nil)
	}

	// Find Done transition.
	transitions, err := client.GetTransitions(ctx, opts.KeyOrID)
	if err != nil {
		return err
	}

	tr, err := FindTransitionByCategory(transitions, CategoryDone)
	if err != nil {
		return err
	}

	toStatus := ""
	if tr.To != nil {
		toStatus = tr.To.Name
	}

	formatter := output.NewFormatter(f.IOStreams, f.OutputJSON, f.JQExpr)

	// Dry-run.
	if f.DryRun {
		payload := map[string]interface{}{
			"key":             opts.KeyOrID,
			"action":          "close",
			"previous_status": prevStatus,
			"to":              toStatus,
		}
		if opts.Reason != "" {
			payload["reason"] = opts.Reason
		}
		return formatter.OutputDryRun(payload, "passed", func(tw table.Writer) {
			tw.AppendHeader(table.Row{"FIELD", "VALUE"})
			tw.AppendRow(table.Row{"Issue", opts.KeyOrID})
			tw.AppendRow(table.Row{"Action", "Close"})
			tw.AppendRow(table.Row{"From", prevStatus})
			tw.AppendRow(table.Row{"To", toStatus})
			if opts.Reason != "" {
				tw.AppendRow(table.Row{"Reason", opts.Reason})
			}
		})
	}

	// Execute transition.
	if err := client.DoTransition(ctx, opts.KeyOrID, &api.DoTransitionInput{
		Transition: api.TransitionRef{ID: tr.ID},
	}); err != nil {
		return err
	}

	// Add close reason as comment.
	if opts.Reason != "" {
		commentMD := fmt.Sprintf("Closed: %s", opts.Reason)
		adfDoc, err := adf.Convert(commentMD)
		if err != nil {
			fmt.Fprintf(f.IOStreams.Err, "Warning: failed to convert close reason to ADF: %v\n", err)
		} else {
			if _, err := client.AddComment(ctx, opts.KeyOrID, adfDoc); err != nil {
				fmt.Fprintf(f.IOStreams.Err, "Warning: failed to add close reason comment: %v\n", err)
			}
		}
	}

	// Find newly unblocked issues (non-nil when flags set, for consistent JSON).
	var unblocked []string
	if opts.SuggestNext || opts.ClaimNext {
		unblocked = findNewlyUnblocked(ctx, client, issue, f.IOStreams.Err)
	}

	// Auto-claim top unblocked.
	if opts.ClaimNext && len(unblocked) > 0 {
		claimOpts := &ClaimOptions{
			Factory: f,
			KeyOrID: unblocked[0],
		}
		if claimErr := runClaim(claimOpts); claimErr != nil {
			fmt.Fprintf(f.IOStreams.Err, "Warning: failed to auto-claim %s: %v\n", unblocked[0], claimErr)
		}
	}

	return renderCloseResult(f, opts.KeyOrID, prevStatus, toStatus, unblocked)
}

// findNewlyUnblocked finds issues that were blocked by the closed issue
// and are now fully unblocked (all their blockers resolved).
// Always returns a non-nil slice for consistent JSON output.
// Errors fetching individual candidates are logged to stderr.
func findNewlyUnblocked(ctx context.Context, client *api.Client, closedIssue *api.Issue, stderr io.Writer) []string {
	var candidates []string

	// Find issues that this issue blocks (outward "blocks" links).
	for _, link := range closedIssue.Fields.IssueLinks {
		if link.Type == nil {
			continue
		}
		if link.Type.Outward == "blocks" && link.OutwardIssue != nil {
			candidates = append(candidates, link.OutwardIssue.Key)
		}
	}

	unblocked := make([]string, 0)
	for _, key := range candidates {
		issue, err := client.GetIssue(ctx, key, &api.GetIssueOptions{
			Fields: []string{"status", "issuelinks"},
		})
		if err != nil {
			fmt.Fprintf(stderr, "Warning: could not check blocker status for %s: %v\n", key, err)
			continue
		}
		if issue.Fields.Status != nil && issue.Fields.Status.StatusCategory != nil &&
			issue.Fields.Status.StatusCategory.Key == CategoryDone {
			continue
		}
		if !IsBlocked(issue) {
			unblocked = append(unblocked, key)
		}
	}
	return unblocked
}

// renderCloseResult outputs the close result.
func renderCloseResult(f *factory.Factory, key, prevStatus, toStatus string, unblocked []string) error {
	if f.Quiet {
		return nil
	}

	formatter := output.NewFormatter(f.IOStreams, f.OutputJSON, f.JQExpr)

	if formatter.IsJSON() {
		extras := map[string]interface{}{
			"key":             key,
			"status":          toStatus,
			"previous_status": prevStatus,
		}
		if unblocked != nil {
			extras["unblocked"] = unblocked
		}
		return formatter.OutputMutation(extras, nil)
	}

	fmt.Fprintf(f.IOStreams.Out, "Closed %s → %s\n", key, toStatus)
	if len(unblocked) > 0 {
		fmt.Fprintf(f.IOStreams.Out, "Unblocked: %v\n", unblocked)
	}
	return nil
}
