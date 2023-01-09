# Goctopus

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
		if key != "" {
			exported = append(exported, key)
		}
	}

	return exported
}
```

3. Compile and run the app:
```
go build -o goctopus src/.
./goctopus --host 0.0.0.0 --port 7890 --workers 1024 --expire 30m --login admin --password password --auth http://localhost/api/v1/is_authenticated --verbose true
```

you may also just use Docker:
```
docker build -t goctopus .
docker run \
    -e WS_HOST=0.0.0.0 \
    -e WS_PORT=7890 \
    -e WS_WORKERS=1024 \
    -e WS_MSG_EXPIRE=30m \
    -e WS_LOGIN=login \
    -e WS_PASSWORD=password \
    -e WS_AUTH_URL=http://localhost/api/v1/is_authenticated \
    -e WS_VERBOSE=1 \
    -p 7890:7890 \
    goctopus
```

*Please note that Goctopus requires the following environment variables to be set in order to work*
- WS_HOST (flag --host): hostname or ip that the server will run on
- WS_PORT (flag --port): port that the server will listen to
- WS_WORKERS (flag --workers): number of workers (goroutines) that will process the websocket connections. Each takes about 8kb of RAM
- WS_MSG_EXPIRE (flag --expire): if a message is sent from backend but Goctopus can't find a corresponding connection from frontend, then it will keep the message for this amount of time and if the requested user finally joins then the message will be delivered
- WS_LOGIN (flag --login): login required from backend to send POST-requests
- WS_PASSWORD (flag --password): password required from backend to send POST-requests
- WS_AUTH_URL (flag --auth): forward incomming requests from frontend to this URL in order to authorize a request
- WS_VERBOSE (flag --verbose): wether or not log everything to console


### API

- frontend should create a websocket instance and declare a handler for incoming messages
```
let socket = new WebSocket("ws://goctopus:7890");

socket.onmessage = function(event) {
  // do something with data that commes from backend
  console.log(event.data)
};
```


- backend sends POST-requests with Basic Authentication ([RFC 2617, Section 2](https://www.rfc-editor.org/rfc/rfc2617.html#section-2))

with curl:
```
curl -X POST http://goctopus:7890
   -H "Content-Type: application/json" 
   -d '{"key":"noreply@google.com","value":{"foo":"bar"},"expire":"30m"}'
   --user "login:password"
```


or with python:
```
import os
import json
import requests

message = {"key": "noreply@goole.com", "value": {"foo": "bar"}, "expire": "30m"}
r = requests.post("http://goctopus:7890", data=json.dumps(message), auth=("login", "password"))
```
