package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/endgameio/jira-cli/internal/auth"
)

const defaultTimeout = 30 * time.Second

// Client is the Jira Cloud REST API v3 HTTP client.
type Client struct {
	httpClient *http.Client
	baseURL    string // e.g. "https://mysite.atlassian.net/rest/api/3"
	instance   string // raw instance hostname for browse URL construction
}

// ClientOption configures optional Client behaviour.
type ClientOption func(*Client)

// WithTimeout overrides the default 30-second request timeout.
func WithTimeout(d time.Duration) ClientOption {
	return func(c *Client) {
		c.httpClient.Timeout = d
	}
}

// withBaseURL overrides the base URL (used by tests with httptest.NewServer).
func withBaseURL(url string) ClientOption {
	return func(c *Client) {
		c.baseURL = url
	}
}

// NewClient creates a Client authenticated with the given credentials.
func NewClient(creds *auth.Credentials, opts ...ClientOption) *Client {
	transport := &authTransport{
		base:  http.DefaultTransport,
		user:  creds.User,
		token: creds.Token,
	}

	c := &Client{
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   defaultTimeout,
		},
		baseURL:  fmt.Sprintf("https://%s/rest/api/3", creds.Instance),
		instance: creds.Instance,
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

// Do sends an HTTP request to the Jira API and decodes the JSON response.
//
// method: HTTP verb (GET, POST, PUT, DELETE).
// path:   API path appended to base URL (e.g. "issue/PROJ-123").
// body:   request body to marshal as JSON (nil for no body).
// out:    pointer to decode response JSON into (nil to skip decode; required for 204).
//
// Successful status codes: 200, 201, 204.
// On 204 (No Content), body decode is skipped regardless of out.
func (c *Client) Do(ctx context.Context, method, path string, body interface{}, out interface{}) error {
	url := c.baseURL + "/" + path

	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request body: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
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

	// Non-success: read body for error details and return a generic error.
	// US-007b will add proper Jira ErrorCollection parsing and CLIError mapping.
	respBody, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(respBody))
}
