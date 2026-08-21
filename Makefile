.PHONY: build test test-e2e test-e2e-one e2e-sweep lint install clean hooks notices

VERSION ?= dev
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

build:
	@mkdir -p bin
	go build -ldflags "$(LDFLAGS)" -o bin/jira ./cmd/jira

test:
	go test ./...

# The e2e package is behind a build tag, so plain `go vet ./...` never sees it.
# Vetting it separately keeps the harness compiling on every PR without needing
# credentials or network access.
lint:
	go vet ./...
	go vet -tags e2e ./...

# End-to-end tests against a real Jira sandbox. See test/e2e/README.md.
# -count=1 is mandatory: results depend on remote state, so a cached PASS lies.
test-e2e:
	@: $${JIRA_INSTANCE:?set JIRA_INSTANCE (e.g. yourco.atlassian.net)}
	@: $${JIRA_USER:?set JIRA_USER (your Atlassian account email)}
	@: $${JIRA_TOKEN:?set JIRA_TOKEN (an API token from id.atlassian.com)}
	@: $${JIRA_E2E_PROJECT:?set JIRA_E2E_PROJECT (sandbox Scrum project key, letters only)}
	JIRA_E2E=1 go test -tags e2e -count=1 -p 1 -timeout 30m -v ./test/e2e/...

# Run one case or one chain: make test-e2e-one E2E_RUN=TestE2E_READY_01
E2E_RUN ?= TestE2E_
test-e2e-one:
	JIRA_E2E=1 go test -tags e2e -count=1 -p 1 -timeout 30m -v -run '$(E2E_RUN)' ./test/e2e/...

# Delete every issue in the sandbox carrying the e2e marker label.
e2e-sweep:
	JIRA_E2E=1 JIRA_E2E_SWEEP=1 go test -tags e2e -count=1 -run TestE2E_SWEEP -v ./test/e2e/...

install:
	go install ./cmd/jira

clean:
	rm -rf bin/

hooks:
	@command -v pre-commit >/dev/null 2>&1 || { echo "Install pre-commit: brew install pre-commit"; exit 1; }
	pre-commit install --install-hooks
	@echo "pre-commit hooks installed"

notices:
	./scripts/gen-third-party-notices.sh
