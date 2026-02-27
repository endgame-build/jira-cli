package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	cliErrors "github.com/endgame-build/jira-cli/internal/errors"
)

// ErrorCollection mirrors the Jira REST API error response shape.
// Jira returns {"errorMessages": [...], "errors": {"field": "msg", ...}}.
type ErrorCollection struct {
	ErrorMessages []string          `json:"errorMessages"`
	Errors        map[string]string `json:"errors"`
}

// Summary returns a single-string summary of all errors.
func (ec *ErrorCollection) Summary() string {
	var parts []string
	parts = append(parts, ec.ErrorMessages...)
	for field, msg := range ec.Errors {
		parts = append(parts, fmt.Sprintf("%s: %s", field, msg))
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "; ")
}

// parseErrorCollection attempts to parse a Jira ErrorCollection from an HTTP
// response body. Returns nil if the body is empty or not valid JSON.
func parseErrorCollection(body []byte) *ErrorCollection {
	if len(body) == 0 {
		return nil
	}
	var ec ErrorCollection
	if err := json.Unmarshal(body, &ec); err != nil {
		return nil
	}
	if len(ec.ErrorMessages) == 0 && len(ec.Errors) == 0 {
		return nil
	}
	return &ec
}

// mapHTTPError converts an HTTP error response to a CLIError.
// It reads the response body, attempts to parse a Jira ErrorCollection,
// and maps the status code to the appropriate CLIError code.
func mapHTTPError(resp *http.Response, instance string) *cliErrors.CLIError {
	body, _ := io.ReadAll(resp.Body)
	ec := parseErrorCollection(body)

	var message string
	if ec != nil {
		message = ec.Summary()
	}
	if message == "" {
		message = fmt.Sprintf("HTTP %d: %s", resp.StatusCode, http.StatusText(resp.StatusCode))
	}

	switch resp.StatusCode {
	case http.StatusBadRequest: // 400
		return cliErrors.NewValidationError(message).
			WithSuggestion("Check your input values and try again")

	case http.StatusUnauthorized: // 401
		return cliErrors.NewAuthError(message).
			WithSuggestion("Run 'jira auth login' to refresh your credentials")

	case http.StatusForbidden: // 403
		return cliErrors.NewPermissionError(message).
			WithSuggestion("You may not have permission to perform this action")

	case http.StatusNotFound: // 404
		return cliErrors.NewNotFoundError(message, "")

	case http.StatusConflict: // 409
		return cliErrors.NewConflictError(message)

	case http.StatusRequestEntityTooLarge: // 413
		return cliErrors.NewValidationError(message).
			WithSuggestion("The request payload is too large. For comments, check comment size limits for your Jira instance")

	case http.StatusTooManyRequests: // 429
		return cliErrors.NewRateLimitError(message).
			WithSuggestion("Wait a moment and try again")

	default:
		if resp.StatusCode >= 500 {
			return cliErrors.NewGeneralError(message).
				WithContext(map[string]interface{}{"status": resp.StatusCode, "instance": instance})
		}
		return cliErrors.NewGeneralError(message).
			WithContext(map[string]interface{}{"status": resp.StatusCode})
	}
}
