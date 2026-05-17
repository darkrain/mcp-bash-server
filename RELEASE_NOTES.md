# Release v1.0.4-alpha.7

## Security Hardening

Comprehensive security audit and fixes based on vulnerability assessment.

### Critical: allowed_commands Bypass Fixed (VULN-04/05)

When `allowed_commands` was set (e.g., `["ls", "cat"]`), commands were still executed through `/bin/bash -c`, making the allowlist trivially bypassable with `ls; cat /etc/shadow` or `ls || malicious_command`.

**Fix:** When an allowlist is active (not wildcard), commands are executed **directly** without a shell:
- The binary is resolved to its full path via `exec.LookPath`
- Complex shell syntax (semicolons, pipes, `&&`, `||`) is rejected — use `command` for the binary and `args` for arguments
- Shell `-c` flag is blocked when a shell binary is in the allowlist
- When `allowed_commands = ["*"]` or `["all"]`, behavior is unchanged (bash -c)

### High: Running Processes Killed on Shutdown (VULN-16)

Previously, `Stop()` only closed the database. All running async/timed-out processes continued as orphans after server shutdown, accumulating over restarts.

**Fix:** `Stop()` now kills all running processes (both PGID and PID), marks them as `killed` in the registry, then closes the database.

Also fixed: `isPIDAlive()` now checks `/proc/[pid]/stat` for zombie state instead of using `Signal(0)`, which incorrectly reported killed processes as alive (zombies still accept signal 0).

### High: Random API Key on First Install (VULN-02/03)

Default installation had no authentication (`api_key = ""`) and bound to `0.0.0.0`, exposing an unauthenticated RCE endpoint to the network.

**Fix:**
- First install generates a random 48-hex-char API key and writes it to `config.toml`
- Default bind changed to `127.0.0.1` (localhost only)
- Key is displayed once during installation
- For remote access, explicitly set `host = "0.0.0.0"` in config

### Medium: File Permissions Hardened (VULN-10/11)

- Process output files: `0644` → `0600` (owner-only read/write)
- Process data directories: `0755` → `0700` (owner-only access)

### Medium: Command Redaction in process_list (VULN-26)

`process_list` now applies `redactCommand()` to command strings, preventing exposure of passwords/tokens in shared environments.

### Medium: Error Messages Sanitized (VULN-21)

Error messages no longer expose the server's PID and port number to clients.

### Low: Timeout Validation (VULN-25)

`timeout=0` and `sync_timeout=0` are now treated as "use defaults" (30s and 5s respectively) instead of "no timeout".

## Sudo Configuration Redesign

The postinst script no longer auto-detects sudo and modifies the packaged systemd unit file with `sed`. Instead:

- **Interactive prompt:** `Grant sudo (root) access to the mcp user? [y/N]`
- **Non-interactive:** No sudo by default, with a hint to run `mcp-bash-server-configure sudo`
- **Systemd override:** Sudo-related sandbox disabling uses `/etc/systemd/system/mcp-bash-server.service.d/sudo-override.conf` instead of modifying the packaged unit file
- **New utility:** `mcp-bash-server-configure {sudo|no-sudo|status}` for reconfiguration after install

## Artifacts

| File | Description |
|------|-------------|
| `mcp-bash-server_amd64` | amd64 static binary |
| `mcp-bash-server_arm64` | arm64 static binary |
| `mcp-bash-server_1.0.4-alpha.7_amd64.deb` | Debian package for amd64 |
| `mcp-bash-server_1.0.4-alpha.7_arm64.deb` | Debian package for arm64 |

---

# Release v1.0.4-alpha.6

## Fix: MCP error -32001 (Request timed out) — sync_timeout 5s

In v1.0.4-alpha.4, timed-out synchronous commands were transferred to async execution, but the timeout value (`bash.timeout`, default 30s) could still exceed the MCP client's own request timeout (~60s). When the client disconnected first, the server had no way to send a response, and the agent received `MCP error -32001: Request timed out` — often causing the agent to terminate.

### Root Cause

The server-side timeout-to-async logic used `bash.timeout` (30s default). MCP clients (Cursor, Claude Desktop) have their own request timeouts. If a command ran longer than the client timeout but shorter than `bash.timeout`, the client would disconnect before the server could respond — resulting in the -32001 error with no recovery path.

### Fix: sync_timeout

New configuration option `sync_timeout` (default **5 seconds**) limits how long a synchronous `bash` command waits before being transferred to background execution. Since sync commands are typically lightweight (`ls`, `cat`, `grep`, etc.), 5 seconds is more than sufficient. Longer-running commands should use `bash_async` explicitly.

The effective timeout is calculated as:
```
min(sync_timeout, bash_timeout, ctx_deadline - 3s margin)
```
This guarantees the server **always** returns a response before the client disconnects.

Additionally, `ctx.Done()` is now monitored — if the client disconnects for any reason, the command is immediately transferred to async execution and a proper response is returned.

### New Configuration

| Setting | Default | Description |
|---------|---------|-------------|
| `sync_timeout` | `5` | Max seconds for synchronous bash execution before transferring to async |
| `MCP_BASH_SYNC_TIMEOUT` | — | Environment variable override |

### Refactoring

- `transferToAsync()` — extracted reusable function for moving a running process to the async registry
- `effectiveSyncTimeout()` — computes the safe deadline from multiple constraints
- Eliminated the old `timeoutCh` goroutine-based timer pattern in favor of `time.NewTimer` with proper cleanup

## Apt Repository (GitHub Pages)

New apt repository hosted on GitHub Pages for automatic updates via `apt`:

```bash
# Add GPG key
curl -fsSL https://darkrain.github.io/mcp-bash-server/repo.gpg.key | sudo gpg --dearmor -o /etc/apt/trusted.gpg.d/mcp-bash-server.gpg

# Add repository
echo "deb https://darkrain.github.io/mcp-bash-server stable main" | sudo tee /etc/apt/sources.list.d/mcp-bash-server.list

# Install / Update
sudo apt update && sudo apt install mcp-bash-server
```

New Makefile targets for repository maintenance:
- `make apt-repo-init` — initialize apt repo with gh-pages worktree
- `make apt-repo-add` — build debs and add to repo
- `make apt-repo-push` — push repo to GitHub Pages

## Artifacts

| File | Description |
|------|-------------|
| `mcp-bash-server_amd64` | amd64 static binary |
| `mcp-bash-server_arm64` | arm64 static binary |
| `mcp-bash-server_1.0.4-alpha.6_amd64.deb` | Debian package for amd64 |
| `mcp-bash-server_1.0.4-alpha.6_arm64.deb` | Debian package for arm64 |

---

# Release v1.0.4-alpha.5

## Fix: systemd ReadWritePaths for bbolt

The systemd service unit had `ProtectSystem=strict` with only `/tmp` in `ReadWritePaths`. This made `/var/lib/mcp-bash-server/processes.db` read-only, causing bbolt to fail on startup.

Fix: added `/var/lib/mcp-bash-server` to `ReadWritePaths` in the service unit and updated the postinst sed pattern to match.

## Artifacts

| File | Description |
|------|-------------|
| `mcp-bash-server_amd64` | amd64 static binary |
| `mcp-bash-server_arm64` | arm64 static binary |
| `mcp-bash-server_1.0.4-alpha.5_amd64.deb` | Debian package for amd64 |
| `mcp-bash-server_1.0.4-alpha.5_arm64.deb` | Debian package for arm64 |

---

# Release v1.0.4-alpha.4

## Timeout-to-Async

When a synchronous `bash` command hits its timeout, the process is **no longer killed**. Instead, it is automatically transferred to the async process registry and continues running in the background. The agent receives a `process_id` and can check progress with `process_status`, retrieve results with `process_output`, or terminate with `process_kill`.

Key behaviors:
- The process is registered in the bbolt-backed `ProcessRegistry` with its real PID
- Accumulated stdout/stderr up to the timeout point is flushed to the process output file
- A goroutine waits for the process to finish and updates status/exit code in the registry
- The response is returned **without** `IsError` — so the agent treats it as an in-progress result, not a failure
- The message explicitly states the command has NOT failed and is still executing in the background

This solves the common scenario where an agent launches a heavy command synchronously, it times out, and the agent interprets the SIGKILL as a failure and retries — instead of just waiting for the result.

## Process Reaping on Restart

Previously, alive processes left in `running` status after a server restart would stay stuck forever — the `recover()` function only handled dead PIDs.

Now `recover()` also **reaps** alive processes:
- If a process PID is still alive, a goroutine is spawned that calls `os.FindProcess(pid).Wait()`
- When the process finishes, status/exit code/duration are updated in the registry
- No process stays stuck in `running` indefinitely after a restart

## Artifacts

| File | Description |
|------|-------------|
| `mcp-bash-server_amd64` | amd64 static binary |
| `mcp-bash-server_arm64` | arm64 static binary |
| `mcp-bash-server_1.0.4-alpha.4_amd64.deb` | Debian package for amd64 |
| `mcp-bash-server_1.0.4-alpha.4_arm64.deb` | Debian package for arm64 |

---

# Release v1.0.4-alpha.3

## Self-Protection

The server now blocks commands that would kill itself. If an agent tries to run a command targeting the MCP server's own port, PID, or service name, execution is denied with a clear error message. This prevents the common scenario where an agent kills the server while trying to free a port.

Blocked patterns:
- `kill $(lsof -t -i:PORT)` or any kill command referencing the server's port
- `fuser -k PORT/tcp`
- `kill PID` where PID matches the server process
- `killall mcp-bash-server`
- `systemctl stop/kill/restart mcp-bash-server`
- `pkill` commands referencing the server's port

Safe commands like `curl http://localhost:PORT/health` are allowed — only kill-intent commands are blocked.

## Process Persistence

Async processes now survive service restarts and upgrades. Architecture redesigned from in-memory to persistent storage:

### bbolt Database
- Process metadata (ID, command, PID, status, exit code, timestamps) stored in bbolt embedded database
- DB file: `{process_dir}/processes.db`
- State persists across restarts — no data loss on upgrade

### File-based Output
- Process stdout/stderr written directly to `{process_dir}/output/{process_id}.log`
- Streamed to disk via `cmd.Stdout = *os.File` — no memory buffering
- Output available after restart, even for long-running processes

### Process Survival
- Processes launched with `Setpgid: true` — they get their own process group
- When the MCP server stops, processes keep running independently
- On restart, `process_status` checks `/proc/{pid}` to detect if process is still alive
- Stale running processes are automatically marked as `failed`

### Recovery on Startup
- Server scans bbolt DB on startup
- Running processes with dead PIDs are marked as `failed`
- All other state is restored from DB

## DEB Package Improvements

### Config Preservation
- `config.toml` is **never** overwritten during package upgrade
- `config.example.toml` is always updated to latest version
- On upgrade, a diff between user config and example is shown, highlighting new/changed options
- User can review changes: `diff /etc/mcp-bash-server/config.toml /etc/mcp-bash-server/config.example.toml`

### Process Data Directory
- `/var/lib/mcp-bash-server/output/` created automatically with correct ownership
- DEB postinst sets `mcp:mcp` ownership recursively

## New Configuration

| Setting | Default | Description |
|---------|---------|-------------|
| `process_dir` | `/var/lib/mcp-bash-server` | Directory for bbolt DB and output files |
| `MCP_PROCESS_DIR` | — | Environment variable override |

## Artifacts

| File | Description |
|------|-------------|
| `mcp-bash-server_amd64` | amd64 static binary |
| `mcp-bash-server_arm64` | arm64 static binary |
| `mcp-bash-server_1.0.4-alpha.3_amd64.deb` | Debian package for amd64 |
| `mcp-bash-server_1.0.4-alpha.3_arm64.deb` | Debian package for arm64 |

---

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
