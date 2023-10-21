package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
)

var (
	host, port, workers, expire, login, password, authUrl, verbose, storageEngine, authorizerEngine string
)

func main() {
	hostDefault, ok := os.LookupEnv("WS_HOST")
	if !ok {
		hostDefault = "0.0.0.0"
	}
	flag.StringVar(&host, "host", hostDefault, "Hostname to listen to")

	portDefault, ok := os.LookupEnv("WS_PORT")
	if !ok {
		portDefault = "7890"
	}
	flag.StringVar(&port, "port", portDefault, "Port to listen to")

	workersDefault, ok := os.LookupEnv("WS_WORKERS")
	if !ok {
		workersDefault = "1024"
	}
	flag.StringVar(&workers, "workers", workersDefault, "N workers (goroutines) that will handle websocket requests")

	expireDefault, ok := os.LookupEnv("WS_MSG_EXPIRE")
	if !ok {
		expireDefault = "30m"
	}
	flag.StringVar(&expire, "expire", expireDefault, "Time to wait before message expires")

	flag.StringVar(&login, "login", os.Getenv("WS_LOGIN"), "Login to authorize sending websocket messages")
	flag.StringVar(&password, "password", os.Getenv("WS_PASSWORD"), "Password to authorize sending websocket messages")

	flag.StringVar(&authUrl, "auth", os.Getenv("WS_AUTH_URL"), "URL to forward websockets requests to in order to obtain user's identifier")

	verboseDefault, ok := os.LookupEnv("WS_VERBOSE")
	if !ok {
		verboseDefault = "False"
	}
	flag.StringVar(&verbose, "verbose", verboseDefault, "Whether or not log everything to console")

	storageDefault, ok := os.LookupEnv("WS_STORAGE")
	if !ok {
		storageDefault = "default"
	}
	flag.StringVar(&storageEngine, "storage", storageDefault, "Storage engine that is used to keep message queues")

	authorizerDefault, ok := os.LookupEnv("WS_STORAGE")
	if !ok {
		authorizerDefault = "dummy"
	}
	flag.StringVar(&authorizerEngine, "storage", authorizerDefault, "Authorizer engine that is used to authorize incomming http requests")

	flag.Parse()

	os.Setenv("WS_WORKERS", workers)
	os.Setenv("WS_MSG_EXPIRE", expire)
	os.Setenv("WS_LOGIN", login)
	os.Setenv("WS_PASSWORD", password)
	os.Setenv("WS_VERBOSE", verbose)

	if authUrl == "" {
		panic("You must set URL for authenticating incoming websocket requests. You may do that by setting WS_AUTH_URL environment variable or by running goctopus with --auth flag")
	}
	os.Setenv("WS_AUTH_URL", authUrl)

	app := Goctopus{}
	app.Start()

	fmt.Printf("----------------------------------\n")
	fmt.Printf("Goctopus websocket app has started\n")
	fmt.Printf("Listening to: %s:%s\n", host, port)
	fmt.Printf("Num workers is: %s\n", os.Getenv("WS_WORKERS"))
	fmt.Printf("Storage is: %s\n", storageEngine)
	fmt.Printf("Authorizer engine: %s\n", authorizerEngine)
	fmt.Printf("Default message expiry is: %s\n", os.Getenv("WS_MSG_EXPIRE"))
	fmt.Printf("----------------------------------\n\n")

	if err := http.ListenAndServe(fmt.Sprintf("%s:%s", host, port), &app); err != nil {
		log.Fatal(err)
	}
}
