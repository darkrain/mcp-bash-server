# Release v1.0.4-alpha.2

## Async Process Execution

### New Tools
- **bash_async**: Execute a command asynchronously. Returns immediately with a `process_id` — no timeout, no blocking. Use this for long-running commands (apt update, build, download, etc.) instead of `bash` which times out.
- **process_status**: Check if an async process is running, completed, failed, or killed. Returns elapsed time and exit code when done.
- **process_output**: Retrieve stdout/stderr of a finished process. Returns an error if the process is still running.
- **process_kill**: Terminate a running async process by its ID.
- **process_list**: List all async processes and their statuses.

### Process Registry
- In-memory process registry with TTL cleanup (default: 60 minutes). Completed processes are automatically removed after TTL expires.
- Graceful shutdown: all running processes are killed on SIGTERM.
- Thread-safe with read/write mutex.

### Configuration
- `process_ttl` setting in `[bash]` section (minutes, default: 60).
- `MCP_PROCESS_TTL` environment variable support.
- `process_ttl` validation (must be non-negative).

### Other Changes
- Version is now passed from `main.go` via ldflag instead of being hardcoded in server.
- `isCommandAllowed` extracted as a shared function — both `bash` and `bash_async` enforce the same allowed_commands policy.
- Full test coverage: 8 unit tests for process registry + 7 integration tests for async tools.

## Artifacts

| File | Description |
|------|-------------|
| `mcp-bash-server_amd64` | amd64 static binary |
| `mcp-bash-server_arm64` | arm64 static binary |
| `mcp-bash-server_1.0.4-alpha.2_amd64.deb` | Debian package for amd64 |
| `mcp-bash-server_1.0.4-alpha.2_arm64.deb` | Debian package for arm64 |

---

# Release v1.0.4-alpha.1

## Security & Reliability Improvements

### Critical Fixes
- **Integration tests fixed**: Added `Mcp-Session-Id` header and dual `Accept` (`application/json, text/event-stream`) for go-sdk v1.6.0 stateless mode. All tests now pass.
- **Double exec.Cmd creation eliminated**: `server.go` now creates a single `exec.CommandContext` and wraps it with `context.WithTimeout` only once, preventing process leaks.
- **Reliable process cleanup on timeout**: Added `Setpgid: true` and `syscall.Kill(-pid, SIGKILL)` when context expires, ensuring no zombie or orphan processes.

### Hardening
- **Dynamic systemd hardening**: `postinst` detects if user `mcp` has `sudo NOPASSWD: ALL`. If yes — hardening directives are commented out (required for `sudo` to work). Otherwise hardening stays enabled by default.
- **Config validation**: enforces valid port (1-65535), non-empty `base_url`, API key length >= 16, non-negative timeout and max_output_size.
- **Log redaction**: commands containing `PASSWORD`, `SECRET`, `TOKEN`, `KEY` now have their values replaced with `***REDACTED***` before logging.

### Other Fixes
- **IPv6 filtering**: link-local and multicast addresses excluded from sysinfo.
- **MaxOutputSize**: now splits total limit proportionally between stdout/stderr instead of fixed 50/50.
- **Makefile race**: `sed` no longer mutates original `packaging/deb/control`; a temp copy is used.
- **go.mod**: Go version corrected to `1.23`.
- **Version unification**: `main.Version` is set via `-X main.Version=` ldflag and passed to MCP server info.

## Artifacts

| File | Description |
|------|-------------|
| `mcp-bash-server_amd64` | amd64 static binary |
| `mcp-bash-server_arm64` | arm64 static binary |
| `mcp-bash-server_1.0.4-alpha.1_amd64.deb` | Debian package for amd64 |
| `mcp-bash-server_1.0.4-alpha.1_arm64.deb` | Debian package for arm64 |
