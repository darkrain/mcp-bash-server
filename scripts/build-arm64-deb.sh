#!/bin/bash
set -e

ARCH=arm64
BINARY=build/mcp-bash-server_arm64
DEB=build/mcp-bash-server_1.0.1_arm64.deb
DEBDIR=build/deb-1.0.1-arm64-final

echo "Building ARM64 deb package..."
echo "Working dir: $(pwd)"

rm -rf $DEBDIR
mkdir -p $DEBDIR
ls -ld $DEBDIR

echo "Creating subdirectories..."
mkdir -p $DEBDIR/DEBIAN
mkdir -p $DEBDIR/usr
mkdir -p $DEBDIR/usr/bin
mkdir -p $DEBDIR/etc
mkdir -p $DEBDIR/etc/mcp-bash-server
mkdir -p $DEBDIR/lib
mkdir -p $DEBDIR/lib/systemd
mkdir -p $DEBDIR/lib/systemd/system
mkdir -p $DEBDIR/usr/share
mkdir -p $DEBDIR/usr/share/doc
mkdir -p $DEBDIR/usr/share/doc/mcp-bash-server

echo "Copying binary..."
cp $BINARY $DEBDIR/usr/bin/mcp-bash-server
chmod 755 $DEBDIR/usr/bin/mcp-bash-server

echo "Copying config..."
cp config.example.toml $DEBDIR/etc/mcp-bash-server/config.toml
chmod 644 $DEBDIR/etc/mcp-bash-server/config.toml

echo "Copying systemd service..."
cp packaging/systemd/mcp-bash-server.service $DEBDIR/lib/systemd/system/
chmod 644 $DEBDIR/lib/systemd/system/mcp-bash-server.service

echo "Copying DEBIAN scripts..."
cp packaging/deb/postinst $DEBDIR/DEBIAN/
cp packaging/deb/prerm $DEBDIR/DEBIAN/
cp packaging/deb/postrm $DEBDIR/DEBIAN/
chmod 755 $DEBDIR/DEBIAN/postinst $DEBDIR/DEBIAN/prerm $DEBDIR/DEBIAN/postrm

echo "Generating control file..."
INSTALLED_SIZE=$(du -sk $DEBDIR | cut -f1)

cat > $DEBDIR/DEBIAN/control <<'CONTROL_EOF'
Package: mcp-bash-server
Version: 1.0.1
Section: net
Priority: optional
Architecture: arm64
Depends: systemd
Maintainer: darkrain
Description: MCP server for executing bash commands via HTTP transport
 MCP Bash Server is a Model Context Protocol (MCP) server that allows
 AI agents to execute bash commands on a remote server through HTTP.
 It supports configurable command allowlists with wildcard support,
 timeouts, and API key authentication.
Installed-Size: SIZE_PLACEHOLDER
Homepage: https://github.com/darkrain/mcp-bash-server
CONTROL_EOF

sed -i "s/SIZE_PLACEHOLDER/$INSTALLED_SIZE/g" $DEBDIR/DEBIAN/control

echo "Building package with dpkg-deb..."
dpkg-deb --root-owner-group --build $DEBDIR $DEB

echo "Success!"
echo "Built: $DEB"
ls -lh $DEB
