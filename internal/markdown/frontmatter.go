package markdown

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/endgame-build/jira-cli/internal/adf"
	"github.com/endgame-build/jira-cli/internal/api"

	"gopkg.in/yaml.v3"
)

var nonAlphanumUnderscore = regexp.MustCompile(`[^a-z0-9_]`)

// NormalizeFieldName converts a Jira field display name to a YAML-safe key.
// It lowercases, replaces spaces with underscores, and strips all characters
// not in [a-z0-9_]. Returns empty string if nothing remains.
func NormalizeFieldName(displayName string) string {
	s := strings.ToLower(displayName)
	s = strings.ReplaceAll(s, " ", "_")
	s = nonAlphanumUnderscore.ReplaceAllString(s, "")
	return s
}

// builtinFrontmatterKeys is the set of YAML keys used by the Frontmatter struct.
var builtinFrontmatterKeys = map[string]bool{
	"key":         true,
	"id":          true,
	"type":        true,
	"summary":     true,
	"status":      true,
	"priority":    true,
	"labels":      true,
	"parent":      true,
	"assignee":    true,
	"assignee_id": true,
	"reporter":    true,
	"reporter_id": true,
	"project":     true,
	"created":     true,
	"updated":     true,
}

// IsBuiltinKey returns true if key is a reserved frontmatter key.
func IsBuiltinKey(key string) bool {
	return builtinFrontmatterKeys[key]
}

// Frontmatter holds the YAML metadata for an issue markdown file.
type Frontmatter struct {
	Key        string   `yaml:"key"`
	ID         string   `yaml:"id,omitempty"`
	Type       string   `yaml:"type,omitempty"`
	Summary    string   `yaml:"summary"`
	Status     string   `yaml:"status,omitempty"`
	Priority   string   `yaml:"priority,omitempty"`
	Labels     []string `yaml:"labels,omitempty"`
	Parent     string   `yaml:"parent,omitempty"`
	Assignee   string   `yaml:"assignee,omitempty"`
	AssigneeID string   `yaml:"assignee_id,omitempty"`
	Reporter   string   `yaml:"reporter,omitempty"`
	ReporterID string   `yaml:"reporter_id,omitempty"`
	Project    string   `yaml:"project,omitempty"`
	Created    string   `yaml:"created,omitempty"`
	Updated    string   `yaml:"updated,omitempty"`
}

// IssueToMarkdown converts an api.Issue to markdown bytes with YAML frontmatter.
// The output has the form:
//
//	---
//	key: PROJ-123
//	summary: Issue title
//	...
//	---
//	Markdown description body
func IssueToMarkdown(issue api.Issue) ([]byte, error) {
	fm := Frontmatter{
		Key:     issue.Key,
		ID:      issue.ID,
		Summary: issue.Fields.Summary,
		Labels:  issue.Fields.Labels,
		Created: issue.Fields.Created,
		Updated: issue.Fields.Updated,
	}

	if issue.Fields.IssueType != nil {
		fm.Type = issue.Fields.IssueType.Name
	}
	if issue.Fields.Status != nil {
		fm.Status = issue.Fields.Status.Name
	}
	if issue.Fields.Priority != nil {
		fm.Priority = issue.Fields.Priority.Name
	}
	if issue.Fields.Project != nil {
		fm.Project = issue.Fields.Project.Key
	}
	if issue.Fields.Parent != nil {
		fm.Parent = issue.Fields.Parent.Key
	}
	if issue.Fields.Assignee != nil {
		fm.Assignee = issue.Fields.Assignee.DisplayName
		fm.AssigneeID = issue.Fields.Assignee.AccountID
	}
	if issue.Fields.Reporter != nil {
		fm.Reporter = issue.Fields.Reporter.DisplayName
		fm.ReporterID = issue.Fields.Reporter.AccountID
	}

	yamlBytes, err := yaml.Marshal(fm)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	buf.WriteString("---\n")
	buf.Write(yamlBytes)
	buf.WriteString("---\n")

	body, err := adf.ToMarkdown(json.RawMessage(issue.Fields.Description))
	if err != nil {
		return nil, fmt.Errorf("convert description for %s: %w", issue.Key, err)
	}
	if body != "" {
		buf.WriteString(body)
		buf.WriteString("\n")
	}

	return buf.Bytes(), nil
}
