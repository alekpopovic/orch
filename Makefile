.PHONY: build test lint run-server run-agent

build:
	go build ./...

test:
	go test ./...

lint:
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "golangci-lint not found; running gofmt check and go vet ./..."; \
		unformatted="$$(gofmt -l $$(git ls-files '*.go'))"; \
		test -z "$$unformatted"; \
		go vet ./...; \
	fi

run-server:
	go run ./cmd/orch-server

run-agent:
	go run ./cmd/orch-agent
