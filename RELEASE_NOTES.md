# Release v1.0.4

## Security Hardening

### Critical Fixes
- **Path Traversal Prevention**: `cwd` parameter now requires absolute paths and resolves symlinks
- **DoS Prevention**: Added 10MB request body size limit and request read timeouts

### Security Features Added
- **Rate Limiting**: 10 requests per second per IP (token bucket algorithm)
- **Secret Redaction**: Commands with PASSWORD, SECRET, TOKEN, KEY are redacted in logs
- **Security Headers**: X-Content-Type-Options, X-Frame-Options, X-XSS-Protection
- **API Key Validation**: Warning if API key is shorter than 16 characters
- **Safe Defaults**: Example config now starts with whitelist, not wildcard

### How to Update

```bash
# Download new version
wget https://github.com/darkrain/mcp-bash-server/releases/download/v1.0.4/mcp-bash-server_1.0.4_amd64.deb

# Install (auto-restarts service)
sudo dpkg -i mcp-bash-server_1.0.4_amd64.deb
```

### Security Checklist

- [x] Path traversal blocked
- [x] Request size limited (10MB)
- [x] Rate limiting active (10 req/sec)
- [x] Secrets redacted in logs
- [x] Security headers present
- [x] Safe config defaults
- [x] Security audit document (SECURITY_AUDIT.md)

### Breaking Changes

None. All changes are backward compatible.

## Artifacts

| File | Size | Description |
|------|------|-------------|
| mcp-bash-server_amd64 | ~7.9MB | amd64 static binary |
| mcp-bash-server_arm64 | ~7.3MB | arm64 static binary |
| mcp-bash-server_1.0.4_amd64.deb | ~2.5MB | Debian package for amd64 |
| mcp-bash-server_1.0.4_arm64.deb | ~2.1MB | Debian package for arm64 |
