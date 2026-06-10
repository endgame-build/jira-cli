package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

// GetIssue fetches a single issue by key or ID.
// opts controls which fields and expansions to request via query params.
func (c *Client) GetIssue(ctx context.Context, keyOrID string, opts *GetIssueOptions) (*Issue, error) {
	path := "issue/" + keyOrID

	var params []string
	if opts != nil {
		if len(opts.Fields) > 0 {
			params = append(params, "fields="+strings.Join(opts.Fields, ","))
		}
		if len(opts.Expand) > 0 {
			params = append(params, "expand="+strings.Join(opts.Expand, ","))
		}
	}
	if len(params) > 0 {
		path += "?" + strings.Join(params, "&")
	}

	var issue Issue
	if err := c.Do(ctx, http.MethodGet, path, nil, &issue); err != nil {
		return nil, withResourceContext(err, "Issue", keyOrID)
	}
	return &issue, nil
}

// CreateIssue creates a new issue and returns the created issue reference.
// Expects HTTP 201 from the API.
func (c *Client) CreateIssue(ctx context.Context, input *CreateIssueInput) (*CreatedIssue, error) {
	var created CreatedIssue
	if err := c.Do(ctx, http.MethodPost, "issue", input, &created); err != nil {
		return nil, err
	}
	return &created, nil
}

// EditIssue updates an existing issue. Expects HTTP 204 (no response body).
func (c *Client) EditIssue(ctx context.Context, keyOrID string, input *EditIssueInput) error {
	return c.Do(ctx, http.MethodPut, "issue/"+keyOrID, input, nil)
}

// DeleteIssue deletes an issue by key or ID.
// The deleteSubtasks parameter controls whether subtasks should also be deleted (true to delete).
func (c *Client) DeleteIssue(ctx context.Context, keyOrID string, deleteSubtasks bool) error {
	subtasks := "false"
	if deleteSubtasks {
		subtasks = "true"
	}
	path := fmt.Sprintf("issue/%s?deleteSubtasks=%s", keyOrID, subtasks)
	return c.Do(ctx, http.MethodDelete, path, nil, nil)
}

// AssignIssue sets the assignee of an issue.
// Pass a non-nil accountID to assign, or nil to unassign.
func (c *Client) AssignIssue(ctx context.Context, keyOrID string, accountID *string) error {
	body := map[string]interface{}{
		"accountId": accountID,
	}
	return c.Do(ctx, http.MethodPut, "issue/"+keyOrID+"/assignee", body, nil)
}

// GetTransitions returns the available workflow transitions for an issue.
func (c *Client) GetTransitions(ctx context.Context, keyOrID string) ([]Transition, error) {
	var resp struct {
		Transitions []Transition `json:"transitions"`
	}
	if err := c.Do(ctx, http.MethodGet, "issue/"+keyOrID+"/transitions", nil, &resp); err != nil {
		return nil, err
	}
	return resp.Transitions, nil
}

// DoTransition performs a workflow transition on an issue.
func (c *Client) DoTransition(ctx context.Context, keyOrID string, input *DoTransitionInput) error {
	return c.Do(ctx, http.MethodPost, "issue/"+keyOrID+"/transitions", input, nil)
}

// GetCreateMeta returns the available issue types for creating issues in a project.
func (c *Client) GetCreateMeta(ctx context.Context, projectKeyOrID string) (*CreateMetaIssueTypes, error) {
	var meta CreateMetaIssueTypes
	path := "issue/createmeta/" + projectKeyOrID + "/issuetypes"
	if err := c.Do(ctx, http.MethodGet, path, nil, &meta); err != nil {
		return nil, err
	}
	return &meta, nil
}

// CreateIssueLink creates a link between two issues.
// POST /issueLink — expects HTTP 201.
func (c *Client) CreateIssueLink(ctx context.Context, input *CreateIssueLinkInput) error {
	return c.Do(ctx, http.MethodPost, "issueLink", input, nil)
}

// SearchIssues executes a JQL search via POST /search/jql.
// Always sends an explicit "fields" parameter — Jira defaults to returning
// only the "id" field if fields is not specified.
func (c *Client) SearchIssues(ctx context.Context, opts *SearchOptions) (*SearchResults, error) {
	body := map[string]interface{}{
		"jql": opts.JQL,
	}

	if len(opts.Fields) > 0 {
		body["fields"] = opts.Fields
	} else {
		body["fields"] = []string{"*all"}
	}

	if opts.MaxResults > 0 {
		body["maxResults"] = opts.MaxResults
	}
	if opts.NextPageToken != "" {
		body["nextPageToken"] = opts.NextPageToken
	}

	var results SearchResults
	if err := c.Do(ctx, http.MethodPost, "search/jql", body, &results); err != nil {
		return nil, err
	}
	return &results, nil
}
