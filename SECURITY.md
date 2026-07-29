# Security Policy

## Reporting

Report vulnerabilities through GitHub Security Advisories. Do not include private endpoints,
authorization headers, or production configuration in a public issue.

## Deployment Boundaries

Endpoint Watchdog assumes `config.yaml` is trusted operator input. A configured URL can reach
private network services, so untrusted users must not be allowed to edit configuration. The
service does not provide authentication or TLS termination. Bind it to a private interface or
place it behind an authenticated TLS reverse proxy when the status data is sensitive.

Headers and URL query strings are never returned by the API or written to application logs.
Response bodies are read only for `expectedText` checks and are never retained.

