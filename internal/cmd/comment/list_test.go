package comment

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

// commentListHandler returns an HTTP handler for comment list tests.
func commentListHandler(t *testing.T, comments []map[string]interface{}, total int) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// GET /issue/{key}/comment
		if r.Method == http.MethodGet && strings.Contains(path, "/comment") {
			// Check for 404 issue
			if strings.Contains(path, "/issue/NOTFOUND-999/") {
				w.WriteHeader(http.StatusNotFound)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"errorMessages": []string{"Issue does not exist or you do not have permission to see it."},
				})
				return
			}

			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"comments":   comments,
				"maxResults": 25,
				"total":      total,
				"startAt":    0,
			})
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}
}

func newTestCommentListFactory(t *testing.T, handler http.Handler) (*factory.Factory, *iostreams.TestIOStreams) {
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

func sampleComments() []map[string]interface{} {
	return []map[string]interface{}{
		{
			"id":     "10042",
			"author": map[string]interface{}{"accountId": "abc123", "displayName": "Alice"},
			"body": map[string]interface{}{
				"type":    "doc",
				"version": 1,
				"content": []map[string]interface{}{
					{
						"type": "paragraph",
						"content": []map[string]interface{}{
							{"type": "text", "text": "This is a comment."},
						},
					},
				},
			},
			"created": "2026-02-27T10:00:00.000+0000",
			"updated": "2026-02-27T10:00:00.000+0000",
		},
		{
			"id":     "10041",
			"author": map[string]interface{}{"accountId": "def456", "displayName": "Bob"},
			"body": map[string]interface{}{
				"type":    "doc",
				"version": 1,
				"content": []map[string]interface{}{
					{
						"type": "paragraph",
						"content": []map[string]interface{}{
							{"type": "text", "text": "Another comment here."},
						},
					},
				},
			},
			"created": "2026-02-26T09:00:00.000+0000",
			"updated": "2026-02-26T09:00:00.000+0000",
		},
	}
}

func TestCommentList_Success(t *testing.T) {
	comments := sampleComments()
	f, tio := newTestCommentListFactory(t, commentListHandler(t, comments, 2))

	cmd := NewCmdList(f)
	cmd.SetArgs([]string{"PROJ-123"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	if !strings.Contains(out, "10042") {
		t.Errorf("output should contain comment ID 10042, got: %s", out)
	}
	if !strings.Contains(out, "Alice") {
		t.Errorf("output should contain author Alice, got: %s", out)
	}
	if !strings.Contains(out, "Bob") {
		t.Errorf("output should contain author Bob, got: %s", out)
	}
	if !strings.Contains(out, "This is a comment.") {
		t.Errorf("output should contain comment body text, got: %s", out)
	}
}

func TestCommentList_JSON(t *testing.T) {
	comments := sampleComments()
	f, tio := newTestCommentListFactory(t, commentListHandler(t, comments, 2))
	f.OutputJSON = true

	cmd := NewCmdList(f)
	cmd.SetArgs([]string{"PROJ-123"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	var envelope struct {
		Data       []json.RawMessage `json:"data"`
		Pagination struct {
			Offset      int  `json:"offset"`
			Limit       int  `json:"limit"`
			Total       *int `json:"total"`
			HasNextPage bool `json:"has_next_page"`
		} `json:"pagination"`
	}
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("failed to parse JSON output: %v\nOutput: %s", err, out)
	}
	if len(envelope.Data) != 2 {
		t.Errorf("data length = %d, want 2", len(envelope.Data))
	}
	if envelope.Pagination.Total == nil {
		t.Fatal("pagination.total should not be nil for offset-based pagination")
	}
	if *envelope.Pagination.Total != 2 {
		t.Errorf("pagination.total = %d, want 2", *envelope.Pagination.Total)
	}

	// Verify timestamps are raw ISO 8601 (not relative) in JSON.
	var firstComment struct {
		Created string `json:"created"`
	}
	if err := json.Unmarshal(envelope.Data[0], &firstComment); err != nil {
		t.Fatalf("failed to parse first comment: %v", err)
	}
	if !strings.Contains(firstComment.Created, "2026-02-27T10:00:00") {
		t.Errorf("JSON timestamp should be ISO 8601, got: %s", firstComment.Created)
	}
}

func TestCommentList_Empty_Text(t *testing.T) {
	f, tio := newTestCommentListFactory(t, commentListHandler(t, nil, 0))

	cmd := NewCmdList(f)
	cmd.SetArgs([]string{"PROJ-123"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	if !strings.Contains(out, "No comments on PROJ-123") {
		t.Errorf("expected empty message, got: %s", out)
	}
}

func TestCommentList_Empty_JSON(t *testing.T) {
	f, tio := newTestCommentListFactory(t, commentListHandler(t, nil, 0))
	f.OutputJSON = true

	cmd := NewCmdList(f)
	cmd.SetArgs([]string{"PROJ-123"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	var envelope struct {
		Data       []json.RawMessage `json:"data"`
		Pagination struct {
			Total *int `json:"total"`
		} `json:"pagination"`
	}
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("failed to parse JSON: %v\nOutput: %s", err, out)
	}
	if len(envelope.Data) != 0 {
		t.Errorf("data length = %d, want 0", len(envelope.Data))
	}
	if envelope.Pagination.Total == nil || *envelope.Pagination.Total != 0 {
		t.Errorf("pagination.total should be 0 for empty results")
	}
}

func TestCommentList_404(t *testing.T) {
	f, _ := newTestCommentListFactory(t, commentListHandler(t, nil, 0))

	cmd := NewCmdList(f)
	cmd.SetArgs([]string{"NOTFOUND-999"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for 404, got nil")
	}
	if !strings.Contains(err.Error(), "not found") && !strings.Contains(err.Error(), "NOT_FOUND") &&
		!strings.Contains(strings.ToLower(err.Error()), "not found") {
		t.Errorf("expected NOT_FOUND error, got: %v", err)
	}
}

func TestCommentList_InvalidKey(t *testing.T) {
	f, _ := newTestCommentListFactory(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("no API call should be made for invalid key")
	}))

	cmd := NewCmdList(f)
	cmd.SetArgs([]string{"not-valid-key"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
	if !strings.Contains(err.Error(), "Invalid issue key") {
		t.Errorf("expected validation error, got: %v", err)
	}
}

func TestCommentList_Pagination(t *testing.T) {
	// Return 2 comments with total=5 to simulate pagination.
	comments := sampleComments()
	f, tio := newTestCommentListFactory(t, func() http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"comments":   comments,
				"maxResults": 2,
				"total":      5,
				"startAt":    0,
			})
		}
	}())
	f.OutputJSON = true

	cmd := NewCmdList(f)
	cmd.SetArgs([]string{"PROJ-123", "--limit", "2"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	var envelope struct {
		Pagination struct {
			Total       *int `json:"total"`
			HasNextPage bool `json:"has_next_page"`
			Limit       int  `json:"limit"`
		} `json:"pagination"`
	}
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}
	if !envelope.Pagination.HasNextPage {
		t.Error("has_next_page should be true when total > offset + limit")
	}
	if envelope.Pagination.Limit != 2 {
		t.Errorf("limit = %d, want 2", envelope.Pagination.Limit)
	}
}

func TestCommentList_ADFToPlaintext(t *testing.T) {
	// Comment with multi-line ADF body.
	comments := []map[string]interface{}{
		{
			"id":     "10050",
			"author": map[string]interface{}{"accountId": "abc123", "displayName": "Charlie"},
			"body": map[string]interface{}{
				"type":    "doc",
				"version": 1,
				"content": []map[string]interface{}{
					{
						"type": "paragraph",
						"content": []map[string]interface{}{
							{"type": "text", "text": "Line one"},
						},
					},
					{
						"type": "paragraph",
						"content": []map[string]interface{}{
							{"type": "text", "text": "Line two"},
						},
					},
					{
						"type": "paragraph",
						"content": []map[string]interface{}{
							{"type": "text", "text": "Line three"},
						},
					},
					{
						"type": "paragraph",
						"content": []map[string]interface{}{
							{"type": "text", "text": "Line four"},
						},
					},
				},
			},
			"created": "2026-02-27T12:00:00.000+0000",
			"updated": "2026-02-27T12:00:00.000+0000",
		},
	}
	f, tio := newTestCommentListFactory(t, commentListHandler(t, comments, 1))

	cmd := NewCmdList(f)
	cmd.SetArgs([]string{"PROJ-123"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	// Body should be truncated to 3 lines with "..."
	if !strings.Contains(out, "Line one") {
		t.Errorf("output should contain first line, got: %s", out)
	}
	if !strings.Contains(out, "...") {
		t.Errorf("output should contain truncation indicator '...', got: %s", out)
	}
}

func TestCommentList_KeyAutoUppercase(t *testing.T) {
	comments := sampleComments()
	var capturedPath string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"comments":   comments,
			"maxResults": 25,
			"total":      2,
			"startAt":    0,
		})
	})
	f, _ := newTestCommentListFactory(t, handler)

	cmd := NewCmdList(f)
	cmd.SetArgs([]string{"proj-123"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(capturedPath, "PROJ-123") {
		t.Errorf("API path should use uppercased key, got: %s", capturedPath)
	}
}

func TestTruncateBody(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxLines int
		want     string
	}{
		{"empty", "", 3, ""},
		{"one line", "hello", 3, "hello"},
		{"three lines exact", "a\nb\nc", 3, "a\nb\nc"},
		{"four lines truncated", "a\nb\nc\nd", 3, "a\nb\nc..."},
		{"many lines", "1\n2\n3\n4\n5\n6", 3, "1\n2\n3..."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateBody(tt.input, tt.maxLines)
			if got != tt.want {
				t.Errorf("truncateBody(%q, %d) = %q, want %q", tt.input, tt.maxLines, got, tt.want)
			}
		})
	}
}
