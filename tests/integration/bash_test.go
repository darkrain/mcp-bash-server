package integration

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"mcp-bash-server/config"
	"mcp-bash-server/tests/helpers"
)

func callTool(t *testing.T, ts *helpers.TestServer, cmd string) (map[string]any, *http.Response) {
	t.Helper()
	ts.MCPRequest(t, "initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo": map[string]any{
			"name":    "test-client",
			"version": "1.0.0",
		},
	}).Body.Close()

	resp := ts.MCPRequest(t, "tools/call", map[string]any{
		"name": "bash",
		"arguments": map[string]any{
			"command": cmd,
		},
	})

	body := helpers.ReadBody(t, resp)
	var result map[string]any
	json.Unmarshal([]byte(body), &result)
	return result, resp
}

func TestHealthEndpoint(t *testing.T) {
	ts := helpers.NewTestServer(t, nil)
	resp, err := ts.Client.Get(ts.HTTPAddr + "/health")
	if err != nil {
		t.Fatalf("health check failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	body := helpers.ReadBody(t, resp)
	if !strings.Contains(body, `"status":"ok"`) {
		t.Errorf("expected ok, got: %s", body)
	}
}

func TestMCPInitialize(t *testing.T) {
	ts := helpers.NewTestServer(t, nil)
	resp := ts.MCPRequest(t, "initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo": map[string]any{
			"name": "test-client", "version": "1.0.0",
		},
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("initialize failed: %d", resp.StatusCode)
	}
	body := helpers.ReadBody(t, resp)
	if !strings.Contains(body, "serverInfo") {
		t.Errorf("expected serverInfo, got: %s", body)
	}
}

func TestToolsList(t *testing.T) {
	ts := helpers.NewTestServer(t, nil)
	ts.MCPRequest(t, "initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo": map[string]any{
			"name": "test-client", "version": "1.0.0",
		},
	}).Body.Close()

	resp := ts.MCPRequest(t, "tools/list", map[string]any{})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("tools/list failed: %d", resp.StatusCode)
	}
	body := helpers.ReadBody(t, resp)
	if !strings.Contains(body, "bash") {
		t.Errorf("expected bash tool, got: %s", body)
	}
}

func TestBashEcho(t *testing.T) {
	ts := helpers.NewTestServer(t, nil)
	result, resp := callTool(t, ts, "echo 'hello world'")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("tool call failed: %d", resp.StatusCode)
	}
	raw, _ := json.Marshal(result)
	if !strings.Contains(string(raw), "hello world") {
		t.Errorf("expected hello world, got: %s", string(raw))
	}
}

func TestBashEmptyCommand(t *testing.T) {
	ts := helpers.NewTestServer(t, nil)
	result, resp := callTool(t, ts, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("tool call failed: %d", resp.StatusCode)
	}
	raw, _ := json.Marshal(result)
	if !strings.Contains(string(raw), "command is empty") {
		t.Errorf("expected empty error, got: %s", string(raw))
	}
}

func TestBashPwd(t *testing.T) {
	ts := helpers.NewTestServer(t, nil)
	result, resp := callTool(t, ts, "pwd")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("tool call failed: %d", resp.StatusCode)
	}
	raw, _ := json.Marshal(result)
	if !strings.Contains(string(raw), "/") {
		t.Errorf("expected path, got: %s", string(raw))
	}
}

func TestAPIKeyRequired(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Server.APIKey = "secret"
	ts := helpers.NewTestServer(t, cfg)
	resp := ts.SendRaw(t, "POST", ts.BaseURL+"/", []byte(`{}`), nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}
}

func TestAPIKeySuccess(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Server.APIKey = "secret"
	ts := helpers.NewTestServer(t, cfg)
	resp := ts.SendRaw(t, "POST", ts.BaseURL+"/", []byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`), map[string]string{
		"Authorization": "Bearer secret",
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestAllowedCommands(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Bash.AllowedCommands = []string{"echo"}
	ts := helpers.NewTestServer(t, cfg)
	result, resp := callTool(t, ts, "cat /etc/passwd")
	raw, _ := json.Marshal(result)
	if !strings.Contains(string(raw), "not in the allowed commands list") {
		t.Errorf("expected rejection, got: %s", string(raw))
	}
	_ = resp
}
