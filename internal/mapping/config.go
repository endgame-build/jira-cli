// Package mapping applies a declarative field-map (jira-sync.yaml) that lets
// `jira issue import --map` translate documents whose frontmatter uses hub keys
// (name, jira_key, jira_issue_type, initiative, stream, …) into the canonical
// markdown.Frontmatter the import pipeline already understands.
//
// The mapping is opt-in: without --map, import behaves exactly as before.
package mapping

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	clierrors "github.com/endgame-build/jira-cli/internal/errors"
	"github.com/endgame-build/jira-cli/internal/markdown"

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
	// ComponentMap translates a doc's `component:` frontmatter (an ownership-area
	// key) to a JIRA component name. Components is a required create-screen field
	// in some projects, so this is pushed like priority — see setCommonFields.
	ComponentMap map[string]string `yaml:"component_map"`
	Streams      map[string]Stream `yaml:"streams"`
	// CreateFields sets constant custom fields per document type ("epic" or
	// "story") on create only. A stream can add or override them (see Stream). Keys are Jira field names, normalized like
	// frontmatter custom fields (e.g. "Investment Category" → investment_category);
	// object-type values resolve through the .jira-field-values.json sidecar.
	CreateFields map[string]map[string]interface{} `yaml:"create_fields"`
	// FieldMap declares how document fields feed JIRA fields. Only summary is
	// configurable; the other keys document fixed behavior (see fixedFieldMap).
	FieldMap map[string]FieldMapping `yaml:"field_map"`
	Pull     Pull                    `yaml:"pull"`
}

// FieldMapping is one field_map entry.
type FieldMapping struct {
	From     string `yaml:"from,omitempty"`
	Via      string `yaml:"via,omitempty"`
	Format   string `yaml:"format,omitempty"`
	Template string `yaml:"template,omitempty"` // summary only, e.g. "[{id}] {title}"
}

// fixedFieldMap is the push behavior that is not configurable yet. A field_map
// entry for these keys must match it, so a config never states a mapping that
// the CLI silently ignores.
var fixedFieldMap = map[string]FieldMapping{
	"description": {From: "body", Format: "adf"},
	"priority":    {From: "priority", Via: "priority_map"},
	"labels":      {From: "stream", Via: "stream_label"},
	"parent":      {Via: "links"},
}

var templatePlaceholderRe = regexp.MustCompile(`\{([A-Za-z0-9_]+)\}`)

// Summary builds the JIRA summary from a document's frontmatter per
// field_map.summary: a template ("[{id}] {title}"), a source key (from: title),
// or the default source key "name".
func (c *Config) Summary(raw map[string]interface{}, path string) (string, error) {
	m := c.FieldMap["summary"]
	if m.Template == "" {
		from := firstNonEmpty(m.From, "name")
		v := str(raw, from)
		if v == "" {
			return "", clierrors.NewValidationError(fmt.Sprintf("mapped file missing '%s': %s", from, path)).
				WithSuggestion(fmt.Sprintf("documents must carry a '%s:' used as the JIRA summary (field_map.summary)", from))
		}
		return v, nil
	}
	var missing []string
	out := templatePlaceholderRe.ReplaceAllStringFunc(m.Template, func(ph string) string {
		key := ph[1 : len(ph)-1]
		v := str(raw, key)
		if v == "" {
			missing = append(missing, key)
		}
		return v
	})
	if len(missing) > 0 {
		return "", clierrors.NewValidationError(
			fmt.Sprintf("mapped file missing %s for field_map.summary template: %s", quoteJoin(missing), path),
		)
	}
	return strings.TrimSpace(out), nil
}

// validateFieldMap rejects field_map entries that the push side does not honor.
func (c *Config) validateFieldMap(path string) error {
	for field, m := range c.FieldMap {
		if field == "summary" {
			if m.From != "" && m.Template != "" {
				return clierrors.NewValidationError("map config field_map.summary: set 'from' or 'template', not both: " + path)
			}
			if m.Via != "" || m.Format != "" {
				return clierrors.NewValidationError("map config field_map.summary: only 'from' or 'template' is supported: " + path)
			}
			continue
		}
		fixed, ok := fixedFieldMap[field]
		if !ok {
			return clierrors.NewValidationError(fmt.Sprintf("map config field_map.%s: unknown field: %s", field, path)).
				WithSuggestion("field_map supports summary, description, priority, labels, parent")
		}
		if m != fixed {
			return clierrors.NewValidationError(fmt.Sprintf("map config field_map.%s: only the default mapping is supported: %s", field, path)).
				WithSuggestion(fmt.Sprintf("use %s, or remove the entry; only field_map.summary is configurable", fixed))
		}
	}
	return nil
}

// String renders a mapping in flow-YAML form for error messages.
func (m FieldMapping) String() string {
	var parts []string
	for _, kv := range [][2]string{{"from", m.From}, {"via", m.Via}, {"format", m.Format}, {"template", m.Template}} {
		if kv[1] != "" {
			parts = append(parts, kv[0]+": "+kv[1])
		}
	}
	return "{ " + strings.Join(parts, ", ") + " }"
}

func quoteJoin(keys []string) string {
	q := make([]string, len(keys))
	for i, k := range keys {
		q[i] = "'" + k + "'"
	}
	return strings.Join(q, ", ")
}

// Pull configures the JIRA-first reconciliation (status + assignee → hub).
type Pull struct {
	Fields     []string  `yaml:"fields"`
	AssigneeAs string    `yaml:"assignee_as"` // "email" (default) or "account_id"
	StatusMap  StatusMap `yaml:"status_map"`
}

// StatusMap maps a JIRA status to a hub status, by category (stable) then by
// explicit name (override).
type StatusMap struct {
	ByCategory map[string]string `yaml:"by_category"`
	ByName     map[string]string `yaml:"by_name"`
}

// MapStatus resolves a JIRA status name + category key to the hub status.
// Explicit name mapping wins; otherwise falls back to the category. Returns ""
// when neither maps (caller leaves the hub value unchanged).
func (c *Config) MapStatus(name, categoryKey string) string {
	if v, ok := c.Pull.StatusMap.ByName[name]; ok {
		return v
	}
	if v, ok := c.Pull.StatusMap.ByCategory[categoryKey]; ok {
		return v
	}
	return ""
}

// WantsPull reports whether the given field is in the pull set.
func (c *Config) WantsPull(field string) bool {
	for _, f := range c.Pull.Fields {
		if f == field {
			return true
		}
	}
	return false
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

// Stream holds the per-stream JIRA label and optional create_fields. A stream's
// create_fields override the per-type create_fields for the same field.
type Stream struct {
	Label        string                 `yaml:"stream_label"`
	CreateFields map[string]interface{} `yaml:"create_fields,omitempty"`
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
	if err := cfg.validateFieldMap(path); err != nil {
		return nil, err
	}
	if err := cfg.normalizeCreateFields(path); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// normalizeCreateFields validates create_fields (per type and per stream) and
// rewrites field keys to the normalized names the import pipeline resolves
// against Jira.
func (c *Config) normalizeCreateFields(path string) error {
	for docType, fields := range c.CreateFields {
		if docType != "epic" && docType != "story" {
			return clierrors.NewValidationError(
				fmt.Sprintf("map config create_fields.%s: unknown document type (want 'epic' or 'story'): %s", docType, path),
			)
		}
		normalized, err := normalizeFieldKeys(fields, "create_fields."+docType, path)
		if err != nil {
			return err
		}
		c.CreateFields[docType] = normalized
	}
	for prefix, stream := range c.Streams {
		normalized, err := normalizeFieldKeys(stream.CreateFields, "streams."+prefix+".create_fields", path)
		if err != nil {
			return err
		}
		stream.CreateFields = normalized
		c.Streams[prefix] = stream
	}
	return nil
}

// normalizeFieldKeys returns fields with each key normalized to its frontmatter
// custom-field name. Built-in keys are rejected.
func normalizeFieldKeys(fields map[string]interface{}, where, path string) (map[string]interface{}, error) {
	if fields == nil {
		return nil, nil
	}
	normalized := make(map[string]interface{}, len(fields))
	for name, val := range fields {
		key := markdown.NormalizeFieldName(name)
		if key == "" || markdown.IsBuiltinKey(key) {
			return nil, clierrors.NewValidationError(
				fmt.Sprintf("map config %s.%s: not a custom field: %s", where, name, path),
			).WithSuggestion("create_fields sets custom fields only; use priority_map, component_map, or streams for built-in fields")
		}
		normalized[key] = val
	}
	return normalized, nil
}
