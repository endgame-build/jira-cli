//go:build e2e

package e2e

import (
	"encoding/json"
	"testing"
)

// TestE2E_CONSIST_01 — `agent status` counts "ready" as every To Do issue in the
// project, with no blocker filtering (internal/cmd/agent/status.go:103), while
// `agent ready` excludes blocked issues. The two numbers therefore disagree
// whenever anything is blocked.
//
// This is a real divergence in a contract that presents them as the same idea.
// The case documents it rather than blessing it: it reports the disagreement
// and does not fail, so a future fix does not look like a regression.
//
// Spec: docs/e2e-agent-sdlc-spec.md#e2e-consist-01
func TestE2E_CONSIST_01(t *testing.T) {
	h := New(t)
	label := h.CaseLabel()

	target := h.Fixtures.Create(IssueSpec{Summary: "consist blocked", Labels: []string{label}})
	blocker := h.Fixtures.Create(IssueSpec{Summary: "consist blocker", Labels: []string{label}})
	h.Fixtures.Block(target, blocker)
	h.Fixtures.WaitForIndexed(target.Key, blocker.Key)

	Eventually(t, "the blocked fixture to be excluded from ready", func() bool {
		res := h.MustRun("agent", "ready", "-p", h.Project, "-l", label, "--limit", "50", "--json")
		items, _ := DecodeList[ReadyItem](t, res)
		return !Contains(ReadyKeys(items), target.Key)
	})

	statusRes := h.MustRun("agent", "status", "-p", h.Project, "--json")
	status := DecodeObject[StatusDoc](t, statusRes)

	readyRes := h.MustRun("agent", "ready", "-p", h.Project, "--limit", "100", "--json")
	readyItems, _ := DecodeList[ReadyItem](t, readyRes)

	if status.BlockedCount == 0 {
		t.Fatalf("blocked_count = 0 although a blocked fixture exists\n%s", statusRes)
	}

	if status.ReadyCount == len(readyItems) {
		t.Logf("status.ready_count (%d) now matches len(agent ready) (%d) — the divergence "+
			"documented for this case may have been fixed", status.ReadyCount, len(readyItems))
		return
	}

	t.Logf("KNOWN DIVERGENCE: status.ready_count = %d but `agent ready` returns %d issues. "+
		"status.go:103 counts every To Do issue project-wide with no blocker filter, so an "+
		"agent reading ready_count sees work that `agent ready` will not hand it.",
		status.ReadyCount, len(readyItems))
}

// TestE2E_CONSIST_02 — `agent status` scopes in_progress_count to the calling
// user (status.go:79) while ready, blocked and done_today are project-wide. The
// four numbers are presented as one summary but do not share a scope.
//
// Spec: docs/e2e-agent-sdlc-spec.md#e2e-consist-02
func TestE2E_CONSIST_02(t *testing.T) {
	h := New(t)

	res := h.MustRun("agent", "status", "-p", h.Project, "--json")
	doc := DecodeObject[StatusDoc](t, res)

	// Count every in-progress issue in the project, regardless of assignee.
	projectWide := h.MustRun("search",
		"project = "+h.Project+" AND statusCategory = \"In Progress\"",
		"--limit", "100", "--json")
	items, _ := DecodeList[map[string]any](t, projectWide)

	if len(items) > doc.InProgressCount {
		t.Logf("KNOWN DIVERGENCE: status.in_progress_count = %d (scoped to the caller) while the "+
			"project holds %d in-progress issues. ready_count, blocked_count and done_today are "+
			"project-wide, so the four numbers in one summary do not share a scope.",
			doc.InProgressCount, len(items))
	}
}

// TestE2E_CONSIST_03 — empty results are not shaped consistently: `agent ready`
// emits "data": [] while `agent blocked` emits "data": null for the same
// condition. A consumer doing data.length on the result breaks on one of them.
//
// Spec: docs/e2e-agent-sdlc-spec.md#e2e-consist-03
func TestE2E_CONSIST_03(t *testing.T) {
	h := New(t)
	absent := "e2e-no-such-label-" + h.RunID

	readyRaw := DecodeRaw(t, h.MustRun("agent", "ready", "-p", h.Project, "-l", absent, "--json"))
	readyData, found := readyRaw["data"]
	if !found {
		t.Fatalf("agent ready emitted no data key")
	}
	if readyData == nil {
		t.Errorf("agent ready emitted data: null for an empty result; it is documented to emit []")
	}

	// blocked has no label filter, so this only pins the shape when the sandbox
	// genuinely has nothing blocked.
	blockedRes := h.MustRun("agent", "blocked", "-p", h.Project, "--limit", "100", "--json")
	items, _ := DecodeList[BlockedItem](t, blockedRes)
	if len(items) > 0 {
		t.Skip("the sandbox currently has blocked issues; the empty-shape check needs a quiet project")
	}

	var blockedRaw struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal([]byte(blockedRes.Stdout), &blockedRaw); err != nil {
		t.Fatalf("decode blocked: %v\n%s", err, blockedRes)
	}
	if string(blockedRaw.Data) == "null" {
		t.Logf("KNOWN DIVERGENCE: `agent blocked` emits \"data\": null for an empty result while " +
			"`agent ready` emits []. A consumer calling data.length breaks on blocked.")
	}
}
