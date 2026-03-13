# Jira-GitHub Sync Setup

Bidirectional sync between markdown issue files in GitHub and Jira Cloud. GitHub owns content, Jira owns status.

## Prerequisites

- `jira-cli` with `issue export` and `issue import` commands
- GitHub App installed on `endgame-build/jira-cli` (for binary downloads)
- Jira Cloud API token

## Required Secrets

Configure these in your repository's Settings → Secrets and variables → Actions.

| Secret | Used by | Description |
|--------|---------|-------------|
| `JIRA_INSTANCE` | Both workflows | Jira Cloud instance (e.g., `myorg.atlassian.net`) |
| `JIRA_USER` | Both workflows | Jira user email for API authentication |
| `JIRA_TOKEN` | Both workflows | Jira API token ([create one](https://id.atlassian.com/manage-profile/security/api-tokens)) |
| `JIRA_CLI_APP_ID` | Both workflows | GitHub App ID for downloading jira-cli binary |
| `JIRA_CLI_APP_PRIVATE_KEY` | Both workflows | GitHub App private key (PEM format) |
| `SYNC_PAT` | sync-from-jira only | GitHub PAT with `repo` scope + bypass branch protection |

## GitHub App Setup

The workflows use a GitHub App to download the jira-cli binary from the private `endgame-build/jira-cli` repository.

1. Create a GitHub App (or use an existing one) with **Contents: Read-only** permission
2. Install it on the `endgame-build` organization, scoped to the `jira-cli` repository
3. Note the App ID and generate a private key
4. Add `JIRA_CLI_APP_ID` and `JIRA_CLI_APP_PRIVATE_KEY` as repository secrets

## SYNC_PAT Setup

The `sync-from-jira` workflow needs a PAT that can:
- Push to the `jira-sync/auto` branch
- Create and merge pull requests
- Bypass branch protection rules (to auto-merge without review)

Create a fine-grained PAT with `repo` scope, or a classic PAT with `repo` permissions. The PAT owner must have bypass permissions on the repository's branch protection rules.

## Installation

1. Copy `deploy/workflows/sync-to-jira.yml` to `<your-repo>/.github/workflows/sync-to-jira.yml`
2. Copy `deploy/workflows/sync-from-jira.yml` to `<your-repo>/.github/workflows/sync-from-jira.yml`
3. Configure all secrets listed above
4. Create the `needs-review` label in your repository

## Configuration

### JQL Query

Edit the `DEFAULT_JQL` environment variable in `sync-from-jira.yml`:

```yaml
env:
  DEFAULT_JQL: 'project = MYPROJECT ORDER BY key ASC'
```

Replace `MYPROJECT` with your Jira project key. You can use any valid JQL — filter by status, label, sprint, etc.

The JQL can also be overridden per-run via the `jql` input on manual dispatch.

### Issues Directory

Both workflows default to `issues/` as the sync directory. Change the `ISSUES_DIR` environment variable in both workflow files to use a different path:

```yaml
env:
  ISSUES_DIR: issues
```

The `sync-to-jira` workflow also accepts a `dir` input on manual dispatch to sync a specific subfolder (e.g., `issues/EPIC-123`).

Files use the `--tree` layout:

```
issues/
  PROJ-1/
    PROJ-1.md
    .jira-field-values.json
  PROJ-2/
    PROJ-2.md
    .jira-field-values.json
```

Each issue gets a directory containing the markdown file and an optional sidecar file for custom field value mappings.

### Auto-merge Behavior

The `sync-from-jira` workflow analyzes each changed file:

- **Frontmatter-only changes** (status, priority, assignee, etc.) → eligible for auto-merge
- **Sidecar `.jira-field-values.json` changes** → eligible for auto-merge
- **Body/description changes** → requires human review (`needs-review` label)
- **New or deleted files** → requires human review

Auto-merge can be disabled per-run by unchecking the `auto_merge` checkbox on manual dispatch.

### Cron Schedule

Default: every 2 hours. Edit the cron expression in `sync-from-jira.yml`:

```yaml
schedule:
  - cron: '0 */2 * * *'
```
