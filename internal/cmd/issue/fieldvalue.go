package issue

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/endgame-build/jira-cli/internal/api"
)

// resolveFieldValues converts --field key=value strings into the value shapes
// Jira expects on create/edit:
//
//   - A value that is a JSON object or array is sent as that JSON
//     (e.g. customfield_10001={"value":"Other"}).
//   - A plain value for a custom field is wrapped per the field schema:
//     option → {"value": v}, user → {"accountId": v}, number → number,
//     array → a one-element array of the wrapped item.
//   - Anything else is sent as a string.
//
// Field metadata is fetched only when a plain value targets a custom field.
func resolveFieldValues(ctx context.Context, client *api.Client, keys []string, values map[string]string) (map[string]interface{}, error) {
	out := make(map[string]interface{}, len(values))
	var plainCustom []string
	for _, key := range keys {
		value := values[key]
		if v, ok := parseJSONValue(value); ok {
			out[key] = v
			continue
		}
		out[key] = value
		if strings.HasPrefix(key, "customfield_") {
			plainCustom = append(plainCustom, key)
		}
	}
	if len(plainCustom) == 0 {
		return out, nil
	}

	allFields, err := client.ListFields(ctx)
	if err != nil {
		return nil, err
	}
	schemas := make(map[string]api.FieldSchema, len(allFields))
	for _, f := range allFields {
		schemas[f.ID] = f.Schema
	}
	for _, key := range plainCustom {
		if schema, ok := schemas[key]; ok {
			out[key] = wrapFieldValue(values[key], schema)
		}
	}
	return out, nil
}

// parseJSONValue decodes value when it is a JSON object or array.
func parseJSONValue(value string) (interface{}, bool) {
	trimmed := strings.TrimSpace(value)
	if !strings.HasPrefix(trimmed, "{") && !strings.HasPrefix(trimmed, "[") {
		return nil, false
	}
	var v interface{}
	if err := json.Unmarshal([]byte(trimmed), &v); err != nil {
		return nil, false
	}
	return v, true
}

// wrapFieldValue wraps a plain string into the write shape for schema.
func wrapFieldValue(value string, schema api.FieldSchema) interface{} {
	switch schema.Type {
	case "number":
		if n, err := strconv.ParseFloat(value, 64); err == nil {
			return n
		}
		return value
	case "array":
		return []interface{}{wrapFieldValue(value, api.FieldSchema{Type: schema.Items})}
	case "option", "user":
		return map[string]interface{}{writeKeyForSchema(schema.Type): value}
	}
	return value
}
