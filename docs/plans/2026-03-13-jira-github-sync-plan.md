# Plan: Jira-GitHub Bidirectional Sync Workflows

## Context

Andrei wants markdown issue files in a GitHub "docs" repo to sync bidirectionally with Jira. GitHub owns content, Jira owns status. The setup action already exists at `endgame-build/actions/setup-jira` (uses GitHub App auth). Remaining work: two workflow templates and setup docs.

**Brainstorm:** `docs/brainstorms/2026-03-13-jira-github-sync-brainstorm.md`

## Existing: `endgame-build/actions/setup-jira` (DONE)

Usage pattern (GitHub App token):
```yaml
- uses: actions/create-github-app-token@v1
  id: app-token
  with:
    app-id: ${{ secrets.JIRA_CLI_APP_ID }}
    private-key: ${{ secrets.JIRA_CLI_APP_PRIVATE_KEY }}
    owner: endgame-build

- uses: endgame-build/actions/setup-jira@v1
  with:
    token: ${{ steps.app-token.outputs.token }}
```

Note: `v1` tag not yet created on the repo.

## Deliverables

1. **`sync-to-jira.yml`** — template workflow (GH → Jira on push)
2. **`sync-from-jira.yml`** — template workflow (Jira → GH on cron/dispatch)
3. **`sync-setup.md`** — setup documentation

---

## Step 1: `sync-to-jira.yml` (GH → Jira)

```yaml
on:
  push:
    branches: [main]
    paths: ['issues/**']
```

Steps:
1. `actions/checkout@v4`
2. `actions/create-github-app-token@v1` → app token for jira-cli download
3. `endgame-build/actions/setup-jira@v1` with app token
4. `jira issue import --dir issues/ --force`

Env: `JIRA_INSTANCE`, `JIRA_USER`, `JIRA_TOKEN` from secrets.

---

## Step 2: `sync-from-jira.yml` (Jira → GH)

```yaml
on:
  schedule:
    - cron: '0 */2 * * *'
  workflow_dispatch:
    inputs:
      auto_merge: { type: boolean, default: true }
      jql: { type: string, description: 'Override JQL query' }
```

Steps:
1. Checkout `main` with `SYNC_PAT` (needs push + bypass branch protection)
2. GitHub App token → setup-jira
3. Export: `jira issue export --jql "$JQL" --output-dir issues/ --tree`
4. `git diff --exit-code` — if clean, exit 0
5. **Diff analysis script:**
   - `git diff --name-status` → classify A/D/M
   - For M files: compare body (after second `---`) — if body changed → `content_changed=true`
   - `.jira-field-values.json` changes → always frontmatter-only (safe)
   - Output: `AUTO_MERGE=true` if only frontmatter changed, else `false`
6. Create/force-push branch `jira-sync/auto`
7. Commit: `chore: sync statuses from Jira [automated]`
8. Close any existing open PR from `jira-sync/auto`
9. `gh pr create`
10. If `auto_merge` input AND `AUTO_MERGE=true` → `gh pr merge --merge`
11. Otherwise → add `needs-review` label

Auth: `SYNC_PAT` for git push + PR ops. GitHub App for jira-cli download.

---

## Step 3: `sync-setup.md`

- Required secrets table:
  - `JIRA_INSTANCE`, `JIRA_USER`, `JIRA_TOKEN` — Jira API access
  - `JIRA_CLI_APP_ID`, `JIRA_CLI_APP_PRIVATE_KEY` — GitHub App for jira-cli binary download
  - `SYNC_PAT` — PAT with push + bypass branch protection (sync-from-jira only)
- GitHub App setup instructions (install on `endgame-build/jira-cli`, Contents: Read-only)
- JQL configuration
- Auto-merge behavior and the disable checkbox
- Directory convention (`issues/` with `--tree` layout)

---

## Implementation Order

All files created locally under `deploy/`:

1. `deploy/workflows/sync-to-jira.yml`
2. `deploy/workflows/sync-from-jira.yml`
3. `deploy/sync-setup.md`

## Verification

1. **sync-to-jira:** Push markdown change to main → verify Jira issue updates
2. **sync-from-jira (manual):** Change status in Jira → trigger dispatch → verify PR created
3. **auto-merge:** Status-only change → PR auto-merges
4. **human review:** Description change in Jira → PR gets `needs-review` label
5. **no-op:** No changes in Jira → workflow exits cleanly, no PR

## Edge Cases

- **No changes:** Export produces identical files → exit cleanly, no PR
- **Rolling branch:** Single `jira-sync/auto` branch prevents PR stacking
- **Rebase conflicts:** Force-push new export (Jira re-export is idempotent)
- **Re-parenting:** `--tree` moves files between dirs → git sees A/D → needs review (correct)
- **Rate limiting:** CLI already retries 429s
- **Sidecar:** `.jira-field-values.json` changes auto-merge
