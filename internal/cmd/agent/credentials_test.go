package agent

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/endgame-build/jira-cli/internal/api"
	clierrors "github.com/endgame-build/jira-cli/internal/errors"
)

// emptySearchHandler serves an empty search result and lets the caller decide
// what GET /myself does, which is what distinguishes "no work" from "bad token".
func emptySearchHandler(myselfStatus int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/myself") {
			if myselfStatus != http.StatusOK {
				w.WriteHeader(myselfStatus)
				return
			}
			writeJSON(w, myselfResponse())
			return
		}
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/search/jql") {
			writeJSON(w, sampleSearchResponse(nil))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}
}

func wantsAuthError(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected an auth error, got nil — an invalid token would look like an empty queue")
	}
	var cliErr *clierrors.CLIError
	if !errors.As(err, &cliErr) || cliErr.Code != clierrors.AUTH_ERROR {
		t.Fatalf("expected AUTH_ERROR, got %v", err)
	}
}

// Jira answers search/jql with 200 and no issues when the token is bad, so an
// empty queue is ambiguous until the credentials are probed.
func TestReadyEmptySearchDetectsBadCredentials(t *testing.T) {
	f, _, _ := newTestFactory(t, emptySearchHandler(http.StatusUnauthorized))
	wantsAuthError(t, runReady(&ReadyOptions{Factory: f, Project: "PROJ", Limit: 10, Sort: "priority"}))
}

func TestBlockedEmptySearchDetectsBadCredentials(t *testing.T) {
	f, _, _ := newTestFactory(t, emptySearchHandler(http.StatusUnauthorized))
	wantsAuthError(t, runBlocked(&BlockedOptions{Factory: f, Project: "PROJ", Limit: 50}))
}

func TestStatusEmptySearchDetectsBadCredentials(t *testing.T) {
	f, _, _ := newTestFactory(t, emptySearchHandler(http.StatusUnauthorized))
	wantsAuthError(t, runStatus(&StatusOptions{Factory: f, Project: "PROJ"}))
}

// A probe that fails for a non-auth reason must not mask a legitimately empty
// result set — it warns and carries on.
func TestReadyEmptySearchToleratesProbeFailure(t *testing.T) {
	f, tio, _ := newTestFactory(t, emptySearchHandler(http.StatusInternalServerError))

	if err := runReady(&ReadyOptions{Factory: f, Project: "PROJ", Limit: 10, Sort: "priority"}); err != nil {
		t.Fatalf("a 5xx probe failure should not fail the command: %v", err)
	}
	if !strings.Contains(tio.ErrBuf.String(), "credential check failed") {
		t.Errorf("expected a warning on stderr, got %q", tio.ErrBuf.String())
	}
}

// The probe keys off the raw search result, not the filtered queue. A queue
// that empties because everything is blocked is a normal state and must not
// trigger a credential check.
func TestReadyAllBlockedDoesNotProbeCredentials(t *testing.T) {
	blocked := sampleIssueWithLinks("PROJ-1", "new", []api.IssueLink{{
		Type:        &api.IssueLinkType{Name: "Blocks", Inward: "is blocked by", Outward: "blocks"},
		InwardIssue: &api.LinkedIssue{Key: "PROJ-2", Fields: &api.LinkedIssueFields{Status: &api.Status{Name: "To Do", StatusCategory: &api.StatusCategory{Key: "new"}}}},
	}})

	probed := false
	f, _, _ := newTestFactory(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/myself") {
			probed = true
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/search/jql") {
			writeJSON(w, sampleSearchResponse([]api.Issue{blocked}))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))

	if err := runReady(&ReadyOptions{Factory: f, Project: "PROJ", Limit: 10, Sort: "priority"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if probed {
		t.Error("credentials were probed for a queue that emptied through blocker filtering")
	}
}
