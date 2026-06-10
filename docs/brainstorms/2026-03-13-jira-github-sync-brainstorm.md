# Brainstorm: Bidirectional Jira-GitHub Sync

**Date:** 2026-03-13
**Status:** Draft

## What We're Building

A bidirectional sync system between markdown issue files in a GitHub repo and Jira, using GitHub Actions and the existing `jira-cli` export/import commands.

**Data flow:**
- **GitHub → Jira** (on push to main): When markdown files change on `main`, run `jira issue import` to push content updates to Jira
- **Jira → GitHub** (cron, every 2h): Run `jira issue export`, create a PR with changes. Auto-merge if only frontmatter metadata changed; require human review if body content changed

**Source of truth:** GitHub owns content (summary, description, custom fields). Jira owns status. Markdown files are initially authored in GitHub, imported to Jira for status tracking. Status changes flow back from Jira without overwriting content.

## Why This Approach

**Pure GitHub Actions + existing CLI (no new Go code):**
- Export and import commands already handle the full round-trip
- Conflict detection via `updated` timestamps already exists
- Git diff naturally surfaces only what changed between syncs
- No new CLI commands or binaries to maintain
- Easy to customize per-repo (just copy/edit workflow files)

## Key Decisions

1. **GitHub is content authority, Jira is status authority** — avoids conflict on who "wins"
2. **Full re-export with smart diff** — re-export all issues each sync; let git show only actual changes. Simpler than tracking last-sync timestamps
3. **Auto-merge rule: frontmatter-only changes** — if git diff shows only YAML frontmatter changes (between `---` delimiters) and no body changes, the PR auto-merges. Any body content change requires human review
4. **Credentials via GitHub Secrets** — `JIRA_INSTANCE`, `JIRA_USER`, `JIRA_TOKEN` stored as repo secrets
5. **Two separate workflow files** — independent triggers, independent failure modes
6. **Trigger: push to main** for GH→Jira; **cron every 2h** for Jira→GH

## Design

### Workflow 1: `sync-to-jira.yml` (GitHub → Jira)

```yaml
on:
  push:
    branches: [main]
    paths: ['issues/**/*.md']  # configurable path

jobs:
  sync:
    runs-on: ubuntu-latest
    steps:
      - checkout
      - install jira-cli (go install or download release binary)
      - jira issue import --dir issues/ --force
```

**Notes:**
- `--force` skips timestamp conflict detection (GitHub is authority)
- Changed files detection via `paths:` filter — only runs when markdown changes
- Needs to handle new files (create) and modified files (update) — import already does this via temp-key pattern

### Workflow 2: `sync-from-jira.yml` (Jira → GitHub)

```yaml
on:
  schedule:
    - cron: '0 */2 * * *'  # every 2 hours
  workflow_dispatch: {}      # manual trigger

jobs:
  sync:
    runs-on: ubuntu-latest
    steps:
      - checkout main
      - install jira-cli
      - jira issue export --jql "project = X" --dir issues/
      - check if git has changes
      - if changes:
          - analyze diff (frontmatter-only vs content changes)
          - create branch jira-sync/YYYY-MM-DD-HHMM
          - commit & push
          - create PR with label
          - if frontmatter-only: auto-merge PR
          - if content changes: label PR "needs-review", assign reviewer
```

### Diff Analysis Script (shell)

Core logic to classify changes:

```bash
# For each changed file, check if only frontmatter changed
frontmatter_only=true
for file in $(git diff --name-only); do
  # Get the diff, check if changes are only between --- delimiters
  body_changes=$(git diff -- "$file" | grep -E '^\+|^\-' | grep -v '^\+\+\+\|^\-\-\-' |
    awk '/^[+-]---$/{in_fm=!in_fm; next} !in_fm{print}' | grep -c '.')
  if [ "$body_changes" -gt 0 ]; then
    frontmatter_only=false
    break
  fi
done
```

This is the trickiest part — needs careful handling of:
- New files (always need review)
- Deleted files (always need review)
- Files where only `status:` line changed vs other frontmatter

### Configuration

Each repo using sync needs:
1. Two workflow files in `.github/workflows/`
2. Three GitHub Secrets: `JIRA_INSTANCE`, `JIRA_USER`, `JIRA_TOKEN`
3. A convention for the issues directory (default: `issues/`)
4. JQL query or project key for which issues to sync

## Resolved Questions

1. **Conflict window:** Auto-rebase sync branches on main before creating/updating PRs
2. **Which issues to sync:** By JQL query (configurable per-repo, e.g., `project = PROJ AND status != Done`)
3. **Sidecar file:** Commit `.jira-field-values.json` to the repo for round-trip fidelity
4. **Branch protection:** Main is protected; a PAT with bypass permissions will be provided as a secret for auto-merge

## What This Is NOT

- Not a real-time webhook integration (polling-based, minutes latency is fine)
- Not a general-purpose Jira sync framework — it's workflow files you copy into a repo
- Not modifying the jira-cli Go codebase (uses existing commands as-is)
