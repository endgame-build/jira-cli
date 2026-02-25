package api

import (
	"io"
	"net/http"
	"strings"
)

// newMockResponse creates an *http.Response with the given status and body string.
// Used by error and retry tests to test mapHTTPError and retryPolicy.
func newMockResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}
}
