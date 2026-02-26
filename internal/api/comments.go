package api

import (
	"context"
	"fmt"
	"net/http"
)

// ListComments fetches comments on an issue, ordered by most recent first.
// Uses offset-based pagination via startAt and maxResults.
func (c *Client) ListComments(ctx context.Context, issueKey string, opts OffsetPaginationOptions) (*CommentPage, error) {
	path := fmt.Sprintf("issue/%s/comment?orderBy=-created", issueKey)
	if opts.StartAt > 0 {
		path += fmt.Sprintf("&startAt=%d", opts.StartAt)
	}
	if opts.MaxResults > 0 {
		path += fmt.Sprintf("&maxResults=%d", opts.MaxResults)
	}

	var page CommentPage
	if err := c.Do(ctx, http.MethodGet, path, nil, &page); err != nil {
		return nil, err
	}
	return &page, nil
}

// GetComment fetches a single comment by ID on the given issue.
func (c *Client) GetComment(ctx context.Context, issueKey, commentID string) (*Comment, error) {
	path := fmt.Sprintf("issue/%s/comment/%s", issueKey, commentID)

	var comment Comment
	if err := c.Do(ctx, http.MethodGet, path, nil, &comment); err != nil {
		return nil, err
	}
	return &comment, nil
}

// AddComment posts a new comment on an issue. Expects HTTP 201.
// body should be an *adf.Node (the ADF document from adf.Convert).
func (c *Client) AddComment(ctx context.Context, issueKey string, body interface{}) (*Comment, error) {
	path := fmt.Sprintf("issue/%s/comment", issueKey)
	payload := map[string]interface{}{
		"body": body,
	}

	var comment Comment
	if err := c.Do(ctx, http.MethodPost, path, payload, &comment); err != nil {
		return nil, err
	}
	return &comment, nil
}

// UpdateComment updates an existing comment. Expects HTTP 200.
// body should be an *adf.Node (the ADF document from adf.Convert).
func (c *Client) UpdateComment(ctx context.Context, issueKey, commentID string, body interface{}) (*Comment, error) {
	path := fmt.Sprintf("issue/%s/comment/%s", issueKey, commentID)
	payload := map[string]interface{}{
		"body": body,
	}

	var comment Comment
	if err := c.Do(ctx, http.MethodPut, path, payload, &comment); err != nil {
		return nil, err
	}
	return &comment, nil
}

// DeleteComment removes a comment from an issue. Expects HTTP 204.
func (c *Client) DeleteComment(ctx context.Context, issueKey, commentID string) error {
	path := fmt.Sprintf("issue/%s/comment/%s", issueKey, commentID)
	return c.Do(ctx, http.MethodDelete, path, nil, nil)
}
