//go:build e2e

package e2e

import (
	"strconv"
	"strings"
	"testing"
)

// TestE2E_DISCOVER_01 — a discovered sub-task inherits the parent's project,
// priority and labels, adds "discovered", and leaves a comment on the parent.
//
// Spec: docs/e2e-agent-sdlc-spec.md#e2e-discover-01
func TestE2E_DISCOVER_01(t *testing.T) {
	h := New(t)
	label := h.CaseLabel()

	parent := h.Fixtures.Create(IssueSpec{
		Summary:  "discover parent",
		Priority: "High",
		Labels:   []string{label},
	})
	commentsBefore := h.Fixtures.CommentCount(parent.Key)

	res := h.MustRun("agent", "discover", parent.Key,
		"--title", "inherited child",
		"--type", h.Sandbox.SubtaskType,
		"--json")
	RequireNoStderr(t, res)

	doc := DecodeObject[MutationDoc](t, res)
	h.Fixtures.Track(doc.Key)

	if doc.Relationship != "subtask" || doc.Parent != parent.Key {
		t.Errorf("expected a subtask of %s, got %+v\n%s", parent.Key, doc, res)
	}
	if doc.Priority != "High" {
		t.Errorf("priority = %q, want the parent's %q\n%s", doc.Priority, "High", res)
	}

	child := h.Fixtures.GetIssue(doc.Key, "labels", "priority", "parent")
	if !Contains(child.Fields.Labels, "discovered") {
		t.Errorf("labels = %v, want them to include %q", child.Fields.Labels, "discovered")
	}
	if !Contains(child.Fields.Labels, label) {
		t.Errorf("labels = %v, want the parent's %q inherited", child.Fields.Labels, label)
	}
	if got := h.Fixtures.CommentCount(parent.Key); got != commentsBefore+1 {
		t.Errorf("parent comment count %d → %d, want one discovery comment", commentsBefore, got)
	}
}

// TestE2E_DISCOVER_02 — explicit --label replaces the parent's labels rather
// than adding to them; "discovered" is still appended.
//
// Spec: docs/e2e-agent-sdlc-spec.md#e2e-discover-02
func TestE2E_DISCOVER_02(t *testing.T) {
	h := New(t)
	label := h.CaseLabel()
	parent := h.Fixtures.Create(IssueSpec{Summary: "label override parent", Labels: []string{label}})

	// The explicit label set must still carry the run label, or the fixture
	// would escape the sweeper.
	res := h.MustRun("agent", "discover", parent.Key,
		"--title", "explicit labels",
		"--type", h.Sandbox.SubtaskType,
		"--label", MarkerLabel,
		"--label", h.RunLabel,
		"--json")

	doc := DecodeObject[MutationDoc](t, res)
	h.Fixtures.Track(doc.Key)

	child := h.Fixtures.GetIssue(doc.Key, "labels")
	if Contains(child.Fields.Labels, label) {
		t.Errorf("labels = %v, want the parent's %q to have been replaced by --label",
			child.Fields.Labels, label)
	}
	if !Contains(child.Fields.Labels, "discovered") {
		t.Errorf("labels = %v, want %q to still be appended", child.Fields.Labels, "discovered")
	}
}

// TestE2E_DISCOVER_03 — --as-subtask=false creates a linked issue, and the
// success payload says "linked" where the dry-run payload says "linked (Type)".
//
// Spec: docs/e2e-agent-sdlc-spec.md#e2e-discover-03
func TestE2E_DISCOVER_03(t *testing.T) {
	h := New(t)
	label := h.CaseLabel()
	parent := h.Fixtures.Create(IssueSpec{Summary: "link parent", Labels: []string{label}})

	dry := h.MustRun("--dry-run", "agent", "discover", parent.Key,
		"--title", "linked preview", "--as-subtask=false", "--json")
	dryDoc := DecodeObject[DryRunDoc](t, dry)
	if got, want := dryDoc.Payload["relationship"], "linked (Relates)"; got != want {
		t.Errorf("dry-run relationship = %v, want %q\n%s", got, want, dry)
	}

	res := h.MustRun("agent", "discover", parent.Key,
		"--title", "linked child", "--as-subtask=false", "--json")
	doc := DecodeObject[MutationDoc](t, res)
	h.Fixtures.Track(doc.Key)

	if doc.Relationship != "linked" {
		t.Errorf("success relationship = %q, want %q — it differs from the dry-run form\n%s",
			doc.Relationship, "linked", res)
	}

	linked := h.Fixtures.GetIssue(doc.Key, "issuelinks")
	if len(linked.Fields.IssueLinks) == 0 {
		t.Errorf("%s was created with no link to %s; the link failure is only a warning, "+
			"so an orphan can be reported as ok:true", doc.Key, parent.Key)
	}
}

// TestE2E_DISCOVER_04 — the default invocation, with no --type, works on any
// project style.
//
// `agent discover` used to hardcode the issue type name "Sub-task". Team-managed
// Jira projects call that type "Subtask", so the default invocation failed
// outright on them — and team-managed is the default project type in Jira Cloud.
// The type is now resolved from the project's own create metadata.
//
// Spec: docs/e2e-agent-sdlc-spec.md#e2e-discover-04
func TestE2E_DISCOVER_04(t *testing.T) {
	h := New(t)
	parent := h.Fixtures.Create(IssueSpec{Summary: "default type parent", Labels: []string{h.CaseLabel()}})

	res := h.Run("agent", "discover", parent.Key, "--title", "default subtask type", "--json")
	if res.ExitCode != 0 {
		t.Fatalf("the default discover failed on a project whose sub-task type is %q; "+
			"the type should be resolved from create metadata, not hardcoded\n%s",
			h.Sandbox.SubtaskType, res)
	}

	doc := DecodeObject[MutationDoc](t, res)
	h.Fixtures.Track(doc.Key)

	if doc.Relationship != "subtask" {
		t.Errorf("relationship = %q, want %q\n%s", doc.Relationship, "subtask", res)
	}
	if doc.Type != h.Sandbox.SubtaskType {
		t.Errorf("type = %q, want the project's own sub-task type %q\n%s",
			doc.Type, h.Sandbox.SubtaskType, res)
	}

	child := h.Fixtures.GetIssue(doc.Key, "issuetype", "parent")
	if child.Fields.IssueType == nil || !child.Fields.IssueType.Subtask {
		t.Errorf("%s was not created as a sub-task", doc.Key)
	}
	if child.Fields.Parent == nil || child.Fields.Parent.Key != parent.Key {
		t.Errorf("%s is not a child of %s", doc.Key, parent.Key)
	}
}

// firstIssueKey pulls an issue key out of the text-mode discover output, which
// reads "Discovered PROJ-1 (subtask of PROJ-2): title".
func firstIssueKey(s string) string {
	for _, field := range strings.Fields(s) {
		i := strings.IndexByte(field, '-')
		if i <= 0 || i == len(field)-1 {
			continue
		}
		if _, err := strconv.Atoi(field[i+1:]); err == nil {
			return field
		}
	}
	return ""
}
