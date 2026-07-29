# Operations

## State Rules

- `UNKNOWN`: no completed check exists for the current process run.
- `UP`: status and optional body text match, and response time is within the threshold.
- `DEGRADED`: the check succeeds but exceeds `server.degradedThreshold`.
- `DOWN`: timeout, connection failure, unexpected status, missing text, or body read failure.

Only the final result after retries is stored. `UP` and `DEGRADED` count as successful for
current-run uptime. History is bounded to the latest 50 scheduled results per endpoint.

## Graceful Shutdown

`SIGINT` and `SIGTERM` cancel schedulers, stop retry backoff, allow active HTTP handlers up to ten
seconds to finish, and then exit. In-memory history is intentionally not persisted.

## Troubleshooting

1. Validate YAML indentation and field names.
2. Check structured logs for the endpoint name, final attempt, HTTP status, and response time.
3. Query `/api/status/<name>` for retained results and recent transitions.
4. Run the same URL from the watchdog network namespace to investigate DNS or firewall behavior.
5. Increase `timeout` only after confirming the service is expected to respond slowly.

