package schema

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/endgame-build/jira-cli/internal/api"
	"github.com/endgame-build/jira-cli/internal/auth"
	"github.com/endgame-build/jira-cli/internal/factory"
	"github.com/endgame-build/jira-cli/internal/iostreams"
)

func newTestSchemaStatusesFactory(t *testing.T, handler http.Handler) (*factory.Factory, *iostreams.TestIOStreams) {
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

func sampleStatuses() []map[string]interface{} {
	return []map[string]interface{}{
		{
			"id":   "1",
			"name": "Open",
			"statusCategory": map[string]interface{}{
				"id":        float64(2),
				"key":       "new",
				"colorName": "blue-gray",
				"name":      "To Do",
			},
		},
		{
			"id":   "3",
			"name": "In Progress",
			"statusCategory": map[string]interface{}{
				"id":        float64(4),
				"key":       "indeterminate",
				"colorName": "yellow",
				"name":      "In Progress",
			},
		},
		{
			"id":   "6",
			"name": "Closed",
			"statusCategory": map[string]interface{}{
				"id":        float64(3),
				"key":       "done",
				"colorName": "green",
				"name":      "Done",
			},
			"description": "The issue is considered finished.",
			"iconUrl":     "https://example.com/closed.svg",
		},
	}
}

func statusesHandler(t *testing.T, statuses []map[string]interface{}) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/status") {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(statuses)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}
}

func TestSchemaStatuses_Success(t *testing.T) {
	f, tio := newTestSchemaStatusesFactory(t, statusesHandler(t, sampleStatuses()))

	cmd := NewCmdStatuses(f)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	if !strings.Contains(out, "Open") {
		t.Errorf("output should contain 'Open', got: %s", out)
	}
	if !strings.Contains(out, "In Progress") {
		t.Errorf("output should contain 'In Progress', got: %s", out)
	}
	if !strings.Contains(out, "Closed") {
		t.Errorf("output should contain 'Closed', got: %s", out)
	}
	if !strings.Contains(out, "To Do") {
		t.Errorf("output should contain category 'To Do', got: %s", out)
	}
	if !strings.Contains(out, "Done") {
		t.Errorf("output should contain category 'Done', got: %s", out)
	}
}

func TestSchemaStatuses_JSON(t *testing.T) {
	f, tio := newTestSchemaStatusesFactory(t, statusesHandler(t, sampleStatuses()))
	f.OutputJSON = true

	cmd := NewCmdStatuses(f)
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

	// Verify structure of a status entry.
	var firstStatus struct {
		ID             string `json:"id"`
		Name           string `json:"name"`
		StatusCategory struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		} `json:"statusCategory"`
	}
	if err := json.Unmarshal(envelope.Data[0], &firstStatus); err != nil {
		t.Fatalf("failed to parse first status: %v", err)
	}
	if firstStatus.ID != "1" {
		t.Errorf("first status id = %q, want '1'", firstStatus.ID)
	}
	if firstStatus.Name != "Open" {
		t.Errorf("first status name = %q, want 'Open'", firstStatus.Name)
	}
	if firstStatus.StatusCategory.Name != "To Do" {
		t.Errorf("first status category = %q, want 'To Do'", firstStatus.StatusCategory.Name)
	}
}

func TestSchemaStatuses_ProjectWarning(t *testing.T) {
	f, tio := newTestSchemaStatusesFactory(t, statusesHandler(t, sampleStatuses()))

	cmd := NewCmdStatuses(f)
	cmd.SetArgs([]string{"--project", "PROJ"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	errOut := tio.ErrBuf.String()
	if !strings.Contains(errOut, "--project filtering is not yet implemented") {
		t.Errorf("expected --project warning on stderr, got: %s", errOut)
	}
	if !strings.Contains(errOut, "jira issue transitions") {
		t.Errorf("expected transitions suggestion on stderr, got: %s", errOut)
	}

	// Data should be unchanged.
	out := tio.OutBuf.String()
	if !strings.Contains(out, "Open") {
		t.Errorf("output should still contain all statuses, got: %s", out)
	}
}

func TestSchemaStatuses_TypeWarning(t *testing.T) {
	f, tio := newTestSchemaStatusesFactory(t, statusesHandler(t, sampleStatuses()))

	cmd := NewCmdStatuses(f)
	cmd.SetArgs([]string{"--type", "Bug"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	errOut := tio.ErrBuf.String()
	if !strings.Contains(errOut, "--type filtering is not yet implemented") {
		t.Errorf("expected --type warning on stderr, got: %s", errOut)
	}

	out := tio.OutBuf.String()
	if !strings.Contains(out, "Open") {
		t.Errorf("output should still contain all statuses, got: %s", out)
	}
}

func TestSchemaStatuses_DataIdenticalWithAndWithoutFlags(t *testing.T) {
	handler := statusesHandler(t, sampleStatuses())

	// Without flags.
	f1, tio1 := newTestSchemaStatusesFactory(t, handler)
	f1.OutputJSON = true
	cmd1 := NewCmdStatuses(f1)
	if err := cmd1.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out1 := tio1.OutBuf.String()

	// With --project and --type flags.
	f2, tio2 := newTestSchemaStatusesFactory(t, handler)
	f2.OutputJSON = true
	cmd2 := NewCmdStatuses(f2)
	cmd2.SetArgs([]string{"--project", "PROJ", "--type", "Bug"})
	if err := cmd2.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out2 := tio2.OutBuf.String()

	if out1 != out2 {
		t.Errorf("JSON output should be identical with and without flags.\nWithout: %s\nWith: %s", out1, out2)
	}
}
