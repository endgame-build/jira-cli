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

// IssuePath returns the relative file path for an issue markdown file.
// Format: {ProjectKey}/{IssueKey} - {SanitizedSummary}.md
// If Project is nil, the project key is extracted from the issue key prefix.
func IssuePath(issue api.Issue) string {
	projectKey := issueProjectKey(issue)
	name := SanitizeFilename(issue.Fields.Summary)
	filename := issue.Key + " - " + name + ".md"
	return filepath.Join(projectKey, filename)
}

// IssueTreePath returns the relative file path for an issue in tree mode.
// Epics become directories containing _epic.md; children of any parent
// are placed inside the parent's directory. Orphans stay flat.
func IssueTreePath(issue api.Issue) string {
	projectKey := issueProjectKey(issue)

	isEpic := issue.Fields.IssueType != nil &&
		strings.EqualFold(issue.Fields.IssueType.Name, "epic")

	if isEpic {
		dirName := issue.Key
		if name := SanitizeFilename(issue.Fields.Summary); name != "" {
			dirName += " - " + name
		}
		return filepath.Join(projectKey, dirName, "_epic.md")
	}

	if issue.Fields.Parent != nil && issue.Fields.Parent.Key != "" {
		parentSummary := ""
		if issue.Fields.Parent.Fields != nil {
			parentSummary = SanitizeFilename(issue.Fields.Parent.Fields.Summary)
		}
		parentDir := issue.Fields.Parent.Key
		if parentSummary != "" {
			parentDir += " - " + parentSummary
		}
		filename := issue.Key + " - " + SanitizeFilename(issue.Fields.Summary) + ".md"
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
