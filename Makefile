.PHONY: build test lint install clean hooks

build:
	@mkdir -p bin
	go build -o bin/jira ./cmd/jira

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
