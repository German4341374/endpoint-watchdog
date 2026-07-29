# Configuration Reference

The process reads one YAML document. Unknown keys fail startup so configuration mistakes are
visible immediately.

## Server

| Field | Default | Description |
| --- | --- | --- |
| `address` | `:8080` | HTTP listen address |
| `title` | `Endpoint Watchdog` | Status page and API service name |
| `maxConcurrent` | `5` | Global limit for simultaneous outbound requests |
| `degradedThreshold` | `750ms` | Successful responses slower than this become `DEGRADED` |

## Retry

| Field | Default | Description |
| --- | --- | --- |
| `attempts` | `3` | Total attempts, between 1 and 10 |
| `initialBackoff` | `200ms` | Delay before retry two; subsequent delays double |

## Endpoint

| Field | Default | Description |
| --- | --- | --- |
| `name` | required | Unique, case-insensitive display and API name |
| `url` | required | Absolute `http` or `https` URL without embedded credentials |
| `method` | `GET` | `GET` or `HEAD` |
| `expectedStatus` | `200` | Required HTTP response status |
| `timeout` | `5s` | Per-attempt deadline |
| `interval` | `30s` | Time between scheduled checks |
| `expectedText` | empty | Optional case-sensitive response substring; not valid with `HEAD` |
| `headers` | empty | Optional request headers; `Host` and `Content-Length` are rejected |

Header values remain in memory for requests. They are not logged or exposed through the API.

