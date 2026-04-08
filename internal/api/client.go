package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/endgame-build/jira-cli/internal/auth"
)

const defaultTimeout = 30 * time.Second

// Client is the Jira Cloud REST API v3 HTTP client.
type Client struct {
	httpClient   *http.Client
	baseURL      string // e.g. "https://mysite.atlassian.net/rest/api/3"
	agileBaseURL string // e.g. "https://mysite.atlassian.net/rest/agile/1.0"
	instance     string // raw instance hostname for browse URL construction
}

// ClientOption configures optional Client behaviour.
type ClientOption func(*Client)

// WithTimeout overrides the default 30-second request timeout.
func WithTimeout(d time.Duration) ClientOption {
	return func(c *Client) {
		c.httpClient.Timeout = d
	}
}

// withBaseURL overrides the base URL (used by tests within api package).
func withBaseURL(url string) ClientOption {
	return func(c *Client) {
		c.baseURL = url
	}
}

// WithBaseURL overrides the base URL. Exported for cross-package test helpers.
func WithBaseURL(url string) ClientOption {
	return withBaseURL(url)
}

// WithAgileBaseURL overrides the Agile API base URL (used by tests).
func WithAgileBaseURL(url string) ClientOption {
	return func(c *Client) {
		c.agileBaseURL = url
	}
}

// NewClient creates a Client authenticated with the given credentials.
// The transport chain is: retryablehttp → authTransport → http.DefaultTransport.
func NewClient(creds *auth.Credentials, opts ...ClientOption) *Client {
	authT := &authTransport{
		base:  http.DefaultTransport,
		user:  creds.User,
		token: creds.Token,
	}

	c := &Client{
		httpClient: &http.Client{
			Transport: newRetryableTransport(authT),
			Timeout:   defaultTimeout,
		},
		baseURL:      fmt.Sprintf("https://%s/rest/api/3", creds.Instance),
		agileBaseURL: fmt.Sprintf("https://%s/rest/agile/1.0", creds.Instance),
		instance:     creds.Instance,
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// Instance returns the Jira instance hostname (e.g. "mysite.atlassian.net").
func (c *Client) Instance() string {
	return c.instance
}

// BrowseURL constructs a browser-navigable URL for a Jira issue key.
func (c *Client) BrowseURL(issueKey string) string {
	return fmt.Sprintf("https://%s/browse/%s", c.instance, issueKey)
}

// Do sends an HTTP request to the Jira REST API v3 and decodes the JSON response.
//
// method: HTTP verb (GET, POST, PUT, DELETE).
// path:   API path appended to base URL (e.g. "issue/PROJ-123").
// body:   request body to marshal as JSON (nil for no body).
// out:    pointer to decode response JSON into (nil to skip decode; required for 204).
//
// Successful status codes: 200, 201, 204.
// On 204 (No Content), body decode is skipped regardless of out.
func (c *Client) Do(ctx context.Context, method, path string, body interface{}, out interface{}) error {
	return c.doRequest(ctx, method, c.baseURL+"/"+path, body, out)
}

// DoAgile sends an HTTP request to the Jira Agile REST API and decodes the JSON response.
// Same contract as Do, but uses the /rest/agile/1.0 base URL.
func (c *Client) DoAgile(ctx context.Context, method, path string, body interface{}, out interface{}) error {
	return c.doRequest(ctx, method, c.agileBaseURL+"/"+path, body, out)
}

// doRequest is the shared HTTP dispatch used by Do and DoAgile.
func (c *Client) doRequest(ctx context.Context, method, fullURL string, body interface{}, out interface{}) error {
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request body: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, fullURL, reqBody)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return mapNetworkError(err, c.instance)
	}
	defer resp.Body.Close()

	// Accept 200, 201, 204 as success.
	if resp.StatusCode == http.StatusOK ||
		resp.StatusCode == http.StatusCreated ||
		resp.StatusCode == http.StatusNoContent {

		// 204 has no body — skip decode.
		if resp.StatusCode == http.StatusNoContent {
			return nil
		}

		if out != nil {
			if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
				return fmt.Errorf("decode response: %w", err)
			}
		}
		return nil
	}

	// Non-success: parse Jira ErrorCollection and map to CLIError.
	return mapHTTPError(resp, c.instance)
}
