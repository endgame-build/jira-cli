# Contributing to jira-cli

Thanks for taking the time to contribute. This guide covers the setup, the rules
the automation enforces, and what a reviewable pull request looks like.

Read [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) first. It applies to every issue,
pull request, and discussion in this repository.

## Before You Start

Open an issue before writing code for anything larger than a bug fix. jira-cli
keeps a deliberately narrow scope: a non-interactive CLI for the Jira Cloud REST
API v3, driven by flags, safe in pipes and CI. Features that need an interactive
terminal UI, or that target Jira Server or Data Center, fall outside that scope
and will be declined. An issue first saves you the work.

Small fixes need no issue. Typos, broken links, and one-line bug fixes can go
straight to a pull request.

## Setup

You need Go at the version pinned in [go.mod](../go.mod) and
[pre-commit](https://pre-commit.com).

```sh
git clone https://github.com/endgame-build/jira-cli
cd jira-cli
make hooks     # installs the pre-commit, commit-msg and pre-push hooks
make build     # writes bin/jira
make test
```

`make hooks` is the step most contributors skip and then trip over. It installs
the hooks defined in [.pre-commit-config.yaml](../.pre-commit-config.yaml):

| Stage | Checks |
|---|---|
| pre-commit | `gofmt`, `go vet ./...`, `go build ./...`, `go test -short ./...`, gitleaks secret scan, trailing whitespace, YAML and TOML syntax, 500 KB file-size cap |
| commit-msg | conventional-commit format |
| pre-push | `go test -race ./...` |

CI runs the same checks plus `govulncheck`, so a clean local run predicts a clean
CI run.

## Commit Messages Cut Releases

jira-cli releases itself. When CI passes on `main`,
[auto-release.yml](workflows/auto-release.yml) derives the next version from the
commit messages with [svu](https://github.com/caarlos0/svu), tags it, and
[goreleaser](../.goreleaser.yaml) builds the release. Your commit subject line
becomes both the version bump and the changelog entry.

Use the [Conventional Commits](https://www.conventionalcommits.org) format. The
commit-msg hook accepts these types and rejects everything else:

| Type | Version bump | In the changelog |
|---|---|---|
| `feat` | minor | yes |
| `fix` | patch | yes |
| `refactor`, `build` | patch | yes |
| `docs`, `chore`, `test`, `ci` | patch | no |
| any type with `!` or a `BREAKING CHANGE:` footer | major | yes |

```
feat(issue): support --parent on edit
fix(api): retry 429 responses without dropping the body
docs: document the --map field mapping
```

Scope the change to the package you touched (`issue`, `api`, `auth`, `output`,
`adf`). Write the subject in the imperative, under 72 characters.

## Architecture

[CLAUDE.md](../CLAUDE.md) is the orientation document: layer order, the Factory
dependency-injection hub, the command pattern every command follows, the error
model, and the output shapes. [SPEC.md](../SPEC.md) holds the functional
specification for the command surface. Read both before adding a command.

Four rules carry most of the design:

1. Layers import downward only: `errors → iostreams → config → auth → api → output → adf → shared → factory → commands → main`.
2. Every command is `NewCmdXxx(f *factory.Factory) *cobra.Command` with an `XxxOptions` struct and a `runXxx` function. No `init()`, no package-level state.
3. Commands return `error`. Only `main.go` renders errors and calls `os.Exit`.
4. Every error is a `CLIError` carrying a code, a message, context, and a suggestion. The exit code comes from the error code.

## Tests

Every leaf package under `internal/` has tests, and new code is expected to keep
that true. The patterns already in the tree:

- `factory.NewTestFactory(ios, cfg, client)` builds a wired factory that resolves no credentials.
- `iostreams.Test()` returns IOStreams plus stdout and stderr buffers.
- `httptest.NewServer` mocks the Jira API.
- `keyring.MockInit()` gives an in-memory keyring.
- `t.TempDir()` for config files, `t.Setenv()` for environment variables.
- Table-driven cases throughout.

Use placeholder data in fixtures. Real project keys, account IDs, email
addresses, and instance hostnames do not belong in this repository.

## Pull Requests

Run `make build test lint` before you open the pull request. Fill in the
template: what changed, why, and how you verified it. Link the issue it closes.

One logical change per pull request. A refactor bundled with a feature is two
pull requests.

Expect a first response within a week. If a pull request goes quiet, comment on
it and it will resurface.

## Dependencies

Adding a Go module changes what ships in the released binaries, so it needs a
reason in the pull request description. After `go get`, run `make notices` to
regenerate [THIRD_PARTY_NOTICES.md](../THIRD_PARTY_NOTICES.md) and add the new
module to [scripts/licenses.tsv](../scripts/licenses.tsv) with its license read
from the module's own LICENSE file. Modules under a strong copyleft license
cannot be added.

## Licensing of Contributions

jira-cli is licensed under the Apache License 2.0. Under section 5 of that
license, any contribution you submit for inclusion is licensed under the same
terms, with no separate agreement to sign.
