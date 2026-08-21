# E2E findings — agentic SDLC workflow

What the end-to-end suite found on its first runs against a live Jira Cloud
sprint. Every finding below was reproduced against the API directly, not
inferred from reading code.

**Run:** `endgame-build.atlassian.net`, project `SCRUM` (team-managed, board 1,
active sprint `SCRUM Sprint 0`). 30 passed, 7 skipped, 0 failed, 140 seconds,
no leaked issues.

The loop itself works. `ready → claim → status → discover → close → ready`
passed all ten steps against real Jira, including idempotent re-claim with an
unchanged `updated` timestamp, and the blocked/unblock chains. The findings are
about the edges around it.

Cases are defined in [e2e-agent-sdlc-spec.md](e2e-agent-sdlc-spec.md) and
implemented in `test/e2e/`.

---

## P0 — An invalid token is indistinguishable from an empty backlog

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

**Recommended fix.** Two changes, both small:

1. Validate the session before trusting an empty search. In the agent commands,
   when a search returns zero issues, call `GetMyself` — which does return 401 —
   and surface `AUTH_ERROR` instead of an empty queue. One extra request only on
   the empty path, so the cost is nil in the common case.
2. Make `auth status` exit non-zero when the token is invalid, so it can be used
   as a gate in a hook or CI step.

---

## P1 — Sprint features are blind to team-managed projects

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

**Recommended fix.** Drop the `type=scrum` server-side filter in
`getScrumBoards` and accept boards of type `scrum` or `simple`. `sprint list`
does its filtering client-side at `list.go` and needs the same change. Consider
selecting boards by "has sprints" rather than by type, since that is the actual
requirement.

Related gap: **no command can add an issue to a sprint.** `issue create --field`
sends every value as a string (`internal/cmd/issue/create.go:183`) and Jira's
sprint field needs a number, and the Agile `POST /sprint/{id}/issue` endpoint is
not exposed. The e2e fixtures call that endpoint directly. Worth adding a
`jira sprint add` command.

---

## P1 — `agent discover` hardcodes the sub-task type name

**Case:** `E2E-DISCOVER-04` · `test/e2e/discover_test.go`

`internal/cmd/agent/discover.go:107` sets the issue type to the literal
`"Sub-task"`. Team-managed projects name that type `Subtask`. Since
`--as-subtask` defaults to true, the **default** invocation fails:

```
$ jira agent discover SCRUM-12 --title "found work"
exit 3
```

Confirmed instance-dependent: `CUSTOMER` on odevo uses `Sub-task`; `SCRUM`,
`TJS` and `QA` use `Subtask`.

**Impact.** On a team-managed project, filing discovered work — a core step of
the loop — fails unless the caller knows to pass `--type` explicitly. The
`prime` output an agent is primed with does not mention that.

**Recommended fix.** Resolve the sub-task type from `GetCreateMeta`, which
`agent prime` already calls, and fall back to the literal only if nothing is
marked `subtask: true`.

---

## P2 — Issue keys with a digit in the project part are rejected

**Case:** `E2E-ERR-04` · `test/e2e/errors_test.go`

`internal/cmd/shared/validate.go:12` is `^[A-Za-z][A-Za-z]*-[0-9]+$`. Jira
permits digits in project keys after the first character, so a project called
`AB1` produces keys that `claim`, `close`, `discover` and `issue delete` all
reject with exit 3 **before any request** — while `agent ready -p AB1` returns
those very keys.

**Impact.** On such a project the loop hands an agent work it then refuses to
accept. `ValidateProjectKeyOrID` has the same gap: `^[A-Za-z]{2,}$`.

**Recommended fix.** `^[A-Za-z][A-Za-z0-9]*-[0-9]+$` for issue keys and
`^[A-Za-z][A-Za-z0-9]+$` for project keys.

---

## P2 — `--sort` has no effect

**Case:** `E2E-READY-03` · `test/e2e/ready_test.go`

`agent ready` builds an `ORDER BY` from `--sort`, then re-sorts the results
client-side by priority and creation date after filtering, discarding the JQL
ordering whenever priorities differ. Observed live: `--sort updated` returned
`[SCRUM-41 SCRUM-42]` (priority order) rather than `[SCRUM-42 SCRUM-41]`
(updated order).

**Impact.** The flag is advertised in `--help` and in the contract, and does
nothing.

**Recommended fix.** Skip the client-side re-sort when `--sort` was given
explicitly, or drop the flag. Keep the priority sort as the default.

---

## P3 — `status` counts disagree with the commands they summarise

**Cases:** `E2E-CONSIST-01`, `E2E-CONSIST-02` · `test/e2e/consistency_test.go`

`status.go:103` counts `ready_count` as every To Do issue project-wide with **no
blocker filtering**, so it disagrees with what `agent ready` returns. Observed
live: `ready_count = 7` while `agent ready` returned 6.

Separately, `in_progress_count` is scoped to `assignee = currentUser()`
(`status.go:79`) while `ready_count`, `blocked_count` and `done_today` are
project-wide. Four numbers in one summary, two different scopes, no labelling.

**Recommended fix.** Apply the same blocker filter to `ready_count`, and either
scope all four counts consistently or rename the field to `my_in_progress_count`.

---

## P3 — Empty results are shaped inconsistently

**Case:** `E2E-CONSIST-03` · `test/e2e/consistency_test.go`

`agent ready` emits `"data": []` for an empty result; `agent blocked` emits
`"data": null`, as does `sprint list` when boards exist but hold no sprints, and
`status.my_work` when empty. A consumer calling `data.length` breaks on some
commands and not others.

**Recommended fix.** Initialise the slices — `make([]T, 0)` — at the three sites
that currently declare a nil slice.

---

## P3 — Pagination metadata is not truthful

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

## P3 — A failed link is reported as success

**Case:** partially covered by `E2E-DISCOVER-03` · `test/e2e/discover_test.go`

When `discover --as-subtask=false` creates the issue but the link call fails,
the failure is downgraded to a stderr warning and the command exits 0 reporting
`"ok": true, "relationship": "linked"`. This contradicts the contract invariant
"always linked to parent, no orphan discoveries", and an agent reading the JSON
cannot detect the orphan.

**Recommended fix.** Either fail the command, or report the real state —
`"relationship": "unlinked"` plus a `link_error` field — so a caller can react.

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

## Suggested order of work

1. **P0 auth blindness** — it silently disables every agent, and the fix is small.
2. **P1 team-managed boards** — restores the sprint half of the feature for the default project type, and unskips 5 cases.
3. **P1 hardcoded `Sub-task`** — unblocks `discover` on team-managed projects.
4. **P2 key regex and `--sort`** — both are small and remove misleading behaviour.
5. **P3 shape and count consistency** — worth doing together, since they all affect what an agent can rely on in the JSON.

Add `jira sprint add` alongside item 2, and give the README the `agent` and
`sprint` sections it currently lacks.
