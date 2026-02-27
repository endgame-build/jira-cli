package issue

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/endgame-build/jira-cli/internal/api"
	"github.com/endgame-build/jira-cli/internal/auth"
	clierrors "github.com/endgame-build/jira-cli/internal/errors"
	"github.com/endgame-build/jira-cli/internal/factory"
	"github.com/endgame-build/jira-cli/internal/iostreams"
)

// transitionsHandler returns an HTTP handler for the transitions endpoint.
func transitionsHandler(transitions []api.Transition) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/transitions") && r.Method == http.MethodGet {
			resp := struct {
				Transitions []api.Transition `json:"transitions"`
			}{Transitions: transitions}
			json.NewEncoder(w).Encode(resp)
			return
		}

		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"errorMessages": []string{"Issue does not exist"},
		})
	}
}

func newTestTransitionsFactory(t *testing.T, handler http.Handler) (*factory.Factory, *iostreams.TestIOStreams, *httptest.Server) {
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
	return f, tio, srv
}

func TestTransitionsText(t *testing.T) {
	transitions := sampleTransitions()

	f, tio, _ := newTestTransitionsFactory(t, transitionsHandler(transitions))

	opts := &TransitionsOptions{
		Factory: f,
		KeyOrID: "PROJ-123",
	}

	err := runTransitions(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	// Verify all transitions appear in table.
	if !strings.Contains(out, "Start Progress") {
		t.Errorf("expected 'Start Progress' in output, got: %s", out)
	}
	if !strings.Contains(out, "In Progress") {
		t.Errorf("expected 'In Progress' in output, got: %s", out)
	}
	if !strings.Contains(out, "Done") {
		t.Errorf("expected 'Done' in output, got: %s", out)
	}
	if !strings.Contains(out, "Reopen") {
		t.Errorf("expected 'Reopen' in output, got: %s", out)
	}
	if !strings.Contains(out, "To Do") {
		t.Errorf("expected 'To Do' in output, got: %s", out)
	}
	// Verify transition IDs.
	if !strings.Contains(out, "11") {
		t.Errorf("expected transition ID '11' in output, got: %s", out)
	}
	if !strings.Contains(out, "31") {
		t.Errorf("expected transition ID '31' in output, got: %s", out)
	}
}

func TestTransitionsJSON(t *testing.T) {
	transitions := sampleTransitions()

	f, tio, _ := newTestTransitionsFactory(t, transitionsHandler(transitions))
	f.OutputJSON = true

	opts := &TransitionsOptions{
		Factory: f,
		KeyOrID: "PROJ-123",
	}

	err := runTransitions(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, out)
	}

	// List envelope: {data:[], pagination:null}.
	data, ok := result["data"].([]interface{})
	if !ok {
		t.Fatalf("expected data array, got: %T", result["data"])
	}
	if len(data) != len(transitions) {
		t.Errorf("expected %d transitions, got %d", len(transitions), len(data))
	}

	// Verify pagination is null.
	if result["pagination"] != nil {
		t.Errorf("expected pagination:null, got: %v", result["pagination"])
	}

	// Verify first transition shape.
	first, ok := data[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected transition object, got: %T", data[0])
	}
	if first["id"] != "11" {
		t.Errorf("expected id:11, got: %v", first["id"])
	}
	if first["name"] != "Start Progress" {
		t.Errorf("expected name:'Start Progress', got: %v", first["name"])
	}
	to, ok := first["to"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected to object, got: %T", first["to"])
	}
	if to["name"] != "In Progress" {
		t.Errorf("expected to.name:'In Progress', got: %v", to["name"])
	}
}

func TestTransitionsEmptyText(t *testing.T) {
	f, tio, _ := newTestTransitionsFactory(t, transitionsHandler([]api.Transition{}))

	opts := &TransitionsOptions{
		Factory: f,
		KeyOrID: "PROJ-123",
	}

	err := runTransitions(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	if !strings.Contains(out, "No transitions available for PROJ-123") {
		t.Errorf("expected empty message, got: %s", out)
	}
}

func TestTransitionsEmptyJSON(t *testing.T) {
	f, tio, _ := newTestTransitionsFactory(t, transitionsHandler([]api.Transition{}))
	f.OutputJSON = true

	opts := &TransitionsOptions{
		Factory: f,
		KeyOrID: "PROJ-123",
	}

	err := runTransitions(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, out)
	}

	data, ok := result["data"].([]interface{})
	if !ok {
		t.Fatalf("expected data array, got: %T", result["data"])
	}
	if len(data) != 0 {
		t.Errorf("expected empty data array, got %d items", len(data))
	}
	if result["pagination"] != nil {
		t.Errorf("expected pagination:null, got: %v", result["pagination"])
	}
}

func TestTransitions404(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"errorMessages": []string{"Issue does not exist"},
		})
	}

	f, _, _ := newTestTransitionsFactory(t, http.HandlerFunc(handler))

	opts := &TransitionsOptions{
		Factory: f,
		KeyOrID: "NONEXIST-999",
	}

	err := runTransitions(opts)
	if err == nil {
		t.Fatal("expected error for 404")
	}

	var cliErr *clierrors.CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("expected CLIError, got: %T", err)
	}
	if cliErr.Code != clierrors.NOT_FOUND {
		t.Errorf("expected NOT_FOUND, got: %s", cliErr.Code)
	}
}

func TestTransitionsInvalidKey(t *testing.T) {
	cmd := NewCmdTransitions(factory.NewTestFactory(iostreams.Test().IOStreams, nil, nil))
	cmd.SetArgs([]string{"invalid key!"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for invalid key")
	}

	var cliErr *clierrors.CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("expected CLIError, got: %T", err)
	}
	if cliErr.Code != clierrors.VALIDATION_ERROR {
		t.Errorf("expected VALIDATION_ERROR, got: %s", cliErr.Code)
	}
}
