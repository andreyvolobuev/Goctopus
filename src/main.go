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

	"gopkg.in/yaml.v3"
)

// envOr returns the value of an environment variable or a default.
func envOr(name, def string) string {
	if v, ok := os.LookupEnv(name); ok {
		return v
	}
	return def
}

// loadConfigFile reads a YAML file into a flag-name -> string-value map. Lists
// (e.g. allowed-origins) are joined with commas to match the flag format.
func loadConfigFile(path string) (map[string]string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var raw map[string]any
	if err := yaml.Unmarshal(b, &raw); err != nil {
		return nil, err
	}
	out := make(map[string]string, len(raw))
	for k, v := range raw {
		if list, ok := v.([]any); ok {
			parts := make([]string, len(list))
			for i, e := range list {
				parts[i] = fmt.Sprint(e)
			}
			out[k] = strings.Join(parts, ",")
			continue
		}
		out[k] = fmt.Sprint(v)
	}
	return out, nil
}

// applyConfigFile overlays YAML values onto flags the user did not set
// explicitly (precedence: explicit flag > config file > env/default).
func applyConfigFile(path string) {
	fileVals, err := loadConfigFile(path)
	if err != nil {
		log.Fatalf("config file %q: %s", path, err)
	}
	explicit := map[string]bool{}
	flag.Visit(func(f *flag.Flag) { explicit[f.Name] = true })
	flag.VisitAll(func(f *flag.Flag) {
		if explicit[f.Name] {
			return
		}
		if v, ok := fileVals[f.Name]; ok {
			_ = f.Value.Set(v)
		}
	})
}

// runHealthcheck probes the local /healthz endpoint and exits 0 (healthy) or 1.
func runHealthcheck(port string) {
	client := http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%s/healthz", port))
	if err != nil {
		fmt.Fprintln(os.Stderr, "healthcheck:", err)
		os.Exit(1)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		os.Exit(1)
	}
}

// validateConfig logs warnings for configurations that are valid but likely
// mistakes.
func validateConfig(cfg *Config) {
	if cfg.ReadTimeout <= cfg.PingInterval {
		fmt.Fprintf(os.Stderr, "warning: read-timeout (%s) <= ping-interval (%s); idle clients may be dropped before a pong arrives\n",
			cfg.ReadTimeout, cfg.PingInterval)
	}
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
		historySize      = flag.String("history-size", envOr(WS_HISTORY_SIZE, "0"), "Per-key number of recent messages kept for the history endpoint; 0 disables")
		historyTTL       = flag.String("history-ttl", envOr(WS_HISTORY_TTL, "1h"), "Max age of retained history messages")
		redisURL         = flag.String("redis-url", envOr(WS_REDIS_URL, "redis://localhost:6379/0"), "Redis connection URL when --storage=redis")
		redisKeyTTL      = flag.String("redis-key-ttl", envOr(WS_REDIS_KEY_TTL, "24h"), "Redis EXPIRE backstop on each queue key; 0 disables")
		tlsCert          = flag.String("tls-cert", os.Getenv(WS_TLS_CERT), "Path to TLS certificate file. Set with --tls-key to serve over TLS (wss://)")
		tlsKey           = flag.String("tls-key", os.Getenv(WS_TLS_KEY), "Path to TLS private key file")
		allowedOrigins   = flag.String("allowed-origins", os.Getenv(WS_ALLOWED_ORIGINS), "Comma-separated whitelist of browser Origins allowed to open websockets (empty = any, '*' = any)")
		maxMessageSize   = flag.String("max-message-size", envOr(WS_MAX_MESSAGE_SIZE, "1048576"), "Maximum size in bytes of a POST body / inbound websocket message")
		compression      = flag.String("compress", envOr(WS_COMPRESS, "false"), "Enable permessage-deflate compression for websocket connections")
		rateLimit        = flag.String("rate-limit", envOr(WS_RATE_LIMIT, "0"), "Per-client-IP request rate (events/sec) for the API and ws upgrades; 0 disables")
		rateBurst        = flag.String("rate-burst", envOr(WS_RATE_BURST, "0"), "Token-bucket burst capacity for --rate-limit")
		trustProxy       = flag.String("trust-proxy-headers", envOr(WS_TRUST_PROXY_HEADERS, "false"), "Honour X-Forwarded-For/X-Real-IP for client IP (only behind a trusted proxy)")
		configPath       = flag.String("config", os.Getenv(WS_CONFIG), "Path to a YAML config file (keys are flag names; explicit flags override it)")
		healthcheck      = flag.Bool("healthcheck", false, "Probe /healthz on the configured port and exit 0/1 (for container HEALTHCHECK)")
	)
	flag.Parse()

	if *configPath != EMPTY_STR {
		applyConfigFile(*configPath)
	}

	// Self-healthcheck mode: usable as a container HEALTHCHECK without a shell.
	if *healthcheck {
		runHealthcheck(*port)
		return
	}

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
	histSize, _ := strconv.Atoi(*historySize)

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
		HistorySize:       histSize,
		HistoryTTL:        parseDurationOr(*historyTTL, time.Hour),
		TLSCert:           *tlsCert,
		TLSKey:            *tlsKey,
		AllowedOrigins:    splitCSV(*allowedOrigins),
		MaxMessageBytes:   maxMsg,
		Compression:       parseBool(*compression),
		RateLimit:         rl,
		RateBurst:         rb,
		TrustProxyHeaders: parseBool(*trustProxy),
	}

	validateConfig(cfg)

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
