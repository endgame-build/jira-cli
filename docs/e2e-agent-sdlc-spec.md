# E2E test spec — agentic SDLC workflow

Numbered cases proving that `jira agent` and `jira sprint` work against a real
Jira Cloud sprint. Every case has a Go test named after its ID in
`test/e2e/`; chain steps are subtests named after their sub-ID.

Read `test/e2e/README.md` first for sandbox setup. The contracts these cases
assert against are in `docs/agent-sdlc-contracts.md`.

## Why this exists

The `agent` command group ships with unit tests only. Those prove the code
parses its own mocks. They cannot catch JQL that Jira rejects, transitions
absent from a real workflow, an inverted issue-link direction, board resolution
failing, or the loop deadlocking because `close --suggest-next` surfaces
nothing. All three design documents for this feature punt real-instance
verification to a manual step that was never automated.

## Conventions

| | |
|---|---|
| **Fixtures** | Created through the REST API, never through the CLI, so a bug cannot build the fixture that hides it |
| **Scoping** | Each case pushes its own label into the command and compares exact sets; no absolute counts |
| **Index lag** | Reads through JQL search are wrapped in `Eventually` with a deadline |
| **Cleanup** | `t.Cleanup` per issue, plus an end-of-run sweep that fails the run on a leak |
| **Chains** | Ordered subtests sharing one fixture, aborting at the first failed step |

Exit codes asserted throughout: 0 ok, 2 auth, 3 validation, 4 not found,
5 permission, 6 rate limited, 7 network, 8 conflict.

---

## E2E-LOOP-01 — the full loop

The centrepiece. Three seeded issues spanning the priority range, all in the
active sprint. Each step consumes the previous step's output.

| Step | Command | Asserts |
|---|---|---|
| **a** | `agent ready -l <case> --limit 50` | Exactly the three seeds, ordered Highest → Medium → Low; every item's status category is `new`; `pagination.total` is 3 |
| **b** | `agent ready --sprint active` | The sprint filter returns the same three |
| **c** | `agent claim <high>` | `ok`, not a no-op, assignee is the caller; re-read through the API confirms an `indeterminate` status and the assignment |
| **d** | `agent claim <high>` again | `noop: true`, status unchanged, and **`updated` is byte-identical** — the zero-write proof |
| **e** | `agent status` | `my_work` contains the claimed key; `in_progress_count` ≥ 1; the sprint block names the active sprint |
| **f** | `agent discover <high> --type <subtask>` | `relationship: subtask`, parent is correct, child inherits the case label and gains `discovered`, parent gains a discovery comment |
| **g** | `agent close <high> --reason` | Status moves to the `done` category; exactly one comment is added |
| **h** | `agent ready` | The closed issue is gone; the two untouched seeds remain |
| **i** | `agent close <high>` again | Idempotent: status unchanged, **no second comment** |
| **j** | `agent claim <high>` | Exit 3 `VALIDATION_ERROR` — a Done issue cannot be claimed |

`--type` is passed explicitly in step f because `agent discover` hardcodes the
type name `Sub-task`; see **E2E-DISCOVER-04**.

## E2E-BLOCK-01 — blocked and unblock

A real `Blocks` link, created via the API so its direction is verified before
any assertion depends on it.

| Step | Asserts |
|---|---|
| **a** | `agent ready` returns exactly the blocker and the unencumbered issue — the blocked one is excluded |
| **b** | `agent blocked` lists the target with exactly one `blocked_by` entry, whose `summary` and `status` are populated |
| **c** | `status.blocked_count` ≥ 1 |
| **d** | `close <blocker> --suggest-next` reports the target in `unblocked` |
| **e** | The target returns to `agent ready` and leaves `agent blocked` |

## E2E-BLOCK-02 — partial unblock

Two blockers on one target. Closing the first must **not** report the target as
unblocked; `agent blocked` must then show one remaining blocker; closing the
second must report it.

## E2E-BLOCK-03 — two JSON documents

`close --claim-next --json` writes **two concatenated JSON documents** to
stdout: the claim mutation, then the close mutation. Asserts both parse, that
the right issue was auto-claimed, and that the claim really happened — verified
through the API, not from the output that claimed it.

## E2E-CLAIM-01…05 — the idempotency matrix

| ID | Precondition | Expected |
|---|---|---|
| **01** | To Do, unassigned | Claims and transitions |
| **02** | Mine, In Progress | `noop: true`, `updated` unchanged |
| **03** | Mine, still To Do | **Not** a no-op — the guard needs both conditions, so it transitions |
| **04** | Assigned to another user | Exit 8 `CONFLICT_ERROR`, empty stdout, `updated` unchanged |
| **05** | Same, with `--force` | Reassigns to the caller |

04 and 05 need `JIRA_E2E_OTHER`; they skip without it, naming the variable.

## E2E-READY-01…03 — the ready queue

- **01** — `--label` (ANDed), `--unassigned`, `--assignee @me`, `--priority` each narrow the set exactly.
- **02** — `--limit` truncates; `pagination.total` reports the **truncated** count and `has_next_page` is false even though more match. A caller paging on this metadata never learns there is more work.
- **03** — `--sort updated` is discarded. Documented below.

## E2E-DISCOVER-01…04

- **01** — Inherits project, priority and labels; appends `discovered`; comments on the parent.
- **02** — Explicit `--label` **replaces** the parent's labels rather than adding to them.
- **03** — `--as-subtask=false` creates a linked issue. The dry-run payload says `linked (Relates)` where the success payload says `linked`.
- **04** — The default invocation with no `--type`. Documented below.

## E2E-SPRINT-01…05

- **01** — `sprint list` finds the board's sprints; dates are raw ISO timestamps.
- **02** — `--board N` resolves no project at all; without either, exit 3.
- **03** — `sprint active` emits a bare object with no envelope; `remaining_days` ≥ 0.
- **04** — `ready --sprint active` narrows to sprint members; `future` returns nothing; filtering by sprint name matches `active`.
- **05** — `status` truncates `end_date` to `YYYY-MM-DD` where `sprint active` emits the full timestamp; the two must agree on the date.

## E2E-PRIME-01…05

- **01** — The documented markdown anchors are present, the project key is interpolated, and `## Extended Reference` is absent without `--full`.
- **02** — `--full` output is a strict superset, adding exactly one section.
- **03** — With an active sprint, the `## Sprint:` section exists and Session Protocol step 1 includes `--sprint active`.
- **04** — `--json`, `--jq`, `--quiet` and `--dry-run` are all ignored: output is byte-identical markdown every time.
- **05** — Statuses and types are the project's real ones, covering all three status categories. Also the cross-check for E2E-DISCOVER-04.

## E2E-DRY-01…03

- **01** — `--dry-run` on `claim`, `close` and `discover` changes nothing, verified by re-reading `updated`, the comment count and the sub-task list through the API.
- **02** — The read-only commands ignore the flag and emit no dry-run envelope, so nobody assumes it is a universal guard.
- **03** — The claim pre-flight guards run *before* the dry-run branch, making `--dry-run claim` a usable validator.

## E2E-ERR-01…04 — exit codes

- **01** — Unknown key → 4; malformed key → 3; `--assignee` with `--unassigned` → 3; `discover` without `--title` → 3. All write to stderr only, with a decodable error document.
- **02** — Both flag guards fire **before any HTTP request**, proved by pointing the CLI at an unroutable host: a guard that fired exits 3, one that did not exits 7.
- **03** — A bad token exits 2; an unresolvable host exits 7.
- **04** — Digit-bearing project keys. Documented below.

## E2E-CONSIST-01…03 — known divergences

These record behaviour that is inconsistent rather than wrong-by-assertion.
They log their findings and do not fail, so that fixing the underlying issue
does not read as a regression.

- **01** — `status.ready_count` counts every To Do issue project-wide with no blocker filtering, so it disagrees with what `agent ready` returns.
- **02** — `status.in_progress_count` is scoped to the calling user while `ready_count`, `blocked_count` and `done_today` are project-wide. Four numbers in one summary, three scopes.
- **03** — `agent ready` emits `"data": []` for an empty result; `agent blocked` emits `"data": null`. A consumer calling `data.length` breaks on one of them.

---

## Defects these cases document

Found by inspection and confirmed against a live instance. Per the agreed scope
these are reported, not fixed.

### Issue keys with a digit in the project part are rejected

`internal/cmd/shared/validate.go:12` is `^[A-Za-z][A-Za-z]*-[0-9]+$`. Jira
allows digits in project keys after the first character, so a project called
`AB1` produces keys `AB1-23` that `claim`, `close`, `discover` and
`issue delete` all reject with exit 3 **before any request** — while
`agent ready -p AB1` happily returns those very keys.

The loop becomes: here is your work → that is not a valid key.

Covered by **E2E-ERR-04**, and the reason the sandbox key must be letters only.

### `agent discover` hardcodes the sub-task type name

`internal/cmd/agent/discover.go:107` sets the type to the literal `"Sub-task"`.
Team-managed Jira Cloud projects name that type `Subtask`. Since `--as-subtask`
defaults to true, the **default** invocation of `agent discover` fails on any
team-managed project.

Confirmed on `odevo.atlassian.net`: `CUSTOMER` uses `Sub-task`, while `TJS` and
`QA` use `Subtask`. The fix is to read the name from `GetCreateMeta`, which
`agent prime` already fetches.

Covered by **E2E-DISCOVER-04**, which adapts to either naming.

### `--sort` has no effect

`agent ready` builds an `ORDER BY` from `--sort`, then re-sorts the results
client-side by priority and creation date after filtering. The JQL ordering is
discarded whenever priorities differ, so `--sort updated` and `--sort created`
are advertised in `--help` and in the contract but do nothing.

Covered by **E2E-READY-03**.

### An orphaned discovery is reported as success

When `discover --as-subtask=false` creates the issue but the link call fails,
the failure is downgraded to a stderr warning and the command still exits 0
reporting `"ok": true, "relationship": "linked"`. This contradicts the contract
invariant "always linked to parent, no orphan discoveries", and an agent reading
the JSON has no way to detect the orphan.

Partially covered by **E2E-DISCOVER-03**, which asserts the link exists.

### Pagination metadata is not truthful

`agent ready` reports `pagination.total` as the count *after* truncation and
always sets `has_next_page: false`. It also fetches only `max(limit*3, 50)`
issues before filtering, so in a project with many open issues a high-priority
ready item can be permanently invisible while the metadata says there is no next
page.

Covered by **E2E-READY-02**.

## Not covered, and why

| | |
|---|---|
| **Exit 6 `RATE_LIMITED`** | The retry transport retries 429 up to three times honouring `Retry-After`, so surfacing the code requires four consecutive rate-limits. Covered by unit tests instead |
| **Exit 5 `PERMISSION_DENIED`** | Needs a restricted issue or a second token; no reliable sandbox setup |
| **`INVALID_TRANSITION`** | Needs a workflow with no transition into a status category. Unreachable on a default Scrum workflow; covered by unit tests |
| **Sprint membership as a CLI operation** | No command can add an issue to a sprint. `issue create --field` sends every value as a string and Jira's sprint field needs a number, so fixtures use the Agile API directly. Filed as a follow-up |
