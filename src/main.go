package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// envOr returns the value of an environment variable or a default.
func envOr(name, def string) string {
	if v, ok := os.LookupEnv(name); ok {
		return v
	}
	return def
}

// splitCSV splits a comma-separated list, trimming spaces and dropping empties.
func splitCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func main() {
	var (
		host             = flag.String("host", envOr(WS_HOST, "0.0.0.0"), "Hostname to listen to")
		port             = flag.String("port", envOr(WS_PORT, "7890"), "Port to listen to")
		workers          = flag.String("workers", envOr(WS_WORKERS, "1024"), "N workers (goroutines) that will handle websocket requests")
		expire           = flag.String("expire", envOr(WS_MSG_EXPIRE, "30m"), "Default time to wait before a message expires")
		login            = flag.String("login", os.Getenv(WS_LOGIN), "Login to authorize sending websocket messages")
		password         = flag.String("password", os.Getenv(WS_PASSWORD), "Password to authorize sending websocket messages")
		authURL          = flag.String("auth", os.Getenv(WS_AUTH_URL), "URL to forward websocket requests to in order to obtain user's identifier")
		verbose          = flag.String("verbose", envOr(WS_VERBOSE, "false"), "Whether or not to log everything to console")
		logFormat        = flag.String("log-format", envOr(WS_LOG_FORMAT, "text"), "Structured log format: text or json")
		storageEngine    = flag.String("storage", envOr(WS_STORAGE, DEFAULT), "Storage engine used to keep message queues (memory/redis)")
		authorizerEngine = flag.String("authorizer", envOr(WS_AUTHORIZER, DEFAULT), "Authorizer engine used to authorize incoming requests (proxy/dummy)")
		insecureNoAuth   = flag.String("insecure-no-auth", envOr(WS_INSECURE_NO_AUTH, "false"), "Allow unauthenticated POST requests (DEVELOPMENT ONLY)")
		authTimeout      = flag.String("auth-timeout", envOr(WS_AUTH_TIMEOUT, "10s"), "Timeout for requests to the auth backend")
		authCacheTTL     = flag.String("auth-cache-ttl", envOr(WS_AUTH_CACHE_TTL, "0"), "Cache proxy-auth results per credential for this duration; 0 disables")
		sweepInterval    = flag.String("sweep-interval", envOr(WS_SWEEP_INTERVAL, "1m"), "How often expired messages are swept from storage")
		pingInterval     = flag.String("ping-interval", envOr(WS_PING_INTERVAL, "30s"), "How often to ping idle websocket connections (keepalive)")
		readTimeout      = flag.String("read-timeout", envOr(WS_READ_TIMEOUT, "70s"), "Drop a websocket connection if no frame (incl. pong) arrives within this time")
		writeTimeout     = flag.String("write-timeout", envOr(WS_WRITE_TIMEOUT, "10s"), "Bound each websocket write so a slow client can't pin a worker")
		reconcileEvery   = flag.String("reconcile-interval", envOr(WS_RECONCILE_INTERVAL, "0"), "Periodically re-flush local keys (safety net for multi-instance Redis pub/sub); 0 disables")
		redisURL         = flag.String("redis-url", envOr(WS_REDIS_URL, "redis://localhost:6379/0"), "Redis connection URL when --storage=redis")
		redisKeyTTL      = flag.String("redis-key-ttl", envOr(WS_REDIS_KEY_TTL, "24h"), "Redis EXPIRE backstop on each queue key; 0 disables")
		tlsCert          = flag.String("tls-cert", os.Getenv(WS_TLS_CERT), "Path to TLS certificate file. Set with --tls-key to serve over TLS (wss://)")
		tlsKey           = flag.String("tls-key", os.Getenv(WS_TLS_KEY), "Path to TLS private key file")
		allowedOrigins   = flag.String("allowed-origins", os.Getenv(WS_ALLOWED_ORIGINS), "Comma-separated whitelist of browser Origins allowed to open websockets (empty = any, '*' = any)")
		maxMessageSize   = flag.String("max-message-size", envOr(WS_MAX_MESSAGE_SIZE, "1048576"), "Maximum size in bytes of a POST body / inbound websocket message")
		rateLimit        = flag.String("rate-limit", envOr(WS_RATE_LIMIT, "0"), "Per-client-IP request rate (events/sec) for the API and ws upgrades; 0 disables")
		rateBurst        = flag.String("rate-burst", envOr(WS_RATE_BURST, "0"), "Token-bucket burst capacity for --rate-limit")
	)
	flag.Parse()

	if *authURL == EMPTY_STR {
		panic("You must set URL for authenticating incoming websocket requests. You may do that by setting WS_AUTH_URL environment variable or by running goctopus with --auth flag")
	}

	nWorkers, err := strconv.Atoi(*workers)
	if err != nil {
		panic(WS_WORKERS_NOT_FOUND)
	}

	maxMsg, err := strconv.ParseInt(*maxMessageSize, 10, 64)
	if err != nil || maxMsg <= 0 {
		maxMsg = 1 << 20
	}

	rl, _ := strconv.ParseFloat(*rateLimit, 64)
	rb, _ := strconv.Atoi(*rateBurst)

	cfg := &Config{
		Host:              *host,
		Port:              *port,
		Workers:           nWorkers,
		DefaultExpire:     *expire,
		Verbose:           parseBool(*verbose),
		LogJSON:           *logFormat == "json",
		Login:             *login,
		Password:          *password,
		InsecureNoAuth:    parseBool(*insecureNoAuth),
		StorageEngine:     *storageEngine,
		AuthorizerEngine:  *authorizerEngine,
		AuthURL:           *authURL,
		AuthTimeout:       parseDurationOr(*authTimeout, 10*time.Second),
		AuthCacheTTL:      parseDurationOr(*authCacheTTL, 0),
		RedisURL:          *redisURL,
		RedisKeyTTL:       parseDurationOr(*redisKeyTTL, 0),
		SweepInterval:     parseDurationOr(*sweepInterval, time.Minute),
		PingInterval:      parseDurationOr(*pingInterval, 30*time.Second),
		ReadTimeout:       parseDurationOr(*readTimeout, 70*time.Second),
		WriteTimeout:      parseDurationOr(*writeTimeout, 10*time.Second),
		ReconcileInterval: parseDurationOr(*reconcileEvery, 0),
		TLSCert:           *tlsCert,
		TLSKey:            *tlsKey,
		AllowedOrigins:    splitCSV(*allowedOrigins),
		MaxMessageBytes:   maxMsg,
		RateLimit:         rl,
		RateBurst:         rb,
	}

	app := Goctopus{}
	app.Start(cfg)

	fmt.Printf("----------------------------------\n")
	fmt.Printf("Goctopus websocket app has started\n")
	fmt.Printf("Listening to: %s:%s\n", cfg.Host, cfg.Port)
	fmt.Printf("Num workers is: %d\n", cfg.Workers)
	fmt.Printf("Storage is: %s\n", cfg.StorageEngine)
	fmt.Printf("Authorizer engine: %s\n", cfg.AuthorizerEngine)
	fmt.Printf("Default message expiry is: %s\n", cfg.DefaultExpire)
	if cfg.tlsEnabled() {
		fmt.Printf("TLS: enabled (wss://)\n")
	}
	fmt.Printf("----------------------------------\n\n")

	srv := &http.Server{
		Addr:              fmt.Sprintf("%s:%s", cfg.Host, cfg.Port),
		Handler:           &app,
		ReadHeaderTimeout: 10 * time.Second, // Slowloris protection
		ReadTimeout:       30 * time.Second,
		IdleTimeout:       120 * time.Second,
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
		app.Stop()
		close(idleClosed)
	}()

	var serveErr error
	if cfg.tlsEnabled() {
		// Serving over TLS makes the websocket endpoint available as wss://.
		serveErr = srv.ListenAndServeTLS(cfg.TLSCert, cfg.TLSKey)
	} else {
		serveErr = srv.ListenAndServe()
	}
	if serveErr != nil && serveErr != http.ErrServerClosed {
		log.Fatal(serveErr)
	}
	<-idleClosed
}
