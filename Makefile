.PHONY: build test lint run-server run-agent

build:
	go build ./...

test:
	go test ./...

lint:
	golangci-lint run ./...

run-server:
	go run ./cmd/orch-server

run-agent:
	go run ./cmd/orch-agent
