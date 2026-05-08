# MCP Bash Server

MCP сервер для выполнения bash команд на сервере через HTTP транспорт. Это "SSH для агентов" — позволяет LLM-агентам безопасно выполнять команды на удалённых серверах.

## Архитектура

- **Транспорт:** Streamable HTTP (https://modelcontextprotocol.io/specification/2024-11-05/basic/transports)
- **SDK:** [modelcontextprotocol/go-sdk](https://github.com/modelcontextprotocol/go-sdk)
- **Принцип работы:** Stateless HTTP handler принимает JSON-RPC запросы MCP, выполняет bash команды и возвращает результат

## Команды

| Команда | Описание |
|---------|----------|
| `make build` | Собрать бинарник |
| `make deb` | Собрать .deb пакет |
| `make install` | Установить локально (требует sudo) |
| `make uninstall` | Удалить локальную установку |
| `make test` | Запустить тесты |
| `make run` | Собрать и запустить |
| `make clean` | Очистить артефакты |

## Установка через .deb

```bash
# Собрать пакет
make deb

# Установить на сервере
sudo dpkg -i build/mcp-bash-server_1.0.0_amd64.deb
sudo systemctl enable --now mcp-bash-server

# Проверить статус
systemctl status mcp-bash-server
journalctl -u mcp-bash-server -f
```

## Конфигурация

### Файл: `/etc/mcp-bash-server/config.toml`

```toml
[server]
host = "0.0.0.0"
port = 8080
base_url = "/mcp"
api_key = "your-secret-api-key"  # Необязательно

[bash]
allowed_commands = ["ls", "cat", "ps", "df", "git"]
timeout = 30
max_output_size = 1048576

[log]
level = "info"
format = "json"
```

### Переменные окружения

| Переменная | Описание |
|------------|----------|
| `MCP_CONFIG_PATH` | Путь к конфигу |
| `MCP_HOST` | Хост |
| `MCP_PORT` | Порт |
| `MCP_API_KEY` | API ключ |
| `MCP_BASH_TIMEOUT` | Таймаут команд (сек) |
| `MCP_LOG_LEVEL` | Уровень логирования |

## API

### Health Check
```bash
curl http://localhost:8080/health
```

### MCP Endpoint
```bash
curl -X POST http://localhost:8080/mcp/v1/initialize \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer your-api-key" \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}'
```

### Выполнение команды (через MCP tool)
Инструмент `bash` принимает:
- `command` — команда для выполнения
- `args` — аргументы (опционально)
- `timeout` — таймаут в секундах (опционально)
- `cwd` — рабочая директория (опционально)

## Безопасность

1. **API Key** — обязательная аутентификация через заголовок `Authorization` или `X-API-Key`
2. **Allowed Commands** — белый список разрешённых команд
3. **Timeout** — лимит времени выполнения
4. **Max Output** — ограничение размера вывода
5. **systemd hardening** — seccomp, namespace isolation

## Структура проекта

```
.
├── main.go                      # Точка входа, HTTP сервер
├── config/
│   └── config.go               # Загрузка конфигурации
├── server/
│   └── server.go               # MCP сервер и tool handler
├── packaging/
│   ├── systemd/                # systemd unit
│   └── deb/                    # DEBIAN скрипты
├── config.example.toml         # Пример конфигурации
├── Makefile                    # Сборка
└── go.mod                      # Go модули
```
