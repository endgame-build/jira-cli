package agent

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/endgame-build/jira-cli/internal/api"
)

func discoverHandler(parent api.Issue) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// POST /issue (create)
		if path == "/issue" && r.Method == http.MethodPost {
			w.WriteHeader(http.StatusCreated)
			writeJSON(w, api.CreatedIssue{ID: "10001", Key: "PROJ-456"})
			return
		}

		// POST /issueLink
		if strings.HasSuffix(path, "/issueLink") && r.Method == http.MethodPost {
			w.WriteHeader(http.StatusCreated)
			return
		}

		// POST /issue/{key}/comment
		if strings.HasSuffix(path, "/comment") && r.Method == http.MethodPost {
			writeJSON(w, api.Comment{ID: "200"})
			return
		}

		// GET /issue/createmeta/{project}/issuetypes — how the sub-task type
		// name is resolved. Must precede the generic /issue/ arm below.
		if strings.Contains(path, "/issue/createmeta/") && r.Method == http.MethodGet {
			writeJSON(w, api.CreateMetaIssueTypes{IssueTypes: []api.IssueTypeCreateMeta{
				{ID: "1", Name: "Task", Subtask: false},
				{ID: "2", Name: "Subtask", Subtask: true},
			}})
			return
		}

		// GET /issue/{key}
		if strings.Contains(path, "/issue/") && r.Method == http.MethodGet {
			writeJSON(w, parent)
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}
}

func TestDiscoverSubtask(t *testing.T) {
	parent := api.Issue{
		Key: "PROJ-100",
		Fields: api.IssueFields{
			Summary:   "Parent issue",
			Project:   &api.Project{Key: "PROJ"},
			Priority:  &api.Priority{Name: "High"},
			Labels:    []string{"backend"},
			IssueType: &api.IssueType{Name: "Story", Subtask: false},
		},
	}

	var createdFields map[string]interface{}
	handler := func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/issue" && r.Method == http.MethodPost {
			body, _ := io.ReadAll(r.Body)
			var req map[string]interface{}
			json.Unmarshal(body, &req)
			createdFields = req["fields"].(map[string]interface{})
			w.WriteHeader(http.StatusCreated)
			writeJSON(w, api.CreatedIssue{ID: "10001", Key: "PROJ-456"})
			return
		}
		discoverHandler(parent)(w, r)
	}

	f, tio, _ := newTestFactory(t, http.HandlerFunc(handler))
	f.OutputJSON = true

	opts := &DiscoverOptions{
		Factory:   f,
		ParentKey: "PROJ-100",
		Title:     "Found edge case",
		AsSubtask: true,
	}

	err := runDiscover(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify sub-task fields.
	if createdFields == nil {
		t.Fatal("expected create API to be called")
	}
	if parentField, ok := createdFields["parent"].(map[string]interface{}); ok {
		if parentField["key"] != "PROJ-100" {
			t.Errorf("expected parent key PROJ-100, got %v", parentField["key"])
		}
	} else {
		t.Error("expected parent field for sub-task")
	}

	// Verify JSON output.
	out := tio.OutBuf.String()
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if result["ok"] != true {
		t.Errorf("expected ok:true")
	}
	if result["key"] != "PROJ-456" {
		t.Errorf("expected key:PROJ-456, got %v", result["key"])
	}
	if result["relationship"] != "subtask" {
		t.Errorf("expected relationship:subtask, got %v", result["relationship"])
	}
}

func TestDiscoverLinkedIssue(t *testing.T) {
	parent := api.Issue{
		Key: "PROJ-100",
		Fields: api.IssueFields{
			Summary:   "Parent issue",
			Project:   &api.Project{Key: "PROJ"},
			Priority:  &api.Priority{Name: "Medium"},
			IssueType: &api.IssueType{Name: "Story", Subtask: false},
		},
	}

	linkCreated := false
	handler := func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/issueLink") && r.Method == http.MethodPost {
			linkCreated = true
			w.WriteHeader(http.StatusCreated)
			return
		}
		discoverHandler(parent)(w, r)
	}

	f, tio, _ := newTestFactory(t, http.HandlerFunc(handler))
	f.OutputJSON = true

	opts := &DiscoverOptions{
		Factory:   f,
		ParentKey: "PROJ-100",
		Title:     "Found tech debt",
		AsSubtask: false,
		LinkType:  "Relates",
	}

	err := runDiscover(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !linkCreated {
		t.Error("expected issue link to be created for non-subtask discovery")
	}

	out := tio.OutBuf.String()
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if result["relationship"] != "linked" {
		t.Errorf("expected relationship:linked, got %v", result["relationship"])
	}
}

func TestDiscoverInheritsLabels(t *testing.T) {
	parent := api.Issue{
		Key: "PROJ-100",
		Fields: api.IssueFields{
			Summary:   "Parent",
			Project:   &api.Project{Key: "PROJ"},
			Priority:  &api.Priority{Name: "High"},
			Labels:    []string{"backend", "urgent"},
			IssueType: &api.IssueType{Name: "Story"},
		},
	}

	var createdLabels []interface{}
	handler := func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/issue" && r.Method == http.MethodPost {
			body, _ := io.ReadAll(r.Body)
			var req map[string]interface{}
			json.Unmarshal(body, &req)
			fields := req["fields"].(map[string]interface{})
			createdLabels = fields["labels"].([]interface{})
			w.WriteHeader(http.StatusCreated)
			writeJSON(w, api.CreatedIssue{Key: "PROJ-456"})
			return
		}
		discoverHandler(parent)(w, r)
	}

	f, _, _ := newTestFactory(t, http.HandlerFunc(handler))

	opts := &DiscoverOptions{
		Factory:   f,
		ParentKey: "PROJ-100",
		Title:     "Test",
		AsSubtask: true,
	}

	err := runDiscover(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have parent labels + "discovered".
	labelSet := map[string]bool{}
	for _, l := range createdLabels {
		labelSet[l.(string)] = true
	}
	if !labelSet["backend"] || !labelSet["urgent"] || !labelSet["discovered"] {
		t.Errorf("expected inherited labels + 'discovered', got %v", createdLabels)
	}
}

func TestDiscoverSubSubtaskFallback(t *testing.T) {
	// Parent is already a subtask — should fall back to linked issue.
	parent := api.Issue{
		Key: "PROJ-100",
		Fields: api.IssueFields{
			Summary:   "Sub-task parent",
			Project:   &api.Project{Key: "PROJ"},
			IssueType: &api.IssueType{Name: "Sub-task", Subtask: true},
		},
	}

	linkCreated := false
	handler := func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/issueLink") && r.Method == http.MethodPost {
			linkCreated = true
			w.WriteHeader(http.StatusCreated)
			return
		}
		discoverHandler(parent)(w, r)
	}

	f, _, _ := newTestFactory(t, http.HandlerFunc(handler))

	opts := &DiscoverOptions{
		Factory:   f,
		ParentKey: "PROJ-100",
		Title:     "Found issue",
		AsSubtask: true, // Explicitly true, but should fall back.
	}

	err := runDiscover(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !linkCreated {
		t.Error("expected linked issue (not subtask) when parent is already a subtask")
	}
}

func TestDiscoverDryRun(t *testing.T) {
	parent := api.Issue{
		Key: "PROJ-100",
		Fields: api.IssueFields{
			Summary:   "Parent",
			Project:   &api.Project{Key: "PROJ"},
			IssueType: &api.IssueType{Name: "Story"},
		},
	}

	createCalled := false
	handler := func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/issue" && r.Method == http.MethodPost {
			createCalled = true
		}
		discoverHandler(parent)(w, r)
	}

	f, _, _ := newTestFactory(t, http.HandlerFunc(handler))
	f.DryRun = true
	f.OutputJSON = true

	opts := &DiscoverOptions{
		Factory:   f,
		ParentKey: "PROJ-100",
		Title:     "Test discovery",
		AsSubtask: true,
	}

	err := runDiscover(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if createCalled {
		t.Error("expected no create call during dry-run")
	}
}

// A link failure leaves the issue created but unlinked. The command still
// succeeds — the issue is real — but the payload must say so, because
// relationship alone asserts a link that is not there and a caller reading
// stdout never sees the stderr warning.
func TestDiscoverReportsLinkFailure(t *testing.T) {
	parent := api.Issue{
		Key: "PROJ-100",
		Fields: api.IssueFields{
			Summary:   "Parent issue",
			Project:   &api.Project{Key: "PROJ"},
			IssueType: &api.IssueType{Name: "Task"},
		},
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case path == "/issue" && r.Method == http.MethodPost:
			w.WriteHeader(http.StatusCreated)
			writeJSON(w, api.CreatedIssue{ID: "10001", Key: "PROJ-456"})
		case strings.HasSuffix(path, "/issueLink") && r.Method == http.MethodPost:
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"errorMessages":["No issue link type with name 'Nonsense' found"]}`))
		case strings.HasSuffix(path, "/comment") && r.Method == http.MethodPost:
			writeJSON(w, api.Comment{ID: "200"})
		case strings.Contains(path, "/issue/createmeta/") && r.Method == http.MethodGet:
			writeJSON(w, api.CreateMetaIssueTypes{IssueTypes: []api.IssueTypeCreateMeta{
				{Name: "Task", Subtask: false},
			}})
		case strings.Contains(path, "/issue/") && r.Method == http.MethodGet:
			writeJSON(w, parent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	f, tio, _ := newTestFactory(t, handler)
	f.OutputJSON = true

	opts := &DiscoverOptions{
		Factory:   f,
		ParentKey: "PROJ-100",
		Title:     "orphan",
		AsSubtask: false,
		LinkType:  "Nonsense",
	}
	if err := runDiscover(opts); err != nil {
		t.Fatalf("the issue was created, so the command should succeed: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(tio.OutBuf.Bytes(), &result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result["link_failed"] != true {
		t.Errorf("payload = %v, want link_failed:true so the caller can detect the orphan", result)
	}
	if !strings.Contains(tio.ErrBuf.String(), "link failed") {
		t.Errorf("stderr = %q, want a warning", tio.ErrBuf.String())
	}
}

// The happy path must not carry the flag.
func TestDiscoverOmitsLinkFailedOnSuccess(t *testing.T) {
	parent := api.Issue{
		Key: "PROJ-100",
		Fields: api.IssueFields{
			Project:   &api.Project{Key: "PROJ"},
			IssueType: &api.IssueType{Name: "Task"},
		},
	}

	f, tio, _ := newTestFactory(t, discoverHandler(parent))
	f.OutputJSON = true

	opts := &DiscoverOptions{
		Factory:   f,
		ParentKey: "PROJ-100",
		Title:     "linked",
		AsSubtask: false,
		LinkType:  "Relates",
	}
	if err := runDiscover(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]interface{}
	json.Unmarshal(tio.OutBuf.Bytes(), &result)
	if _, found := result["link_failed"]; found {
		t.Errorf("payload = %v, want no link_failed key when the link succeeded", result)
	}
}
