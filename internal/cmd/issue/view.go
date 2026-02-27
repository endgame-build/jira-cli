package issue

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/cli/browser"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"

	"github.com/endgameio/jira-cli/internal/adf"
	"github.com/endgameio/jira-cli/internal/api"
	"github.com/endgameio/jira-cli/internal/cmd/shared"
	"github.com/endgameio/jira-cli/internal/factory"
	"github.com/endgameio/jira-cli/internal/output"
)

// ViewOptions holds all resolved inputs for the issue view command.
type ViewOptions struct {
	Factory *factory.Factory
	KeyOrID string
	Fields  []string // --fields filter
	NoPager bool     // --no-pager

	Comments bool // --comments
	Web      bool // --web

	// BrowserOpen is the function used to open a URL in the browser.
	// Defaults to browser.OpenURL; overridden in tests.
	BrowserOpen func(string) error
}

// NewCmdView creates the "issue view" command.
func NewCmdView(f *factory.Factory) *cobra.Command {
	opts := &ViewOptions{
		Factory:     f,
		BrowserOpen: browser.OpenURL,
	}

	cmd := &cobra.Command{
		Use:   "view <key-or-id>",
		Short: "View a Jira issue",
		Long:  "Display details of a Jira issue by key (e.g. PROJ-123) or numeric ID.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			key, err := ValidateIssueKeyOrID(args[0])
			if err != nil {
				return err
			}
			opts.KeyOrID = key

			if opts.NoPager {
				f.IOStreams.NoPager = true
			}

			return runView(opts)
		},
	}

	cmd.Flags().StringSliceVar(&opts.Fields, "fields", nil, "Comma-separated list of fields to display")
	cmd.Flags().BoolVar(&opts.NoPager, "no-pager", false, "Do not pipe output through a pager")
	cmd.Flags().BoolVar(&opts.Comments, "comments", false, "Show comments (last 20)")
	cmd.Flags().BoolVar(&opts.Web, "web", false, "Open issue in browser")

	return cmd
}

// runView fetches an issue and renders it.
func runView(opts *ViewOptions) error {
	f := opts.Factory

	// --web: open in browser. Uses auth credentials for instance URL (no extra API call).
	if opts.Web {
		return runViewWeb(opts)
	}

	ios := f.IOStreams

	client, err := f.APIClient()
	if err != nil {
		return err
	}

	issue, err := client.GetIssue(context.Background(), opts.KeyOrID, &api.GetIssueOptions{
		Expand: []string{"transitions"},
	})
	if err != nil {
		return err
	}

	formatter := output.NewFormatter(ios, f.OutputJSON, f.JQExpr)

	if f.Quiet {
		return nil
	}

	// JSON mode: output full issue as bare object.
	if formatter.IsJSON() {
		return formatter.RawJSON(issue)
	}

	// Text mode: render key-value table.
	fields := issue.Fields
	wantFields := shared.FieldSet(opts.Fields)

	ios.StartPager()
	defer ios.StopPager()

	return formatter.OutputData(issue, func(t table.Writer) {
		if shared.ShowField(wantFields, "key") {
			fmt.Fprintf(ios.Out, "Key:       %s\n", issue.Key)
		}
		if shared.ShowField(wantFields, "summary") {
			fmt.Fprintf(ios.Out, "Summary:   %s\n", fields.Summary)
		}
		if shared.ShowField(wantFields, "status") {
			statusText := shared.StatusWithColor(ios, fields.Status)
			fmt.Fprintf(ios.Out, "Status:    %s\n", statusText)
		}
		if (shared.ShowField(wantFields, "issuetype") || shared.ShowField(wantFields, "type")) && fields.IssueType != nil {
			fmt.Fprintf(ios.Out, "Type:      %s\n", fields.IssueType.Name)
		}
		if shared.ShowField(wantFields, "priority") && fields.Priority != nil {
			fmt.Fprintf(ios.Out, "Priority:  %s\n", fields.Priority.Name)
		}
		if shared.ShowField(wantFields, "assignee") {
			assignee := "Unassigned"
			if fields.Assignee != nil {
				assignee = fields.Assignee.DisplayName
			}
			fmt.Fprintf(ios.Out, "Assignee:  %s\n", assignee)
		}
		if shared.ShowField(wantFields, "reporter") && fields.Reporter != nil {
			fmt.Fprintf(ios.Out, "Reporter:  %s\n", fields.Reporter.DisplayName)
		}
		if shared.ShowField(wantFields, "labels") && len(fields.Labels) > 0 {
			fmt.Fprintf(ios.Out, "Labels:    %s\n", strings.Join(fields.Labels, ", "))
		}
		if shared.ShowField(wantFields, "created") && fields.Created != "" {
			fmt.Fprintf(ios.Out, "Created:   %s\n", fields.Created)
		}
		if shared.ShowField(wantFields, "updated") && fields.Updated != "" {
			fmt.Fprintf(ios.Out, "Updated:   %s\n", fields.Updated)
		}
		if shared.ShowField(wantFields, "description") {
			desc := adf.ToPlaintext(fields.Description)
			if desc != "" {
				lines := strings.Split(desc, "\n")
				if len(lines) > 5 {
					lines = append(lines[:5], "... (truncated)")
				}
				fmt.Fprintf(ios.Out, "\n%s\n", strings.Join(lines, "\n"))
			}
		}

		// Linked issues section.
		if shared.ShowField(wantFields, "links") && len(fields.IssueLinks) > 0 {
			fmt.Fprintf(ios.Out, "\nLinked Issues:\n")
			for _, link := range fields.IssueLinks {
				renderLink(ios.Out, link)
			}
		}

		// Subtasks section.
		if shared.ShowField(wantFields, "subtasks") && len(fields.SubTasks) > 0 {
			fmt.Fprintf(ios.Out, "\nSubtasks:\n")
			for _, sub := range fields.SubTasks {
				statusName := ""
				if sub.Fields.Status != nil {
					statusName = sub.Fields.Status.Name
				}
				fmt.Fprintf(ios.Out, "  %s  %s  [%s]\n", sub.Key, sub.Fields.Summary, statusName)
			}
		}

		// Comments section (only when --comments flag is set).
		if opts.Comments {
			renderComments(ios.Out, fields.Comment)
		}
	})
}

// runViewWeb handles the --web flag: opens the issue in a browser.
// Uses AuthCredentials for instance (flag > env > profile chain, no API call).
// --web + --json: prints {ok:true, url} AND opens browser (dual action).
// --web + --quiet: opens browser silently (no stdout).
// --web + --fields/--comments: web takes precedence, flags ignored.
func runViewWeb(opts *ViewOptions) error {
	f := opts.Factory

	creds, err := f.AuthCredentials()
	if err != nil {
		return err
	}

	browseURL := fmt.Sprintf("https://%s/browse/%s", creds.Instance, opts.KeyOrID)

	formatter := output.NewFormatter(f.IOStreams, f.OutputJSON, f.JQExpr)

	// --json: print {ok:true, url} AND open browser (dual action).
	if formatter.IsJSON() {
		extras := map[string]interface{}{
			"url": browseURL,
		}
		if err := formatter.OutputMutation(extras, nil); err != nil {
			return err
		}
	} else if !f.Quiet {
		// Text mode (not quiet): print the URL.
		fmt.Fprintf(f.IOStreams.Out, "Opening %s in browser...\n", browseURL)
	}

	return opts.BrowserOpen(browseURL)
}

// renderLink formats a single issue link as text output.
func renderLink(w io.Writer, link api.IssueLink) {
	if link.Type == nil {
		return
	}
	if link.OutwardIssue != nil {
		status := ""
		if link.OutwardIssue.Fields != nil && link.OutwardIssue.Fields.Status != nil {
			status = link.OutwardIssue.Fields.Status.Name
		}
		fmt.Fprintf(w, "  %s %s (%s)\n", link.Type.Outward, link.OutwardIssue.Key, status)
	}
	if link.InwardIssue != nil {
		status := ""
		if link.InwardIssue.Fields != nil && link.InwardIssue.Fields.Status != nil {
			status = link.InwardIssue.Fields.Status.Name
		}
		fmt.Fprintf(w, "  %s %s (%s)\n", link.Type.Inward, link.InwardIssue.Key, status)
	}
}

// renderComments outputs the last 20 comments. Body is truncated to 3 lines.
func renderComments(w io.Writer, commentPage *api.CommentPage) {
	if commentPage == nil || len(commentPage.Comments) == 0 {
		fmt.Fprintf(w, "\nComments: (none)\n")
		return
	}

	comments := commentPage.Comments
	// Show last 20 only (MVP limit).
	if len(comments) > 20 {
		comments = comments[len(comments)-20:]
	}

	fmt.Fprintf(w, "\nComments (%d):\n", len(comments))
	for _, c := range comments {
		author := "Unknown"
		if c.Author != nil {
			author = c.Author.DisplayName
		}
		body := adf.ToPlaintext(c.Body)
		lines := strings.Split(body, "\n")
		if len(lines) > 3 {
			lines = append(lines[:3], "... (truncated)")
		}
		fmt.Fprintf(w, "  %s — %s\n", author, c.Created)
		for _, line := range lines {
			fmt.Fprintf(w, "    %s\n", line)
		}
	}
}
