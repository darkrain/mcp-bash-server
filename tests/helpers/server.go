package helpers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"mcp-bash-server/config"
	mcpserver "mcp-bash-server/server"
)

type TestServer struct {
	Server     *mcp.Server
	HTTPAddr   string
	Client     *http.Client
	APIKey     string
	BaseURL    string
	httpServer *http.Server
	registry   *mcpserver.ProcessRegistry
	tempDir    string
}

func NewTestServer(t *testing.T, cfg *config.Config) *TestServer {
	t.Helper()

	tempDir := t.TempDir()

	if cfg == nil {
		cfg = config.DefaultConfig()
	}

	cfg.Server.Host = "127.0.0.1"
	cfg.Server.Port = 0
	cfg.Bash.ProcessDir = filepath.Join(tempDir, "proc")

	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
	mcpServer, _, registry, err := mcpserver.NewMCPServer(cfg, logger, "test")
	if err != nil {
		t.Fatalf("failed to create MCP server: %v", err)
	}

	handler := mcp.NewStreamableHTTPHandler(
		func(r *http.Request) *mcp.Server { return mcpServer },
		&mcp.StreamableHTTPOptions{
			Stateless:    true,
			JSONResponse: true,
			Logger:       logger,
		},
	)

	mux := http.NewServeMux()
	baseURL := cfg.Server.BaseURL
	if baseURL == "" {
		baseURL = "/mcp"
	}
	mux.Handle(baseURL+"/", handler)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})

	var wrappedHandler http.Handler = mux
	if cfg.Server.APIKey != "" {
		wrappedHandler = apiKeyMiddleware(wrappedHandler, cfg.Server.APIKey)
	}

	httpServer := &http.Server{
		Handler:      wrappedHandler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 300 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	ln, err := net.Listen("tcp", cfg.ListenAddr())
	if err != nil {
		t.Fatalf("failed to create listener: %v", err)
	}

	go func() {
		httpServer.Serve(ln)
	}()

	addr := fmt.Sprintf("http://%s", ln.Addr().String())

	client := &http.Client{Timeout: 5 * time.Second}
	for i := 0; i < 50; i++ {
		resp, err := client.Get(addr + "/health")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
	}

	ts := &TestServer{
		Server:     mcpServer,
		HTTPAddr:   addr,
		Client:     client,
		APIKey:     cfg.Server.APIKey,
		BaseURL:    baseURL,
		httpServer: httpServer,
		registry:   registry,
		tempDir:    tempDir,
	}

	t.Cleanup(func() {
		registry.Stop()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		ts.httpServer.Shutdown(ctx)
	})

	return ts
}

func (ts *TestServer) MCPRequest(t *testing.T, method string, params map[string]any) *http.Response {
	t.Helper()
	return ts.SendJSON(t, ts.BaseURL+"/", map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  method,
		"params":  params,
	})
}

func (ts *TestServer) SendJSON(t *testing.T, path string, payload any) *http.Response {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("failed to marshal JSON: %v", err)
	}
	req, err := http.NewRequest("POST", ts.HTTPAddr+path, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Mcp-Session-Id", "test-session-0001")
	if ts.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+ts.APIKey)
	}

	resp, err := ts.Client.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	return resp
}

func (ts *TestServer) SendRaw(t *testing.T, method, path string, body []byte, headers map[string]string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, ts.HTTPAddr+path, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := ts.Client.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	return resp
}

func ReadBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read body: %v", err)
	}
	return string(body)
}

func apiKeyMiddleware(next http.Handler, apiKey string) http.Handler {
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
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"unauthorized"}`))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func init() {
	_ = os.Stderr
}
