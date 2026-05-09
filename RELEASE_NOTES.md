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
