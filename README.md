# MCP Bash Server

MCP сервер для выполнения bash команд на сервере через HTTP транспорт. Это "SSH для AI агентов" — позволяет LLM-агентам безопасно выполнять команды на удалённых серверах.

## Архитектура

- **Транспорт:** Streamable HTTP (https://modelcontextprotocol.io/specification/2024-11-05/basic/transports)
- **SDK:** [modelcontextprotocol/go-sdk](https://github.com/modelcontextprotocol/go-sdk)
- **Принцип работы:** Stateless HTTP handler принимает JSON-RPC запросы MCP, выполняет bash команды и возвращает результат
- **Сборка:** Статическая линковка (CGO_ENABLED=0) — работает на любой Linux системе без зависимостей от libc

## Команды

| Команда | Описание |
|---------|----------|
| `make build` | Собрать бинарник для текущей архитектуры |
| `make build-all` | Собрать для amd64 + arm64 |
| `make deb-all` | Собрать .deb пакеты для обеих архитектур |
| `make release` | Собрать все артефакты релиза |
| `make test` | Запустить тесты |
| `make install` | Установить локально (требует sudo) |
| `make uninstall` | Удалить локальную установку |
| `make run` | Собрать и запустить |
| `make clean` | Очистить артефакты |

## Установка через .deb

```bash
# Скачать и установить
wget https://github.com/darkrain/mcp-bash-server/releases/download/v1.0.3/mcp-bash-server_1.0.3_amd64.deb
sudo dpkg -i mcp-bash-server_1.0.3_amd64.deb
sudo systemctl enable --now mcp-bash-server

# Проверить статус
systemctl status mcp-bash-server
journalctl -u mcp-bash-server -f
```

### Обновление

```bash
# Скачать новую версию
wget https://github.com/darkrain/mcp-bash-server/releases/download/v1.0.3/mcp-bash-server_1.0.3_amd64.deb

# Установить (перезапускает сервис автоматически)
sudo dpkg -i mcp-bash-server_1.0.3_amd64.deb

# Или если нужно перезапустить вручную
sudo systemctl restart mcp-bash-server
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
# Разрешить ВСЕ команды (wildcard):
allowed_commands = ["*"]
# Или разрешить конкретные команды:
# allowed_commands = ["ls", "cat", "ps", "df", "git"]
timeout = 30
max_output_size = 1048576

# Логирование всех выполненных команд (аналог bash history)
# По умолчанию: true
log_commands = true

[log]
level = "info"
format = "json"
```

### Wildcard для allowed_commands

Версия 1.0.1+ поддерживает wildcard:
- `["*"]` или `["all"]` — разрешить любые команды (осторожно!)
- `["ls", "cat", "echo"]` — только указанные команды
- `[]` (пустой список) — никакие команды не разрешены

**Рекомендация:** Несмотря на поддержку wildcard, рекомендуется использовать белый список команд для безопасности.

### Логирование команд (аналог bash history)

По умолчанию все выполненные команды логируются:
```json
{"time":"...","level":"INFO","msg":"command started","command":"ls -la","cwd":"/home","timeout":30}
{"time":"...","level":"INFO","msg":"command completed","command":"ls -la","exit_code":0,"duration_ms":42}
```

Отключить логирование:
```toml
[bash]
log_commands = false
```

Логи доступны через journalctl:
```bash
journalctl -u mcp-bash-server -f
```

### Переменные окружения

| Переменная | Описание |
|------------|----------|
| `MCP_CONFIG_PATH` | Путь к конфигу |
| `MCP_HOST` | Хост |
| `MCP_PORT` | Порт |
| `MCP_API_KEY` | API ключ |
| `MCP_BASE_URL` | Базовый URL |
| `MCP_BASH_TIMEOUT` | Таймаут команд (сек) |
| `MCP_LOG_LEVEL` | Уровень логирования (debug/info/warn/error) |

## Идентификация сервера

Каждый MCP инструмент содержит системную информацию (как SSH greeting):
- Hostname
- IP адреса
- Пользователь
- Операционная система
- Архитектура
- Текущая директория

Это позволяет агенту понимать с каким сервером он работает при подключении к нескольким MCP.

## Подключение к n8n

### URL должен заканчиваться на слэш

В настройках MCP Client Tool в n8n **всегда** указывайте URL со слэшем на конце:

```
http://<your-server-ip>:8080/mcp/    ← правильно (со слэшем)
http://<your-server-ip>:8080/mcp     ← неправильно (без слэша)
```

### Права sudo для команды rm, apt и т.д.

По умолчанию сервер запускается от пользователя `mcp` без sudo. Чтобы агент мог выполнять команды с правами root (apt, systemctl, редактирование системных файлов), нужно дать sudo:

```bash
# Дать sudo без пароля пользователю mcp
sudo visudo -f /etc/sudoers.d/mcp
```

Добавьте строку:
```
mcp ALL=(ALL) NOPASSWD: ALL
```

После этого команды в MCP будут работать с `sudo`:
```bash
sudo apt update
sudo rm /etc/some/file
```

**Проверка:**
```bash
# Выполнить команду от имени mcp
sudo -u mcp bash -c "sudo whoami"
# Должно вывести: root
```

**Важно:** Пользователь `mcp` — системный, у него нет shell. Не пытайтесь зайти под ним через `su mcp`. Для дебага используйте `sudo -u mcp bash -c "команда"`.

## API

### Health Check
```bash
curl http://localhost:8080/health
```

### MCP Initialize
```bash
curl -X POST http://localhost:8080/mcp/ \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer your-api-key" \
  -d '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "initialize",
    "params": {
      "protocolVersion": "2024-11-05",
      "capabilities": {},
      "clientInfo": {"name": "test", "version": "1.0"}
    }
  }'
```

### Выполнение команды (через MCP tool)
Инструмент `bash` принимает:
- `command` — команда для выполнения
- `args` — аргументы (опционально)
- `timeout` — таймаут в секундах (опционально)
- `cwd` — рабочая директория (опционально)

```bash
curl -X POST http://localhost:8080/mcp/ \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer your-api-key" \
  -d '{
    "jsonrpc": "2.0",
    "id": 2,
    "method": "tools/call",
    "params": {
      "name": "bash",
      "arguments": {"command": "ls -la"}
    }
  }'
```

## Безопасность

1. **API Key** — аутентификация через заголовок `Authorization: Bearer ...` или `X-API-Key`
2. **Allowed Commands** — белый список разрешённых команд с wildcard поддержкой
3. **Timeout** — лимит времени выполнения (по умолчанию 30 сек)
4. **Max Output** — ограничение размера вывода (по умолчанию 1MB)
5. **systemd hardening** — seccomp, namespace isolation, readonly filesystem

## Архитектуры

| Архитектура | Бинарник | DEB пакет |
|-------------|----------|-----------|
| amd64 | `mcp-bash-server_amd64` | `mcp-bash-server_1.0.1_amd64.deb` |
| arm64 | `mcp-bash-server_arm64` | `mcp-bash-server_1.0.1_arm64.deb` |

Бинарники статически слинкованы (CGO_ENABLED=0) и работают без зависимостей от libc.

## Структура проекта

```
.
├── main.go                      # Точка входа, HTTP сервер
├── config/
│   └── config.go               # Загрузка конфигурации (TOML + env vars)
├── server/
│   └── server.go               # MCP сервер и tool handler
├── sysinfo/
│   └── sysinfo.go              # Системная информация (hostname, IP и т.д.)
├── packaging/
│   ├── systemd/                # systemd unit
│   └── deb/                    # DEBIAN скрипты
├── tests/
│   └── integration/            # Интеграционные тесты
├── config.example.toml         # Пример конфигурации
├── Makefile                    # Сборка
└── go.mod                      # Go модули
```
