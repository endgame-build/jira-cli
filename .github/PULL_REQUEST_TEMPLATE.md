## What changed

<!-- One or two sentences. The commit subject drives the changelog and the
     version bump, so make sure it matches what this PR does. -->

Closes #

## Why

<!-- The problem this solves. Link the issue that discussed it. -->

## How it was verified

<!-- Commands you ran and what you saw. Tick only the boxes you completed. -->

- [ ] `make build test lint`
- [ ] `go test -race ./...`
- [ ] Ran the affected command against a real Jira instance
- [ ] Added or updated tests covering the change

## Checklist

- [ ] Commit messages follow [Conventional Commits](https://www.conventionalcommits.org) with an allowed type
- [ ] Fixtures use placeholder data, no real project keys, account IDs, or hostnames
- [ ] README updated if the command surface or flags changed
- [ ] `make notices` run if dependencies changed
