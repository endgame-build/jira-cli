# Security Policy

## Reporting a Vulnerability

Report vulnerabilities through GitHub's private vulnerability reporting on this
repository: **Security → Report a vulnerability**. The report stays private until
a fix ships. If you cannot use that form, email opensource@end.game.

Please do not open a public issue for a vulnerability.

Include the jira-cli version from `jira meta version`, your operating system, the
steps to reproduce, and what an attacker gains. A working proof of concept helps
but is not required.

You can expect an acknowledgement within three working days and an assessment
within ten. Fixes ship in a patch release, and the advisory credits you unless
you ask otherwise.

## Supported Versions

Fixes land on the latest released minor version. Older versions receive no
backports; upgrade to the latest release before reporting.

## How jira-cli Handles Your Credentials

jira-cli authenticates to Jira Cloud with an Atlassian API token and resolves it
from three sources, in order: command-line flags, the `JIRA_INSTANCE`,
`JIRA_USER` and `JIRA_TOKEN` environment variables, then a stored profile.
Stored profiles keep the token in the operating system keyring (Keychain on
macOS, Secret Service on Linux) rather than in the config file. The config file
itself holds no secrets.

Two consequences are worth knowing before you script against this CLI:

**`--token` is visible to other processes.** A token passed as a flag appears in
your shell history and in the output of `ps` for every user on the machine. Use
`jira auth login` for interactive work and `JIRA_TOKEN` in CI. The `--token` flag
exists for cases where neither is available.

**Tokens carry your full Jira permissions.** Atlassian API tokens are not
scoped. A token that leaks grants the holder everything your account can do,
including deleting issues. Rotate it at
https://id.atlassian.com/manage-profile/security/api-tokens if it is exposed.

## Scope

In scope: credential handling, the API client's transport and TLS behavior,
command injection through flags or file input, path traversal in the export and
import commands, and dependency vulnerabilities that reach a released binary.

Out of scope: vulnerabilities in Atlassian's own products or APIs, which belong
in [Atlassian's bug bounty](https://bugcrowd.com/atlassian); anything requiring
an attacker who already controls the user's machine or shell; and the risk that
an unscoped Atlassian API token is powerful, which is Atlassian's design and is
documented above.

## Supply Chain

Release binaries are built by [goreleaser](../.goreleaser.yaml) from a tagged
commit in a GitHub Actions runner, and every release publishes a `checksums.txt`
with SHA-256 digests. Verify a download against it before installing. CI pins
every action to a commit SHA and runs `govulncheck` on each push. Dependabot
opens weekly updates for Go modules and actions.
