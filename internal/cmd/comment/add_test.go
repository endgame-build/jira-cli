package comment

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/endgameio/jira-cli/internal/api"
	"github.com/endgameio/jira-cli/internal/auth"
	"github.com/endgameio/jira-cli/internal/factory"
	"github.com/endgameio/jira-cli/internal/iostreams"
)

// commentAddHandler returns an HTTP handler for comment add tests.
func commentAddHandler(t *testing.T) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// POST /issue/{key}/comment — add comment
		if r.Method == http.MethodPost && strings.HasSuffix(path, "/comment") {
			if strings.Contains(path, "/issue/NOTFOUND-999/") {
				w.WriteHeader(http.StatusNotFound)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"errorMessages": []string{"Issue does not exist or you do not have permission to see it."},
				})
				return
			}
			if strings.Contains(path, "/issue/NOPERM-123/") {
				w.WriteHeader(http.StatusForbidden)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"errorMessages": []string{"You do not have permission to comment on this issue."},
				})
				return
			}
			if strings.Contains(path, "/issue/TOOBIG-123/") {
				w.WriteHeader(http.StatusRequestEntityTooLarge)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"errorMessages": []string{"Comment body exceeds maximum size."},
				})
				return
			}

			// Read and verify the request body has ADF content.
			bodyBytes, _ := io.ReadAll(r.Body)
			var payload map[string]interface{}
			if err := json.Unmarshal(bodyBytes, &payload); err != nil {
				t.Errorf("failed to parse request body: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if _, ok := payload["body"]; !ok {
				t.Error("request body should have 'body' field")
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":     "10042",
				"self":   "https://test.atlassian.net/rest/api/3/issue/PROJ-123/comment/10042",
				"author": map[string]interface{}{"accountId": "abc123", "displayName": "TestUser"},
				"body":   payload["body"],
			})
			return
		}

		// GET /issue/{key}/comment — for dry-run validation
		if r.Method == http.MethodGet && strings.Contains(path, "/comment") {
			if strings.Contains(path, "/issue/NOTFOUND-999/") {
				w.WriteHeader(http.StatusNotFound)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"errorMessages": []string{"Issue does not exist."},
				})
				return
			}
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"comments":   []interface{}{},
				"maxResults": 1,
				"total":      0,
				"startAt":    0,
			})
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}
}

func newTestCommentAddFactory(t *testing.T, handler http.Handler) (*factory.Factory, *iostreams.TestIOStreams) {
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

func TestCommentAdd_Success(t *testing.T) {
	f, tio := newTestCommentAddFactory(t, commentAddHandler(t))

	cmd := NewCmdAdd(f)
	cmd.SetArgs([]string{"PROJ-123", "--body", "Hello world"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	if !strings.Contains(out, "Added comment 10042 to PROJ-123") {
		t.Errorf("output should contain success message, got: %s", out)
	}
	if !strings.Contains(out, "focusedCommentId=10042") {
		t.Errorf("output should contain permalink URL, got: %s", out)
	}
}

func TestCommentAdd_JSON(t *testing.T) {
	f, tio := newTestCommentAddFactory(t, commentAddHandler(t))
	f.OutputJSON = true

	cmd := NewCmdAdd(f)
	cmd.SetArgs([]string{"PROJ-123", "--body", "Hello world"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("failed to parse JSON: %v\nOutput: %s", err, out)
	}
	if result["ok"] != true {
		t.Errorf("ok should be true, got: %v", result["ok"])
	}
	if result["key"] != "PROJ-123" {
		t.Errorf("key should be PROJ-123, got: %v", result["key"])
	}
	if result["comment_id"] != "10042" {
		t.Errorf("comment_id should be 10042, got: %v", result["comment_id"])
	}
	if result["action"] != "added" {
		t.Errorf("action should be added, got: %v", result["action"])
	}
	if result["url"] == nil || !strings.Contains(result["url"].(string), "focusedCommentId=10042") {
		t.Errorf("url should contain permalink, got: %v", result["url"])
	}
}

func TestCommentAdd_MissingBody(t *testing.T) {
	f, _ := newTestCommentAddFactory(t, commentAddHandler(t))

	cmd := NewCmdAdd(f)
	cmd.SetArgs([]string{"PROJ-123"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing body, got nil")
	}
	if !strings.Contains(err.Error(), "Provide --body or --body-file") {
		t.Errorf("expected validation error about missing body, got: %v", err)
	}
}

func TestCommentAdd_404(t *testing.T) {
	f, _ := newTestCommentAddFactory(t, commentAddHandler(t))

	cmd := NewCmdAdd(f)
	cmd.SetArgs([]string{"NOTFOUND-999", "--body", "test"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for 404, got nil")
	}
	errMsg := strings.ToLower(err.Error())
	if !strings.Contains(errMsg, "not_found") && !strings.Contains(errMsg, "not found") && !strings.Contains(errMsg, "does not exist") {
		t.Errorf("expected NOT_FOUND error, got: %v", err)
	}
}

func TestCommentAdd_413(t *testing.T) {
	f, _ := newTestCommentAddFactory(t, commentAddHandler(t))

	cmd := NewCmdAdd(f)
	cmd.SetArgs([]string{"TOOBIG-123", "--body", "test"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for 413, got nil")
	}
	// HTTP 413 is mapped to VALIDATION_ERROR in mapHTTPError.
	if !strings.Contains(strings.ToLower(err.Error()), "validation") &&
		!strings.Contains(strings.ToLower(err.Error()), "size") {
		t.Errorf("expected validation error about size, got: %v", err)
	}
}

func TestCommentAdd_BodyFile(t *testing.T) {
	f, tio := newTestCommentAddFactory(t, commentAddHandler(t))

	// Create a temp file with comment content.
	tmpFile := t.TempDir() + "/comment.md"
	if err := os.WriteFile(tmpFile, []byte("Comment from file"), 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	cmd := NewCmdAdd(f)
	cmd.SetArgs([]string{"PROJ-123", "--body-file", tmpFile})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	if !strings.Contains(out, "Added comment 10042 to PROJ-123") {
		t.Errorf("output should contain success message, got: %s", out)
	}
}

func TestCommentAdd_BodyFileNonExistent(t *testing.T) {
	f, _ := newTestCommentAddFactory(t, commentAddHandler(t))

	cmd := NewCmdAdd(f)
	cmd.SetArgs([]string{"PROJ-123", "--body-file", "/nonexistent/file.md"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for nonexistent file, got nil")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "not found") &&
		!strings.Contains(strings.ToLower(err.Error()), "file") {
		t.Errorf("expected file not found error, got: %v", err)
	}
}

func TestCommentAdd_DryRun(t *testing.T) {
	f, tio := newTestCommentAddFactory(t, commentAddHandler(t))
	f.DryRun = true

	cmd := NewCmdAdd(f)
	cmd.SetArgs([]string{"PROJ-123", "--body", "Dry run comment"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	if !strings.Contains(out, "DRY RUN") {
		t.Errorf("output should contain DRY RUN header, got: %s", out)
	}
	if !strings.Contains(out, "PROJ-123") {
		t.Errorf("output should contain issue key, got: %s", out)
	}
}

func TestCommentAdd_DryRun_JSON(t *testing.T) {
	f, tio := newTestCommentAddFactory(t, commentAddHandler(t))
	f.DryRun = true
	f.OutputJSON = true

	cmd := NewCmdAdd(f)
	cmd.SetArgs([]string{"PROJ-123", "--body", "Dry run comment"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("failed to parse JSON: %v\nOutput: %s", err, out)
	}
	if result["dry_run"] != true {
		t.Errorf("dry_run should be true, got: %v", result["dry_run"])
	}
	if result["key"] != "PROJ-123" {
		t.Errorf("key should be PROJ-123, got: %v", result["key"])
	}
	if result["validation"] != "passed (issue exists)" {
		t.Errorf("validation should be 'passed (issue exists)', got: %v", result["validation"])
	}
}

func TestCommentAdd_Quiet(t *testing.T) {
	f, tio := newTestCommentAddFactory(t, commentAddHandler(t))
	f.Quiet = true

	cmd := NewCmdAdd(f)
	cmd.SetArgs([]string{"PROJ-123", "--body", "Quiet comment"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	if out != "" {
		t.Errorf("quiet mode should produce no output, got: %s", out)
	}
}

func TestCommentAdd_InvalidKey(t *testing.T) {
	f, _ := newTestCommentAddFactory(t, commentAddHandler(t))

	cmd := NewCmdAdd(f)
	cmd.SetArgs([]string{"bad-key", "--body", "test"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
	if !strings.Contains(err.Error(), "Invalid issue key") {
		t.Errorf("expected validation error, got: %v", err)
	}
}
