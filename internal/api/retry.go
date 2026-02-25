package api

import (
	"context"
	"net/http"
	"time"

	"github.com/hashicorp/go-retryablehttp"
)

const (
	maxRetries     = 3
	retryWaitMin   = 1 * time.Second
	retryWaitMax   = 30 * time.Second
	defaultBackoff = 2 * time.Second
)

// newRetryableTransport creates an http.RoundTripper that wraps base with
// automatic retry logic: retries on 429 (respecting Retry-After) and 5xx,
// with exponential backoff. Never retries 401 or other 4xx errors.
func newRetryableTransport(base http.RoundTripper) http.RoundTripper {
	rc := retryablehttp.NewClient()
	rc.RetryMax = maxRetries
	rc.RetryWaitMin = retryWaitMin
	rc.RetryWaitMax = retryWaitMax
	rc.Logger = nil // Suppress default logger output.
	rc.CheckRetry = retryPolicy
	rc.ErrorHandler = retryablehttp.PassthroughErrorHandler

	// Wrap base transport into the retryable client.
	rc.HTTPClient.Transport = base

	return &retryablehttp.RoundTripper{Client: rc}
}

// retryPolicy determines whether a request should be retried.
// Retry on: 429 (rate limited) and 5xx (server errors).
// Never retry: 401 (auth), timeouts, or other 4xx.
func retryPolicy(ctx context.Context, resp *http.Response, err error) (bool, error) {
	// Network error: don't retry timeouts, do retry transient network issues.
	if err != nil {
		if isTimeout(err) {
			return false, err
		}
		// For other network errors (connection refused, DNS, etc.),
		// let retryablehttp's default policy handle it.
		return retryablehttp.DefaultRetryPolicy(ctx, resp, err)
	}

	// 429 — always retry (Retry-After is respected by retryablehttp automatically).
	if resp.StatusCode == http.StatusTooManyRequests {
		return true, nil
	}

	// 5xx — retry with backoff.
	if resp.StatusCode >= 500 {
		return true, nil
	}

	// All other statuses (including 401, 403, 404, etc.) — do not retry.
	return false, nil
}
