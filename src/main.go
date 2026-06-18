package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

var (
	host, port, workers, expire, login, password, authUrl, verbose, storageEngine, authorizerEngine string
	insecureNoAuth, authTimeout, sweepInterval                                                      string
)

func main() {
	// TODO: MOVE ALL OF THESE LITERALS TO CONSTANTS

	hostDefault, ok := os.LookupEnv(WS_HOST)
	if !ok {
		hostDefault = "0.0.0.0"
	}
	flag.StringVar(&host, "host", hostDefault, "Hostname to listen to")

	portDefault, ok := os.LookupEnv(WS_PORT)
	if !ok {
		portDefault = "7890"
	}
	flag.StringVar(&port, "port", portDefault, "Port to listen to")

	workersDefault, ok := os.LookupEnv(WS_WORKERS)
	if !ok {
		workersDefault = "1024"
	}
	flag.StringVar(&workers, "workers", workersDefault, "N workers (goroutines) that will handle websocket requests")

	expireDefault, ok := os.LookupEnv(WS_MSG_EXPIRE)
	if !ok {
		expireDefault = "30m"
	}
	flag.StringVar(&expire, "expire", expireDefault, "Time to wait before message expires")

	flag.StringVar(&login, "login", os.Getenv(WS_LOGIN), "Login to authorize sending websocket messages")
	flag.StringVar(&password, "password", os.Getenv(WS_PASSWORD), "Password to authorize sending websocket messages")

	flag.StringVar(&authUrl, "auth", os.Getenv(WS_AUTH_URL), "URL to forward websockets requests to in order to obtain user's identifier")

	verboseDefault, ok := os.LookupEnv(WS_VERBOSE)
	if !ok {
		verboseDefault = "False"
	}
	flag.StringVar(&verbose, "verbose", verboseDefault, "Whether or not log everything to console")

	storageDefault, ok := os.LookupEnv(WS_STORAGE)
	if !ok {
		storageDefault = DEFAULT
	}
	flag.StringVar(&storageEngine, "storage", storageDefault, "Storage engine that is used to keep message queues")

	authorizerDefault, ok := os.LookupEnv(WS_AUTHORIZER)
	if !ok {
		authorizerDefault = DEFAULT
	}
	flag.StringVar(&authorizerEngine, "authorizer", authorizerDefault, "Authorizer engine that is used to authorize incomming http requests")

	insecureDefault, ok := os.LookupEnv(WS_INSECURE_NO_AUTH)
	if !ok {
		insecureDefault = "false"
	}
	flag.StringVar(&insecureNoAuth, "insecure-no-auth", insecureDefault, "Allow unauthenticated POST requests (DEVELOPMENT ONLY)")

	authTimeoutDefault, ok := os.LookupEnv(WS_AUTH_TIMEOUT)
	if !ok {
		authTimeoutDefault = "10s"
	}
	flag.StringVar(&authTimeout, "auth-timeout", authTimeoutDefault, "Timeout for requests to the auth backend")

	sweepDefault, ok := os.LookupEnv(WS_SWEEP_INTERVAL)
	if !ok {
		sweepDefault = "1m"
	}
	flag.StringVar(&sweepInterval, "sweep-interval", sweepDefault, "How often expired messages are swept from storage")

	flag.Parse()

	os.Setenv(WS_WORKERS, workers)
	os.Setenv(WS_MSG_EXPIRE, expire)
	os.Setenv(WS_LOGIN, login)
	os.Setenv(WS_PASSWORD, password)
	os.Setenv(WS_VERBOSE, verbose)
	os.Setenv(WS_INSECURE_NO_AUTH, insecureNoAuth)
	os.Setenv(WS_AUTH_TIMEOUT, authTimeout)
	os.Setenv(WS_SWEEP_INTERVAL, sweepInterval)

	if authUrl == EMPTY_STR {
		panic("You must set URL for authenticating incoming websocket requests. You may do that by setting WS_AUTH_URL environment variable or by running goctopus with --auth flag")
	}
	os.Setenv(WS_AUTH_URL, authUrl)

	app := Goctopus{}
	app.Start()

	fmt.Printf("----------------------------------\n")
	fmt.Printf("Goctopus websocket app has started\n")
	fmt.Printf("Listening to: %s:%s\n", host, port)
	fmt.Printf("Num workers is: %s\n", os.Getenv(WS_WORKERS))
	fmt.Printf("Storage is: %s\n", storageEngine)
	fmt.Printf("Authorizer engine: %s\n", authorizerEngine)
	fmt.Printf("Default message expiry is: %s\n", os.Getenv(WS_MSG_EXPIRE))
	fmt.Printf("----------------------------------\n\n")

	srv := &http.Server{
		Addr:    fmt.Sprintf("%s:%s", host, port),
		Handler: &app,
	}

	// Graceful shutdown: stop accepting on SIGINT/SIGTERM and drain in-flight
	// requests before exiting.
	idleClosed := make(chan struct{})
	go func() {
		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
		<-sigs

		fmt.Printf("\nShutting down Goctopus...\n")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			log.Printf("graceful shutdown error: %s", err)
		}
		close(idleClosed)
	}()

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
	<-idleClosed
}
