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

func newTestSchemaLabelsFactory(t *testing.T, handler http.Handler) (*factory.Factory, *iostreams.TestIOStreams) {
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

func labelsHandler(t *testing.T, labels []string, total int) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/label") {
			startAt := 0
			maxResults := 50
			q := r.URL.Query()
			if v := q.Get("startAt"); v != "" {
				var sa int
				for _, c := range v {
					sa = sa*10 + int(c-'0')
				}
				startAt = sa
			}
			if v := q.Get("maxResults"); v != "" {
				var mr int
				for _, c := range v {
					mr = mr*10 + int(c-'0')
				}
				maxResults = mr
			}

			// Slice labels based on pagination.
			end := startAt + maxResults
			if end > len(labels) {
				end = len(labels)
			}
			page := labels
			if startAt < len(labels) {
				page = labels[startAt:end]
			} else {
				page = []string{}
			}

			resp := map[string]interface{}{
				"values":     page,
				"startAt":    startAt,
				"maxResults": maxResults,
				"total":      total,
				"isLast":     end >= len(labels),
			}
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(resp)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}
}

func TestSchemaLabels_Success(t *testing.T) {
	labels := []string{"bug", "enhancement", "documentation", "frontend", "backend"}
	f, tio := newTestSchemaLabelsFactory(t, labelsHandler(t, labels, len(labels)))

	cmd := NewCmdLabels(f)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	for _, label := range labels {
		if !strings.Contains(out, label) {
			t.Errorf("output should contain %q, got: %s", label, out)
		}
	}
}

func TestSchemaLabels_Pagination(t *testing.T) {
	labels := []string{"bug", "enhancement", "documentation", "frontend", "backend"}
	f, tio := newTestSchemaLabelsFactory(t, labelsHandler(t, labels, len(labels)))
	f.OutputJSON = true

	cmd := NewCmdLabels(f)
	cmd.SetArgs([]string{"--limit", "2", "--offset", "2"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	var envelope struct {
		Data       []string `json:"data"`
		Pagination struct {
			Offset      int  `json:"offset"`
			Limit       int  `json:"limit"`
			Total       *int `json:"total"`
			HasNextPage bool `json:"has_next_page"`
		} `json:"pagination"`
	}
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("failed to parse JSON: %v\nOutput: %s", err, out)
	}
	if len(envelope.Data) != 2 {
		t.Errorf("data length = %d, want 2", len(envelope.Data))
	}
	if envelope.Data[0] != "documentation" {
		t.Errorf("first item = %q, want 'documentation'", envelope.Data[0])
	}
	if envelope.Data[1] != "frontend" {
		t.Errorf("second item = %q, want 'frontend'", envelope.Data[1])
	}
	if envelope.Pagination.Offset != 2 {
		t.Errorf("offset = %d, want 2", envelope.Pagination.Offset)
	}
	if envelope.Pagination.Total == nil || *envelope.Pagination.Total != 5 {
		t.Errorf("total = %v, want 5", envelope.Pagination.Total)
	}
	if !envelope.Pagination.HasNextPage {
		t.Error("has_next_page should be true (2+2 < 5)")
	}
}

func TestSchemaLabels_Empty(t *testing.T) {
	f, tio := newTestSchemaLabelsFactory(t, labelsHandler(t, []string{}, 0))

	cmd := NewCmdLabels(f)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	if !strings.Contains(out, "No labels found") {
		t.Errorf("output should contain 'No labels found', got: %s", out)
	}
}

func TestSchemaLabels_EmptyJSON(t *testing.T) {
	f, tio := newTestSchemaLabelsFactory(t, labelsHandler(t, []string{}, 0))
	f.OutputJSON = true

	cmd := NewCmdLabels(f)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	var envelope struct {
		Data       []string        `json:"data"`
		Pagination json.RawMessage `json:"pagination"`
	}
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("failed to parse JSON: %v\nOutput: %s", err, out)
	}
	if len(envelope.Data) != 0 {
		t.Errorf("data length = %d, want 0", len(envelope.Data))
	}
}

func TestSchemaLabels_JSON(t *testing.T) {
	labels := []string{"bug", "enhancement", "documentation"}
	f, tio := newTestSchemaLabelsFactory(t, labelsHandler(t, labels, len(labels)))
	f.OutputJSON = true

	cmd := NewCmdLabels(f)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	var envelope struct {
		Data       []string `json:"data"`
		Pagination struct {
			Offset      int  `json:"offset"`
			Limit       int  `json:"limit"`
			Total       *int `json:"total"`
			HasNextPage bool `json:"has_next_page"`
		} `json:"pagination"`
	}
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("failed to parse JSON: %v\nOutput: %s", err, out)
	}
	if len(envelope.Data) != 3 {
		t.Errorf("data length = %d, want 3", len(envelope.Data))
	}
	if envelope.Data[0] != "bug" {
		t.Errorf("first label = %q, want 'bug'", envelope.Data[0])
	}
	if envelope.Pagination.Total == nil || *envelope.Pagination.Total != 3 {
		t.Errorf("total = %v, want 3", envelope.Pagination.Total)
	}
	if envelope.Pagination.HasNextPage {
		t.Error("has_next_page should be false for last page")
	}
}

func TestSchemaLabels_ProjectWarning(t *testing.T) {
	labels := []string{"bug"}
	f, tio := newTestSchemaLabelsFactory(t, labelsHandler(t, labels, len(labels)))

	cmd := NewCmdLabels(f)
	cmd.SetArgs([]string{"--project", "PROJ"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	stderr := tio.ErrBuf.String()
	if !strings.Contains(stderr, "Warning:") {
		t.Errorf("stderr should contain warning, got: %s", stderr)
	}
	if !strings.Contains(stderr, "--project") {
		t.Errorf("warning should mention --project, got: %s", stderr)
	}

	// Data should still be returned regardless of the flag.
	out := tio.OutBuf.String()
	if !strings.Contains(out, "bug") {
		t.Errorf("output should still contain data, got: %s", out)
	}
}
