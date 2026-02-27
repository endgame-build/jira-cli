package schema

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/endgameio/jira-cli/internal/api"
	"github.com/endgameio/jira-cli/internal/auth"
	"github.com/endgameio/jira-cli/internal/factory"
	"github.com/endgameio/jira-cli/internal/iostreams"
)

func newTestSchemaFieldsFactory(t *testing.T, handler http.Handler) (*factory.Factory, *iostreams.TestIOStreams) {
	t.Helper()

	tio := iostreams.Test()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	creds := &auth.Credentials{
		Instance: "test.atlassian.net",
		User:     "test@example.com",
		Token:    "test-token",
	}
	client := api.NewClient(creds, api.WithBaseURL(srv.URL))

	f := factory.NewTestFactory(tio.IOStreams, nil, client)
	return f, tio
}

func fieldsHandler(t *testing.T, fields []map[string]interface{}) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/field") {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(fields)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}
}

func sampleFields() []map[string]interface{} {
	return []map[string]interface{}{
		{
			"id":     "summary",
			"key":    "summary",
			"name":   "Summary",
			"custom": false,
			"schema": map[string]interface{}{
				"type":   "string",
				"system": "summary",
			},
		},
		{
			"id":     "customfield_10001",
			"key":    "customfield_10001",
			"name":   "Story Points",
			"custom": true,
			"schema": map[string]interface{}{
				"type":   "number",
				"custom": "com.atlassian.jira.plugin.system.customfieldtypes:float",
			},
		},
		{
			"id":     "labels",
			"key":    "labels",
			"name":   "Labels",
			"custom": false,
			"schema": map[string]interface{}{
				"type":   "array",
				"items":  "string",
				"system": "labels",
			},
		},
	}
}

func TestSchemaFields_Success(t *testing.T) {
	f, tio := newTestSchemaFieldsFactory(t, fieldsHandler(t, sampleFields()))

	cmd := NewCmdFields(f)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	if !strings.Contains(out, "Summary") {
		t.Errorf("output should contain 'Summary', got: %s", out)
	}
	if !strings.Contains(out, "Story Points") {
		t.Errorf("output should contain 'Story Points', got: %s", out)
	}
	if !strings.Contains(out, "Labels") {
		t.Errorf("output should contain 'Labels', got: %s", out)
	}
	if !strings.Contains(out, "yes") {
		t.Errorf("output should contain 'yes' for custom field, got: %s", out)
	}
	if !strings.Contains(out, "no") {
		t.Errorf("output should contain 'no' for system field, got: %s", out)
	}
}

func TestSchemaFields_JSON(t *testing.T) {
	f, tio := newTestSchemaFieldsFactory(t, fieldsHandler(t, sampleFields()))
	f.OutputJSON = true

	cmd := NewCmdFields(f)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	var envelope struct {
		Data       []json.RawMessage `json:"data"`
		Pagination *json.RawMessage  `json:"pagination"`
	}
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("failed to parse JSON output: %v\nOutput: %s", err, out)
	}
	if len(envelope.Data) != 3 {
		t.Errorf("data length = %d, want 3", len(envelope.Data))
	}
	if envelope.Pagination != nil {
		t.Error("pagination should be null for unpaginated list")
	}

	// Verify field structure in JSON.
	var firstField struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Custom bool   `json:"custom"`
		Schema struct {
			Type string `json:"type"`
		} `json:"schema"`
	}
	if err := json.Unmarshal(envelope.Data[0], &firstField); err != nil {
		t.Fatalf("failed to parse first field: %v", err)
	}
	if firstField.ID != "summary" {
		t.Errorf("first field id = %q, want 'summary'", firstField.ID)
	}
	if firstField.Name != "Summary" {
		t.Errorf("first field name = %q, want 'Summary'", firstField.Name)
	}
	if firstField.Schema.Type != "string" {
		t.Errorf("first field schema.type = %q, want 'string'", firstField.Schema.Type)
	}
	if firstField.Custom {
		t.Error("first field custom should be false")
	}
}

func TestSchemaFields_ProjectWarning(t *testing.T) {
	f, tio := newTestSchemaFieldsFactory(t, fieldsHandler(t, sampleFields()))

	cmd := NewCmdFields(f)
	cmd.SetArgs([]string{"--project", "PROJ"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Warning should appear on stderr.
	errOut := tio.ErrBuf.String()
	if !strings.Contains(errOut, "--project filtering is not yet implemented") {
		t.Errorf("expected --project warning on stderr, got: %s", errOut)
	}

	// Data should be unchanged (all fields returned).
	out := tio.OutBuf.String()
	if !strings.Contains(out, "Summary") {
		t.Errorf("output should still contain all fields, got: %s", out)
	}
}

func TestSchemaFields_TypeWarning(t *testing.T) {
	f, tio := newTestSchemaFieldsFactory(t, fieldsHandler(t, sampleFields()))

	cmd := NewCmdFields(f)
	cmd.SetArgs([]string{"--type", "Bug"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	errOut := tio.ErrBuf.String()
	if !strings.Contains(errOut, "--type filtering is not yet implemented") {
		t.Errorf("expected --type warning on stderr, got: %s", errOut)
	}

	// Data should be unchanged.
	out := tio.OutBuf.String()
	if !strings.Contains(out, "Summary") {
		t.Errorf("output should still contain all fields, got: %s", out)
	}
}

func TestSchemaFields_DataIdenticalWithAndWithoutFlags(t *testing.T) {
	handler := fieldsHandler(t, sampleFields())

	// Without flags.
	f1, tio1 := newTestSchemaFieldsFactory(t, handler)
	f1.OutputJSON = true
	cmd1 := NewCmdFields(f1)
	if err := cmd1.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out1 := tio1.OutBuf.String()

	// With --project and --type flags.
	f2, tio2 := newTestSchemaFieldsFactory(t, handler)
	f2.OutputJSON = true
	cmd2 := NewCmdFields(f2)
	cmd2.SetArgs([]string{"--project", "PROJ", "--type", "Bug"})
	if err := cmd2.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out2 := tio2.OutBuf.String()

	if out1 != out2 {
		t.Errorf("JSON output should be identical with and without flags.\nWithout: %s\nWith: %s", out1, out2)
	}
}
