package issue

import "github.com/endgameio/jira-cli/internal/cmd/shared"

// ValidateIssueKeyOrID delegates to shared.ValidateIssueKeyOrID.
// Kept for backward compatibility within the issue package.
func ValidateIssueKeyOrID(input string) (string, error) {
	return shared.ValidateIssueKeyOrID(input)
}
