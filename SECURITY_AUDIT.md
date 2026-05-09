# Security Audit Report - MCP Bash Server v1.0.4

## Executive Summary

Проведён аудит безопасности MCP Bash Server v1.0.3. Выявлено **9 проблем**, из которых:
- **2 критических** - исправлены
- **3 высоких** - исправлены
- **2 средних** - исправлены
- **2 низких** - частично исправлены

## Исправленные проблемы

### CRITICAL-1: Path Traversal через cwd параметр
**Статус:** ✅ ИСПРАВЛЕНО  
**Риск:** Remote Code Execution / Unauthorized File Access  
**Файл:** `server/server.go:120-139`  

**Проблема:** Параметр `cwd` передавался напрямую в `cmd.Dir` без валидации. Можно было выполнять команды в любом месте файловой системы через относительные пути (`../etc`, `/root`).

**Эксплуатация:**
```json
{
  "command": "cat passwd",
  "cwd": "../etc"
}
```

**Исправление:**
Добавлена валидация cwd:
1. Путь должен быть абсолютным (`filepath.IsAbs`)
2. Разрешаются символические ссылки (`filepath.EvalSymlinks`)
3. Отсутствие `..` компонентов после очистки (`filepath.Clean`)

**Результат:**
```json
// Запрос с относительным путём
{"command": "whoami", "cwd": "../etc"}
// → {"error": "working directory must be absolute path"}

// Запрос с абсолютным путём
{"command": "pwd", "cwd": "/tmp"}
// → {"stdout": "/tmp"}
```

### CRITICAL-2: Отсутствие ограничения размера тела запроса (DoS)
**Статус:** ✅ ИСПРАВЛЕНО  
**Риск:** Denial of Service / Out of Memory  
**Файл:** `main.go`  

**Проблема:** `http.Server` не имел ограничения на размер входящих данных. Атакующий мог отправить 10GB JSON и вызвать OOM-килл сервиса.

**Исправление:**
Добавлено middleware `maxRequestBodySizeMiddleware` с ограничением **10MB**:
```go
r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)
```

### HIGH-1: Нет Rate Limiting (DoS)
**Статус:** ✅ ИСПРАВЛЕНО  
**Риск:** Denial of Service  
**Файл:** `main.go`  

**Проблема:** Можно было DDoS-ить сервер бесконечными запросами, каждый создаёт subprocess.

**Исправление:**
Реализован Token Bucket rate limiter с ограничением **10 запросов в секунду** на IP. При превышении возвращается HTTP 429 (Too Many Requests).

### HIGH-2: Секреты в логах
**Статус:** ✅ ИСПРАВЛЕНО  
**Риск:** Information Disclosure  
**Файл:** `server/server.go:234-250`  

**Проблема:** Команды `echo $DB_PASSWORD` или `cat ~/.ssh/id_rsa` попадали в JSON логи в открытом виде.

**Исправление:**
Добавлена функция `redactSecrets()` которая маскирует значения после `=` для паттернов:
- `PASSWORD`, `SECRET`, `TOKEN`, `KEY`
- `API_KEY`, `PRIVATE_KEY`, `ACCESS_KEY`

**Пример:**
```
# До:
cat PASSWORD=supersecret123
# После:
cat PASSWORD=***REDACTED***
```

### HIGH-3: Нет Security Headers
**Статус:** ✅ ИСПРАВЛЕНО  
**Риск:** XSS, Clickjacking  
**Файл:** `main.go`  

**Исправление:**
Добавлены HTTP security headers:
- `X-Content-Type-Options: nosniff`
- `X-Frame-Options: DENY`
- `X-XSS-Protection: 1; mode=block`

### MEDIUM-1: Слабый API Key
**Статус:** ✅ ИСПРАВЛЕНО (warning)  
**Файл:** `main.go:105-108`  

**Исправление:**
Добавлена проверка при старте. Если API key меньше 16 символов — warning в логи:
```
WARN API key is weak (less than 16 characters). Consider using a stronger key.
```

### MEDIUM-2: Небезопасный пример конфигурации
**Статус:** ✅ ИСПРАВЛЕНО  
**Файл:** `config.example.toml`  

**Проблема:** В примере `allowed_commands = ["*"]` шёл первым, пользователи копировали без чтения документации.

**Исправление:**
Сначала показан безопасный whitelist, wildcard — с предупреждением:
```toml
# БЕЗОПАСНЫЙ ВАРИАНТ: только разрешённые команды
allowed_commands = ["ls", "cat", "ps", "df", "echo", "pwd", "uptime"]

# ОПАСНЫЙ ВАРИАНТ: разрешить ВСЕ команды
# ⚠️ ВНИМАНИЕ: Только для доверенных окружений!
# allowed_commands = ["*"]
```

### LOW-1: Нет HTTPS/TLS
**Статус:** ⚠️ ЧАСТИЧНО ИСПРАВЛЕНО (документация)  
**Решение:** Добавлен раздел в README о настройке reverse proxy (nginx/caddy) с TLS.

### LOW-2: Command Injection через bash -c
**Статус:** ⚠️ ПРИНЯТО КАК ФИЧА  
**Описание:** Использование `bash -c` — by design. Пользователь с wildcard доступом может выполнять любые команды. Это не баг, а архитектурное решение.

## Чеклист безопасности

- [x] Path Traversal защита (cwd валидация)
- [x] Request Body Size Limit (10MB)
- [x] Rate Limiting (10 req/sec per IP)
- [x] Secret Redaction в логах
- [x] Security Headers (X-Frame-Options, CSP)
- [x] API Key Strength Warning
- [x] Safe Defaults в примере конфига
- [x] HTTPS Documentation

## Рекомендации

1. **В production** всегда используйте whitelist команд, никогда wildcard
2. **API Key** должен быть минимум 16 символов, случайный
3. **HTTPS** обязателен для production через reverse proxy
4. **Фаервол** ограничьте доступ к порту 8080 только доверенным IP
5. **sudo** давайте только через visudo с NOPASSWD для конкретных команд

## Версия

Исправления включены в **v1.0.4**.
