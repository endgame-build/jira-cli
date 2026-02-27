package comment

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

// commentDeleteHandler returns an HTTP handler for comment delete tests.
func commentDeleteHandler(t *testing.T) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// DELETE /issue/{key}/comment/{id}
		if r.Method == http.MethodDelete && strings.Contains(path, "/comment/") {
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
					"errorMessages": []string{"You do not have permission to delete this comment."},
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

			w.WriteHeader(http.StatusNoContent)
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
				"id":   "10042",
				"self": "https://test.atlassian.net/rest/api/3/issue/PROJ-123/comment/10042",
				"author": map[string]interface{}{
					"accountId":   "abc123",
					"displayName": "TestUser",
				},
				"body": map[string]interface{}{
					"type":    "doc",
					"version": 1,
					"content": []interface{}{
						map[string]interface{}{
							"type": "paragraph",
							"content": []interface{}{
								map[string]interface{}{
									"type": "text",
									"text": "Hello world",
								},
							},
						},
					},
				},
				"created": "2026-02-27T10:00:00.000+0000",
			})
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}
}

func newTestCommentDeleteFactory(t *testing.T, handler http.Handler) (*factory.Factory, *iostreams.TestIOStreams) {
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

func TestCommentDelete_Success(t *testing.T) {
	f, tio := newTestCommentDeleteFactory(t, commentDeleteHandler(t))

	cmd := NewCmdDelete(f)
	cmd.SetArgs([]string{"PROJ-123", "10042", "--yes"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	if !strings.Contains(out, "Deleted comment 10042 from PROJ-123") {
		t.Errorf("output should contain success message, got: %s", out)
	}
}

func TestCommentDelete_JSON(t *testing.T) {
	f, tio := newTestCommentDeleteFactory(t, commentDeleteHandler(t))
	f.OutputJSON = true

	cmd := NewCmdDelete(f)
	cmd.SetArgs([]string{"PROJ-123", "10042", "--yes"})
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
	if result["action"] != "deleted" {
		t.Errorf("action should be deleted, got: %v", result["action"])
	}
}

func TestCommentDelete_MissingYes(t *testing.T) {
	f, _ := newTestCommentDeleteFactory(t, commentDeleteHandler(t))

	cmd := NewCmdDelete(f)
	cmd.SetArgs([]string{"PROJ-123", "10042"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing --yes, got nil")
	}
	if !strings.Contains(err.Error(), "Use --yes to confirm deletion") {
		t.Errorf("expected validation error about --yes, got: %v", err)
	}
}

func TestCommentDelete_Comment404(t *testing.T) {
	f, _ := newTestCommentDeleteFactory(t, commentDeleteHandler(t))

	cmd := NewCmdDelete(f)
	cmd.SetArgs([]string{"PROJ-123", "99999", "--yes"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for comment 404, got nil")
	}
	errMsg := strings.ToLower(err.Error())
	if !strings.Contains(errMsg, "not_found") && !strings.Contains(errMsg, "not found") && !strings.Contains(errMsg, "does not exist") {
		t.Errorf("expected NOT_FOUND error, got: %v", err)
	}
}

func TestCommentDelete_Issue404(t *testing.T) {
	f, _ := newTestCommentDeleteFactory(t, commentDeleteHandler(t))

	cmd := NewCmdDelete(f)
	cmd.SetArgs([]string{"NOTFOUND-999", "10042", "--yes"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for issue 404, got nil")
	}
	errMsg := strings.ToLower(err.Error())
	if !strings.Contains(errMsg, "not_found") && !strings.Contains(errMsg, "not found") && !strings.Contains(errMsg, "does not exist") {
		t.Errorf("expected NOT_FOUND error, got: %v", err)
	}
}

func TestCommentDelete_DryRun(t *testing.T) {
	f, tio := newTestCommentDeleteFactory(t, commentDeleteHandler(t))
	f.DryRun = true

	cmd := NewCmdDelete(f)
	// No --yes needed — dry-run bypasses confirmation.
	cmd.SetArgs([]string{"PROJ-123", "10042"})
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
	if !strings.Contains(out, "TestUser") {
		t.Errorf("output should contain author name, got: %s", out)
	}
	if !strings.Contains(out, "Hello world") {
		t.Errorf("output should contain body preview, got: %s", out)
	}
}

func TestCommentDelete_DryRun_JSON(t *testing.T) {
	f, tio := newTestCommentDeleteFactory(t, commentDeleteHandler(t))
	f.DryRun = true
	f.OutputJSON = true

	cmd := NewCmdDelete(f)
	cmd.SetArgs([]string{"PROJ-123", "10042"})
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

func TestCommentDelete_DryRun_Quiet(t *testing.T) {
	f, tio := newTestCommentDeleteFactory(t, commentDeleteHandler(t))
	f.DryRun = true
	f.Quiet = true

	cmd := NewCmdDelete(f)
	cmd.SetArgs([]string{"PROJ-123", "10042"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	if out != "" {
		t.Errorf("--quiet + --dry-run should produce no output, got: %s", out)
	}
}

func TestCommentDelete_InvalidKey(t *testing.T) {
	f, _ := newTestCommentDeleteFactory(t, commentDeleteHandler(t))

	cmd := NewCmdDelete(f)
	cmd.SetArgs([]string{"bad-key", "10042", "--yes"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
	if !strings.Contains(err.Error(), "Invalid issue key") {
		t.Errorf("expected validation error, got: %v", err)
	}
}
