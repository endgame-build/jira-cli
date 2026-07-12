// Package mapping applies a declarative field-map (jira-sync.yaml) that lets
// `jira issue import --map` translate documents whose frontmatter uses hub keys
// (name, jira_key, jira_issue_type, initiative, stream, …) into the canonical
// markdown.Frontmatter the import pipeline already understands.
//
// The mapping is opt-in: without --map, import behaves exactly as before.
package mapping

import (
	"os"

	clierrors "github.com/endgame-build/jira-cli/internal/errors"

	"gopkg.in/yaml.v3"
)

// Config is the subset of jira-sync.yaml that the push (import) side consumes.
// Blocks used only by other consumers (pull, transitions, attachments) are
// ignored here — yaml.v3 tolerates unknown keys.
type Config struct {
	Project     string            `yaml:"project"`
	IssueTypes  IssueTypes        `yaml:"issue_types"`
	Links       map[string]Link   `yaml:"links"`
	PriorityMap map[string]string `yaml:"priority_map"`
	Streams     map[string]Stream `yaml:"streams"`
}

// IssueTypes names the JIRA issue types epics and stories map to.
type IssueTypes struct {
	Epic  string `yaml:"epic"`
	Story string `yaml:"story"`
}

// Link declares the mechanism used to attach a parent (e.g. via: parent).
// The target (which initiative / which epic) comes from the document's frontmatter.
type Link struct {
	Via string `yaml:"via"`
}

// Stream holds the per-stream JIRA label (and optional investment category).
type Stream struct {
	Label              string `yaml:"stream_label"`
	InvestmentCategory string `yaml:"investment_category,omitempty"`
}

// LoadConfig reads and validates a jira-sync.yaml mapping config.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, clierrors.NewValidationError("failed to read map config: " + path).WithErr(err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, clierrors.NewValidationError("failed to parse map config: " + path).WithErr(err)
	}
	if cfg.Project == "" {
		return nil, clierrors.NewValidationError("map config missing 'project': " + path).
			WithSuggestion("jira-sync.yaml must set a top-level 'project:' key")
	}
	if cfg.IssueTypes.Epic == "" || cfg.IssueTypes.Story == "" {
		return nil, clierrors.NewValidationError("map config missing 'issue_types.epic' or 'issue_types.story': " + path)
	}
	return &cfg, nil
}
