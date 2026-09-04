#!/bin/bash
set -euo pipefail

repo_root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)"
override="$repo_root/packaging/systemd/sudo-override.conf"

required_false=(
    NoNewPrivileges
    ProtectSystem
    ProtectHome
    PrivateTmp
    ProtectKernelTunables
    ProtectKernelModules
    ProtectControlGroups
    RestrictSUIDSGID
    RestrictRealtime
    RestrictNamespaces
    LockPersonality
    MemoryDenyWriteExecute
)

for setting in "${required_false[@]}"; do
    grep -qx "${setting}=false" "$override"
done

grep -qx 'ReadWritePaths=' "$override"
grep -qx 'SystemCallFilter=' "$override"
grep -qx 'SystemCallErrorNumber=' "$override"

if grep -Eq '^(ProtectSystem=(full|strict)|ReadWritePaths=/.+)' "$override"; then
    echo "sudo override still contains a read-only filesystem restriction" >&2
    exit 1
fi

grep -q 'OVERRIDE_SOURCE=/usr/share/mcp-bash-server/sudo-override.conf' \
    "$repo_root/packaging/deb/configure-helper"
grep -q 'OVERRIDE_SOURCE=/usr/share/mcp-bash-server/sudo-override.conf' \
    "$repo_root/packaging/deb/postinst"
grep -q 'Existing sudo access found' "$repo_root/packaging/deb/postinst"

echo "systemd sudo override packaging: OK"
