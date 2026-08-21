package agent

import (
	"context"
	"fmt"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"

	"github.com/endgame-build/jira-cli/internal/api"
	"github.com/endgame-build/jira-cli/internal/cmd/shared"
	clierrors "github.com/endgame-build/jira-cli/internal/errors"
	"github.com/endgame-build/jira-cli/internal/factory"
	"github.com/endgame-build/jira-cli/internal/output"
)

// ClaimOptions holds all resolved inputs for the agent claim command.
type ClaimOptions struct {
	Factory *factory.Factory
	KeyOrID string
	Force   bool
}

// NewCmdClaim creates the "agent claim" command.
func NewCmdClaim(f *factory.Factory) *cobra.Command {
	opts := &ClaimOptions{
		Factory: f,
	}

	cmd := &cobra.Command{
		Use:   "claim <key>",
		Short: "Assign an issue to yourself and transition to In Progress",
		Long:  "Atomically assign an issue to the current user and transition it to In Progress. Idempotent if already claimed by the same user.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			key, err := shared.ValidateIssueKeyOrID(args[0])
			if err != nil {
				return err
			}
			opts.KeyOrID = key
			return runClaim(opts)
		},
	}

	cmd.Flags().BoolVar(&opts.Force, "force", false, "Claim even if assigned to someone else")

	return cmd
}

// runClaim assigns the issue to @me and transitions to In Progress.
func runClaim(opts *ClaimOptions) error {
	f := opts.Factory
	ctx := context.Background()

	client, err := f.APIClient()
	if err != nil {
		return err
	}

	// Fetch issue to check current state.
	issue, err := client.GetIssue(ctx, opts.KeyOrID, &api.GetIssueOptions{
		Fields: []string{"status", "assignee"},
	})
	if err != nil {
		return err
	}

	prevStatus := ""
	if issue.Fields.Status != nil {
		prevStatus = issue.Fields.Status.Name
	}

	// Check if already done — no need to resolve user for this.
	if issue.Fields.Status != nil && issue.Fields.Status.StatusCategory != nil &&
		issue.Fields.Status.StatusCategory.Key == CategoryDone {
		return clierrors.NewValidationError(
			fmt.Sprintf("Issue %s is already Done — cannot claim", opts.KeyOrID),
		).WithSuggestion("Only open or to-do issues can be claimed")
	}

	// Resolve current user.
	myAccountID, err := api.ResolveUser(ctx, client, "@me")
	if err != nil {
		return err
	}

	// Check idempotency: already claimed by me and in progress.
	if issue.Fields.Assignee != nil && issue.Fields.Assignee.AccountID == myAccountID {
		if issue.Fields.Status != nil && issue.Fields.Status.StatusCategory != nil &&
			issue.Fields.Status.StatusCategory.Key == CategoryInProgress {
			return renderClaimResult(f, opts.KeyOrID, myAccountID, prevStatus, prevStatus, true)
		}
	}

	// Check conflict: assigned to someone else.
	var prevAssignee *string
	if issue.Fields.Assignee != nil {
		prevAssignee = &issue.Fields.Assignee.AccountID
		if issue.Fields.Assignee.AccountID != myAccountID && !opts.Force {
			return clierrors.NewConflictError(
				fmt.Sprintf("Issue %s is already assigned to %s", opts.KeyOrID, issue.Fields.Assignee.DisplayName),
			).WithSuggestion("Use --force to claim anyway")
		}
	}

	formatter := output.NewFormatter(f.IOStreams, f.OutputJSON, f.JQExpr)

	// Dry-run: show preview.
	if f.DryRun {
		return formatter.OutputDryRun(
			map[string]interface{}{
				"key":             opts.KeyOrID,
				"action":          "claim",
				"assignee":        myAccountID,
				"previous_status": prevStatus,
			},
			"passed",
			func(tw table.Writer) {
				tw.AppendHeader(table.Row{"FIELD", "VALUE"})
				tw.AppendRow(table.Row{"Issue", opts.KeyOrID})
				tw.AppendRow(table.Row{"Action", "Claim (assign + transition)"})
				tw.AppendRow(table.Row{"From", prevStatus})
				tw.AppendRow(table.Row{"To", "In Progress"})
			},
		)
	}

	// Step 1: Assign to @me.
	if err := client.AssignIssue(ctx, opts.KeyOrID, &myAccountID); err != nil {
		return err
	}

	// Step 2: Transition to In Progress.
	transitions, err := client.GetTransitions(ctx, opts.KeyOrID)
	if err != nil {
		// Rollback assignment on failure.
		_ = client.AssignIssue(ctx, opts.KeyOrID, prevAssignee)
		return err
	}

	tr, err := FindTransitionByCategory(transitions, CategoryInProgress)
	if err != nil {
		// Rollback assignment.
		_ = client.AssignIssue(ctx, opts.KeyOrID, prevAssignee)
		return err
	}

	if err := client.DoTransition(ctx, opts.KeyOrID, &api.DoTransitionInput{
		Transition: api.TransitionRef{ID: tr.ID},
	}); err != nil {
		// Rollback assignment.
		_ = client.AssignIssue(ctx, opts.KeyOrID, prevAssignee)
		return err
	}

	toStatus := ""
	if tr.To != nil {
		toStatus = tr.To.Name
	}

	return renderClaimResult(f, opts.KeyOrID, myAccountID, prevStatus, toStatus, false)
}

// renderClaimResult outputs the claim result.
func renderClaimResult(f *factory.Factory, key, assignee, prevStatus, toStatus string, noop bool) error {
	if f.Quiet {
		return nil
	}

	formatter := output.NewFormatter(f.IOStreams, f.OutputJSON, f.JQExpr)

	if formatter.IsJSON() {
		extras := map[string]interface{}{
			"key":             key,
			"assignee":        assignee,
			"status":          toStatus,
			"previous_status": prevStatus,
		}
		if noop {
			extras["noop"] = true
		}
		return formatter.OutputMutation(extras, nil)
	}

	if noop {
		fmt.Fprintf(f.IOStreams.Out, "Already claimed %s (no-op)\n", key)
	} else {
		fmt.Fprintf(f.IOStreams.Out, "Claimed %s → %s\n", key, toStatus)
	}
	return nil
}
