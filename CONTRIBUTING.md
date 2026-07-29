# Contributing

Use Go 1.26.5 or a compatible newer patch release. Keep runtime dependencies minimal and prefer
the standard library when it provides a clear solution.

Before opening a pull request:

```bash
go mod verify
test -z "$(gofmt -l .)"
go vet ./...
go test -race ./...
go build ./cmd/watchdog
```

Use Conventional Commits, for example `feat: add certificate expiry checks` or
`fix(config): reject invalid header names`. Tests must use `httptest`, localhost, reserved
domains, or documentation-only addresses. Never commit production URLs containing secrets.

