# MCP Bash Server Makefile
# Multi-architecture build supported: amd64, arm64

BINARY_NAME := mcp-bash-server
PACKAGE_NAME := mcp-bash-server
VERSION := 1.0.0
MAINTAINER := darkrain
DESCRIPTION := MCP server for executing bash commands via HTTP transport

# Build settings
BUILD_DIR := build
GO := go
GOCLEAN := $(GO) clean
GOTEST := $(GO) test
GOFLAGS := -ldflags="-s -w -extldflags=-static -X main.Version=$(VERSION)"

# Architectures
ARCH_AMD64 := amd64
ARCH_ARM64 := arm64
GOARCH_AMD64 := amd64
GOARCH_ARM64 := arm64

# Release directory
RELEASE_DIR := $(BUILD_DIR)/release

# Colors
BLUE := \033[36m
GREEN := \033[32m
YELLOW := \033[33m
RED := \033[31m
NC := \033[0m

.PHONY: all build build-all test clean deb-all release lint run

all: build

build: ## Build binary for current architecture (statically linked)
	@echo "$(BLUE)Building $(BINARY_NAME) v$(VERSION) for current arch...$(NC)"
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 $(GO) build $(GOFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) .
	@echo "$(GREEN)Built: $(BUILD_DIR)/$(BINARY_NAME)$(NC)"

build-amd64: ## Build amd64 binary (cross-compile if needed)
	@echo "$(BLUE)Building $(BINARY_NAME) v$(VERSION) for amd64...$(NC)"
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=$(GOARCH_AMD64) CGO_ENABLED=0 $(GO) build $(GOFLAGS) \
		-o $(BUILD_DIR)/$(BINARY_NAME)_$(ARCH_AMD64) .
	@echo "$(GREEN)Built: $(BUILD_DIR)/$(BINARY_NAME)_$(ARCH_AMD64)$(NC)"

build-arm64: ## Build arm64 binary (cross-compile if needed)
	@echo "$(BLUE)Building $(BINARY_NAME) v$(VERSION) for arm64...$(NC)"
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=$(GOARCH_ARM64) CGO_ENABLED=0 $(GO) build $(GOFLAGS) \
		-o $(BUILD_DIR)/$(BINARY_NAME)_$(ARCH_ARM64) .
	@echo "$(GREEN)Built: $(BUILD_DIR)/$(BINARY_NAME)_$(ARCH_ARM64)$(NC)"

build-all: build-amd64 build-arm64 ## Build for all architectures

test: ## Run all tests including integration
	@echo "$(BLUE)Running tests...$(NC)"
	$(GOTEST) -v -timeout 60s ./...
	@echo "$(GREEN)Tests complete$(NC)"

lint: ## Run linter
	@echo "$(BLUE)Running linter...$(NC)"
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		go vet ./...; \
	fi

clean: ## Clean build artifacts
	@echo "$(YELLOW)Cleaning...$(NC)"
	$(GOCLEAN)
	@rm -rf $(BUILD_DIR)
	@echo "$(GREEN)Clean complete$(NC)"

run: build ## Build and run the server
	@echo "$(GREEN)Starting $(BINARY_NAME)...$(NC)"
	@$(BUILD_DIR)/$(BINARY_NAME)

# Debian package creation for amd64
deb-amd64: build-amd64
	@echo "$(BLUE)Building Debian package for $(ARCH_AMD64)...$(NC)"
	$(call build-deb,$(ARCH_AMD64),$(BUILD_DIR)/$(BINARY_NAME)_$(ARCH_AMD64))

# Debian package creation for arm64
deb-arm64: build-arm64
	@echo "$(BLUE)Building Debian package for $(ARCH_ARM64)...$(NC)"
	$(call build-deb,$(ARCH_ARM64),$(BUILD_DIR)/$(BINARY_NAME)_$(ARCH_ARM64))

# Build all deb packages
deb-all: deb-amd64 deb-arm64

define build-deb
	@rm -rf $(BUILD_DIR)/deb-$1
	@mkdir -p $(BUILD_DIR)/deb-$1/DEBIAN
	@mkdir -p $(BUILD_DIR)/deb-$1/usr/bin
	@mkdir -p $(BUILD_DIR)/deb-$1/etc/$(BINARY_NAME)
	@mkdir -p $(BUILD_DIR)/deb-$1/lib/systemd/system
	@mkdir -p $(BUILD_DIR)/deb-$1/usr/share/doc/$(BINARY_NAME)

	@cp $2 $(BUILD_DIR)/deb-$1/usr/bin/$(BINARY_NAME)
	@chmod 755 $(BUILD_DIR)/deb-$1/usr/bin/$(BINARY_NAME)

	@cp config.example.toml $(BUILD_DIR)/deb-$1/etc/$(BINARY_NAME)/config.toml
	@chmod 644 $(BUILD_DIR)/deb-$1/etc/$(BINARY_NAME)/config.toml

	@cp packaging/systemd/$(BINARY_NAME).service $(BUILD_DIR)/deb-$1/lib/systemd/system/
	@chmod 644 $(BUILD_DIR)/deb-$1/lib/systemd/system/$(BINARY_NAME).service

	@echo "Format: https://www.debian.org/doc/packaging-manuals/copyright-format/1.0/" > $(BUILD_DIR)/deb-$1/usr/share/doc/$(BINARY_NAME)/copyright
	@echo "Upstream-Name: $(PACKAGE_NAME)" >> $(BUILD_DIR)/deb-$1/usr/share/doc/$(BINARY_NAME)/copyright
	@echo "Source: https://github.com/darkrain/mcp-bash-server" >> $(BUILD_DIR)/deb-$1/usr/share/doc/$(BINARY_NAME)/copyright
	@echo "" >> $(BUILD_DIR)/deb-$1/usr/share/doc/$(BINARY_NAME)/copyright
	@echo "Files: *" >> $(BUILD_DIR)/deb-$1/usr/share/doc/$(BINARY_NAME)/copyright
	@echo "Copyright: 2024 $(MAINTAINER)" >> $(BUILD_DIR)/deb-$1/usr/share/doc/$(BINARY_NAME)/copyright
	@echo "License: MIT" >> $(BUILD_DIR)/deb-$1/usr/share/doc/$(BINARY_NAME)/copyright

	@cp packaging/deb/control $(BUILD_DIR)/deb-$1/DEBIAN/
	@cp packaging/deb/postinst $(BUILD_DIR)/deb-$1/DEBIAN/
	@cp packaging/deb/prerm $(BUILD_DIR)/deb-$1/DEBIAN/
	@cp packaging/deb/postrm $(BUILD_DIR)/deb-$1/DEBIAN/

	@chmod 755 $(BUILD_DIR)/deb-$1/DEBIAN/postinst
	@chmod 755 $(BUILD_DIR)/deb-$1/DEBIAN/prerm
	@chmod 755 $(BUILD_DIR)/deb-$1/DEBIAN/postrm

	@INSTALLED_SIZE=$$(du -sk $(BUILD_DIR)/deb-$1 | cut -f1); \
	sed -i "s/VERSION/$(VERSION)/g" $(BUILD_DIR)/deb-$1/DEBIAN/control; \
	sed -i "s/ARCH/$1/g" $(BUILD_DIR)/deb-$1/DEBIAN/control; \
	sed -i "s/INSTALLED_SIZE/$$INSTALLED_SIZE/g" $(BUILD_DIR)/deb-$1/DEBIAN/control

	@dpkg-deb --build $(BUILD_DIR)/deb-$1 $(BUILD_DIR)/$(PACKAGE_NAME)_$(VERSION)_$1.deb
	@echo "$(GREEN)Package: $(BUILD_DIR)/$(PACKAGE_NAME)_$(VERSION)_$1.deb$(NC)"
endef

# Release targets
release: clean test build-all deb-all ## Create release artifacts
	@echo "$(BLUE)Creating release v$(VERSION)...$(NC)"
	@mkdir -p $(RELEASE_DIR)
	@cp $(BUILD_DIR)/$(BINARY_NAME)_$(ARCH_AMD64) $(RELEASE_DIR)/
	@cp $(BUILD_DIR)/$(BINARY_NAME)_$(ARCH_ARM64) $(RELEASE_DIR)/
	@cp $(BUILD_DIR)/$(PACKAGE_NAME)_$(VERSION)_$(ARCH_AMD64).deb $(RELEASE_DIR)/
	@cp $(BUILD_DIR)/$(PACKAGE_NAME)_$(VERSION)_$(ARCH_ARM64).deb $(RELEASE_DIR)/
	@echo "$(GREEN)Release artifacts:$(NC)"
	@ls -lh $(RELEASE_DIR)/

install: ## Install binary locally (requires sudo)
	@echo "$(BLUE)Installing $(BINARY_NAME)...$(NC)"
	@cp $(BUILD_DIR)/$(BINARY_NAME) /usr/local/bin/
	@mkdir -p /etc/$(BINARY_NAME)
	@cp config.example.toml /etc/$(BINARY_NAME)/config.toml
	@cp packaging/systemd/$(BINARY_NAME).service /etc/systemd/system/
	@systemctl daemon-reload
	@echo "$(GREEN)Installed. Run: sudo systemctl enable --now $(BINARY_NAME)$(NC)"

uninstall: ## Uninstall binary (requires sudo)
	@echo "$(YELLOW)Uninstalling $(BINARY_NAME)...$(NC)"
	@systemctl stop $(BINARY_NAME) 2>/dev/null || true
	@systemctl disable $(BINARY_NAME) 2>/dev/null || true
	@rm -f /usr/local/bin/$(BINARY_NAME)
	@rm -rf /etc/$(BINARY_NAME)
	@rm -f /etc/systemd/system/$(BINARY_NAME).service
	@systemctl daemon-reload
	@echo "$(GREEN)Uninstall complete$(NC)"

help: ## Show this help
	@echo "$(GREEN)$(BINARY_NAME) v$(VERSION) - Available targets:$(NC)"
	@grep -E '^[a-zA-Z0-9_/-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  $(YELLOW)%-15s$(NC) %s\n", $$1, $$2}'
