package integration

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"mcp-bash-server/config"
	"mcp-bash-server/tests/helpers"
)

func initMCP(t *testing.T, ts *helpers.TestServer) {
	t.Helper()
	ts.MCPRequest(t, "initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo": map[string]any{
			"name":    "test-client",
			"version": "1.0.0",
		},
	}).Body.Close()
}

func callTool(t *testing.T, ts *helpers.TestServer, cmd string) (map[string]any, *http.Response) {
	t.Helper()
	initMCP(t, ts)

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

func callAsyncTool(t *testing.T, ts *helpers.TestServer, cmd string) (map[string]any, *http.Response) {
	t.Helper()
	initMCP(t, ts)

	resp := ts.MCPRequest(t, "tools/call", map[string]any{
		"name": "bash_async",
		"arguments": map[string]any{
			"command": cmd,
		},
	})

	body := helpers.ReadBody(t, resp)
	var result map[string]any
	json.Unmarshal([]byte(body), &result)
	return result, resp
}

func callProcessTool(t *testing.T, ts *helpers.TestServer, toolName, processID string) (map[string]any, *http.Response) {
	t.Helper()
	resp := ts.MCPRequest(t, "tools/call", map[string]any{
		"name": toolName,
		"arguments": map[string]any{
			"process_id": processID,
		},
	})

	body := helpers.ReadBody(t, resp)
	var result map[string]any
	json.Unmarshal([]byte(body), &result)
	return result, resp
}

func extractProcessID(t *testing.T, result map[string]any) string {
	t.Helper()
	raw, _ := json.Marshal(result)
	s := string(raw)
	idx := strings.Index(s, "ID: ")
	if idx < 0 {
		t.Fatalf("process ID not found in result: %s", s)
	}
	remainder := s[idx+4:]
	end := strings.IndexAny(remainder, " \n\\\"")
	if end < 0 {
		end = len(remainder)
	}
	pid := strings.TrimSpace(remainder[:end])
	if len(pid) != 16 {
		t.Fatalf("unexpected process ID length: %q", pid)
	}
	return pid
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
	initMCP(t, ts)

	resp := ts.MCPRequest(t, "tools/list", map[string]any{})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("tools/list failed: %d", resp.StatusCode)
	}
	body := helpers.ReadBody(t, resp)
	for _, name := range []string{"bash", "bash_async", "process_status", "process_output", "process_kill", "process_list"} {
		if !strings.Contains(body, name) {
			t.Errorf("expected %s tool, got: %s", name, body)
		}
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

func TestBashAsyncQuick(t *testing.T) {
	ts := helpers.NewTestServer(t, nil)
	result, resp := callAsyncTool(t, ts, "echo 'async hello'")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("bash_async call failed: %d", resp.StatusCode)
	}
	raw, _ := json.Marshal(result)
	if !strings.Contains(string(raw), "Process started") {
		t.Errorf("expected process start, got: %s", string(raw))
	}
	pid := extractProcessID(t, result)

	time.Sleep(500 * time.Millisecond)

	statusResult, _ := callProcessTool(t, ts, "process_status", pid)
	statusRaw, _ := json.Marshal(statusResult)
	if !strings.Contains(string(statusRaw), "completed") {
		t.Errorf("expected completed status, got: %s", string(statusRaw))
	}

	outputResult, _ := callProcessTool(t, ts, "process_output", pid)
	outputRaw, _ := json.Marshal(outputResult)
	if !strings.Contains(string(outputRaw), "async hello") {
		t.Errorf("expected async hello in output, got: %s", string(outputRaw))
	}
}

func TestBashAsyncEmptyCommand(t *testing.T) {
	ts := helpers.NewTestServer(t, nil)
	result, resp := callAsyncTool(t, ts, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("bash_async call failed: %d", resp.StatusCode)
	}
	raw, _ := json.Marshal(result)
	if !strings.Contains(string(raw), "command is empty") {
		t.Errorf("expected empty error, got: %s", string(raw))
	}
}

func TestProcessStatusNotFound(t *testing.T) {
	ts := helpers.NewTestServer(t, nil)
	initMCP(t, ts)
	result, resp := callProcessTool(t, ts, "process_status", "nonexistent")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("call failed: %d", resp.StatusCode)
	}
	raw, _ := json.Marshal(result)
	if !strings.Contains(string(raw), "not found") {
		t.Errorf("expected not found, got: %s", string(raw))
	}
}

func TestProcessOutputWhileRunning(t *testing.T) {
	ts := helpers.NewTestServer(t, nil)
	result, resp := callAsyncTool(t, ts, "sleep 10 && echo done")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("bash_async call failed: %d", resp.StatusCode)
	}
	pid := extractProcessID(t, result)

	time.Sleep(200 * time.Millisecond)

	outputResult, _ := callProcessTool(t, ts, "process_output", pid)
	outputRaw, _ := json.Marshal(outputResult)
	if !strings.Contains(string(outputRaw), "still running") {
		t.Errorf("expected still running error, got: %s", string(outputRaw))
	}

	callProcessTool(t, ts, "process_kill", pid)
	time.Sleep(200 * time.Millisecond)
}

func TestProcessKill(t *testing.T) {
	ts := helpers.NewTestServer(t, nil)
	result, resp := callAsyncTool(t, ts, "sleep 60")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("bash_async call failed: %d", resp.StatusCode)
	}
	pid := extractProcessID(t, result)

	time.Sleep(200 * time.Millisecond)

	killResult, _ := callProcessTool(t, ts, "process_kill", pid)
	killRaw, _ := json.Marshal(killResult)
	if !strings.Contains(string(killRaw), "Kill signal sent") {
		t.Errorf("expected kill signal, got: %s", string(killRaw))
	}

	time.Sleep(200 * time.Millisecond)

	statusResult, _ := callProcessTool(t, ts, "process_status", pid)
	statusRaw, _ := json.Marshal(statusResult)
	if !strings.Contains(string(statusRaw), "killed") {
		t.Errorf("expected killed status, got: %s", string(statusRaw))
	}
}

func TestProcessList(t *testing.T) {
	ts := helpers.NewTestServer(t, nil)
	initMCP(t, ts)

	listResp := ts.MCPRequest(t, "tools/call", map[string]any{
		"name":      "process_list",
		"arguments": map[string]any{},
	})
	listBody := helpers.ReadBody(t, listResp)
	if !strings.Contains(listBody, "No async processes") {
		t.Errorf("expected no processes, got: %s", listBody)
	}

	result, resp := callAsyncTool(t, ts, "sleep 10")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("bash_async call failed: %d", resp.StatusCode)
	}
	pid := extractProcessID(t, result)

	time.Sleep(200 * time.Millisecond)

	listResp2 := ts.MCPRequest(t, "tools/call", map[string]any{
		"name":      "process_list",
		"arguments": map[string]any{},
	})
	listBody2 := helpers.ReadBody(t, listResp2)
	if !strings.Contains(listBody2, pid) {
		t.Errorf("expected process %s in list, got: %s", pid, listBody2)
	}

	callProcessTool(t, ts, "process_kill", pid)
	time.Sleep(200 * time.Millisecond)
}

func TestBashAsyncAllowedCommands(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Bash.AllowedCommands = []string{"echo"}
	ts := helpers.NewTestServer(t, cfg)
	result, resp := callAsyncTool(t, ts, "cat /etc/passwd")
	raw, _ := json.Marshal(result)
	if !strings.Contains(string(raw), "not in the allowed commands list") {
		t.Errorf("expected rejection, got: %s", string(raw))
	}
	_ = resp
}
