// Package shared provides common utilities used across multiple command packages.
package shared

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/endgameio/jira-cli/internal/api"
	clierrors "github.com/endgameio/jira-cli/internal/errors"
)

// ColorHelper is the interface for colorizing output. Satisfied by iostreams.IOStreams.
type ColorHelper interface {
	Green(string) string
	Yellow(string) string
	Cyan(string) string
}

// FieldSet converts a string slice of field names to a set for O(1) lookup.
// Returns nil if no fields specified (meaning show all).
func FieldSet(fields []string) map[string]bool {
	if len(fields) == 0 {
		return nil
	}
	set := make(map[string]bool, len(fields))
	for _, f := range fields {
		key := strings.ToLower(strings.TrimSpace(f))
		if key == "" {
			continue
		}
		set[key] = true
	}
	return set
}

// ShowField returns true if the field should be displayed.
// If wantFields is nil (no filter), all fields are shown.
// The name is normalized to lowercase to match FieldSet's behavior.
func ShowField(wantFields map[string]bool, name string) bool {
	if wantFields == nil {
		return true
	}
	return wantFields[strings.ToLower(name)]
}

// CheckEmptyResultsAuth probes credentials when a search returns zero results.
// Jira returns HTTP 200 with empty results for unauthenticated search requests
// instead of 401. This function calls GET /myself to detect that case.
// Returns an auth error if credentials are invalid, or nil otherwise.
// Transient probe failures (5xx, rate limit, etc.) are logged to stderr but
// not propagated, to avoid masking legitimate empty result sets.
func CheckEmptyResultsAuth(ctx context.Context, client *api.Client, stderr io.Writer) error {
	authErr := client.VerifyCredentials(ctx)
	if authErr == nil {
		return nil
	}
	var cliErr *clierrors.CLIError
	if errors.As(authErr, &cliErr) && cliErr.Code == clierrors.AUTH_ERROR {
		return authErr
	}
	// Non-auth probe failure — warn but don't mask a legitimate empty result set.
	fmt.Fprintf(stderr, "Warning: credential check failed (%v); results may be incomplete\n", authErr)
	return nil
}

// FilterIssueFields returns a new slice where each issue's fields map contains
// only the keys in wantFields. Key, ID, and Self are always preserved.
// If wantFields is nil, the original issues are returned unmodified.
func FilterIssueFields(issues []api.Issue, wantFields map[string]bool) interface{} {
	if wantFields == nil {
		return issues
	}

	result := make([]map[string]interface{}, len(issues))
	for i, issue := range issues {
		filtered := map[string]interface{}{
			"id":   issue.ID,
			"key":  issue.Key,
			"self": issue.Self,
		}
		fields := make(map[string]interface{})
		if wantFields["summary"] {
			fields["summary"] = issue.Fields.Summary
		}
		if wantFields["status"] {
			fields["status"] = issue.Fields.Status
		}
		if wantFields["issuetype"] || wantFields["type"] {
			fields["issuetype"] = issue.Fields.IssueType
		}
		if wantFields["priority"] {
			fields["priority"] = issue.Fields.Priority
		}
		if wantFields["assignee"] {
			fields["assignee"] = issue.Fields.Assignee
		}
		if wantFields["reporter"] {
			fields["reporter"] = issue.Fields.Reporter
		}
		if wantFields["project"] {
			fields["project"] = issue.Fields.Project
		}
		if wantFields["labels"] {
			fields["labels"] = issue.Fields.Labels
		}
		if wantFields["created"] {
			fields["created"] = issue.Fields.Created
		}
		if wantFields["updated"] {
			fields["updated"] = issue.Fields.Updated
		}
		if wantFields["description"] {
			fields["description"] = issue.Fields.Description
		}
		if wantFields["resolution"] {
			fields["resolution"] = issue.Fields.Resolution
		}
		if wantFields["parent"] {
			fields["parent"] = issue.Fields.Parent
		}
		if wantFields["subtasks"] {
			fields["subtasks"] = issue.Fields.SubTasks
		}
		if wantFields["issuelinks"] {
			fields["issuelinks"] = issue.Fields.IssueLinks
		}
		if wantFields["comment"] {
			fields["comment"] = issue.Fields.Comment
		}
		filtered["fields"] = fields
		result[i] = filtered
	}
	return result
}

// StatusWithColor colorizes a status name based on its category.
func StatusWithColor(c ColorHelper, status *api.Status) string {
	if status == nil {
		return "Unknown"
	}
	name := status.Name
	if status.StatusCategory == nil {
		return name
	}
	switch status.StatusCategory.Key {
	case "done":
		return c.Green(name)
	case "indeterminate":
		return c.Cyan(name)
	case "new":
		return c.Yellow(name)
	default:
		return name
	}
}
