# E2E findings — agentic SDLC workflow

What the end-to-end suite found on its first runs against a live Jira Cloud
sprint. Every finding below was reproduced against the API directly, not
inferred from reading code.

**Run:** an ENDGAME sandbox instance, project `SCRUM` (team-managed, board 1,
active sprint `SCRUM Sprint 0`). First run: 30 passed, 7 skipped, 0 failed.

**Status: six of the nine are fixed.** After the fixes the same suite reports
**38 passed, 2 skipped, 0 failed** — the remaining skips need a second Jira
account and the manual sweep utility. Each fixed item is marked below, and its
E2E case now asserts the corrected behaviour rather than documenting the defect,
so a regression fails the suite.

The three still open all change a contract that clients depend on, so they are
deferred to the next major version rather than fixed in place.

The loop itself works. `ready → claim → status → discover → close → ready`
passed all ten steps against real Jira, including idempotent re-claim with an
unchanged `updated` timestamp, and the blocked/unblock chains. The findings are
about the edges around it.

Cases are defined in [e2e-agent-sdlc-spec.md](e2e-agent-sdlc-spec.md) and
implemented in `test/e2e/`.

---

## P0 — An invalid token is indistinguishable from an empty backlog — FIXED

**Case:** `E2E-ERR-05` · `test/e2e/errors_test.go`

Jira Cloud answers `POST /rest/api/3/search/jql` with **HTTP 200 and zero
issues** when the credentials are bad, instead of 401. Verified directly:

Sending both requests with a deliberately invalid API token:

```
POST /rest/api/3/search/jql   {"jql":"project = SCRUM", ...}
  → HTTP 200   {"issues":[],"isLast":true}

GET  /rest/api/3/myself
  → HTTP 401
```

The CLI relays that faithfully, so with an expired token the whole loop reports
healthy:

| command | exit | output |
|---|---|---|
| `agent ready` | **0** | `No ready issues found` |
| `agent blocked` | **0** | `No blocked issues found` |
| `agent status` | **0** | a full summary of zeroes |
| `agent claim SCRUM-1` | 4 | `Issue 'SCRUM-1' not found` — blames the issue, not the session |
| `auth status` | **0** | prints `Token: invalid`, but exits 0 |

**Impact.** An unattended agent with expired credentials idles forever believing
there is no work. Nothing in the loop separates "you are not authenticated" from
"there is nothing to do", and the one command that detects it cannot gate a
script because it exits 0. Token rotation turns every agent into a no-op
silently.

**Fixed.** `agent ready`, `blocked` and `status` now call
`shared.CheckEmptyResultsAuth` when a search returns nothing — the helper
`issue list`, `search` and `project list` already used. It probes `GET /myself`,
returns `AUTH_ERROR`, and treats a non-auth probe failure as a warning so a
legitimately empty result is never masked. The probe keys off the raw search
result, so a queue that empties through blocker filtering does not trigger it.

`auth status` gains an opt-in `--check` that turns an invalid token into an
error. The default exit code is unchanged, because scripts depend on it.

Verified live: all three now exit 2 with a bad token.

---

## P1 — Sprint features are blind to team-managed projects — FIXED

**Case:** `E2E-SPRINT-06` · `test/e2e/sprint_test.go`

`internal/api/agile.go:92` asks Jira for boards with `type=scrum`. A
team-managed project's board carries sprints exactly like a company-managed one
but reports its type as `simple`, so the filter matches nothing and
`GetActiveSprint` returns `(nil, nil)` — which callers read as "no sprint"
rather than as an error.

Verified against a project with a running sprint:

```
board 1 "SCRUM board"  type: simple
  sprint 2  "SCRUM Sprint 0"  state: active  ends 2026-09-04
  sprint 1  "SCRUM Sprint 1"  state: future

$ jira sprint active -p SCRUM     → exit 4, "No active sprint found"
$ jira sprint list -p SCRUM       → "No sprints found"
$ jira agent status -p SCRUM      → no Sprint line
$ jira agent prime -p SCRUM       → no "## Sprint:" section
$ jira agent ready --sprint active → works (goes through JQL, not the board API)
```

**Impact.** Team-managed is the default project type in Jira Cloud, so the
sprint-aware half of this feature does not work for most new projects. That
`ready --sprint active` still works makes the failure easy to miss: sprint
filtering appears functional while every sprint *display* is empty.

**Fixed.** Both filters now accept either type: the server-side one sends
`type=scrum,simple`, and `sprint list` uses the shared `api.IsSprintBoardType`
predicate. Five E2E cases that had been skipped now run.

The related gap — no way to put an issue into a sprint — is closed by the new
`jira sprint add`, which wraps `POST /rest/agile/1.0/sprint/{id}/issue`.

---

## P1 — `agent discover` hardcodes the sub-task type name — FIXED

**Case:** `E2E-DISCOVER-04` · `test/e2e/discover_test.go`

`internal/cmd/agent/discover.go:107` sets the issue type to the literal
`"Sub-task"`. Team-managed projects name that type `Subtask`. Since
`--as-subtask` defaults to true, the **default** invocation fails:

```
$ jira agent discover SCRUM-12 --title "found work"
exit 3
```

Confirmed instance-dependent: some projects report the type as `Sub-task` and
others as `Subtask`. The `SCRUM` project used for this run reports `Subtask`.

**Impact.** On a team-managed project, filing discovered work — a core step of
the loop — fails unless the caller knows to pass `--type` explicitly. The
`prime` output an agent is primed with does not mention that.

**Fixed.** `ResolveIssueTypeName` in `internal/cmd/agent/shared.go` reads the
project's create metadata and picks the type by its `subtask` flag, falling back
to the previous literal when the metadata cannot be read — a forbidden
`createmeta` should not stop the command from trying.

---

## P2 — Issue keys with a digit in the project part are rejected — FIXED

**Case:** `E2E-ERR-04` · `test/e2e/errors_test.go`

`internal/cmd/shared/validate.go:12` is `^[A-Za-z][A-Za-z]*-[0-9]+$`. Jira
permits digits in project keys after the first character, so a project called
`AB1` produces keys that `claim`, `close`, `discover` and `issue delete` all
reject with exit 3 **before any request** — while `agent ready -p AB1` returns
those very keys.

**Impact.** On such a project the loop hands an agent work it then refuses to
accept. `ValidateProjectKeyOrID` has the same gap: `^[A-Za-z]{2,}$`.

**Fixed.** The regexes are now `^[A-Za-z][A-Za-z0-9]*-[0-9]+$` for issue keys and
`^[A-Za-z][A-Za-z0-9]+$` for project keys. A leading digit is still rejected, as
Jira requires.

---

## P2 — `--sort` has no effect — FIXED

**Case:** `E2E-READY-03` · `test/e2e/ready_test.go`

`agent ready` builds an `ORDER BY` from `--sort`, then re-sorts the results
client-side by priority and creation date after filtering, discarding the JQL
ordering whenever priorities differ. Observed live: `--sort updated` returned
`[SCRUM-41 SCRUM-42]` (priority order) rather than `[SCRUM-42 SCRUM-41]`
(updated order).

**Impact.** The flag is advertised in `--help` and in the contract, and does
nothing.

**Fixed.** The client-side re-sort now runs only for the default. An explicit
`--sort` keeps the order Jira returned, and an unrecognised value is rejected
rather than silently falling through to the default.

---

## P3 — `status` counts disagree with the commands they summarise — PARTLY FIXED

**Cases:** `E2E-CONSIST-01`, `E2E-CONSIST-02` · `test/e2e/consistency_test.go`

`status.go:103` counts `ready_count` as every To Do issue project-wide with **no
blocker filtering**, so it disagrees with what `agent ready` returns. Observed
live: `ready_count = 7` while `agent ready` returned 6.

Separately, `in_progress_count` is scoped to `assignee = currentUser()`
(`status.go:79`) while `ready_count`, `blocked_count` and `done_today` are
project-wide. Four numbers in one summary, two different scopes, no labelling.

**Partly fixed.** `ready_count` keeps its meaning, because clients read it.
`status` now also reports **`actionable_count`** — the same figure `agent ready`
hands out, computed with the blocker filter in the pass that already fetches
those issues, so it costs no extra request. The help text states which counts
are project-wide and which are yours. Changing `ready_count` itself and renaming
`in_progress_count` are deferred.

---

## P3 — Empty results are shaped inconsistently — DEFERRED

**Case:** `E2E-CONSIST-03` · `test/e2e/consistency_test.go`

`agent ready` emits `"data": []` for an empty result; `agent blocked` emits
`"data": null`, as does `sprint list` when boards exist but hold no sprints, and
`status.my_work` when empty. A consumer calling `data.length` breaks on some
commands and not others.

**Recommended fix.** Initialise the slices — `make([]T, 0)` — at the three sites
that currently declare a nil slice.

---

## P3 — Pagination metadata is not truthful — DEFERRED

**Case:** `E2E-READY-02` · `test/e2e/ready_test.go`

`agent ready` reports `pagination.total` as the count **after** truncation and
always sets `has_next_page: false`. Observed live: 3 issues matched, `--limit 2`
returned `total: 2, has_next_page: false`.

It also fetches only `max(limit*3, 50)` issues before filtering, so in a busy
project a high-priority ready item can sit past the fetch window and never
appear, while the metadata insists there is no next page.

**Recommended fix.** Report the pre-truncation match count and set
`has_next_page` accordingly, or page properly through the search.

---

## P3 — A failed link is reported as success — PARTLY FIXED

**Case:** partially covered by `E2E-DISCOVER-03` · `test/e2e/discover_test.go`

When `discover --as-subtask=false` creates the issue but the link call fails,
the failure is downgraded to a stderr warning and the command exits 0 reporting
`"ok": true, "relationship": "linked"`. This contradicts the contract invariant
"always linked to parent, no orphan discoveries", and an agent reading the JSON
cannot detect the orphan.

**Partly fixed.** The payload now carries `link_failed: true` when the link call
fails, so a caller reading stdout can detect the orphan without parsing stderr.
`relationship` is unchanged and the command still exits 0, because both are
contracts clients depend on; failing outright is deferred.

---

## Not a defect, but worth knowing

The `Blocks` link direction is the reverse of the reading most people take from
Atlassian's example. Posting `{Blocks, inwardIssue: A, outwardIssue: B}` yields
**"B is blocked by A"** — the blocker goes in `inwardIssue`. Verified from both
sides against Jira Cloud.

`agent discover --link-type Blocks` posts `{inwardIssue: PARENT, outwardIssue:
NEW}`, which therefore makes the **discovered issue blocked by its parent**.
That is defensible for the loop, but it is worth stating in the contract, since
the opposite reading is the natural one.

---

## Coverage gaps

The suite skips 7 cases. These are honest gaps, not passes:

| Skipped | Why |
|---|---|
| `SPRINT-01/02/03/05`, `PRIME-03` | Unreachable while the team-managed board defect stands |
| `CLAIM-04/05` | Need `JIRA_E2E_OTHER` — a second account in the sandbox |
| `SWEEP` | Manual cleanup utility |

Also uncovered, with reasons in the spec: exit 6 `RATE_LIMITED` (the retry
transport absorbs it), exit 5 `PERMISSION_DENIED` (no reliable fixture), and
`INVALID_TRANSITION` (unreachable on a default workflow).

## What is left

Three items remain, all deferred because they change a contract clients already
depend on. They belong in the next major version, where the release tooling will
pick them up from a `BREAKING CHANGE` footer:

- Empty results emitting `null` instead of `[]` in `agent blocked`, `sprint list`
  and `status.my_work`.
- `pagination.total` reporting the returned count rather than the match count,
  `has_next_page` always false, and the `max(limit*3, 50)` fetch window that can
  hide actionable work.
- `ready_count` semantics and `in_progress_count` scoping, once `actionable_count`
  has bedded in.

Also still outstanding, and unrelated to these findings: PR #29 has no README
coverage for the `agent` and `sprint` command groups.
