package api

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
)

// GetMyself returns the currently authenticated user via GET /myself.
func (c *Client) GetMyself(ctx context.Context) (*User, error) {
	var user User
	if err := c.Do(ctx, http.MethodGet, "myself", nil, &user); err != nil {
		return nil, err
	}
	return &user, nil
}

// SearchUsers finds users by display name or email via GET /user/search.
// The Jira API returns a plain []User array (not an envelope).
// startAt and maxResults control offset-based pagination.
func (c *Client) SearchUsers(ctx context.Context, query string, startAt, maxResults int) ([]User, error) {
	params := url.Values{}
	params.Set("query", query)

	if startAt > 0 {
		params.Set("startAt", fmt.Sprintf("%d", startAt))
	}
	if maxResults > 0 {
		params.Set("maxResults", fmt.Sprintf("%d", maxResults))
	}

	path := "user/search?" + params.Encode()

	var users []User
	if err := c.Do(ctx, http.MethodGet, path, nil, &users); err != nil {
		return nil, err
	}
	return users, nil
}
