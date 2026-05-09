# Release v1.0.2

## What's New

- **Command Logging (Bash History)** — все выполненные команды теперь логируются с exit_code и duration. Аналог `bash history` для аудита.
  ```json
  {"time":"...","level":"INFO","msg":"command started","command":"ls -la","cwd":"/home","timeout":30}
  {"time":"...","level":"INFO","msg":"command completed","command":"ls -la","exit_code":0,"duration_ms":42}
  ```
- Отключить: `log_commands = false` в конфиге

## Features (from v1.0.1)

- **Wildcard Support** — `allowed_commands = ["*"]` или `["all"]` для разрешения любых команд
- **Streamable HTTP Transport** — официальный MCP протокол через HTTP
- **SSH-like Server Identification** — hostname, IP, user, OS в описании инструментов
- **Bash Command Execution** — с таймаутом, ограничением вывода, UTF-8 валидацией
- **API Key Authentication** — `Authorization: Bearer ...` или `X-API-Key`
- **Static Linking** — работает на любой Linux без зависимостей от libc
- **Multi-Architecture** — amd64 и arm64
- **Debian Packages** — готовые `.deb` для установки
- **systemd Service** — с security hardening

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
| `mcp-bash-server_1.0.2_amd64.deb` | ~2.5MB | Debian package for amd64 |
| `mcp-bash-server_1.0.2_arm64.deb` | ~2.1MB | Debian package for arm64 |

## Links

- Repository: https://github.com/darkrain/mcp-bash-server
- MCP Spec: https://modelcontextprotocol.io
