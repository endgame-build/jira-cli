package api

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"

	cliErrors "github.com/endgame-build/jira-cli/internal/errors"
)

// isTimeout returns true if err (or any wrapped error) is a timeout.
func isTimeout(err error) bool {
	var netErr net.Error
	if errors.As(err, &netErr) {
		return netErr.Timeout()
	}
	return false
}

// mapNetworkError converts a Go network error into a CLIError(NETWORK_ERROR).
// It inspects wrapped errors for DNS, connection refused, TLS, and timeout
// failures to provide actionable suggestions.
func mapNetworkError(err error, instance string) *cliErrors.CLIError {
	message, suggestion := classifyNetworkError(err)
	return cliErrors.NewNetworkError(message, instance).
		WithSuggestion(suggestion).
		WithErr(err)
}

// classifyNetworkError returns a human-readable message and suggestion for
// common network failure modes.
func classifyNetworkError(err error) (message, suggestion string) {
	// DNS resolution failure.
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return fmt.Sprintf("DNS lookup failed for %s", dnsErr.Name),
			"Check the instance hostname in your configuration"
	}

	// Connection refused.
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		var sysErr *os.SyscallError
		if errors.As(opErr.Err, &sysErr) {
			if strings.Contains(sysErr.Err.Error(), "connection refused") {
				return "Connection refused",
					"Check that the Jira instance is reachable"
			}
		}
	}

	// TLS errors.
	var tlsRecordErr *tls.RecordHeaderError
	if errors.As(err, &tlsRecordErr) {
		return "TLS handshake failed",
			"Check that the Jira instance supports HTTPS"
	}
	// Also catch tls.CertificateVerificationError (Go 1.20+).
	var certErr *tls.CertificateVerificationError
	if errors.As(err, &certErr) {
		return "TLS certificate verification failed",
			"The server's TLS certificate could not be verified"
	}

	// Timeout.
	if isTimeout(err) {
		return "Request timed out",
			"The server did not respond in time; try again later"
	}

	// URL parse errors.
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return fmt.Sprintf("Request failed: %s", urlErr.Err.Error()),
			"Check your network connection and try again"
	}

	// Generic fallback.
	return fmt.Sprintf("Network error: %s", err.Error()),
		"Check your network connection and try again"
}
