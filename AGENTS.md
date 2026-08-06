# Repository guidance

- Keep source code, tests, documentation, configuration, and commit messages in English.
- Prefer the Go standard library unless a dependency provides a clear, documented benefit.
- Preserve bounded concurrency, request timeouts, retry backoff, and graceful shutdown behavior.
- Validate endpoint URLs and never weaken the supported-scheme restrictions.
- Add tests for configuration, state calculation, handlers, and timeout behavior when changing them.
- Run formatting, vet, tests, build, and configuration validation before publishing changes.
- Never commit real credentials, private endpoints, tokens, personal data, or generated reports.
