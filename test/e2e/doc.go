// Package e2e contains end-to-end tests that drive the built jira binary
// against a real Jira Cloud sandbox project.
//
// Every test file is behind the `e2e` build tag and additionally gated on
// JIRA_E2E=1, so `go test ./...` and `go test -short ./...` compile nothing
// here and reach no network. See README.md for setup.
package e2e
