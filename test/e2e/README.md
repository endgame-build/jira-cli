# End-to-end tests

These tests drive the built `jira` binary against a **real Jira Cloud project**.
They create, claim, link, comment on, close and delete issues.

They are gated three ways and cannot run by accident:

1. The `e2e` build tag — `go test ./...` compiles nothing here.
2. `JIRA_E2E=1` — without it every test skips.
3. `testing.Short()` — the pre-commit hook runs `go test -short`, so the suite stays inert there.

## Sandbox setup

Create a **dedicated** project. Do not point these tests at a real team's board.

1. **Create a Scrum project whose key is letters only** — `AGENTLAB`, not `AGENT2`.

   A key containing a digit makes every `claim`, `close`, `discover` and
   `issue delete` call fail with exit 3 before any HTTP request, because
   `internal/cmd/shared/validate.go:12` matches `^[A-Za-z][A-Za-z]*-[0-9]+$`.
   Jira itself permits digits; the CLI does not.

2. **Start a sprint** on its board, with an end date at least a day out.
   `preflight` requires exactly one active sprint.

3. Check the project has a `Task` type, a sub-task type, and the priorities
   `Highest` through `Lowest`.

   Note which name the project uses for sub-tasks. Company-managed projects say
   `Sub-task`; team-managed projects say `Subtask`. `agent discover` hardcodes
   `Sub-task`, so on a team-managed project its default invocation fails —
   `TestE2E_DISCOVER_04` reports this.

4. Confirm the workflow offers transitions into both an In Progress and a Done
   status. `claim` and `close` resolve transitions by status *category*, not by
   name, so a workflow missing either one fails with `INVALID_TRANSITION`.

5. Create an API token at <https://id.atlassian.com/manage-profile/security/api-tokens>.

## Running

```sh
export JIRA_INSTANCE=yourco.atlassian.net
export JIRA_USER=you@yourco.com
export JIRA_TOKEN=...            # never commit this
export JIRA_E2E_PROJECT=AGENTLAB

make test-e2e                                    # everything
make test-e2e-one E2E_RUN=TestE2E_LOOP_01        # one chain
make e2e-sweep                                   # delete every e2e issue in the sandbox
```

Optional:

- `JIRA_E2E_OTHER` — a second account ID in the project. Enables the
  `CONFLICT_ERROR` cases, which skip without it.
- `JIRA_E2E_BIN` — path to a prebuilt binary, so the suite skips its own build.

Expect 8–15 minutes. The suite runs serially and paces its calls, because it
authenticates as one user and Jira Cloud rate limits per user.

## How it stays honest

**Fixtures are built through the REST API, never through the CLI.** If
`agent discover` built the fixture for a test asserting on `agent discover`, a
bug could produce the wrong fixture and make its own assertion pass.

**A misconfigured sandbox fails; it does not skip.** A skipped suite and a
passing suite look identical in CI output, which is how regressions ship. Only
an unset `JIRA_E2E` skips.

**Nothing is left behind.** Every issue carries `jira-cli-e2e` plus a per-run
label. `t.Cleanup` deletes them; `TestMain` sweeps anything still labelled at the
end and **fails the run** if it finds any; a signal handler sweeps on Ctrl-C;
and the next run clears orphans older than two hours. The sweeper always
requires the marker label, so it can never reach an issue the suite did not
create.

**Assertions are scoped, never absolute.** The project is shared between tests,
so each case pushes its own label into `agent ready` and compares exact sets.
Commands without a label filter are narrowed client-side by ownership, and
`agent status` counts are compared as deltas.

**Index lag is waited out, not slept through.** Jira's JQL search is eventually
consistent, so reads go through `Eventually`, which retries with a deadline. A
real regression still fails.
