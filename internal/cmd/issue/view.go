package issue

import (
	"context"
	"fmt"
	"strings"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"

	"github.com/endgameio/jira-cli/internal/adf"
	"github.com/endgameio/jira-cli/internal/api"
	"github.com/endgameio/jira-cli/internal/factory"
	"github.com/endgameio/jira-cli/internal/output"
)

// ViewOptions holds all resolved inputs for the issue view command.
type ViewOptions struct {
	Factory *factory.Factory
	KeyOrID string
	Fields  []string // --fields filter
	NoPager bool     // --no-pager
}

// NewCmdView creates the "issue view" command.
func NewCmdView(f *factory.Factory) *cobra.Command {
	opts := &ViewOptions{Factory: f}

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

	return cmd
}

// runView fetches an issue and renders it.
func runView(opts *ViewOptions) error {
	f := opts.Factory
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
	wantFields := fieldSet(opts.Fields)

	ios.StartPager()
	defer ios.StopPager()

	return formatter.OutputData(issue, func(t table.Writer) {
		if showField(wantFields, "key") {
			fmt.Fprintf(ios.Out, "Key:       %s\n", issue.Key)
		}
		if showField(wantFields, "summary") {
			fmt.Fprintf(ios.Out, "Summary:   %s\n", fields.Summary)
		}
		if showField(wantFields, "status") {
			statusText := statusWithColor(ios, fields.Status)
			fmt.Fprintf(ios.Out, "Status:    %s\n", statusText)
		}
		if showField(wantFields, "type") && fields.IssueType != nil {
			fmt.Fprintf(ios.Out, "Type:      %s\n", fields.IssueType.Name)
		}
		if showField(wantFields, "priority") && fields.Priority != nil {
			fmt.Fprintf(ios.Out, "Priority:  %s\n", fields.Priority.Name)
		}
		if showField(wantFields, "assignee") {
			assignee := "Unassigned"
			if fields.Assignee != nil {
				assignee = fields.Assignee.DisplayName
			}
			fmt.Fprintf(ios.Out, "Assignee:  %s\n", assignee)
		}
		if showField(wantFields, "reporter") && fields.Reporter != nil {
			fmt.Fprintf(ios.Out, "Reporter:  %s\n", fields.Reporter.DisplayName)
		}
		if showField(wantFields, "labels") && len(fields.Labels) > 0 {
			fmt.Fprintf(ios.Out, "Labels:    %s\n", strings.Join(fields.Labels, ", "))
		}
		if showField(wantFields, "created") && fields.Created != "" {
			fmt.Fprintf(ios.Out, "Created:   %s\n", fields.Created)
		}
		if showField(wantFields, "updated") && fields.Updated != "" {
			fmt.Fprintf(ios.Out, "Updated:   %s\n", fields.Updated)
		}
		if showField(wantFields, "description") {
			desc := adf.ExtractText(fields.Description)
			if desc != "" {
				lines := strings.Split(desc, "\n")
				if len(lines) > 5 {
					lines = append(lines[:5], "... (truncated)")
				}
				fmt.Fprintf(ios.Out, "\n%s\n", strings.Join(lines, "\n"))
			}
		}
	})
}

// fieldSet converts a string slice of field names to a set for O(1) lookup.
// Returns nil if no fields specified (meaning show all).
func fieldSet(fields []string) map[string]bool {
	if len(fields) == 0 {
		return nil
	}
	set := make(map[string]bool, len(fields))
	for _, f := range fields {
		set[strings.ToLower(strings.TrimSpace(f))] = true
	}
	return set
}

// showField returns true if the field should be displayed.
// If wantFields is nil (no filter), all fields are shown.
func showField(wantFields map[string]bool, name string) bool {
	if wantFields == nil {
		return true
	}
	return wantFields[name]
}

// statusWithColor colorizes a status name based on its category.
func statusWithColor(ios interface {
	Green(string) string
	Yellow(string) string
	Cyan(string) string
}, status *api.Status) string {
	if status == nil {
		return "Unknown"
	}
	name := status.Name
	if status.StatusCategory == nil {
		return name
	}
	switch status.StatusCategory.Key {
	case "done":
		return ios.Green(name)
	case "indeterminate":
		return ios.Cyan(name)
	case "new":
		return ios.Yellow(name)
	default:
		return name
	}
}
