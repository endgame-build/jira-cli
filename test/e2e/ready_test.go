//go:build e2e

package e2e

import "testing"

// TestE2E_READY_01 — the filter flags narrow the queue as documented.
//
// Every assertion pushes a per-case label into the command, so the expected set
// is exactly this test's fixtures even though the sandbox project is shared.
//
// Spec: docs/e2e-agent-sdlc-spec.md#e2e-ready-01
func TestE2E_READY_01(t *testing.T) {
	h := New(t)
	label := h.CaseLabel()
	extra := label + "x"

	tagged := h.Fixtures.Create(IssueSpec{
		Summary:  "ready tagged",
		Priority: "Highest",
		Labels:   []string{label, extra},
	})
	unassigned := h.Fixtures.Create(IssueSpec{
		Summary:  "ready unassigned",
		Priority: "High",
		Labels:   []string{label},
	})
	mine := h.Fixtures.Create(IssueSpec{
		Summary:  "ready mine",
		Priority: "Low",
		Labels:   []string{label},
		AssignMe: true,
	})
	h.Fixtures.WaitForIndexed(tagged.Key, unassigned.Key, mine.Key)

	Eventually(t, "all three fixtures in the ready queue", func() bool {
		res := h.MustRun("agent", "ready", "-p", h.Project, "-l", label, "--limit", "50", "--json")
		items, _ := DecodeList[ReadyItem](t, res)
		return len(items) == 3
	})

	cases := []struct {
		id   string
		args []string
		want []string
	}{
		{
			id:   "E2E-READY-01a-labels-are-ANDed",
			args: []string{"-l", label, "-l", extra},
			want: []string{tagged.Key},
		},
		{
			id:   "E2E-READY-01b-unassigned",
			args: []string{"-l", label, "--unassigned"},
			want: []string{tagged.Key, unassigned.Key},
		},
		{
			id:   "E2E-READY-01c-assignee-me",
			args: []string{"-l", label, "--assignee", "@me"},
			want: []string{mine.Key},
		},
		{
			id:   "E2E-READY-01d-priority",
			args: []string{"-l", label, "--priority", "High"},
			want: []string{unassigned.Key},
		},
	}

	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			args := append([]string{"agent", "ready", "-p", h.Project, "--limit", "50", "--json"}, tc.args...)
			res := h.MustRun(args...)
			items, _ := DecodeList[ReadyItem](t, res)
			if got := ReadyKeys(items); !EqualSets(got, tc.want) {
				t.Errorf("ready = %v, want %v\n%s", got, tc.want, res)
			}
		})
	}
}

// TestE2E_READY_02 — --limit truncates, and pagination reports the truncated
// count with has_next_page false even though more issues match.
//
// Spec: docs/e2e-agent-sdlc-spec.md#e2e-ready-02
func TestE2E_READY_02(t *testing.T) {
	h := New(t)
	label := h.CaseLabel()

	for i := 0; i < 3; i++ {
		h.Fixtures.Create(IssueSpec{Summary: "limit fixture", Labels: []string{label}})
	}

	Eventually(t, "all three fixtures to be indexed", func() bool {
		res := h.MustRun("agent", "ready", "-p", h.Project, "-l", label, "--limit", "50", "--json")
		items, _ := DecodeList[ReadyItem](t, res)
		return len(items) == 3
	})

	// The limited query needs the same index-lag tolerance as the query above:
	// a search that has just seen three issues can still answer the next one
	// from a lagging replica.
	var res Result
	var items []ReadyItem
	var page *Pagination
	Eventually(t, "the limited query to see the fixtures", func() bool {
		res = h.MustRun("agent", "ready", "-p", h.Project, "-l", label, "--limit", "2", "--json")
		items, page = DecodeList[ReadyItem](t, res)
		return len(items) == 2
	})
	if page == nil {
		t.Fatalf("no pagination envelope\n%s", res)
	}
	if page.Limit != 2 {
		t.Errorf("pagination.limit = %d, want 2", page.Limit)
	}
	if page.Total == nil || *page.Total != 2 {
		t.Errorf("pagination.total = %v, want the post-truncation count 2", page.Total)
	}
	if page.HasNextPage {
		t.Errorf("has_next_page = true; it is hardcoded false today")
	} else {
		t.Logf("NOTE: 3 issues match but has_next_page is false and total reports the truncated "+
			"count (%d). A caller paging on this metadata never learns there is more work.", *page.Total)
	}
}

// TestE2E_READY_03 — --sort is discarded. The JQL orders by the requested field,
// but the results are re-sorted client-side by priority and creation date
// afterwards (internal/cmd/agent/ready.go), so --sort updated has no effect
// whenever priorities differ.
//
// Spec: docs/e2e-agent-sdlc-spec.md#e2e-ready-03
func TestE2E_READY_03(t *testing.T) {
	h := New(t)
	label := h.CaseLabel()

	first := h.Fixtures.Create(IssueSpec{Summary: "sort highest", Priority: "Highest", Labels: []string{label}})
	last := h.Fixtures.Create(IssueSpec{Summary: "sort low", Priority: "Low", Labels: []string{label}})
	h.Fixtures.WaitForIndexed(first.Key, last.Key)

	// Touch the low-priority issue so it is unambiguously the most recently
	// updated of the two.
	h.MustRun("issue", "edit", last.Key, "--summary", last.Summary+" (touched)", "--json")

	var keys []string
	Eventually(t, "both fixtures in the ready queue", func() bool {
		res := h.MustRun("agent", "ready", "-p", h.Project, "-l", label,
			"--sort", "updated", "--limit", "50", "--json")
		items, _ := DecodeList[ReadyItem](t, res)
		keys = ReadyKeys(items)
		return len(keys) == 2
	})

	if EqualOrdered(keys, []string{last.Key, first.Key}) {
		t.Logf("--sort updated is honoured; the client-side re-sort documented for this case " +
			"may have been removed")
		return
	}

	if !EqualOrdered(keys, []string{first.Key, last.Key}) {
		t.Errorf("unexpected order %v; wanted either priority order %v or updated order %v",
			keys, []string{first.Key, last.Key}, []string{last.Key, first.Key})
		return
	}

	t.Logf("KNOWN DEFECT: --sort updated returned priority order %v, not updated order %v. "+
		"The JQL ORDER BY is overwritten by a client-side re-sort, so --sort is advertised in "+
		"--help but has no effect when priorities differ.", keys, []string{last.Key, first.Key})
}
