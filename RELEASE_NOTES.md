# Release v1.0.3

## What's New

- **Fixed sudo in systemd service** — убран `NoNewPrivileges=true` и другие hardening опции из systemd unit, которые блокировали sudo. Теперь команды с `sudo` работают корректно.

## Features (from v1.0.2)

- **Command Logging (Bash History)** — все выполненные команды логируются с exit_code и duration
- **Wildcard Support** — `allowed_commands = ["*"]` или `["all"]` для разрешения любых команд
- **Streamable HTTP Transport** — официальный MCP протокол через HTTP
- **SSH-like Server Identification** — hostname, IP, user, OS в описании инструментов
- **Bash Command Execution** — с таймаутом, ограничением вывода, UTF-8 валидацией
- **API Key Authentication** — `Authorization: Bearer ...` или `X-API-Key`
- **Static Linking** — работает на любой Linux без зависимостей от libc
- **Multi-Architecture** — amd64 и arm64
- **Debian Packages** — готовые `.deb` для установки

## Configuration

```toml
[server]
host = "0.0.0.0"
port = 8080
base_url = "/mcp"
api_key = "your-secret-api-key"

[bash]
allowed_commands = ["*"]
log_commands = true
timeout = 30
max_output_size = 1048576

[log]
level = "info"
format = "json"
```

## Artifacts

| File | Size | Description |
|------|------|-------------|
| `mcp-bash-server_amd64` | ~7.8MB | amd64 static binary |
| `mcp-bash-server_arm64` | ~7.3MB | arm64 static binary |
| `mcp-bash-server_1.0.3_amd64.deb` | ~2.5MB | Debian package for amd64 |
| `mcp-bash-server_1.0.3_arm64.deb` | ~2.1MB | Debian package for arm64 |

## Links

- Repository: https://github.com/darkrain/mcp-bash-server
- MCP Spec: https://modelcontextprotocol.io
