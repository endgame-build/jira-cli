.PHONY: build test lint install clean hooks notices

VERSION ?= dev
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

build:
	@mkdir -p bin
	go build -ldflags "$(LDFLAGS)" -o bin/jira ./cmd/jira

test:
	go test ./...

lint:
	go vet ./...

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
