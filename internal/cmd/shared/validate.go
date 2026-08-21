package shared

import (
	"regexp"
	"strings"

	"github.com/endgame-build/jira-cli/internal/errors"
)

// issueKeyRe matches Jira issue keys: a letter, then letters or digits, then a
// hyphen and 1+ digits. Jira permits digits in a project key after the first
// character, so ABC123-45 is legal and must be accepted.
// Lowercase letters are accepted and auto-uppercased by ValidateIssueKeyOrID.
var issueKeyRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]*-[0-9]+$`)

// numericIDRe matches plain numeric Jira issue IDs (e.g., "10001").
var numericIDRe = regexp.MustCompile(`^[0-9]+$`)

// ValidateCommentID validates the input as a numeric Jira comment ID.
// Returns the input unchanged or a CLIError(VALIDATION_ERROR).
func ValidateCommentID(input string) (string, error) {
	if input == "" || !numericIDRe.MatchString(input) {
		return "", errors.NewValidationError("Invalid comment ID").
			WithContext(map[string]interface{}{"input": input}).
			WithSuggestion("Comment IDs are numeric (e.g., 10042)")
	}
	return input, nil
}

// projectKeyRe matches Jira project keys: a letter followed by 1+ letters or
// digits. An all-digit input is treated as a numeric ID before this is reached.
var projectKeyRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]+$`)

// ValidateProjectKeyOrID validates the input as either a Jira project key (PROJ)
// or a numeric project ID (10001). Keys are auto-uppercased. Returns the normalized
// value or a CLIError(VALIDATION_ERROR) if the input matches neither pattern.
func ValidateProjectKeyOrID(input string) (string, error) {
	if numericIDRe.MatchString(input) {
		return input, nil
	}
	if projectKeyRe.MatchString(input) {
		return strings.ToUpper(input), nil
	}
	return "", errors.NewValidationError("Invalid project key or ID").
		WithContext(map[string]interface{}{"input": input}).
		WithSuggestion("Use a key like PROJ or a numeric ID like 10001")
}

// ValidateIssueKeyOrID validates the input as either a Jira issue key (PROJ-123)
// or a numeric issue ID (10001). Keys are auto-uppercased. Returns the normalized
// value or a CLIError(VALIDATION_ERROR) if the input matches neither pattern.
func ValidateIssueKeyOrID(input string) (string, error) {
	if numericIDRe.MatchString(input) {
		return input, nil
	}
	if issueKeyRe.MatchString(input) {
		return strings.ToUpper(input), nil
	}
	return "", errors.NewValidationError("Invalid issue key or ID").
		WithContext(map[string]interface{}{"input": input}).
		WithSuggestion("Use a key like PROJ-123 or a numeric ID like 10001")
}
