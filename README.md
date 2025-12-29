## WebhookRelay

Tiny Go service that accepts webhook requests and **relays** them to one-or-more destination endpoints.

### Behavior
- **Immediately returns `202 Accepted`** to the caller.
- **Forwards the request body intact** to each configured destination.
- Does **not** return destination responses to the caller (it only logs them to stdout).
- If a relay omits `listen_path`, a **random path is generated on startup** and printed to logs.

### Quickstart (local)

```bash
go run ./cmd/webhookrelay -config ./config/example.json
```

On startup you’ll see logs that include each relay’s resolved `path`.

### Quickstart (docker compose)

```bash
docker compose up --build
```

### Config

See [`config/example.json`](config/example.json).

Top-level fields:
- `server.listen_addr` (required): e.g. `":8080"`
- `server.base_path` (optional): e.g. `"/hook"` (prefix for all relay paths)
- `server.forward_timeout_ms` (optional): per-destination HTTP timeout (default `10000`)
- `server.concurrency` (optional): max in-flight destination forwards (default `50`)
- `relays` (required): array of relay definitions

Each relay:
- `name` (optional): used for logging
- `listen_path` (optional): if omitted, generated at startup
- `methods` (optional): default `["POST"]`
- `destinations` (required non-empty):
  - `url` (required)
  - `method` (optional): override HTTP method sent to destination
  - `headers` (optional): static headers to set on destination request


