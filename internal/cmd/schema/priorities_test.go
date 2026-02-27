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

func newTestSchemaPrioritiesFactory(t *testing.T, handler http.Handler) (*factory.Factory, *iostreams.TestIOStreams) {
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

func samplePriorities() []map[string]interface{} {
	return []map[string]interface{}{
		{
			"id":          "1",
			"name":        "Highest",
			"description": "This problem will block progress.",
			"iconUrl":     "https://example.com/highest.svg",
			"statusColor": "#d04437",
			"isDefault":   false,
		},
		{
			"id":          "2",
			"name":        "High",
			"description": "Serious problem that could block progress.",
			"iconUrl":     "https://example.com/high.svg",
			"statusColor": "#f15C75",
			"isDefault":   false,
		},
		{
			"id":          "3",
			"name":        "Medium",
			"description": "Has the potential to affect progress.",
			"iconUrl":     "https://example.com/medium.svg",
			"statusColor": "#f79232",
			"isDefault":   true,
		},
	}
}

func prioritiesHandler(t *testing.T, priorities []map[string]interface{}) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/priority") {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(priorities)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}
}

func TestSchemaPriorities_Success(t *testing.T) {
	f, tio := newTestSchemaPrioritiesFactory(t, prioritiesHandler(t, samplePriorities()))

	cmd := NewCmdPriorities(f)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	if !strings.Contains(out, "Highest") {
		t.Errorf("output should contain 'Highest', got: %s", out)
	}
	if !strings.Contains(out, "High") {
		t.Errorf("output should contain 'High', got: %s", out)
	}
	if !strings.Contains(out, "Medium") {
		t.Errorf("output should contain 'Medium', got: %s", out)
	}
	if !strings.Contains(out, "This problem will block progress.") {
		t.Errorf("output should contain description, got: %s", out)
	}
}

func TestSchemaPriorities_JSON(t *testing.T) {
	f, tio := newTestSchemaPrioritiesFactory(t, prioritiesHandler(t, samplePriorities()))
	f.OutputJSON = true

	cmd := NewCmdPriorities(f)
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

	// Verify structure of a priority entry includes all expected fields.
	var firstPriority struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description"`
		IconURL     string `json:"iconUrl"`
		StatusColor string `json:"statusColor"`
		IsDefault   bool   `json:"isDefault"`
	}
	if err := json.Unmarshal(envelope.Data[0], &firstPriority); err != nil {
		t.Fatalf("failed to parse first priority: %v", err)
	}
	if firstPriority.ID != "1" {
		t.Errorf("first priority id = %q, want '1'", firstPriority.ID)
	}
	if firstPriority.Name != "Highest" {
		t.Errorf("first priority name = %q, want 'Highest'", firstPriority.Name)
	}
	if firstPriority.Description != "This problem will block progress." {
		t.Errorf("first priority description = %q, want 'This problem will block progress.'", firstPriority.Description)
	}
	if firstPriority.IconURL != "https://example.com/highest.svg" {
		t.Errorf("first priority iconUrl = %q, want 'https://example.com/highest.svg'", firstPriority.IconURL)
	}
	if firstPriority.StatusColor != "#d04437" {
		t.Errorf("first priority statusColor = %q, want '#d04437'", firstPriority.StatusColor)
	}
	if firstPriority.IsDefault != false {
		t.Errorf("first priority isDefault = %v, want false", firstPriority.IsDefault)
	}

	// Verify third priority has isDefault=true.
	var thirdPriority struct {
		IsDefault bool `json:"isDefault"`
	}
	if err := json.Unmarshal(envelope.Data[2], &thirdPriority); err != nil {
		t.Fatalf("failed to parse third priority: %v", err)
	}
	if thirdPriority.IsDefault != true {
		t.Errorf("third priority isDefault = %v, want true", thirdPriority.IsDefault)
	}
}
