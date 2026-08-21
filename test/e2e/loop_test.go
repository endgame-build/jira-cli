//go:build e2e

package e2e

import "testing"

// TestE2E_LOOP_01 — the full agentic SDLC loop against a real sprint:
// seed → ready → claim → status → discover → close → ready.
//
// The steps share one fixture and must run in order, so they are subtests of a
// single test rather than separate tests. `go test -run` selects Test functions,
// and a standalone close step would have nothing claimed to close.
//
// Spec: docs/e2e-agent-sdlc-spec.md#e2e-loop-01
func TestE2E_LOOP_01(t *testing.T) {
	h := New(t)
	label := h.CaseLabel()

	// Three seeds spanning the priority range, created highest-first so that
	// creation order and priority order agree and a tie-break bug is visible.
	high := h.Fixtures.Create(IssueSpec{Summary: "loop high", Priority: "Highest", Labels: []string{label}, InSprint: true})
	mid := h.Fixtures.Create(IssueSpec{Summary: "loop medium", Priority: "Medium", Labels: []string{label}, InSprint: true})
	low := h.Fixtures.Create(IssueSpec{Summary: "loop low", Priority: "Low", Labels: []string{label}, InSprint: true})
	h.Fixtures.WaitForIndexed(high.Key, mid.Key, low.Key)

	var discovered string

	steps := []struct {
		id string
		fn func(t *testing.T)
	}{
		{"E2E-LOOP-01a-ready-orders-by-priority", func(t *testing.T) {
			var items []ReadyItem
			var page *Pagination
			Eventually(t, "all three seeds in the ready queue", func() bool {
				res := h.MustRun("agent", "ready", "--json", "-p", h.Project, "-l", label, "--limit", "50")
				items, page = DecodeList[ReadyItem](t, res)
				return len(items) == 3
			})

			if got, want := ReadyKeys(items), []string{high.Key, mid.Key, low.Key}; !EqualOrdered(got, want) {
				t.Errorf("ready order = %v, want %v (priority rank, then created)", got, want)
			}
			for _, it := range items {
				if it.Status.Category != "new" {
					t.Errorf("%s has status category %q, want %q", it.Key, it.Status.Category, "new")
				}
			}
			if items[0].Priority.Rank != 0 || items[0].Priority.Name != "Highest" {
				t.Errorf("first item priority = %+v, want {Highest 0}", items[0].Priority)
			}
			if page == nil || page.Total == nil || *page.Total != 3 {
				t.Errorf("pagination.total = %v, want 3", page)
			}
		}},

		{"E2E-LOOP-01b-ready-respects-sprint-filter", func(t *testing.T) {
			res := h.MustRun("agent", "ready", "--json", "-p", h.Project, "-l", label, "--sprint", "active", "--limit", "50")
			items, _ := DecodeList[ReadyItem](t, res)
			if len(items) != 3 {
				t.Errorf("sprint-filtered ready returned %d issues, want 3 (all seeds are in the active sprint)\n%s",
					len(items), res)
			}
		}},

		{"E2E-LOOP-01c-claim", func(t *testing.T) {
			res := h.MustRun("agent", "claim", high.Key, "--json")
			RequireNoStderr(t, res)

			doc := DecodeObject[MutationDoc](t, res)
			if !doc.OK || doc.Key != high.Key {
				t.Fatalf("claim did not report success for %s\n%s", high.Key, res)
			}
			if doc.Noop {
				t.Errorf("first claim reported noop:true\n%s", res)
			}
			if doc.Assignee != h.Sandbox.AccountID {
				t.Errorf("assignee = %q, want %q\n%s", doc.Assignee, h.Sandbox.AccountID, res)
			}

			// Confirm through the API, not through the command under test.
			issue := h.Fixtures.GetIssue(high.Key, "status", "assignee")
			if issue.Fields.Status == nil || issue.Fields.Status.StatusCategory == nil ||
				issue.Fields.Status.StatusCategory.Key != "indeterminate" {
				t.Errorf("%s is not in an In Progress category status after claim", high.Key)
			}
			if issue.Fields.Assignee == nil || issue.Fields.Assignee.AccountID != h.Sandbox.AccountID {
				t.Errorf("%s is not assigned to the caller after claim", high.Key)
			}
		}},

		{"E2E-LOOP-01d-claim-is-idempotent", func(t *testing.T) {
			before := h.Fixtures.GetIssue(high.Key, "updated").Fields.Updated

			res := h.MustRun("agent", "claim", high.Key, "--json")
			doc := DecodeObject[MutationDoc](t, res)
			if !doc.Noop {
				t.Errorf("second claim should report noop:true\n%s", res)
			}
			if doc.Status != doc.PreviousStatus {
				t.Errorf("no-op claim changed status: %q → %q\n%s", doc.PreviousStatus, doc.Status, res)
			}
			if after := h.Fixtures.GetIssue(high.Key, "updated").Fields.Updated; after != before {
				t.Errorf("no-op claim wrote to the issue: updated %q → %q", before, after)
			}
		}},

		{"E2E-LOOP-01e-status-reflects-the-claim", func(t *testing.T) {
			Eventually(t, "status to list the claimed issue in my_work", func() bool {
				res := h.MustRun("agent", "status", "--json", "-p", h.Project)
				doc := DecodeObject[StatusDoc](t, res)
				for _, w := range doc.MyWork {
					if w.Key == high.Key {
						return true
					}
				}
				return false
			})

			res := h.MustRun("agent", "status", "--json", "-p", h.Project)
			doc := DecodeObject[StatusDoc](t, res)
			if doc.Project != h.Project {
				t.Errorf("status project = %q, want %q", doc.Project, h.Project)
			}
			if doc.InProgressCount < 1 {
				t.Errorf("in_progress_count = %d, want at least 1\n%s", doc.InProgressCount, res)
			}
			if doc.Sprint == nil {
				t.Errorf("status omitted the sprint block on a project with an active sprint\n%s", res)
			} else if doc.Sprint.Name != h.Sandbox.Sprint.Name {
				t.Errorf("sprint name = %q, want %q", doc.Sprint.Name, h.Sandbox.Sprint.Name)
			}
		}},

		{"E2E-LOOP-01f-discover", func(t *testing.T) {
			// --type is passed explicitly because agent discover hardcodes the
			// type name "Sub-task" (internal/cmd/agent/discover.go:107) and
			// team-managed projects call it "Subtask". TestE2E_DISCOVER_04 pins
			// that bug; here we only want a child issue to exist.
			res := h.MustRun("agent", "discover", high.Key,
				"--title", "loop discovered work",
				"--type", h.Sandbox.SubtaskType,
				"--json")
			RequireNoStderr(t, res)

			doc := DecodeObject[MutationDoc](t, res)
			discovered = doc.Key
			h.Fixtures.Track(discovered)

			if !doc.OK || discovered == "" {
				t.Fatalf("discover did not report a created issue\n%s", res)
			}
			if doc.Parent != high.Key {
				t.Errorf("parent = %q, want %q\n%s", doc.Parent, high.Key, res)
			}
			if doc.Relationship != "subtask" {
				t.Errorf("relationship = %q, want %q\n%s", doc.Relationship, "subtask", res)
			}

			child := h.Fixtures.GetIssue(discovered, "parent", "labels", "issuetype")
			if child.Fields.Parent == nil || child.Fields.Parent.Key != high.Key {
				t.Errorf("%s is not a child of %s", discovered, high.Key)
			}
			if !Contains(child.Fields.Labels, "discovered") {
				t.Errorf("labels = %v, want them to include %q", child.Fields.Labels, "discovered")
			}
			if !Contains(child.Fields.Labels, label) {
				t.Errorf("labels = %v, want the parent's %q to be inherited", child.Fields.Labels, label)
			}
		}},

		{"E2E-LOOP-01g-close-with-reason", func(t *testing.T) {
			before := h.Fixtures.CommentCount(high.Key)

			res := h.MustRun("agent", "close", high.Key, "--reason", "e2e loop complete", "--json")
			RequireNoStderr(t, res)

			doc := DecodeObject[MutationDoc](t, res)
			if !doc.OK || doc.Key != high.Key {
				t.Fatalf("close did not report success\n%s", res)
			}
			if doc.Status == doc.PreviousStatus {
				t.Errorf("close did not change status: still %q\n%s", doc.Status, res)
			}

			issue := h.Fixtures.GetIssue(high.Key, "status")
			if issue.Fields.Status == nil || issue.Fields.Status.StatusCategory == nil ||
				issue.Fields.Status.StatusCategory.Key != "done" {
				t.Errorf("%s is not in a Done category status after close", high.Key)
			}
			if after := h.Fixtures.CommentCount(high.Key); after != before+1 {
				t.Errorf("comment count %d → %d, want exactly one close-reason comment added", before, after)
			}
		}},

		{"E2E-LOOP-01h-ready-drops-the-closed-issue", func(t *testing.T) {
			Eventually(t, "the closed issue to leave the ready queue", func() bool {
				res := h.MustRun("agent", "ready", "--json", "-p", h.Project, "-l", label, "--limit", "50")
				items, _ := DecodeList[ReadyItem](t, res)
				return !Contains(ReadyKeys(items), high.Key)
			})

			res := h.MustRun("agent", "ready", "--json", "-p", h.Project, "-l", label, "--limit", "50")
			items, _ := DecodeList[ReadyItem](t, res)
			keys := ReadyKeys(items)

			if Contains(keys, high.Key) {
				t.Errorf("ready still lists the closed %s: %v", high.Key, keys)
			}
			for _, want := range []string{mid.Key, low.Key} {
				if !Contains(keys, want) {
					t.Errorf("ready no longer lists the still-open %s: %v", want, keys)
				}
			}
		}},

		{"E2E-LOOP-01i-close-is-idempotent", func(t *testing.T) {
			before := h.Fixtures.CommentCount(high.Key)

			res := h.MustRun("agent", "close", high.Key, "--reason", "closing again", "--json")
			doc := DecodeObject[MutationDoc](t, res)
			if doc.Status != doc.PreviousStatus {
				t.Errorf("re-closing changed status: %q → %q\n%s", doc.PreviousStatus, doc.Status, res)
			}
			if after := h.Fixtures.CommentCount(high.Key); after != before {
				t.Errorf("re-closing posted another comment: count %d → %d", before, after)
			}
		}},

		{"E2E-LOOP-01j-claiming-a-done-issue-is-rejected", func(t *testing.T) {
			res := h.RunExpectExit(3, "agent", "claim", high.Key, "--json")
			RequireEmptyStdout(t, res)
			doc := DecodeError(t, res, "VALIDATION_ERROR")
			if doc.Suggestion == "" {
				t.Errorf("error carries no suggestion\n%s", res)
			}
		}},
	}

	for _, s := range steps {
		if !t.Run(s.id, s.fn) {
			t.Fatalf("chain aborted at %s — the remaining steps would be meaningless", s.id)
		}
	}
}
