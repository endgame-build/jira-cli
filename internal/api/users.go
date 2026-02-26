package api

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"

	cliErrors "github.com/endgameio/jira-cli/internal/errors"
)

// GetMyself returns the currently authenticated user via GET /myself.
func (c *Client) GetMyself(ctx context.Context) (*User, error) {
	var user User
	if err := c.Do(ctx, http.MethodGet, "myself", nil, &user); err != nil {
		return nil, err
	}
	return &user, nil
}

// VerifyCredentials checks that the current credentials are valid by calling
// GET /myself. Returns nil if credentials are valid. On failure, returns the
// underlying error from GetMyself — callers must check the error code to
// distinguish AUTH_ERROR from transient failures (network, rate limit, 5xx).
// This is useful because some Jira endpoints (e.g. POST /search/jql) return
// HTTP 200 with empty results for unauthenticated requests instead of 401.
func (c *Client) VerifyCredentials(ctx context.Context) error {
	_, err := c.GetMyself(ctx)
	return err
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

// accountIDPattern matches Jira Cloud account IDs (empirical: 24 hex chars).
// NOTE: Swagger allows maxLength:128 with no pattern constraint — this heuristic
// works today but is not spec-guaranteed.
var accountIDPattern = regexp.MustCompile(`^[0-9a-f]{24}$`)

// ResolveUser resolves a user input string to a Jira account ID.
//
// Resolution rules:
//   - "@me" → calls GET /myself and returns the current user's account ID
//   - Matches ^[0-9a-f]{24}$ → treated as an account ID and returned as-is (no API call)
//   - Anything else → calls GET /user/search?query={input}:
//     exactly 1 result → returns that user's account ID
//     0 results → CLIError(NOT_FOUND)
//     2+ results → CLIError(AMBIGUOUS_USER) with match details
func ResolveUser(ctx context.Context, client *Client, input string) (string, error) {
	// @me: resolve via /myself
	if input == "@me" {
		user, err := client.GetMyself(ctx)
		if err != nil {
			return "", err
		}
		return user.AccountID, nil
	}

	// Direct account ID passthrough
	if accountIDPattern.MatchString(input) {
		return input, nil
	}

	// Search by name/email
	users, err := client.SearchUsers(ctx, input, 0, 50)
	if err != nil {
		return "", err
	}

	switch len(users) {
	case 0:
		return "", cliErrors.NewNotFoundError(
			fmt.Sprintf("No user found matching %q", input), "",
		).WithContext(map[string]interface{}{
			"resource": "user",
			"query":    input,
		}).WithSuggestion("Check the spelling or use the full account ID")
	case 1:
		return users[0].AccountID, nil
	default:
		matches := make([]map[string]interface{}, len(users))
		for i, u := range users {
			m := map[string]interface{}{
				"accountId":   u.AccountID,
				"displayName": u.DisplayName,
			}
			if u.EmailAddress != nil {
				m["email"] = *u.EmailAddress
			}
			matches[i] = m
		}
		return "", cliErrors.NewAmbiguousUserError(
			fmt.Sprintf("Multiple users match %q", input), matches,
		).WithSuggestion("Use the account ID directly or refine the search term")
	}
}
