//go:build e2e

package e2e

import (
	"strconv"
	"strings"
	"testing"
)

// TestE2E_SPRINT_01 — `sprint list` finds the board's sprints and emits raw ISO
// timestamps.
//
// Spec: docs/e2e-agent-sdlc-spec.md#e2e-sprint-01
func TestE2E_SPRINT_01(t *testing.T) {
	h := New(t)
	requireCLISeesBoard(t, h)

	res := h.MustRun("sprint", "list", "-p", h.Project, "--json")
	items, page := DecodeList[SprintListItem](t, res)

	if len(items) == 0 {
		t.Fatalf("sprint list returned nothing for a project with a Scrum board\n%s", res)
	}

	var active *SprintListItem
	for i := range items {
		if items[i].State == "active" {
			active = &items[i]
			break
		}
	}
	if active == nil {
		t.Fatalf("no active sprint in the list, but preflight found one\n%s", res)
	}
	if active.Name != h.Sandbox.Sprint.Name {
		t.Errorf("active sprint = %q, want %q", active.Name, h.Sandbox.Sprint.Name)
	}
	if active.BoardID == 0 {
		t.Errorf("board_id = 0, want the board the sprint was fetched from")
	}
	// sprint list emits the raw ISO value; agent status truncates to a date.
	if active.EndDate != "" && len(active.EndDate) <= 10 {
		t.Errorf("end_date = %q, want the full ISO timestamp here", active.EndDate)
	}
	if page == nil {
		t.Errorf("sprint list emitted no pagination envelope\n%s", res)
	}
}

// TestE2E_SPRINT_02 — `sprint list --board N` resolves no project at all, so it
// works with neither -p nor a configured default.
//
// Spec: docs/e2e-agent-sdlc-spec.md#e2e-sprint-02
func TestE2E_SPRINT_02(t *testing.T) {
	h := New(t)
	requireCLISeesBoard(t, h)

	withBoard := h.MustRun("sprint", "list", "--board", strconv.Itoa(h.Sandbox.Board.ID), "--json")
	items, _ := DecodeList[SprintListItem](t, withBoard)
	if len(items) == 0 {
		t.Errorf("--board returned nothing for board %d\n%s", h.Sandbox.Board.ID, withBoard)
	}

	// Without --board and without a project, resolution must fail.
	noProject := h.RunExpectExit(3, "sprint", "list", "--json")
	DecodeError(t, noProject, "VALIDATION_ERROR")
}

// TestE2E_SPRINT_03 — `sprint active` emits a bare object and a non-negative
// remaining_days.
//
// Spec: docs/e2e-agent-sdlc-spec.md#e2e-sprint-03
func TestE2E_SPRINT_03(t *testing.T) {
	h := New(t)
	requireCLISeesBoard(t, h)

	res := h.MustRun("sprint", "active", "-p", h.Project, "--json")
	doc := DecodeObject[SprintDoc](t, res)

	if doc.State != "active" {
		t.Errorf("state = %q, want %q\n%s", doc.State, "active", res)
	}
	if doc.Name != h.Sandbox.Sprint.Name {
		t.Errorf("name = %q, want %q", doc.Name, h.Sandbox.Sprint.Name)
	}
	// remaining_days is wall-clock derived, so only its sign is contractual.
	if doc.RemainingDays < 0 {
		t.Errorf("remaining_days = %d, want >= 0", doc.RemainingDays)
	}

	raw := DecodeRaw(t, res)
	for _, envelopeKey := range []string{"data", "ok", "pagination"} {
		if _, found := raw[envelopeKey]; found {
			t.Errorf("sprint active emitted a %q key; it is documented as a bare object", envelopeKey)
		}
	}
}

// TestE2E_SPRINT_04 — `ready --sprint active` narrows the queue to sprint
// members, and a future-sprint filter returns nothing.
//
// Spec: docs/e2e-agent-sdlc-spec.md#e2e-sprint-04
func TestE2E_SPRINT_04(t *testing.T) {
	h := New(t)
	label := h.CaseLabel()

	inSprint := h.Fixtures.Create(IssueSpec{Summary: "sprint member", Labels: []string{label}, InSprint: true})
	backlog := h.Fixtures.Create(IssueSpec{Summary: "backlog only", Labels: []string{label}})
	h.Fixtures.WaitForIndexed(inSprint.Key, backlog.Key)

	t.Run("E2E-SPRINT-04a-unfiltered-sees-both", func(t *testing.T) {
		Eventually(t, "both fixtures in the unfiltered ready queue", func() bool {
			res := h.MustRun("agent", "ready", "-p", h.Project, "-l", label, "--limit", "50", "--json")
			items, _ := DecodeList[ReadyItem](t, res)
			return len(items) == 2
		})
	})

	t.Run("E2E-SPRINT-04b-active-filter-narrows", func(t *testing.T) {
		Eventually(t, "the sprint filter to narrow to the sprint member", func() bool {
			res := h.MustRun("agent", "ready", "-p", h.Project, "-l", label,
				"--sprint", "active", "--limit", "50", "--json")
			items, _ := DecodeList[ReadyItem](t, res)
			return EqualSets(ReadyKeys(items), []string{inSprint.Key})
		})
	})

	t.Run("E2E-SPRINT-04c-future-filter-is-empty", func(t *testing.T) {
		res := h.MustRun("agent", "ready", "-p", h.Project, "-l", label,
			"--sprint", "future", "--limit", "50", "--json")
		items, _ := DecodeList[ReadyItem](t, res)
		if len(items) != 0 {
			t.Errorf("future-sprint filter returned %v, want nothing\n%s", ReadyKeys(items), res)
		}
	})

	t.Run("E2E-SPRINT-04d-filter-by-sprint-name", func(t *testing.T) {
		res := h.MustRun("agent", "ready", "-p", h.Project, "-l", label,
			"--sprint", h.Sandbox.Sprint.Name, "--limit", "50", "--json")
		items, _ := DecodeList[ReadyItem](t, res)
		if !EqualSets(ReadyKeys(items), []string{inSprint.Key}) {
			t.Errorf("name filter = %v, want %v\n%s", ReadyKeys(items), []string{inSprint.Key}, res)
		}
	})
}

// TestE2E_SPRINT_05 — `agent status` carries the sprint block, truncating
// end_date to a date where `sprint active` emits the raw timestamp.
//
// Spec: docs/e2e-agent-sdlc-spec.md#e2e-sprint-05
func TestE2E_SPRINT_05(t *testing.T) {
	h := New(t)
	requireCLISeesBoard(t, h)

	statusRes := h.MustRun("agent", "status", "-p", h.Project, "--json")
	status := DecodeObject[StatusDoc](t, statusRes)
	if status.Sprint == nil {
		t.Fatalf("status omitted the sprint block\n%s", statusRes)
	}
	if status.Sprint.Name != h.Sandbox.Sprint.Name {
		t.Errorf("status sprint = %q, want %q", status.Sprint.Name, h.Sandbox.Sprint.Name)
	}
	if len(status.Sprint.EndDate) != 10 {
		t.Errorf("status end_date = %q, want it truncated to YYYY-MM-DD", status.Sprint.EndDate)
	}

	activeRes := h.MustRun("sprint", "active", "-p", h.Project, "--json")
	active := DecodeObject[SprintDoc](t, activeRes)
	if len(active.EndDate) >= 10 && !strings.HasPrefix(active.EndDate, status.Sprint.EndDate) {
		t.Errorf("end_date disagrees between commands: status %q, sprint active %q",
			status.Sprint.EndDate, active.EndDate)
	}
}

// requireCLISeesBoard skips a case that depends on the CLI's board lookup,
// naming the defect that makes it unreachable.
func requireCLISeesBoard(t *testing.T, h *Harness) {
	t.Helper()
	if !h.Sandbox.CLISeesBoard {
		t.Skipf("this project's board reports type %q, which the CLI's board lookup filters out; "+
			"see TestE2E_SPRINT_06", h.Sandbox.Board.Type)
	}
}

// TestE2E_SPRINT_06 — every board-derived sprint feature is invisible on a
// team-managed project.
//
// internal/api/agile.go asks Jira for boards with type=scrum. A team-managed
// project's board carries sprints exactly like a company-managed one but
// reports its type as "simple", so the filter matches nothing and
// GetActiveSprint returns (nil, nil) — which the callers read as "no sprint"
// rather than as an error.
//
// The result is that `sprint active` exits 4, `sprint list` prints nothing, and
// the sprint blocks vanish from `agent status` and `agent prime`, on a project
// that demonstrably has a running sprint. Only `ready --sprint active` still
// works, because it goes through JQL rather than the board API.
//
// Team-managed is the default project type in Jira Cloud today, so the
// sprint-aware half of this feature does not work for most new projects.
//
// This case reads the truth from the Agile API directly and compares it with
// what the CLI reports.
//
// Spec: docs/e2e-agent-sdlc-spec.md#e2e-sprint-06
func TestE2E_SPRINT_06(t *testing.T) {
	h := New(t)

	// Established by preflight straight from the Agile API.
	realSprint := h.Sandbox.Sprint
	if realSprint.Name == "" {
		t.Fatal("preflight found no active sprint; this case needs one")
	}

	active := h.Run("sprint", "active", "-p", h.Project, "--json")

	if h.Sandbox.CLISeesBoard {
		if active.ExitCode != 0 {
			t.Errorf("board type is %q but `sprint active` still failed with exit %d\n%s",
				h.Sandbox.Board.Type, active.ExitCode, active)
		}
		return
	}

	if active.ExitCode == 0 {
		t.Errorf("board type is %q, so the CLI's type=scrum filter should have hidden it, "+
			"but `sprint active` succeeded — the filter may have been fixed, in which case "+
			"delete this case", h.Sandbox.Board.Type)
		return
	}

	if active.ExitCode != 4 {
		t.Errorf("`sprint active` exit = %d, want 4 (NOT_FOUND)\n%s", active.ExitCode, active)
	}

	// The same blindness must show up in every other board-derived surface.
	status := DecodeObject[StatusDoc](t, h.MustRun("agent", "status", "-p", h.Project, "--json"))
	if status.Sprint != nil {
		t.Errorf("status reported a sprint block though the board lookup found nothing: %+v", status.Sprint)
	}

	prime := h.MustRun("agent", "prime", "-p", h.Project).Stdout
	if strings.Contains(prime, "## Sprint:") {
		t.Errorf("prime emitted a sprint section though the board lookup found nothing")
	}

	list, _ := DecodeList[SprintListItem](t, h.MustRun("sprint", "list", "-p", h.Project, "--json"))
	if len(list) != 0 {
		t.Errorf("sprint list returned %d sprint(s) though the board lookup found nothing", len(list))
	}

	// The JQL path is unaffected, which is what makes the failure so easy to miss.
	ready := h.MustRun("agent", "ready", "-p", h.Project, "--sprint", "active", "--limit", "50", "--json")
	if ready.ExitCode != 0 {
		t.Errorf("`ready --sprint active` also failed; the JQL path was expected to still work\n%s", ready)
	}

	t.Logf("KNOWN DEFECT: board %d (%q) has active sprint %q ending %s, but its type is %q. "+
		"internal/api/agile.go filters boards with type=scrum, so `sprint active` exits 4, "+
		"`sprint list` is empty, and the sprint blocks are missing from `agent status` and "+
		"`agent prime`. Team-managed is the default project type in Jira Cloud.",
		h.Sandbox.Board.ID, h.Sandbox.Board.Name, realSprint.Name, realSprint.EndDate, h.Sandbox.Board.Type)
}
