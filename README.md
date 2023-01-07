# Goctopus

Simple websocket service that will work with literally any backend and frontend. You can use it with Django, FastAPI, Flask etc. All that a backend has to be able to do, in order to send websocket messages to frontend, is to be able to do HTTP POST requests with Basic authorization.


### Overview

Goctopus runs as a server app on selected hostname and port waiting for incomming connections. Frontend should connect via websocket protocol. 

The HTTP request for establishing websocket connection will be forwarded to address pointed to by `WS_AUTH_URL` environment variable. The structure of response from that endpoint is described in [src/authorization.go](https://github.com/andreyvolobuev/goctopus/blob/master/src/authorization.go). There's func `Export` that will return a list of strings. These are keys (one might call them topics) that will identify current connection. That may be user id or email or even an organization name (in case you want to send a message to multiple users).

Backend on the other hand has to know how to identify a user. Once an event happended, backend app may use that identifier to send a message to the corresponding user/group of users. That is achieved by sending a POST-request go Goctopus.



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
	return []string{r.User.Email, r.User.Ogranization}
}
```

3. Set required environment variables, compile and run the app:
```
export WS_HOST=0.0.0.0
export WS_PORT=7890
export WS_WORKERS=128
export WS_MSG_EXPIRE=30m
export WS_LOGIN=amdin
export WS_PASSWORD=password
export WS_AUTH_URL=http://localhost/api/v1/is_authenticated

go build -o goctopus src/.
./goctopus
```

you may also just use Docker:
```
docker build -t goctopus .
docker run \
    -e WS_HOST=0.0.0.0 \
    -e WS_PORT=7890 \
    -e WS_WORKERS=128 \
    -e WS_MSG_EXPIRE=30m \
    -e WS_LOGIN=admin \
    -e WS_PASSWORD=password \
    -e WS_AUTH_URL=http://localhost/api/v1/is_authenticated \
    goctopus
```

*Please note that Goctopus requires the following environment variables to be set in order to work*
- WS_HOST: hostname or ip that the server will run on
- WS_PORT: port that the server will listen to
- WS_WORKERS: number of workers (goroutines) that will process the websocket connections. Each takes about 8kb of RAM
- WS_MSG_EXPIRE: if a message is sent from backend but Goctopus can't find a corresponding connection from frontend, then it will keep the message for this amount of time and if the requested user finally joins then the message will be delivered
- WS_LOGIN: login required from backend to send POST-requests
- WS_PASSWORD: password required from backend to send POST-requests
- WS_AUTH_URL: forward incomming requests from frontend to this URL in order to authorize a request


### API

- frontend should create a websocket instance and declare a handler for incoming messages
```
let socket = new WebSocket("ws://localhost:7890");

socket.onmessage = function(event) {
  // do something with data that commes from backend
  console.log(event.data)
};
```


- backend sends POST-requests with Basic Authentication ([RFC 2617, Section 2](https://www.rfc-editor.org/rfc/rfc2617.html#section-2))

with curl:
```
curl -X POST http://localhost
   -H "Content-Type: application/json" 
   -d '{"key":"noreply@google.com","value":{"foo":"bar"},"expire":"30m"}'
   --user "login:password"
```


or with python:
```
import os
import json
import requests

login = os.environ.get("WS_LOGIN")
password = os.environ.get("WS_PASSWORD")

message = {"key": "noreply@goole.com", "value": {"foo": "bar"}, "expire": "30m"}
r = requests.post("http://localhost", data=json.dumps(message), auth=(login, password))
```
