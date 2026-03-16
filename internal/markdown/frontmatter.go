package markdown

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"sort"
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

	// CustomFields holds custom field values keyed by normalized name.
	// Excluded from default YAML marshal; appended by MarshalYAML.
	CustomFields map[string]interface{} `yaml:"-"`
}

// MarshalYAML marshals built-in fields first, then appends custom fields
// in alphabetical order.
func (fm Frontmatter) MarshalYAML() (interface{}, error) {
	// Marshal built-in fields via alias to avoid infinite recursion.
	type Alias Frontmatter
	builtinBytes, err := yaml.Marshal(Alias(fm))
	if err != nil {
		return nil, err
	}

	var node yaml.Node
	if err := yaml.Unmarshal(builtinBytes, &node); err != nil {
		return nil, err
	}

	// node is a document node; the mapping is its first content child.
	if len(node.Content) == 0 {
		return nil, fmt.Errorf("unexpected empty YAML document node")
	}
	mapping := node.Content[0]

	if len(fm.CustomFields) == 0 {
		return mapping, nil
	}

	// Sort custom field keys for deterministic output.
	keys := make([]string, 0, len(fm.CustomFields))
	for k := range fm.CustomFields {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		v := fm.CustomFields[k]

		keyNode := &yaml.Node{Kind: yaml.ScalarNode, Value: k}

		var valNode yaml.Node
		valBytes, err := yaml.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("marshal custom field %q: %w", k, err)
		}
		if err := yaml.Unmarshal(valBytes, &valNode); err != nil {
			return nil, fmt.Errorf("unmarshal custom field %q node: %w", k, err)
		}
		// valNode is a document node; extract its content.
		if len(valNode.Content) == 0 {
			return nil, fmt.Errorf("unexpected empty YAML node for custom field %q", k)
		}
		mapping.Content = append(mapping.Content, keyNode, valNode.Content[0])
	}

	return mapping, nil
}

// ExtractObjectDisplay extracts a display value from a JSON object field value.
// Returns the display string and true if the value is an object with a .value or
// .name key. Returns "", false for non-objects, nil, or objects without displayable keys.
func ExtractObjectDisplay(raw json.RawMessage) (string, bool) {
	var v interface{}
	if err := json.Unmarshal(raw, &v); err != nil {
		return "", false
	}
	objMap, ok := v.(map[string]interface{})
	if !ok {
		return "", false
	}
	if value, ok := objMap["value"]; ok {
		return fmt.Sprintf("%v", value), true
	}
	if name, ok := objMap["name"]; ok {
		return fmt.Sprintf("%v", name), true
	}
	return "", false
}

// extractCustomFieldValue determines the YAML value from a json.RawMessage.
// Returns (displayValue, rawObject, true, nil) if it should be included.
// rawObject is non-nil only when the value was extracted from a JSON object
// (e.g. {"value": "Critical"} or {"name": "Endgame", "id": "..."}), so the
// caller can preserve the full object for round-trip import.
// Returns (nil, nil, false, nil) for unsupported but valid types,
// or (nil, nil, false, err) for invalid JSON.
func extractCustomFieldValue(raw json.RawMessage) (interface{}, json.RawMessage, bool, error) {
	var v interface{}
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, nil, false, err
	}

	switch val := v.(type) {
	case string:
		return val, nil, true, nil
	case float64:
		return val, nil, true, nil
	case bool:
		return val, nil, true, nil
	case nil:
		return nil, nil, false, nil
	case map[string]interface{}:
		// Try .value first, then .name
		if value, ok := val["value"]; ok {
			switch sv := value.(type) {
			case string:
				return sv, raw, true, nil
			case float64:
				return sv, raw, true, nil
			}
		}
		if name, ok := val["name"]; ok {
			switch sv := name.(type) {
			case string:
				return sv, raw, true, nil
			case float64:
				return sv, raw, true, nil
			}
		}
		return nil, nil, false, nil
	case []interface{}:
		return nil, nil, false, nil
	default:
		return nil, nil, false, nil
	}
}

// IssueToMarkdown converts an api.Issue to markdown bytes with YAML frontmatter.
// The fields map (keyed by field ID, e.g. "customfield_10001") provides display
// names for custom field resolution. Pass nil to omit custom fields.
// Warnings (collisions, skipped values) are written to warnWriter; nil is safe.
//
// Returns the markdown bytes and a FieldValueMap containing raw API objects
// for any custom fields whose values were extracted from objects (for sidecar).
//
// The output has the form:
//
//	---
//	key: PROJ-123
//	summary: Issue title
//	...
//	---
//	Markdown description body
func IssueToMarkdown(issue api.Issue, fields map[string]api.Field, warnWriter io.Writer) ([]byte, FieldValueMap, error) {
	if warnWriter == nil {
		warnWriter = io.Discard
	}

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

	// Process custom fields if field metadata is provided.
	var rawValues FieldValueMap
	if fields != nil && len(issue.Fields.CustomFields) > 0 {
		fm.CustomFields, rawValues = resolveCustomFields(issue.Fields.CustomFields, fields, warnWriter)
	}

	yamlBytes, err := yaml.Marshal(fm)
	if err != nil {
		return nil, nil, err
	}

	var buf bytes.Buffer
	buf.WriteString("---\n")
	buf.Write(yamlBytes)
	buf.WriteString("---\n")

	body, err := adf.ToMarkdown(json.RawMessage(issue.Fields.Description))
	if err != nil {
		return nil, nil, fmt.Errorf("convert description for %s: %w", issue.Key, err)
	}
	if body != "" {
		buf.WriteString(body)
		buf.WriteString("\n")
	}

	return buf.Bytes(), rawValues, nil
}

// resolveCustomFields maps raw JSON custom field values from the API to
// normalized YAML key→value pairs, using the field catalog for name resolution.
// Skips fields with empty keys, builtin collisions, duplicate names, invalid
// JSON, and unsupported value types — writing a warning for each.
// Returns the display values for frontmatter and a FieldValueMap of raw API
// objects for the sidecar (nil if no object-type fields were found).
func resolveCustomFields(customFields map[string]json.RawMessage, fields map[string]api.Field, warnWriter io.Writer) (map[string]interface{}, FieldValueMap) {
	result := make(map[string]interface{})
	var rawValues FieldValueMap
	seen := make(map[string]string) // normalizedName → fieldID (for collision detection)

	for fieldID, raw := range customFields {
		field, ok := fields[fieldID]
		if !ok {
			continue
		}

		key := NormalizeFieldName(field.Name)
		if key == "" {
			fmt.Fprintf(warnWriter, "warning: field %q (%s) normalized to empty key, skipping\n", field.Name, fieldID)
			continue
		}

		if IsBuiltinKey(key) {
			fmt.Fprintf(warnWriter, "warning: field %q (%s) collides with built-in key %q, skipping\n", field.Name, fieldID, key)
			continue
		}

		if prevID, exists := seen[key]; exists {
			fmt.Fprintf(warnWriter, "warning: field %q (%s) collides with %s (both normalize to %q), keeping first\n", field.Name, fieldID, prevID, key)
			continue
		}

		// Claim the slot before value extraction so a second field with the
		// same normalized name always triggers the collision warning above,
		// even if this field's value is unsupported.
		seen[key] = fieldID

		val, rawObj, ok, err := extractCustomFieldValue(raw)
		if err != nil {
			fmt.Fprintf(warnWriter, "warning: field %q (%s) has invalid value: %v, skipping\n", field.Name, fieldID, err)
			continue
		}
		if !ok {
			fmt.Fprintf(warnWriter, "warning: field %q (%s) has unsupported value type, skipping\n", field.Name, fieldID)
			continue
		}

		result[key] = val

		// Capture raw API object for sidecar (only for object-type values).
		if rawObj != nil {
			displayStr := fmt.Sprintf("%v", val)
			if rawValues == nil {
				rawValues = make(FieldValueMap)
			}
			if rawValues[key] == nil {
				rawValues[key] = make(map[string]json.RawMessage)
			}
			rawValues[key][displayStr] = rawObj
		}
	}

	if len(result) == 0 {
		return nil, rawValues
	}
	return result, rawValues
}
