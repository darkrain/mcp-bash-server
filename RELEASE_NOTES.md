# Release v1.0.1

## What's New

- **Command Logging (Bash History)** — все выполненные команды теперь логируются с exit_code и duration. Аналог `bash history` для аудита.
- **Wildcard Support for `allowed_commands`** — теперь можно разрешить все команды через `allowed_commands = ["*"]` или `["all"]`

## Features

- **Streamable HTTP Transport** — официальный MCP протокол через HTTP (modelcontextprotocol/go-sdk)
- **SSH-like Server Identification** — каждый инструмент показывает hostname, IP, user, OS чтобы агент понимал с каким сервером работает
- **Bash Command Execution** — выполнение команд с таймаутом, ограничением вывода и валидацией UTF-8
- **Command Allowlist** — белый список разрешённых команд для безопасности с wildcard поддержкой
- **Command Logging** — логирование всех команд (start/completed) для аудита
- **API Key Authentication** — через заголовок `Authorization: Bearer ...` или `X-API-Key`
- **Static Linking** — бинарник не зависит от версии libc, работает на любой Linux системе
- **Multi-Architecture** — сборки для `amd64` и `arm64`
- **Debian Packages** — готовые `.deb` пакеты для установки на сервер
- **systemd Service** — автозапуск с security hardening (seccomp, namespaces)
- **JSON Logging** — configurable log levels и форматы

## Configuration

Config file: `/etc/mcp-bash-server/config.toml`

```toml
[server]
host = "0.0.0.0"
port = 8080
base_url = "/mcp"
api_key = "your-secret-api-key"

[bash]
# Allow all commands:
allowed_commands = ["*"]
# Or specific commands:
# allowed_commands = ["ls", "cat", "ps", "df", "git"]

# Log all executed commands (bash history)
log_commands = true

timeout = 30
max_output_size = 1048576

[log]
level = "info"
format = "json"
```

## Quick Start

```bash
# Download binary
wget https://github.com/darkrain/mcp-bash-server/releases/download/v1.0.1/mcp-bash-server_amd64
chmod +x mcp-bash-server_amd64
MCP_API_KEY=your-secret ./mcp-bash-server_amd64

# Or install via .deb
sudo dpkg -i mcp-bash-server_1.0.1_amd64.deb
sudo systemctl enable --now mcp-bash-server
```

## Artifacts

| File | Size | Description |
|------|------|-------------|
| `mcp-bash-server_amd64` | ~7.8MB | amd64 static binary |
| `mcp-bash-server_arm64` | ~7.3MB | arm64 static binary |
| `mcp-bash-server_1.0.1_amd64.deb` | ~2.5MB | Debian package for amd64 |
| `mcp-bash-server_1.0.1_arm64.deb` | ~2.1MB | Debian package for arm64 |

## Links

- Repository: https://github.com/darkrain/mcp-bash-server
- MCP Spec: https://modelcontextprotocol.io
- Go SDK: https://github.com/modelcontextprotocol/go-sdk
