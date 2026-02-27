package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	cliErrors "github.com/endgame-build/jira-cli/internal/errors"
)

// ListProjects searches for projects using offset-based pagination.
// GET /project/search — returns PageBeanProject with 'values' array.
func (c *Client) ListProjects(ctx context.Context, opts OffsetPaginationOptions) (*ProjectSearchResult, error) {
	path := "project/search"
	sep := "?"
	if opts.StartAt > 0 {
		path += fmt.Sprintf("%sstartAt=%d", sep, opts.StartAt)
		sep = "&"
	}
	if opts.MaxResults > 0 {
		path += fmt.Sprintf("%smaxResults=%d", sep, opts.MaxResults)
	}

	var result ProjectSearchResult
	if err := c.Do(ctx, http.MethodGet, path, nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetProject fetches a single project by key or numeric ID.
// GET /project/{keyOrId}
// On 404, wraps the error with the project key in context.
func (c *Client) GetProject(ctx context.Context, keyOrID string) (*ProjectDetail, error) {
	path := fmt.Sprintf("project/%s", keyOrID)

	var project ProjectDetail
	if err := c.Do(ctx, http.MethodGet, path, nil, &project); err != nil {
		var cliErr *cliErrors.CLIError
		if errors.As(err, &cliErr) && cliErr.Code == cliErrors.NOT_FOUND {
			return nil, cliErrors.NewNotFoundError(cliErr.Message, keyOrID).
				WithSuggestion("Check the project key or ID and try again")
		}
		return nil, err
	}
	return &project, nil
}
