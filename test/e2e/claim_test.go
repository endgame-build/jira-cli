//go:build e2e

package e2e

import (
	"os"
	"testing"
)

// TestE2E_CLAIM_01 — claiming a To Do issue assigns it and transitions it.
//
// Spec: docs/e2e-agent-sdlc-spec.md#e2e-claim-01
func TestE2E_CLAIM_01(t *testing.T) {
	h := New(t)
	issue := h.Fixtures.Create(IssueSpec{Summary: "claim fresh", Labels: []string{h.CaseLabel()}})

	res := h.MustRun("agent", "claim", issue.Key, "--json")
	RequireNoStderr(t, res)

	doc := DecodeObject[MutationDoc](t, res)
	if !doc.OK || doc.Noop {
		t.Errorf("expected a real claim, got %+v\n%s", doc, res)
	}
	if doc.Assignee != h.Sandbox.AccountID {
		t.Errorf("assignee = %q, want %q\n%s", doc.Assignee, h.Sandbox.AccountID, res)
	}
	if doc.PreviousStatus == "" || doc.Status == doc.PreviousStatus {
		t.Errorf("status did not change: %q → %q\n%s", doc.PreviousStatus, doc.Status, res)
	}
}

// TestE2E_CLAIM_02 — claiming an issue already assigned to the caller AND
// already In Progress is a true no-op that performs no writes.
//
// Spec: docs/e2e-agent-sdlc-spec.md#e2e-claim-02
func TestE2E_CLAIM_02(t *testing.T) {
	h := New(t)
	issue := h.Fixtures.Create(IssueSpec{Summary: "claim idempotent", Labels: []string{h.CaseLabel()}})

	h.MustRun("agent", "claim", issue.Key, "--json")
	before := h.Fixtures.GetIssue(issue.Key, "updated").Fields.Updated

	res := h.MustRun("agent", "claim", issue.Key, "--json")
	doc := DecodeObject[MutationDoc](t, res)

	if !doc.Noop {
		t.Errorf("expected noop:true on the second claim\n%s", res)
	}
	if after := h.Fixtures.GetIssue(issue.Key, "updated").Fields.Updated; after != before {
		t.Errorf("the no-op claim wrote to the issue: updated %q → %q", before, after)
	}
}

// TestE2E_CLAIM_03 — an issue assigned to the caller but still in To Do is NOT
// a no-op: the guard requires both assignment and In Progress status, so the
// claim proceeds and transitions it.
//
// Spec: docs/e2e-agent-sdlc-spec.md#e2e-claim-03
func TestE2E_CLAIM_03(t *testing.T) {
	h := New(t)
	issue := h.Fixtures.Create(IssueSpec{
		Summary:  "assigned but not started",
		Labels:   []string{h.CaseLabel()},
		AssignMe: true,
	})

	res := h.MustRun("agent", "claim", issue.Key, "--json")
	doc := DecodeObject[MutationDoc](t, res)

	if doc.Noop {
		t.Errorf("claiming an assigned-but-To-Do issue reported noop:true; it should transition\n%s", res)
	}

	got := h.Fixtures.GetIssue(issue.Key, "status")
	if got.Fields.Status == nil || got.Fields.Status.StatusCategory == nil ||
		got.Fields.Status.StatusCategory.Key != "indeterminate" {
		t.Errorf("%s was not moved to an In Progress category status", issue.Key)
	}
}

// TestE2E_CLAIM_04 — claiming an issue held by another user exits 8 and writes
// nothing. Requires a second account in the sandbox.
//
// Spec: docs/e2e-agent-sdlc-spec.md#e2e-claim-04
func TestE2E_CLAIM_04(t *testing.T) {
	h := New(t)
	other := os.Getenv(EnvOther)
	if other == "" {
		t.Skipf("set %s to a second account ID in the sandbox to cover CONFLICT_ERROR", EnvOther)
	}

	issue := h.Fixtures.Create(IssueSpec{Summary: "held by someone else", Labels: []string{h.CaseLabel()}})
	if err := h.API.AssignIssue(t.Context(), issue.Key, &other); err != nil {
		t.Fatalf("assign %s to %s: %v", issue.Key, other, err)
	}
	before := h.Fixtures.GetIssue(issue.Key, "updated").Fields.Updated

	res := h.RunExpectExit(8, "agent", "claim", issue.Key, "--json")
	RequireEmptyStdout(t, res)
	doc := DecodeError(t, res, "CONFLICT_ERROR")
	if doc.Suggestion == "" {
		t.Errorf("conflict error carries no suggestion\n%s", res)
	}

	if after := h.Fixtures.GetIssue(issue.Key, "updated").Fields.Updated; after != before {
		t.Errorf("the rejected claim still wrote to the issue: updated %q → %q", before, after)
	}

	t.Run("E2E-CLAIM-05-force-overrides", func(t *testing.T) {
		res := h.MustRun("agent", "claim", issue.Key, "--force", "--json")
		doc := DecodeObject[MutationDoc](t, res)
		if doc.Assignee != h.Sandbox.AccountID {
			t.Errorf("--force did not reassign to the caller: %+v\n%s", doc, res)
		}
	})
}
