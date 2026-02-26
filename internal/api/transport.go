// Package api implements the Jira Cloud REST API v3 client.
package api

import (
	"encoding/base64"
	"net/http"
)

// authTransport injects Basic auth and Content-Type headers into every request.
type authTransport struct {
	base  http.RoundTripper
	user  string
	token string
}

func (t *authTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Clone the request to avoid mutating the caller's original.
	r := req.Clone(req.Context())

	cred := base64.StdEncoding.EncodeToString([]byte(t.user + ":" + t.token))
	r.Header.Set("Authorization", "Basic "+cred)
	if r.Body != nil && r.Body != http.NoBody {
		r.Header.Set("Content-Type", "application/json")
	}
	r.Header.Set("Accept", "application/json")

	return t.base.RoundTrip(r)
}
