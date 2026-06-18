# Goctopus client SDKs

Small reference clients that handle the things every Goctopus consumer needs:

- **ACK** every message by its `id` so the server marks it delivered,
- **de-duplicate** re-deliveries (delivery is at-least-once),
- **reconnect** automatically with exponential backoff.

| Language | File | Dependency |
|----------|------|------------|
| JavaScript (browser/Node) | [`javascript/goctopus.js`](javascript/goctopus.js) | none (browser) / `ws` (Node) |
| Python (asyncio) | [`python/goctopus_client.py`](python/goctopus_client.py) | `websockets` |
| Go | [`go/goctopus.go`](go/goctopus.go) | `github.com/gobwas/ws` |

Each client connects to the websocket endpoint (`ws://` or, behind TLS, `wss://`).
Authentication of the connection is performed by your own backend via the
`WS_AUTH_URL` endpoint — pass cookies/headers the same way your app already does.
