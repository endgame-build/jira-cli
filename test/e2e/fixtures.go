//go:build e2e

package e2e

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/endgame-build/jira-cli/internal/api"
	clierrors "github.com/endgame-build/jira-cli/internal/errors"
)

// Fixtures creates seed data through the Jira REST API rather than through the
// CLI. This is deliberate: if `agent discover` built the fixture for a test
// asserting on `agent discover`, a bug could produce the wrong fixture and make
// its own assertion pass.
type Fixtures struct {
	t   *testing.T
	h   *Harness
	api *api.Client

	mu  sync.Mutex
	own map[string]bool
}

func newFixtures(t *testing.T, h *Harness) *Fixtures {
	return &Fixtures{t: t, h: h, api: h.API, own: map[string]bool{}}
}

// IssueSpec describes a seed issue. MarkerLabel and the run label are always
// added and the summary is always prefixed, so nothing this suite creates is
// indistinguishable from real project data.
type IssueSpec struct {
	Summary  string   // required
	Type     string   // default: the sandbox's Task type
	Priority string   // default "Medium"
	Labels   []string // extra labels, e.g. a per-case discriminator
	Parent   string   // parent key, for a sub-task
	AssignMe bool
	InSprint bool
}

// Issue is a created fixture.
type Issue struct {
	Key     string
	Summary string // the full prefixed summary as stored in Jira
}

// Create makes one issue and registers its deletion before doing anything else
// that could fail.
func (f *Fixtures) Create(spec IssueSpec) Issue {
	t := f.t
	t.Helper()

	if spec.Type == "" {
		spec.Type = f.h.Sandbox.TaskType
	}
	if spec.Priority == "" {
		spec.Priority = "Medium"
	}

	summary := fmt.Sprintf("[e2e %s] %s", f.h.RunID, spec.Summary)
	labels := append([]string{MarkerLabel, f.h.RunLabel}, spec.Labels...)

	fields := map[string]any{
		"project":   map[string]any{"key": f.h.Project},
		"issuetype": map[string]any{"name": spec.Type},
		"summary":   summary,
		"labels":    labels,
		"priority":  map[string]any{"name": spec.Priority},
	}
	if spec.Parent != "" {
		fields["parent"] = map[string]any{"key": spec.Parent}
	}

	created, err := f.api.CreateIssue(context.Background(), &api.CreateIssueInput{Fields: fields})
	if err != nil {
		t.Fatalf("fixture create %q: %v", summary, err)
	}

	// Register cleanup first: everything below can t.Fatal, and an unregistered
	// issue leaks until the next sweep.
	f.Track(created.Key)

	if spec.AssignMe {
		me := f.h.Sandbox.AccountID
		if err := f.api.AssignIssue(context.Background(), created.Key, &me); err != nil {
			t.Fatalf("fixture assign %s: %v", created.Key, err)
		}
	}
	if spec.InSprint {
		f.AddToActiveSprint(created.Key)
	}

	return Issue{Key: created.Key, Summary: summary}
}

// Block links the two issues so that `blocked` reports "blocked is blocked by
// blocker".
//
// The POST body's inwardIssue is the issue that does the blocking, so the
// blocker goes in inwardIssue and the blocked issue in outwardIssue. Verified
// against Jira Cloud: posting {Blocks, inwardIssue: A, outwardIssue: B} yields
// "B is blocked by A". This is the reverse of the reading most people take from
// Atlassian's example, which is why requireBlockedBy below re-reads the issue
// and fails loudly rather than trusting the write.
func (f *Fixtures) Block(blocked, blocker Issue) {
	f.t.Helper()
	err := f.api.CreateIssueLink(context.Background(), &api.CreateIssueLinkInput{
		Type:         api.IssueLinkTypeRef{Name: "Blocks"},
		InwardIssue:  api.LinkedIssueRef{Key: blocker.Key},
		OutwardIssue: api.LinkedIssueRef{Key: blocked.Key},
	})
	if err != nil {
		f.t.Fatalf("fixture link %s blocked-by %s: %v", blocked.Key, blocker.Key, err)
	}
	f.requireBlockedBy(blocked.Key, blocker.Key)
}

// requireBlockedBy proves the fixture produced the link direction the test
// intends. Without it, a reversed link would silently turn "ready hides blocked
// issues" into "ready hides an issue that was never blocked".
func (f *Fixtures) requireBlockedBy(blockedKey, blockerKey string) {
	f.t.Helper()
	issue, err := f.api.GetIssue(context.Background(), blockedKey, &api.GetIssueOptions{
		Fields: []string{"issuelinks"},
	})
	if err != nil {
		f.t.Fatalf("verify link on %s: %v", blockedKey, err)
	}
	for _, l := range issue.Fields.IssueLinks {
		if l.Type != nil && l.Type.Inward == "is blocked by" &&
			l.InwardIssue != nil && l.InwardIssue.Key == blockerKey {
			return
		}
	}
	f.t.Fatalf("fixture is wrong: %s does not report 'is blocked by %s'", blockedKey, blockerKey)
}

// AddToActiveSprint moves issues into the sandbox's active sprint. It calls the
// Agile API directly because no CLI command exposes it: `issue create --field`
// sends every value as a string (internal/cmd/issue/create.go:183) and Jira's
// sprint field needs a number.
func (f *Fixtures) AddToActiveSprint(keys ...string) {
	f.t.Helper()
	path := "sprint/" + strconv.Itoa(f.h.Sandbox.Sprint.ID) + "/issue"
	body := map[string]any{"issues": keys}
	if err := f.api.DoAgile(context.Background(), http.MethodPost, path, body, nil); err != nil {
		f.t.Fatalf("add %v to sprint %d: %v", keys, f.h.Sandbox.Sprint.ID, err)
	}
}

// Track adopts a key the CLI created — the issue `agent discover` returned, for
// instance — so it is deleted with the rest of the test's fixtures.
func (f *Fixtures) Track(key string) {
	if key == "" {
		return
	}
	f.mu.Lock()
	already := f.own[key]
	f.own[key] = true
	f.mu.Unlock()
	if already {
		return
	}
	f.t.Cleanup(func() { f.Delete(key) })
}

// Owns reports whether this test created the key. Commands without a label
// filter return project-wide results, which callers narrow with this.
func (f *Fixtures) Owns(key string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.own[key]
}

// Delete removes an issue, tolerating a prior deletion. It reports with Errorf
// rather than Fatalf so the remaining cleanups still run.
func (f *Fixtures) Delete(key string) {
	err := f.api.DeleteIssue(context.Background(), key, true)
	if err == nil {
		return
	}
	var cliErr *clierrors.CLIError
	if errors.As(err, &cliErr) && cliErr.Code == clierrors.NOT_FOUND {
		return // already gone: deleted with its parent, or by an earlier cleanup
	}
	f.t.Errorf("LEAK: could not delete %s: %v — sweep with `make e2e-sweep` (label %s)",
		key, err, f.h.RunLabel)
}

// GetIssue reads an issue through the API, for post-conditions that must not go
// through the command under test.
func (f *Fixtures) GetIssue(key string, fields ...string) *api.Issue {
	f.t.Helper()
	issue, err := f.api.GetIssue(context.Background(), key, &api.GetIssueOptions{Fields: fields})
	if err != nil {
		f.t.Fatalf("read %s: %v", key, err)
	}
	return issue
}

// CommentCount returns how many comments an issue has, for asserting that a
// dry-run added none.
func (f *Fixtures) CommentCount(key string) int {
	f.t.Helper()
	page, err := f.api.ListComments(context.Background(), key, api.OffsetPaginationOptions{MaxResults: 100})
	if err != nil {
		f.t.Fatalf("list comments on %s: %v", key, err)
	}
	return len(page.Comments)
}

// WaitForIndexed blocks until every key is returned by a JQL search.
//
// Jira Cloud's search index is eventually consistent, so an issue created a
// moment ago is invisible to `agent ready` for a few seconds. Call this once
// after building fixtures rather than wrapping each real assertion in a retry,
// so that a genuine regression still fails instead of being retried away.
func (f *Fixtures) WaitForIndexed(keys ...string) {
	f.t.Helper()
	if len(keys) == 0 {
		return
	}
	quoted := make([]string, len(keys))
	for i, k := range keys {
		quoted[i] = strconv.Quote(k)
	}
	jql := fmt.Sprintf("key in (%s)", strings.Join(quoted, ","))

	Eventually(f.t, fmt.Sprintf("%d fixture(s) to be indexed", len(keys)), func() bool {
		res, err := f.api.SearchIssues(context.Background(), &api.SearchOptions{
			JQL:        jql,
			Fields:     []string{"key"},
			MaxResults: len(keys),
		})
		return err == nil && len(res.Issues) == len(keys)
	})
}
