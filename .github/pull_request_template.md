## Summary

Describe the monitoring, API, or operational change.

## Validation

- [ ] `test -z "$(gofmt -l .)"`
- [ ] `go vet ./...`
- [ ] `go test -race ./...`
- [ ] `go build ./cmd/watchdog`

## Security

- [ ] Configuration examples contain no real credentials.
- [ ] Headers, URL query strings, and response bodies are not logged.

