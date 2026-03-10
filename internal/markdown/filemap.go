package markdown

import (
	"path/filepath"
	"strings"

	"github.com/endgame-build/jira-cli/internal/api"
)

// issueProjectKey extracts the project key from an issue.
// Prefers issue.Fields.Project.Key; falls back to the issue key prefix.
func issueProjectKey(issue api.Issue) string {
	if issue.Fields.Project != nil {
		return issue.Fields.Project.Key
	}
	if idx := strings.LastIndex(issue.Key, "-"); idx > 0 {
		return issue.Key[:idx]
	}
	return ""
}

// keySummarySegment returns "KEY - SanitizedSummary". If summary sanitizes to
// empty, returns just the key.
func keySummarySegment(key, summary string) string {
	if name := SanitizeFilename(summary); name != "" {
		return key + " - " + name
	}
	return key
}

// IssuePath returns the relative file path for an issue markdown file.
// Format: {ProjectKey}/{IssueKey} - {SanitizedSummary}.md
// If Project is nil, the project key is extracted from the issue key prefix.
func IssuePath(issue api.Issue) string {
	projectKey := issueProjectKey(issue)
	return filepath.Join(projectKey, keySummarySegment(issue.Key, issue.Fields.Summary)+".md")
}

// IssueTreePath returns the relative file path for an issue in tree mode.
// Epics become directories containing _epic.md; children of any parent
// are placed inside the parent's directory. Orphans stay flat.
func IssueTreePath(issue api.Issue) string {
	projectKey := issueProjectKey(issue)

	isEpic := issue.Fields.IssueType != nil &&
		strings.EqualFold(issue.Fields.IssueType.Name, "epic")

	if isEpic {
		return filepath.Join(projectKey, keySummarySegment(issue.Key, issue.Fields.Summary), "_epic.md")
	}

	if issue.Fields.Parent != nil && issue.Fields.Parent.Key != "" {
		parentSummary := ""
		if issue.Fields.Parent.Fields != nil {
			parentSummary = issue.Fields.Parent.Fields.Summary
		}
		parentDir := keySummarySegment(issue.Fields.Parent.Key, parentSummary)
		filename := keySummarySegment(issue.Key, issue.Fields.Summary) + ".md"
		return filepath.Join(projectKey, parentDir, filename)
	}

	// Orphan — same as flat.
	return IssuePath(issue)
}

// filenameReplacer replaces characters that are invalid in file names with dashes.
var filenameReplacer = strings.NewReplacer(
	"/", "-",
	"\\", "-",
	":", "-",
	"*", "-",
	"?", "-",
	"\"", "-",
	"<", "-",
	">", "-",
	"|", "-",
)

// SanitizeFilename replaces characters that are invalid in file names,
// collapses multiple spaces, trims whitespace, and enforces a max length of 100 chars.
func SanitizeFilename(name string) string {
	name = filenameReplacer.Replace(name)

	// Collapse multiple spaces into one
	for strings.Contains(name, "  ") {
		name = strings.ReplaceAll(name, "  ", " ")
	}

	name = strings.TrimSpace(name)

	// Enforce max length (by rune to avoid splitting multi-byte UTF-8)
	runes := []rune(name)
	if len(runes) > 100 {
		name = string(runes[:100])
		name = strings.TrimSpace(name)
	}

	return name
}
