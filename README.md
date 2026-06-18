# Goctopus

![Goctopus logo](logo.png)

Add websocket support to any project independently of it's tech stack.

Simple websocket service that will work with literally any backend and frontend. You can use it with Django, FastAPI, Flask etc. All that a backend has to do is just send POST-requests to Goctopus and it will handle forwarding those to frontend websocket connections.


### Overview

Goctopus runs as a server app on selected hostname and port waiting for incomming connections. Frontend should connect via websocket protocol. Backend just sends POST-requests.

When frontend tries to establist a new websocket connection with Goctopus, it will forward the request (along with all of it's headers and cookies) to address pointed to by `WS_AUTH_URL` environment variable in order to authenticate request. The structure of response from that endpoint is described in [src/authorization.go](https://github.com/andreyvolobuev/goctopus/blob/master/src/authorization.go). There's func `Export` that will return a list of strings. These are keys (one might call them topics) that will identify current connection. That may be a user id or an email or even an organization name (in case you want to send a message to multiple users).

Backend on the other hand just sends POST-requests to Goctopus pointing to user id that it wishes to send the message to.


### Install

1. clone the repo
```
git clone git@github.com:andreyvolobuev/Goctopus.git
cd Goctopus
```

2. Define format of authentication response in [src/authorization.go](https://github.com/andreyvolobuev/goctopus/blob/master/src/authorization.go)
Example:
```
type AuthResponse struct {
	User struct {
		Email        string `json:"email"`
		Ogranization string `json:"organization_name"`
	}
}

func (r *AuthResponse) Export() []string {
	exported := []string{}

	keys := []string{r.User.Email, r.User.Ogranization}
	for _, key := range keys {
		if key != NULL {
			exported = append(exported, key)
		}
	}

	return exported
}
```

3. Compile and run the app:
```
go build -o goctopus src/.
./goctopus --host 0.0.0.0 --port 7890 --workers 1024 --expire 30m --login admin --password mystrongpassword --auth http://localhost/api/v1/is_authenticated --verbose true
```

you may also just use Docker:
```
docker build -t goctopus .
docker run \
    -e WS_HOST=0.0.0.0 \
    -e WS_PORT=7890 \
    -e WS_WORKERS=1024 \
    -e WS_MSG_EXPIRE=30m \
    -e WS_LOGIN=admin \
    -e WS_PASSWORD=mystrongpassword \
    -e WS_AUTH_URL=http://localhost/api/v1/is_authenticated \
    -e WS_VERBOSE=1 \
    -p 7890:7890 \
    goctopus
```

*Please note that Goctopus requires the following environment variables to be set in order to work*
- WS_HOST (flag --host): hostname or ip that the server will run on
- WS_PORT (flag --port): port that the server will listen to
- WS_WORKERS (flag --workers): number of workers (goroutines) that will process the websocket connections. Each takes about 8kb of RAM
- WS_MSG_EXPIRE (flag --expire): if a message is sent from backend but Goctopus can't find a corresponding connection from frontend, then it will keep the message for this amount of time and if the requested user finally joins then the message will be delivered. Valid time units are "ns", "us" (or "µs"), "ms", "s", "m", "h"
- WS_LOGIN (flag --login): login required from backend to send POST-requests
- WS_PASSWORD (flag --password): password required from backend to send POST-requests
- WS_VERBOSE (flag --verbose): wether or not log everything to console
- WS_STORAGE (flag --storage): which message storage to use. Valid options are: "default" / "memory" (these two are same. More storages are to be implemented later. Redis is the next one to come)
- WS_AUTHORIZER (flag --authorizer): which authorization engine to use. Valid options are: "default" / "proxy" (same) or "dummy" (use for development only)
- WS_AUTH_URL (flag --auth): forward incomming requests from frontend to this URL in order to authorize a request or use this value as a dummy authorizer return
- WS_AUTH_TIMEOUT (flag --auth-timeout): timeout for requests to the auth backend (default `10s`). Prevents a hung auth server from pinning a worker
- WS_SWEEP_INTERVAL (flag --sweep-interval): how often expired messages are swept from storage in the background (default `1m`)
- WS_INSECURE_NO_AUTH (flag --insecure-no-auth): allow unauthenticated POST requests. **DEVELOPMENT ONLY** — see Security below


### Security

- **Backend POST requests fail closed.** If `WS_LOGIN`/`WS_PASSWORD` are not set, POST requests are rejected with `401` instead of being silently accepted. To intentionally run without backend auth (local development), set `WS_INSECURE_NO_AUTH=1`.
- Credentials are compared in constant time to avoid timing attacks.
- Run Goctopus behind a TLS-terminating reverse proxy (nginx/Caddy/Traefik) so that both the websocket (`wss://`) and the backend POST traffic are encrypted. Goctopus itself speaks plain HTTP/ws.
- The auth backend (`WS_AUTH_URL`) receives the raw upgrade request including cookies/headers; make sure it is trusted and reachable only over a private network or TLS.


### Operational endpoints

- `GET /healthz` — liveness probe, always `200 ok` while the process is up
- `GET /readyz` — readiness probe, `200` once storage and authorizer are initialized
- `GET /metrics` — Prometheus text exposition format with counters
  (`goctopus_messages_received_total`, `goctopus_messages_delivered_total`,
  `goctopus_messages_expired_total`, `goctopus_auth_failures_total`) and gauges
  (`goctopus_connections`, `goctopus_keys`)


### Use

- frontend should create a websocket instance and declare a handler for incoming messages. Because delivery is at-least-once (a message is re-sent if its ACK times out), clients should de-duplicate by message id:
```js
const seen = new Set();

function connect() {
  const socket = new WebSocket("ws://goctopus:7890");

  socket.onmessage = (event) => {
    const d = JSON.parse(event.data);
    // ACK the message id so Goctopus marks it delivered
    socket.send(JSON.stringify({ id: d.id }));

    if (seen.has(d.id)) return; // ignore duplicates
    seen.add(d.id);

    // do something with the payload that came from the backend
    console.log(d.payload);
  };

  // auto-reconnect with a small backoff
  socket.onclose = () => setTimeout(connect, 1000);
}

connect();
```


- backend forms a message to be sent and sends it to Goctopus via POST-requests with Basic Authentication ([RFC 2617, Section 2](https://www.rfc-editor.org/rfc/rfc2617.html#section-2))

Message structure should be as follows:
- key (REQUIRED): a user identifier, if many users are identified by same key, then message will be sent to all of them. The key itself by which the user was identified will not be sent to user
- value (REQUIRED): the message that will be sent out to the end user
- expire (OPTIONAL): if the message was sent out but there was no receiver with the required key connected, then Goctopus will wait for this amout of time before the message will be considered expired and will be removed from send queue. If this parameter is ommited then default value (set by WS_MSG_EXPIRE env variable or --expire flag) will be used

with curl:
```
curl -X POST http://goctopus:7890
   -H "Content-Type: application/json" 
   -d '{"key":"noreply@google.com","value":{"foo":"bar"},"expire":"30m"}'
   --user "admin:mystrongpassword"
```


or with python:
```
import os
import json
import requests

message = {"key": "noreply@goole.com", "value": {"foo": "bar"}, "expire": "30m"}
r = requests.post("http://goctopus:7890", data=json.dumps(message), auth=("admin", "mystrongpassword"))
```

### API

Goctopus server supports the following HTTP API:
* `GET ` Returns list of keys currently stored
* `GET ?key=<key>` Returns all messages stored for a provided key
* `POST ?key=<key>&value=<value>&expire=<expire:optional>` Saves a message (value) for a given username (key)
* `DELETE ?key=<key:optional>&id=<id:optional>` Deletes messages. If key is set but message id is not, then all messages for given key will be deleted. If both key and message id are set then only message with given key will be deleted. If key and message id are not set then all messages for all keys will be deleted.


### How it compares

Goctopus targets one specific niche: **add push without changing your backend's stack**. Your backend keeps doing what it does and just fires HTTP POSTs.

- **Centrifugo / Soketi** are full-featured realtime servers (channels, presence, history) but expect you to integrate their client SDKs and publish API.
- **Mercure** is great for SSE/hub semantics over HTTP.
- **Goctopus** deliberately stays tiny: HTTP in, websocket out, pluggable auth via your existing endpoint, at-least-once delivery with per-message TTL.


### Roadmap

- [ ] Redis storage backend (interface is already in place — see [src/storage.go](src/storage.go)) for persistence and horizontal scaling
- [ ] Asynchronous ACK protocol with per-connection read loop and ping/pong keepalive (replaces the current synchronous ACK)
- [ ] Native TLS / `wss://` listener option
- [ ] Wildcard topics and explicit broadcast vs direct messaging
- [ ] Client SDKs (JS/TS, Python, Go) with built-in reconnect and de-duplication
