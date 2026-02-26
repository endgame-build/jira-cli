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
go install github.com/endgameio/jira-cli/cmd/jira@latest
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
