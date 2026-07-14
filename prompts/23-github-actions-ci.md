# 23. GitHub Actions CI

```text
Add GitHub Actions CI.

Workflow:
- checkout
- setup Go
- cache Go modules
- run gofmt check
- run go vet
- run golangci-lint
- run go test ./...
- optionally run integration tests with PostgreSQL service

Requirements:
- Add .github/workflows/ci.yml.
- Add golangci-lint config.
- Add race detector job if practical.
- Add coverage output.
- Update README with CI badge placeholder.
```
