## mcp-bash-server v1.0.5

Stable release of the async process registry and security hardening series.

The version is `1.0.5` so APT correctly upgrades legacy
`1.0.4-alpha.*` installations.

### Fixed

- Fixed `failed to open bbolt: read-only file system` in sudo-enabled installations.
- Sudo mode now disables the base systemd filesystem sandbox completely, matching the documented full-root behavior and allowing custom `process_dir` paths such as `/opt/mcp-bash-server`.
- Package upgrades preserve existing sudo access and automatically replace broken overrides from alpha releases.
- The systemd sudo override is shipped once and shared by both the Debian `postinst` and `mcp-bash-server-configure`, preventing future drift.
- A stale sandbox override is removed when sudo is not enabled.

### Install on Debian/Ubuntu

```bash
sudo dpkg -i mcp-bash-server_1.0.5_amd64.deb
sudo mcp-bash-server-configure sudo  # only when full root access is required
sudo systemctl enable --now mcp-bash-server
```

ARM64 systems should use `mcp-bash-server_1.0.5_arm64.deb`.

Existing `/etc/mcp-bash-server/config.toml` settings are preserved during upgrades.

### Verify

```bash
sha256sum -c SHA256SUMS
sudo systemctl show mcp-bash-server \
  -p ProtectSystem -p NoNewPrivileges -p ReadWritePaths
```

With sudo enabled, the effective values should include:

```text
ProtectSystem=no
NoNewPrivileges=no
ReadWritePaths=
```

### Artifacts

- `mcp-bash-server_amd64`
- `mcp-bash-server_arm64`
- `mcp-bash-server_1.0.5_amd64.deb`
- `mcp-bash-server_1.0.5_arm64.deb`
- `SHA256SUMS`
