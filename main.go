package main

import (
	"context"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"mcp-bash-server/config"
	"mcp-bash-server/server"
)

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

	logger := setupLogger(cfg)
	logger.Info("starting MCP bash server", "addr", cfg.ListenAddr())

	mcpServer, sysInfo := server.NewMCPServer(cfg, logger)
	logger.Info("server info", "hostname", sysInfo.Hostname, "ips", sysInfo.IPs, "user", sysInfo.User)

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
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})

	var wrappedHandler http.Handler = mux
	if cfg.Server.APIKey != "" {
		wrappedHandler = apiKeyMiddleware(wrappedHandler, cfg.Server.APIKey, logger)
	}

	wrappedHandler = loggingMiddleware(wrappedHandler, logger)

	srv := &http.Server{
		Addr:         cfg.ListenAddr(),
		Handler:      wrappedHandler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 300 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

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

	var opts *slog.HandlerOptions
	opts = &slog.HandlerOptions{Level: level}

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
			w.Write([]byte(`{"error":"unauthorized"}`))
			return
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
