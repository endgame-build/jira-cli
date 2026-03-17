# jira-cli

Non-interactive CLI for Jira Cloud REST API v3 — built for developers, agents, and CI/CD.

```
jira issue create --project PROJ --type Bug --summary "Login fails on Safari"
jira issue view PROJ-123 --json
jira issue move PROJ-123 "In Progress"
jira search "project = PROJ AND status = Open" --limit 10
```

## Features

- **Non-interactive** — flags supply all input; every command runs unattended
- **Pipe-safe** — structured JSON output, correct exit codes, plain text when piped
- **Agent-friendly** — errors include codes, context, and fix suggestions so LLMs self-correct
- **Three credential sources** — flags, environment variables, or stored profiles (keyring-backed)
- **Markdown input** — write descriptions and comments in Markdown; the CLI converts them to Atlassian Document Format
- **JQL support** — raw JQL via `jira search` or flag-based filtering via `jira issue list`
- **Inline filtering** — `--jq` flag extracts fields directly in the CLI
- **Bulk export/import** — round-trip issues to markdown files with YAML frontmatter
- **Aliases** — save frequently used commands as shortcuts

## Install

**Binary (macOS / Linux):**

```sh
# Available platforms: darwin_arm64, darwin_amd64, linux_arm64, linux_amd64
gh release download --repo endgame-build/jira-cli --pattern '*darwin_arm64*'
tar xzf jira_*_darwin_arm64.tar.gz jira
sudo mv jira /usr/local/bin/
```

Or download from [GitHub Releases](https://github.com/endgame-build/jira-cli/releases/latest).

**From source (requires Go):**

```sh
go install github.com/endgame-build/jira-cli/cmd/jira@latest
```

## Quick Start

```sh
# Authenticate
jira auth login --instance mycompany.atlassian.net --user me@company.com --token <api-token>

# View an issue
jira issue view PROJ-123

# Create an issue
jira issue create --project PROJ --type Task --summary "Add retry logic" --description "Handle 429 responses"

# Edit an issue
jira issue edit PROJ-123 --add-labels urgent --priority High

# Transition an issue
jira issue move PROJ-123 "In Progress"

# Assign an issue
jira issue assign PROJ-123 "Jane Doe"

# Search with JQL
jira search "project = PROJ AND assignee = currentUser()" --json

# List my open issues
jira issue list --assignee @me
```

## Commands

### Issues

```sh
jira issue view <key-or-id>            # View issue details (--web to open in browser)
jira issue create                      # Create an issue (--project, --type, --summary required)
jira issue edit <key-or-id>            # Edit fields (--summary, --priority, --add-labels, etc.)
jira issue delete <key-or-id> --yes    # Delete an issue (requires --yes to confirm)
jira issue move <key-or-id> <status>   # Transition to a new status
jira issue assign <key-or-id> <user>   # Assign (display name, account ID, or @me)
jira issue assign <key-or-id> --unassign  # Remove assignee
jira issue list                        # List issues (--project, --assignee, --status, --sort)
jira issue transitions <key-or-id>     # Show available workflow transitions
jira issue export                      # Export issues to markdown (--project, --jql, --tree)
jira issue import <files...>           # Create/update issues from markdown (--dir, --force)
jira issue reconcile                   # Detect orphaned Jira issues (--dir, --epic, --project)
```

### Search

```sh
jira search "<jql>"                    # Search with raw JQL (--limit, --fields)
```

### Comments

```sh
jira comment list <issue-key>          # List comments on an issue
jira comment add <issue-key>           # Add a comment (--body or --body-file)
jira comment edit <issue-key> <id>     # Edit a comment
jira comment delete <issue-key> <id>   # Delete a comment (requires --yes)
```

### Projects

```sh
jira project list                      # List all projects
jira project view <key-or-id>          # View project details
```

### Users

```sh
jira user me                           # Show authenticated user
jira user search <query>               # Search for users by name or email
```

### Schema

```sh
jira schema fields                     # List all fields
jira schema types                      # List issue types
jira schema statuses                   # List statuses
jira schema priorities                 # List priorities
jira schema labels                     # List labels
jira schema field-values               # Build field value mappings (--project, --output)
```

### Config

```sh
jira config set <key> <value>          # Set a config value
jira config get <key>                  # Get a config value
jira config list                       # List all config values
```

### Aliases

```sh
jira alias set <name> <command>        # Create or update an alias
jira alias list                        # List all aliases
```

### Auth

```sh
jira auth login                        # Store credentials (--instance, --user, --token)
jira auth logout --yes                 # Remove stored credentials
jira auth status                       # Show current authentication
jira auth switch <profile>             # Switch active profile
```

### Meta

```sh
jira meta version                      # Show CLI version and build info
jira meta commands                     # List all commands (machine-readable)
```

## Global Flags

| Flag | Description |
|------|-------------|
| `--json` | Output in JSON format |
| `--jq <expr>` | Filter JSON output with a jq expression (implies `--json`) |
| `--text` | Force text output (overrides `output.format` config) |
| `--quiet` / `-q` | Suppress non-essential output |
| `--dry-run` | Preview changes without executing |
| `--no-color` | Disable color output |
| `--profile <name>` | Use a named authentication profile |
| `--instance <url>` | Override Jira instance URL |
| `--user <email>` | Override Jira user email |
| `--token <token>` | Override Jira API token |

## Configuration

The CLI stores config at `$XDG_CONFIG_HOME/jira-cli/config.toml` (`~/.config/jira-cli/` on Linux, `~/Library/Application Support/jira-cli/` on macOS). Set defaults to avoid repeating flags:

```sh
jira config set default.project PROJ
jira config set output.format json
```

## CI/CD Usage

```sh
export JIRA_INSTANCE=mycompany.atlassian.net
export JIRA_USER=ci@company.com
export JIRA_TOKEN=$JIRA_API_TOKEN

jira issue create --project PROJ --type Bug --summary "Build failed" --json
```

## Export / Import

Round-trip issues between Jira and local markdown files:

```sh
# Export all issues from a project
jira issue export --project PROJ --output-dir ./issues

# Export as a tree (epics become directories)
jira issue export --project PROJ --output-dir ./issues --tree

# Export specific custom fields only
jira issue export --project PROJ --fields "Team, Sprint, Story Points"

# Export with JQL
jira issue export --jql "project = PROJ AND status != Done" --output-dir ./issues

# Edit markdown files locally, then push changes back
jira issue import ./issues/PROJ/*.md

# Import from a directory
jira issue import --dir ./issues/PROJ

# Force import (skip conflict detection)
jira issue import --dir ./issues/PROJ --force

# Preview import without making changes
jira issue import --dir ./issues/PROJ --dry-run
```

Exported files use YAML frontmatter for metadata (key, type, status, priority, labels, assignee, custom fields) and Markdown body for the description. Files with temporary keys (`PROJ-NEW-1`) create new issues; files with real keys update existing ones.

Custom fields with object values (teams, options, users) are round-tripped via a `.jira-field-values.json` sidecar file that maps display names to Jira API objects. The sidecar is generated automatically during export and consumed during import.

### Reconcile

Detect issues that exist in Jira but have no corresponding markdown file:

```sh
# List orphaned issues under an epic
jira issue reconcile --dir ./issues --epic PROJ-10

# List orphans across a project
jira issue reconcile --dir ./issues --project PROJ

# Close orphaned issues
jira issue reconcile --dir ./issues --epic PROJ-10 --action close --yes

# Delete orphaned issues
jira issue reconcile --dir ./issues --project PROJ --action delete --yes
```

### Field Value Mappings

Build a sidecar mapping file from existing Jira issues (useful when starting without an export):

```sh
jira schema field-values --project PROJ --output .jira-field-values.json
```

## Error Codes

| Exit | Meaning |
|------|---------|
| 0 | Success |
| 1 | General error |
| 2 | Authentication error |
| 3 | Validation error |
| 4 | Not found |
| 5 | Permission denied |
| 6 | Rate limited |
| 7 | Network error |
| 8 | Conflict |

## Status

Active development. Covers Jira Cloud REST API v3.

## License

Proprietary.
