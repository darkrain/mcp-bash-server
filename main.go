package main

import (
	"context"
	"encoding/json"
	"log"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"mcp-bash-server/config"
	"mcp-bash-server/server"
)

// Security Configuration
const (
	maxRequestBodySize = 10 * 1024 * 1024 // 10MB
	maxHeaderBytes     = 1 * 1024 * 1024  // 1MB
	rateLimitRequests  = 10               // per second per IP
	rateLimitWindow    = time.Second
)

// RateLimiter implements token bucket algorithm per IP
type RateLimiter struct {
	mu     sync.RWMutex
	limits map[string]*tokenBucket
	window time.Duration
	maxReq int
}

type tokenBucket struct {
	tokens     int
	lastRefill time.Time
}

func newRateLimiter(maxReq int, window time.Duration) *RateLimiter {
	return &RateLimiter{
		limits: make(map[string]*tokenBucket),
		window: window,
		maxReq: maxReq,
	}
}

func (rl *RateLimiter) allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	bucket, exists := rl.limits[ip]
	now := time.Now()

	if !exists {
		rl.limits[ip] = &tokenBucket{tokens: rl.maxReq - 1, lastRefill: now}
		return true
	}

	// Refill tokens based on elapsed time
	elapsed := now.Sub(bucket.lastRefill)
	refillAmount := int(elapsed / rl.window)
	if refillAmount > 0 {
		bucket.tokens = min(rl.maxReq, bucket.tokens+refillAmount)
		bucket.lastRefill = now
	}

	// Decrement and check
	if bucket.tokens > 0 {
		bucket.tokens--
		return true
	}
	return false
}

func (rl *RateLimiter) cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	for ip, bucket := range rl.limits {
		// Remove IPs that haven't had activity in 5 minutes
		if now.Sub(bucket.lastRefill) > 5*time.Minute {
			delete(rl.limits, ip)
		}
	}
}

func main() {
	configPath := os.Getenv("MCP_CONFIG_PATH")
	if configPath == "" {
		if _, err := os.Stat("/etc/mcp-bash-server/config.toml"); err == nil {
			configPath = "/etc/mcp-bash-server/config.toml"
		}
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Validate API key strength if set
	if cfg.Server.APIKey != "" && len(cfg.Server.APIKey) < 16 {
		logger := setupLogger(cfg)
		logger.Warn("API key is weak (less than 16 characters). Consider using a stronger key.")
	}

	logger := setupLogger(cfg)
	logger.Info("starting MCP bash server", "addr", cfg.ListenAddr())

	mcpServer, sysInfo := server.NewMCPServer(cfg, logger)
	logger.Info("server info", "hostname", sysInfo.Hostname, "ips", sysInfo.IPs, "user", sysInfo.User)

	// Setup Streamable HTTP handler
	handler := mcp.NewStreamableHTTPHandler(
		func(r *http.Request) *mcp.Server {
			return mcpServer
		},
		&mcp.StreamableHTTPOptions{
			Stateless:    true,
			JSONResponse: true,
			Logger:       logger,
		},
	)

	mux := http.NewServeMux()
	baseURL := strings.TrimSuffix(cfg.Server.BaseURL, "/")
	mux.Handle(baseURL+"/", handler)

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		headers := securityHeaders()
		for k, v := range headers {
			w.Header().Set(k, v)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})

	// Middleware chain: security -> rate limit -> body size -> auth -> log
	var wrappedHandler http.Handler = mux
	wrappedHandler = securityHeadersMiddleware(wrappedHandler)
	wrappedHandler = rateLimitMiddleware(wrappedHandler, newRateLimiter(rateLimitRequests, rateLimitWindow))
	wrappedHandler = maxRequestBodySizeMiddleware(wrappedHandler, maxRequestBodySize)

	if cfg.Server.APIKey != "" {
		wrappedHandler = apiKeyMiddleware(wrappedHandler, cfg.Server.APIKey, logger)
	}
	wrappedHandler = loggingMiddleware(wrappedHandler, logger)

	srv := &http.Server{
		Addr:           cfg.ListenAddr(),
		Handler:        wrappedHandler,
		ReadTimeout:    30 * time.Second,
		WriteTimeout:   300 * time.Second,
		IdleTimeout:    120 * time.Second,
		MaxHeaderBytes: maxHeaderBytes,
	}

	// Start rate limiter cleanup goroutine
	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			if rl, ok := wrappedHandler.(*rateLimitMiddlewareWrapper); ok {
				if rlm := rl.limiter; rlm != nil {
					rlm.cleanup()
				}
			}
		}
	}()

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server failed", "error", err)
			os.Exit(1)
		}
	}()

	logger.Info("server listening", "addr", cfg.ListenAddr())

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	logger.Info("shutting down server")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("server shutdown error", "error", err)
	}

	logger.Info("server stopped")
}

func setupLogger(cfg *config.Config) *slog.Logger {
	var level slog.Level
	switch strings.ToLower(cfg.Log.Level) {
	case "debug":
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "warn", "warning":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{Level: level}

	var handler slog.Handler
	switch strings.ToLower(cfg.Log.Format) {
	case "text":
		handler = slog.NewTextHandler(os.Stderr, opts)
	default:
		handler = slog.NewJSONHandler(os.Stderr, opts)
	}

	if cfg.Log.Output == "stdout" {
		switch strings.ToLower(cfg.Log.Format) {
		case "text":
			handler = slog.NewTextHandler(os.Stdout, opts)
		default:
			handler = slog.NewJSONHandler(os.Stdout, opts)
		}
	}

	return slog.New(handler)
}

func apiKeyMiddleware(next http.Handler, apiKey string, logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			next.ServeHTTP(w, r)
			return
		}

		auth := r.Header.Get("Authorization")
		if auth == "" {
			auth = r.Header.Get("X-API-Key")
		} else if strings.HasPrefix(auth, "Bearer ") {
			auth = strings.TrimPrefix(auth, "Bearer ")
		}

		if auth != apiKey {
			logger.Warn("unauthorized request", "remote_addr", r.RemoteAddr, "path", r.URL.Path)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
			return
		}

		next.ServeHTTP(w, r)
	})
}

type rateLimitMiddlewareWrapper struct {
	next    http.Handler
	limiter *RateLimiter
}

func rateLimitMiddleware(next http.Handler, limiter *RateLimiter) http.Handler {
	return &rateLimitMiddlewareWrapper{next: next, limiter: limiter}
}

func (rlmw *rateLimitMiddlewareWrapper) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		ip = r.RemoteAddr
	}

	if !rlmw.limiter.allow(ip) {
		headers := securityHeaders()
		for k, v := range headers {
			w.Header().Set(k, v)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		json.NewEncoder(w).Encode(map[string]string{"error": "rate limit exceeded"})
		return
	}

	rlmw.next.ServeHTTP(w, r)
}

func maxRequestBodySizeMiddleware(next http.Handler, maxBytes int64) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
		next.ServeHTTP(w, r)
	})
}

func securityHeaders() map[string]string {
	return map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"X-XSS-Protection":       "1; mode=block",
	}
}

func securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers := securityHeaders()
		for k, v := range headers {
			w.Header().Set(k, v)
		}
		next.ServeHTTP(w, r)
	})
}

func loggingMiddleware(next http.Handler, logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		logger.Debug("request started", "method", r.Method, "path", r.URL.Path, "remote_addr", r.RemoteAddr)
		next.ServeHTTP(w, r)
		logger.Debug("request completed", "method", r.Method, "path", r.URL.Path, "duration", time.Since(start).Milliseconds())
	})
}
