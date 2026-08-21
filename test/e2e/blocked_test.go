//go:build e2e

package e2e

import "testing"

// TestE2E_BLOCK_01 — the blocked/unblock chain: a real "is blocked by" link
// removes an issue from the ready queue, surfaces it in `agent blocked` with
// its blocker, and closing the blocker returns it to ready and reports it as
// newly unblocked.
//
// Spec: docs/e2e-agent-sdlc-spec.md#e2e-block-01
func TestE2E_BLOCK_01(t *testing.T) {
	h := New(t)
	label := h.CaseLabel()

	target := h.Fixtures.Create(IssueSpec{Summary: "block target", Priority: "Medium", Labels: []string{label}})
	blocker := h.Fixtures.Create(IssueSpec{Summary: "the blocker", Priority: "High", Labels: []string{label}})
	free := h.Fixtures.Create(IssueSpec{Summary: "unencumbered", Priority: "Low", Labels: []string{label}})
	h.Fixtures.Block(target, blocker)
	h.Fixtures.WaitForIndexed(target.Key, blocker.Key, free.Key)

	steps := []struct {
		id string
		fn func(t *testing.T)
	}{
		{"E2E-BLOCK-01a-ready-excludes-the-blocked-issue", func(t *testing.T) {
			var keys []string
			Eventually(t, "ready to settle on the two unblocked issues", func() bool {
				res := h.MustRun("agent", "ready", "--json", "-p", h.Project, "-l", label, "--limit", "50")
				items, _ := DecodeList[ReadyItem](t, res)
				keys = ReadyKeys(items)
				return len(keys) == 2
			})

			if !EqualSets(keys, []string{blocker.Key, free.Key}) {
				t.Errorf("ready = %v, want exactly %v — the blocked %s must be excluded",
					keys, []string{blocker.Key, free.Key}, target.Key)
			}
		}},

		{"E2E-BLOCK-01b-blocked-reports-the-blocker", func(t *testing.T) {
			var mine []BlockedItem
			Eventually(t, "blocked to list the target", func() bool {
				res := h.MustRun("agent", "blocked", "--json", "-p", h.Project, "--limit", "100")
				all, _ := DecodeList[BlockedItem](t, res)
				mine = Filter(all, func(b BlockedItem) bool { return h.Fixtures.Owns(b.Key) })
				return len(mine) == 1
			})

			got := mine[0]
			if got.Key != target.Key {
				t.Fatalf("blocked reported %s, want %s", got.Key, target.Key)
			}
			if len(got.BlockedBy) != 1 {
				t.Fatalf("blocked_by = %+v, want exactly one blocker", got.BlockedBy)
			}
			if got.BlockedBy[0].Key != blocker.Key {
				t.Errorf("blocked_by[0].key = %q, want %q", got.BlockedBy[0].Key, blocker.Key)
			}
			// summary and status are only populated when issuelinks carries the
			// embedded fields, so an empty value here is a real regression.
			if got.BlockedBy[0].Summary == "" || got.BlockedBy[0].Status == "" {
				t.Errorf("blocked_by[0] = %+v, want summary and status to be populated", got.BlockedBy[0])
			}
		}},

		{"E2E-BLOCK-01c-status-blocked-count-agrees", func(t *testing.T) {
			res := h.MustRun("agent", "status", "--json", "-p", h.Project)
			doc := DecodeObject[StatusDoc](t, res)
			if doc.BlockedCount < 1 {
				t.Errorf("blocked_count = %d, want at least 1\n%s", doc.BlockedCount, res)
			}
		}},

		{"E2E-BLOCK-01d-close-blocker-reports-unblocked", func(t *testing.T) {
			res := h.MustRun("agent", "close", blocker.Key, "--reason", "blocker resolved", "--suggest-next", "--json")
			RequireNoStderr(t, res)

			doc := DecodeObject[MutationDoc](t, res)
			if !Contains(doc.Unblocked, target.Key) {
				t.Errorf("unblocked = %v, want it to contain %s\n%s", doc.Unblocked, target.Key, res)
			}
		}},

		{"E2E-BLOCK-01e-target-returns-to-ready", func(t *testing.T) {
			Eventually(t, "the unblocked target to reappear in ready", func() bool {
				res := h.MustRun("agent", "ready", "--json", "-p", h.Project, "-l", label, "--limit", "50")
				items, _ := DecodeList[ReadyItem](t, res)
				return Contains(ReadyKeys(items), target.Key)
			})

			res := h.MustRun("agent", "blocked", "--json", "-p", h.Project, "--limit", "100")
			all, _ := DecodeList[BlockedItem](t, res)
			if Contains(BlockedKeys(all), target.Key) {
				t.Errorf("blocked still lists %s after its only blocker was closed\n%s", target.Key, res)
			}
		}},
	}

	for _, s := range steps {
		if !t.Run(s.id, s.fn) {
			t.Fatalf("chain aborted at %s — the remaining steps would be meaningless", s.id)
		}
	}
}

// TestE2E_BLOCK_02 — an issue with two blockers is only reported as unblocked
// once every blocker is resolved.
//
// Spec: docs/e2e-agent-sdlc-spec.md#e2e-block-02
func TestE2E_BLOCK_02(t *testing.T) {
	h := New(t)
	label := h.CaseLabel()

	target := h.Fixtures.Create(IssueSpec{Summary: "doubly blocked", Labels: []string{label}})
	first := h.Fixtures.Create(IssueSpec{Summary: "first blocker", Labels: []string{label}})
	second := h.Fixtures.Create(IssueSpec{Summary: "second blocker", Labels: []string{label}})
	h.Fixtures.Block(target, first)
	h.Fixtures.Block(target, second)
	h.Fixtures.WaitForIndexed(target.Key, first.Key, second.Key)

	t.Run("E2E-BLOCK-02a-partial-unblock-reports-nothing", func(t *testing.T) {
		res := h.MustRun("agent", "close", first.Key, "--reason", "one down", "--suggest-next", "--json")
		doc := DecodeObject[MutationDoc](t, res)
		if Contains(doc.Unblocked, target.Key) {
			t.Errorf("unblocked = %v, want it NOT to contain %s — one blocker is still open\n%s",
				doc.Unblocked, target.Key, res)
		}
	})

	t.Run("E2E-BLOCK-02b-still-blocked", func(t *testing.T) {
		Eventually(t, "blocked to report the remaining blocker only", func() bool {
			res := h.MustRun("agent", "blocked", "--json", "-p", h.Project, "--limit", "100")
			all, _ := DecodeList[BlockedItem](t, res)
			for _, b := range all {
				if b.Key == target.Key {
					return len(b.BlockedBy) == 1 && b.BlockedBy[0].Key == second.Key
				}
			}
			return false
		})
	})

	t.Run("E2E-BLOCK-02c-final-unblock-reports-the-target", func(t *testing.T) {
		res := h.MustRun("agent", "close", second.Key, "--reason", "two down", "--suggest-next", "--json")
		doc := DecodeObject[MutationDoc](t, res)
		if !Contains(doc.Unblocked, target.Key) {
			t.Errorf("unblocked = %v, want it to contain %s — every blocker is now closed\n%s",
				doc.Unblocked, target.Key, res)
		}
	})
}

// TestE2E_BLOCK_03 — close --claim-next writes two JSON documents to stdout: the
// auto-claim mutation, then the close mutation. A consumer that unmarshals the
// whole buffer fails on the second one.
//
// Spec: docs/e2e-agent-sdlc-spec.md#e2e-block-03
func TestE2E_BLOCK_03(t *testing.T) {
	h := New(t)
	label := h.CaseLabel()

	target := h.Fixtures.Create(IssueSpec{Summary: "claim-next target", Labels: []string{label}})
	blocker := h.Fixtures.Create(IssueSpec{Summary: "claim-next blocker", Labels: []string{label}})
	h.Fixtures.Block(target, blocker)
	h.Fixtures.WaitForIndexed(target.Key, blocker.Key)

	res := h.MustRun("agent", "close", blocker.Key, "--claim-next", "--json")

	docs := DecodeDocs(t, res)
	if len(docs) != 2 {
		t.Fatalf("stdout carries %d JSON document(s), want 2 (claim then close)\n%s", len(docs), res)
	}

	claim := DecodeDocAt[MutationDoc](t, res, 0)
	closed := DecodeDocAt[MutationDoc](t, res, 1)

	if claim.Key != target.Key {
		t.Errorf("auto-claimed %s, want %s\n%s", claim.Key, target.Key, res)
	}
	if closed.Key != blocker.Key {
		t.Errorf("closed %s, want %s\n%s", closed.Key, blocker.Key, res)
	}
	if !Contains(closed.Unblocked, target.Key) {
		t.Errorf("unblocked = %v, want it to contain %s\n%s", closed.Unblocked, target.Key, res)
	}

	issue := h.Fixtures.GetIssue(target.Key, "assignee", "status")
	if issue.Fields.Assignee == nil || issue.Fields.Assignee.AccountID != h.Sandbox.AccountID {
		t.Errorf("%s was reported as auto-claimed but is not assigned to the caller", target.Key)
	}
}
