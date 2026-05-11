package server

import (
	"os"
	"testing"

	"mcp-bash-server/config"
)

func TestWouldKillServer_KillPortDirect(t *testing.T) {
	cfg := &config.Config{Server: config.ServerConfig{Port: 8080}}
	cmd := "sudo kill $(sudo lsof -t -i:8080) 2>/dev/null"
	if !wouldKillServer(cmd, cfg) {
		t.Error("expected kill via lsof port to be blocked")
	}
}

func TestWouldKillServer_KillPortFuser(t *testing.T) {
	cfg := &config.Config{Server: config.ServerConfig{Port: 8080}}
	cmd := "fuser -k 8080/tcp"
	if !wouldKillServer(cmd, cfg) {
		t.Error("expected fuser kill on port to be blocked")
	}
}

func TestWouldKillServer_KillPID(t *testing.T) {
	cfg := &config.Config{Server: config.ServerConfig{Port: 8080}}
	cmd := "kill -9 " + func() string { return "" }()
	pid := os.Getpid()
	cmd = "kill -9 " + intToStr(pid)
	if !wouldKillServer(cmd, cfg) {
		t.Error("expected kill by own PID to be blocked")
	}
}

func TestWouldKillServer_KillServiceName(t *testing.T) {
	cfg := &config.Config{Server: config.ServerConfig{Port: 8080}}
	cmd := "systemctl stop mcp-bash-server"
	if !wouldKillServer(cmd, cfg) {
		t.Error("expected systemctl stop by name to be blocked")
	}
}

func TestWouldKillServer_Killall(t *testing.T) {
	cfg := &config.Config{Server: config.ServerConfig{Port: 8080}}
	cmd := "killall mcp-bash-server"
	if !wouldKillServer(cmd, cfg) {
		t.Error("expected killall by name to be blocked")
	}
}

func TestWouldKillServer_SafeKillOtherPort(t *testing.T) {
	cfg := &config.Config{Server: config.ServerConfig{Port: 8080}}
	cmd := "sudo kill $(sudo lsof -t -i:3000) 2>/dev/null"
	if wouldKillServer(cmd, cfg) {
		t.Error("expected kill on different port to be allowed")
	}
}

func TestWouldKillServer_SafeCommand(t *testing.T) {
	cfg := &config.Config{Server: config.ServerConfig{Port: 8080}}
	cmd := "apt update && apt install -y nginx"
	if wouldKillServer(cmd, cfg) {
		t.Error("expected safe command to be allowed")
	}
}

func TestWouldKillServer_SafeEcho(t *testing.T) {
	cfg := &config.Config{Server: config.ServerConfig{Port: 8080}}
	cmd := "echo 'hello world'"
	if wouldKillServer(cmd, cfg) {
		t.Error("expected echo command to be allowed")
	}
}

func TestWouldKillServer_SafeCurlOwnPort(t *testing.T) {
	cfg := &config.Config{Server: config.ServerConfig{Port: 8080}}
	cmd := "curl -s http://localhost:8080/health"
	if wouldKillServer(cmd, cfg) {
		t.Error("expected curl to own port to be allowed")
	}
}

func TestWouldKillServer_SystemctlRestart(t *testing.T) {
	cfg := &config.Config{Server: config.ServerConfig{Port: 8080}}
	cmd := "sudo systemctl restart mcp-bash-server"
	if !wouldKillServer(cmd, cfg) {
		t.Error("expected systemctl restart by name to be blocked")
	}
}

func TestWouldKillServer_PkillPort(t *testing.T) {
	cfg := &config.Config{Server: config.ServerConfig{Port: 8080}}
	cmd := "pkill -f 'lsof -t -i:8080'"
	if !wouldKillServer(cmd, cfg) {
		t.Error("expected pkill targeting own port to be blocked")
	}
}

func TestWouldKillServer_SystemctlKill(t *testing.T) {
	cfg := &config.Config{Server: config.ServerConfig{Port: 8080}}
	cmd := "sudo systemctl kill mcp-bash-server"
	if !wouldKillServer(cmd, cfg) {
		t.Error("expected systemctl kill by name to be blocked")
	}
}

func intToStr(n int) string {
	if n < 0 {
		return "-" + intToStr(-n)
	}
	if n < 10 {
		return string(rune('0' + n))
	}
	return intToStr(n/10) + string(rune('0'+n%10))
}
