package markdown

import (
	"path/filepath"
	"strings"

	"github.com/endgame-build/jira-cli/internal/api"
)

// IssuePath returns the relative file path for an issue markdown file.
// Format: {ProjectKey}/{IssueKey} - {SanitizedSummary}.md
// If Project is nil, the project key is extracted from the issue key prefix.
func IssuePath(issue api.Issue) string {
	projectKey := ""
	if issue.Fields.Project != nil {
		projectKey = issue.Fields.Project.Key
	} else {
		// Fall back to extracting project prefix from issue key (e.g. "PROJ-123" → "PROJ")
		if idx := strings.LastIndex(issue.Key, "-"); idx > 0 {
			projectKey = issue.Key[:idx]
		}
	}

	name := SanitizeFilename(issue.Fields.Summary)
	filename := issue.Key + " - " + name + ".md"
	return filepath.Join(projectKey, filename)
}

// SanitizeFilename replaces characters that are invalid in file names,
// collapses multiple spaces, trims whitespace, and enforces a max length of 100 chars.
func SanitizeFilename(name string) string {
	// Replace invalid filename characters with dash
	replacer := strings.NewReplacer(
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
	name = replacer.Replace(name)

	// Collapse multiple spaces into one
	for strings.Contains(name, "  ") {
		name = strings.ReplaceAll(name, "  ", " ")
	}

	name = strings.TrimSpace(name)

	// Enforce max length
	if len(name) > 100 {
		name = name[:100]
		name = strings.TrimSpace(name)
	}

	return name
}
