package api

import (
	"context"
	"fmt"
	"net/http"
)

// ListFields returns all field definitions.
// GET /field — returns a plain JSON array.
func (c *Client) ListFields(ctx context.Context) ([]Field, error) {
	var result []Field
	if err := c.Do(ctx, http.MethodGet, "field", nil, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// ListIssueTypes returns all issue types globally.
// GET /issuetype — returns a plain JSON array.
func (c *Client) ListIssueTypes(ctx context.Context) ([]IssueType, error) {
	var result []IssueType
	if err := c.Do(ctx, http.MethodGet, "issuetype", nil, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// ListIssueTypesForProject returns issue types scoped to a project.
// GET /issuetype/project?projectId={id} — requires numeric project ID.
func (c *Client) ListIssueTypesForProject(ctx context.Context, projectID string) ([]IssueType, error) {
	path := fmt.Sprintf("issuetype/project?projectId=%s", projectID)

	var result []IssueType
	if err := c.Do(ctx, http.MethodGet, path, nil, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// ListStatuses returns all workflow statuses.
// GET /status — returns a plain JSON array.
func (c *Client) ListStatuses(ctx context.Context) ([]StatusDetail, error) {
	var result []StatusDetail
	if err := c.Do(ctx, http.MethodGet, "status", nil, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// ListPriorities returns all issue priorities.
// GET /priority — returns a plain JSON array.
func (c *Client) ListPriorities(ctx context.Context) ([]Priority, error) {
	var result []Priority
	if err := c.Do(ctx, http.MethodGet, "priority", nil, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// ListLabels returns labels with offset-based pagination.
// GET /label — returns PageBeanString with 'values' array.
func (c *Client) ListLabels(ctx context.Context, opts OffsetPaginationOptions) (*LabelPage, error) {
	path := "label"
	sep := "?"
	if opts.StartAt > 0 {
		path += fmt.Sprintf("%sstartAt=%d", sep, opts.StartAt)
		sep = "&"
	}
	if opts.MaxResults > 0 {
		path += fmt.Sprintf("%smaxResults=%d", sep, opts.MaxResults)
	}

	var result LabelPage
	if err := c.Do(ctx, http.MethodGet, path, nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
