//go:build e2e

package e2e

import "testing"

// TestE2E_DRY_01 — --dry-run on the three mutating commands changes nothing.
// Each case re-reads the issue through the API afterwards, because a dry-run
// that silently wrote would still print a convincing preview.
//
// Spec: docs/e2e-agent-sdlc-spec.md#e2e-dry-01
func TestE2E_DRY_01(t *testing.T) {
	h := New(t)
	label := h.CaseLabel()

	t.Run("E2E-DRY-01a-claim", func(t *testing.T) {
		issue := h.Fixtures.Create(IssueSpec{Summary: "dry claim", Labels: []string{label}})
		before := h.Fixtures.GetIssue(issue.Key, "updated", "status", "assignee")

		res := h.MustRun("--dry-run", "agent", "claim", issue.Key, "--json")
		doc := DecodeObject[DryRunDoc](t, res)
		if !doc.DryRun || doc.Validation == "" {
			t.Errorf("expected a dry-run envelope, got %+v\n%s", doc, res)
		}
		if doc.Payload["action"] != "claim" {
			t.Errorf("payload.action = %v, want %q\n%s", doc.Payload["action"], "claim", res)
		}

		after := h.Fixtures.GetIssue(issue.Key, "updated", "status", "assignee")
		if after.Fields.Updated != before.Fields.Updated {
			t.Errorf("dry-run claim wrote to %s: updated %q → %q",
				issue.Key, before.Fields.Updated, after.Fields.Updated)
		}
		if after.Fields.Assignee != nil {
			t.Errorf("dry-run claim assigned %s", issue.Key)
		}
	})

	t.Run("E2E-DRY-01b-close", func(t *testing.T) {
		issue := h.Fixtures.Create(IssueSpec{Summary: "dry close", Labels: []string{label}})
		before := h.Fixtures.GetIssue(issue.Key, "updated", "status")
		commentsBefore := h.Fixtures.CommentCount(issue.Key)

		res := h.MustRun("--dry-run", "agent", "close", issue.Key, "--reason", "not really", "--json")
		doc := DecodeObject[DryRunDoc](t, res)
		if doc.Payload["action"] != "close" {
			t.Errorf("payload.action = %v, want %q\n%s", doc.Payload["action"], "close", res)
		}
		if doc.Payload["reason"] != "not really" {
			t.Errorf("payload.reason = %v, want the supplied reason\n%s", doc.Payload["reason"], res)
		}

		after := h.Fixtures.GetIssue(issue.Key, "updated", "status")
		if after.Fields.Updated != before.Fields.Updated {
			t.Errorf("dry-run close wrote to %s: updated %q → %q",
				issue.Key, before.Fields.Updated, after.Fields.Updated)
		}
		if got := h.Fixtures.CommentCount(issue.Key); got != commentsBefore {
			t.Errorf("dry-run close posted a comment: count %d → %d", commentsBefore, got)
		}
	})

	t.Run("E2E-DRY-01c-discover", func(t *testing.T) {
		parent := h.Fixtures.Create(IssueSpec{Summary: "dry discover parent", Labels: []string{label}})
		commentsBefore := h.Fixtures.CommentCount(parent.Key)

		res := h.MustRun("--dry-run", "agent", "discover", parent.Key,
			"--title", "never created", "--type", h.Sandbox.SubtaskType, "--json")
		doc := DecodeObject[DryRunDoc](t, res)
		if doc.Payload["parent"] != parent.Key {
			t.Errorf("payload.parent = %v, want %q\n%s", doc.Payload["parent"], parent.Key, res)
		}
		if doc.Payload["relationship"] != "subtask" {
			t.Errorf("payload.relationship = %v, want %q\n%s", doc.Payload["relationship"], "subtask", res)
		}

		if got := h.Fixtures.CommentCount(parent.Key); got != commentsBefore {
			t.Errorf("dry-run discover commented on the parent: count %d → %d", commentsBefore, got)
		}
		children := h.Fixtures.GetIssue(parent.Key, "subtasks")
		if n := len(children.Fields.SubTasks); n != 0 {
			t.Errorf("dry-run discover created %d sub-task(s) on %s", n, parent.Key)
		}
	})
}

// TestE2E_DRY_02 — the read-only commands ignore --dry-run entirely and still
// produce their normal output. Asserted so nobody later assumes --dry-run is a
// universal no-op guard.
//
// Spec: docs/e2e-agent-sdlc-spec.md#e2e-dry-02
func TestE2E_DRY_02(t *testing.T) {
	h := New(t)

	for _, args := range [][]string{
		{"--dry-run", "agent", "ready", "-p", h.Project, "--json"},
		{"--dry-run", "agent", "blocked", "-p", h.Project, "--json"},
		{"--dry-run", "agent", "status", "-p", h.Project, "--json"},
	} {
		res := h.MustRun(args...)
		raw := DecodeRaw(t, res)
		if _, found := raw["dry_run"]; found {
			t.Errorf("%v emitted a dry_run envelope; read-only commands ignore the flag\n%s", args, res)
		}
	}
}

// TestE2E_DRY_03 — the claim pre-flight guards run before the dry-run branch,
// so --dry-run is a usable validator: it reports the conflict rather than a
// preview.
//
// Spec: docs/e2e-agent-sdlc-spec.md#e2e-dry-03
func TestE2E_DRY_03(t *testing.T) {
	h := New(t)
	issue := h.Fixtures.Create(IssueSpec{Summary: "dry guard", Labels: []string{h.CaseLabel()}})

	h.MustRun("agent", "close", issue.Key, "--json")

	res := h.RunExpectExit(3, "--dry-run", "agent", "claim", issue.Key, "--json")
	RequireEmptyStdout(t, res)
	DecodeError(t, res, "VALIDATION_ERROR")
}
