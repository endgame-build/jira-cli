package comment

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/endgame-build/jira-cli/internal/api"
	"github.com/endgame-build/jira-cli/internal/auth"
	"github.com/endgame-build/jira-cli/internal/factory"
	"github.com/endgame-build/jira-cli/internal/iostreams"
)

// commentEditHandler returns an HTTP handler for comment edit tests.
func commentEditHandler(t *testing.T) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// PUT /issue/{key}/comment/{id} — update comment
		if r.Method == http.MethodPut && strings.Contains(path, "/comment/") {
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
					"errorMessages": []string{"You do not have permission to edit this comment."},
				})
				return
			}
			// Comment not found — use a special comment ID.
			if strings.HasSuffix(path, "/comment/99999") {
				w.WriteHeader(http.StatusNotFound)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"errorMessages": []string{"Comment does not exist."},
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

			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":     "10042",
				"self":   "https://test.atlassian.net/rest/api/3/issue/PROJ-123/comment/10042",
				"author": map[string]interface{}{"accountId": "abc123", "displayName": "TestUser"},
				"body":   payload["body"],
			})
			return
		}

		// GET /issue/{key}/comment/{id} — for dry-run validation
		if r.Method == http.MethodGet && strings.Contains(path, "/comment/") {
			if strings.Contains(path, "/issue/NOTFOUND-999/") {
				w.WriteHeader(http.StatusNotFound)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"errorMessages": []string{"Issue does not exist."},
				})
				return
			}
			if strings.HasSuffix(path, "/comment/99999") {
				w.WriteHeader(http.StatusNotFound)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"errorMessages": []string{"Comment does not exist."},
				})
				return
			}
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":     "10042",
				"self":   "https://test.atlassian.net/rest/api/3/issue/PROJ-123/comment/10042",
				"author": map[string]interface{}{"accountId": "abc123", "displayName": "TestUser"},
				"body": map[string]interface{}{
					"type":    "doc",
					"version": 1,
					"content": []interface{}{},
				},
				"created": "2026-02-27T10:00:00.000+0000",
			})
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}
}

func newTestCommentEditFactory(t *testing.T, handler http.Handler) (*factory.Factory, *iostreams.TestIOStreams) {
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

func TestCommentEdit_Success(t *testing.T) {
	f, tio := newTestCommentEditFactory(t, commentEditHandler(t))

	cmd := NewCmdEdit(f)
	cmd.SetArgs([]string{"PROJ-123", "10042", "--body", "Updated comment"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	if !strings.Contains(out, "Updated comment 10042 on PROJ-123") {
		t.Errorf("output should contain success message, got: %s", out)
	}
}

func TestCommentEdit_JSON(t *testing.T) {
	f, tio := newTestCommentEditFactory(t, commentEditHandler(t))
	f.OutputJSON = true

	cmd := NewCmdEdit(f)
	cmd.SetArgs([]string{"PROJ-123", "10042", "--body", "Updated comment"})
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
	if result["action"] != "updated" {
		t.Errorf("action should be updated, got: %v", result["action"])
	}
}

func TestCommentEdit_MissingBody(t *testing.T) {
	f, _ := newTestCommentEditFactory(t, commentEditHandler(t))

	cmd := NewCmdEdit(f)
	cmd.SetArgs([]string{"PROJ-123", "10042"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing body, got nil")
	}
	if !strings.Contains(err.Error(), "Provide --body or --body-file") {
		t.Errorf("expected validation error about missing body, got: %v", err)
	}
}

func TestCommentEdit_Comment404(t *testing.T) {
	f, _ := newTestCommentEditFactory(t, commentEditHandler(t))

	cmd := NewCmdEdit(f)
	cmd.SetArgs([]string{"PROJ-123", "99999", "--body", "test"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for comment 404, got nil")
	}
	errMsg := strings.ToLower(err.Error())
	if !strings.Contains(errMsg, "not_found") && !strings.Contains(errMsg, "not found") && !strings.Contains(errMsg, "does not exist") {
		t.Errorf("expected NOT_FOUND error, got: %v", err)
	}
}

func TestCommentEdit_Issue404(t *testing.T) {
	f, _ := newTestCommentEditFactory(t, commentEditHandler(t))

	cmd := NewCmdEdit(f)
	cmd.SetArgs([]string{"NOTFOUND-999", "10042", "--body", "test"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for issue 404, got nil")
	}
	errMsg := strings.ToLower(err.Error())
	if !strings.Contains(errMsg, "not_found") && !strings.Contains(errMsg, "not found") && !strings.Contains(errMsg, "does not exist") {
		t.Errorf("expected NOT_FOUND error, got: %v", err)
	}
}

func TestCommentEdit_BodyFile(t *testing.T) {
	f, tio := newTestCommentEditFactory(t, commentEditHandler(t))

	tmpFile := t.TempDir() + "/comment.md"
	if err := os.WriteFile(tmpFile, []byte("Updated from file"), 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	cmd := NewCmdEdit(f)
	cmd.SetArgs([]string{"PROJ-123", "10042", "--body-file", tmpFile})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	if !strings.Contains(out, "Updated comment 10042 on PROJ-123") {
		t.Errorf("output should contain success message, got: %s", out)
	}
}

func TestCommentEdit_DryRun(t *testing.T) {
	f, tio := newTestCommentEditFactory(t, commentEditHandler(t))
	f.DryRun = true

	cmd := NewCmdEdit(f)
	cmd.SetArgs([]string{"PROJ-123", "10042", "--body", "Dry run edit"})
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
	if !strings.Contains(out, "10042") {
		t.Errorf("output should contain comment ID, got: %s", out)
	}
}

func TestCommentEdit_DryRun_JSON(t *testing.T) {
	f, tio := newTestCommentEditFactory(t, commentEditHandler(t))
	f.DryRun = true
	f.OutputJSON = true

	cmd := NewCmdEdit(f)
	cmd.SetArgs([]string{"PROJ-123", "10042", "--body", "Dry run edit"})
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
	if result["comment_id"] != "10042" {
		t.Errorf("comment_id should be 10042, got: %v", result["comment_id"])
	}
	if result["validation"] != "passed (comment exists)" {
		t.Errorf("validation should be 'passed (comment exists)', got: %v", result["validation"])
	}
}

func TestCommentEdit_DryRun_Quiet(t *testing.T) {
	f, tio := newTestCommentEditFactory(t, commentEditHandler(t))
	f.DryRun = true
	f.Quiet = true

	cmd := NewCmdEdit(f)
	cmd.SetArgs([]string{"PROJ-123", "10042", "--body", "Silent dry run"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	if out != "" {
		t.Errorf("--quiet + --dry-run should produce no output, got: %s", out)
	}
}

func TestCommentEdit_Forbidden(t *testing.T) {
	f, _ := newTestCommentEditFactory(t, commentEditHandler(t))

	cmd := NewCmdEdit(f)
	cmd.SetArgs([]string{"NOPERM-123", "10042", "--body", "test"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for 403, got nil")
	}
	errMsg := strings.ToLower(err.Error())
	if !strings.Contains(errMsg, "forbidden") && !strings.Contains(errMsg, "permission") {
		t.Errorf("expected forbidden error, got: %v", err)
	}
}

func TestCommentEdit_Quiet(t *testing.T) {
	f, tio := newTestCommentEditFactory(t, commentEditHandler(t))
	f.Quiet = true

	cmd := NewCmdEdit(f)
	cmd.SetArgs([]string{"PROJ-123", "10042", "--body", "Quiet edit"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	if out != "" {
		t.Errorf("--quiet should produce no output, got: %s", out)
	}
}

func TestCommentEdit_InvalidKey(t *testing.T) {
	f, _ := newTestCommentEditFactory(t, commentEditHandler(t))

	cmd := NewCmdEdit(f)
	cmd.SetArgs([]string{"bad-key", "10042", "--body", "test"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
	if !strings.Contains(err.Error(), "Invalid issue key") {
		t.Errorf("expected validation error, got: %v", err)
	}
}
