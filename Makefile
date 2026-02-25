# SD-WAN Makefile
# Version 0.1.0

.PHONY: all build build-aria build-agent build-controller clean test run-controller run-agent \
        build-linux-amd64 build-linux-arm64 package package-deb package-rpm \
        package-all docker-build docker-push lint fmt help release release-deploy \
        clean-old-releases release-deploy-web docker-build-controller sync-version \
        ebpf-generate ebpf-clean ebpf-test build-aria-cli

# Variables
# 版本号：优先使用命令行传入，否则从 VERSION 文件读取，最后使用默认值
VERSION ?= $(shell cat VERSION 2>/dev/null || echo "0.1.0")
BUILD_TIME := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
LDFLAGS := -X aria/internal/cli.Version=$(VERSION) -X main.buildTime=$(BUILD_TIME) -X main.commit=$(COMMIT)

# 保留的历史版本数量（不包括 latest 指向的当前版本）
KEEP_RELEASES := 5

# Directories
BIN_DIR := bin
DIST_DIR := dist
RELEASE_DIR := releases

# Architecture
ARCH ?= amd64

#------------------------------------------------------------------------------
# Build Targets
#------------------------------------------------------------------------------

all: build

# Build unified aria binary (new)
build: build-aria

build-aria:
	@echo "Building aria $(VERSION)..."
	@mkdir -p $(BIN_DIR)
	go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/aria ./cmd

# Build aria-cli binary
build-aria-cli:
	@echo "Building aria-cli $(VERSION)..."
	@mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/aria-cli ./cmd/aria-cli

# Legacy targets for backwards compatibility
build-agent: build-aria
	@echo "Note: agent is now part of unified 'aria' binary"
	@ln -sf aria $(BIN_DIR)/agent 2>/dev/null || true

build-controller: build-aria
	@echo "Note: controller is now part of unified 'aria' binary"
	@ln -sf aria $(BIN_DIR)/controller 2>/dev/null || true

# Cross compilation for Linux
build-linux-amd64:
	@echo "Building aria for Linux AMD64..."
	@mkdir -p $(BIN_DIR)
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/aria-linux-amd64 ./cmd

build-linux-arm64:
	@echo "Building aria for Linux ARM64..."
	@mkdir -p $(BIN_DIR)
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/aria-linux-arm64 ./cmd

build-linux-amd64-cli:
	@echo "Building aria-cli for Linux AMD64..."
	@mkdir -p $(BIN_DIR)
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o $(BIN_DIR)/aria-cli-linux-amd64 ./cmd/aria-cli

build-all: build-linux-amd64 build-linux-arm64

#------------------------------------------------------------------------------
# Packaging Targets (using nFPM)
#------------------------------------------------------------------------------

# Ensure packaging scripts are executable
prepare-packaging:
	@chmod +x scripts/packaging/*.sh

# Build DEB packages
package-deb-agent: build-linux-$(ARCH) prepare-packaging
	@echo "Building Agent DEB package for $(ARCH)..."
	@mkdir -p $(DIST_DIR)
	ARCH=$(ARCH) VERSION=$(VERSION) nfpm pkg \
		--config nfpm-agent.yaml \
		--packager deb \
		--target $(DIST_DIR)/

package-deb-controller: build-linux-$(ARCH) prepare-packaging
	@echo "Building Controller DEB package for $(ARCH)..."
	@mkdir -p $(DIST_DIR)
	ARCH=$(ARCH) VERSION=$(VERSION) nfpm pkg \
		--config nfpm-controller.yaml \
		--packager deb \
		--target $(DIST_DIR)/

package-deb: package-deb-agent package-deb-controller

# Build RPM packages
package-rpm-agent: build-linux-$(ARCH) prepare-packaging
	@echo "Building Agent RPM package for $(ARCH)..."
	@mkdir -p $(DIST_DIR)
	ARCH=$(ARCH) VERSION=$(VERSION) nfpm pkg \
		--config nfpm-agent.yaml \
		--packager rpm \
		--target $(DIST_DIR)/

package-rpm-controller: build-linux-$(ARCH) prepare-packaging
	@echo "Building Controller RPM package for $(ARCH)..."
	@mkdir -p $(DIST_DIR)
	ARCH=$(ARCH) VERSION=$(VERSION) nfpm pkg \
		--config nfpm-controller.yaml \
		--packager rpm \
		--target $(DIST_DIR)/

package-rpm: package-rpm-agent package-rpm-controller

# Build all packages for a specific architecture
package-arch: package-deb package-rpm

# Build all packages for all architectures
package-all:
	@echo "Building all packages..."
	@$(MAKE) ARCH=amd64 package-arch
	@$(MAKE) ARCH=arm64 package-arch
	@echo ""
	@echo "Packages created in $(DIST_DIR)/"
	@ls -la $(DIST_DIR)/

# Legacy tarball package
package-tarball: build-linux-amd64 build-linux-arm64
	@echo "Creating tarball package $(VERSION)..."
	@mkdir -p aria-package
	cp $(BIN_DIR)/* aria-package/
	cp scripts/install.sh aria-package/ 2>/dev/null || true
	cp -r deployments/config/* aria-package/ 2>/dev/null || true
	cd aria-package && tar -czvf ../$(DIST_DIR)/aria-v$(VERSION)-linux.tar.gz *
	rm -rf aria-package
	@echo "Package created: $(DIST_DIR)/aria-v$(VERSION)-linux.tar.gz"

#------------------------------------------------------------------------------
# eBPF Targets
BPF_CLANG ?= clang
BPF_LLVM_STRIP ?= llvm-strip
BPFTOOL ?= bpftool

# Generate eBPF byte code
ebpf-generate:
	@echo "Generating eBPF programs..."
	@cd internal/eBPF && go generate
	@echo "eBPF Go code generated successfully"

# Validate eBPF structure compatibility
ebpf-validate:
	@echo "Validating eBPF structure compatibility..."
	@go run ./cmd/validate-ebpf

# Clean eBPF objects
ebpf-clean:
	@echo "Cleaning eBPF objects..."
	rm -rf $(BIN_DIR)/bpf/

# Test eBPF programs
ebpf-test: ebpf-generate
	@echo "Testing eBPF programs..."
	@if command -v $(BPFTOOL) > /dev/null; then \
		$(BPFTOOL) prog load $(BIN_DIR)/bpf/acl.o /sys/fs/bpf/test_acl pinmaps /sys/fs/bpf/test_acl_maps && \
		$(BPFTOOL) prog load $(BIN_DIR)/bpf/qos.o /sys/fs/bpf/test_qos pinmaps /sys/fs/bpf/test_qos_maps && \
		echo "eBPF programs loaded successfully" && \
		$(BPFTOOL) prog unload /sys/fs/bpf/test_acl && \
		$(BPFTOOL) map delete id $$(($(BPFTOOL) map list | grep -E "test_acl_maps" | head -n1 | awk '{print $$3}')) 2>/dev/null || true && \
		$(BPFTOOL) prog unload /sys/fs/bpf/test_qos && \
		$(BPFTOOL) map delete id $$(($(BPFTOOL) map list | grep -E "test_qos_maps" | head -n1 | awk '{print $$3}')) 2>/dev/null || true; \
	else \
		echo "bpftool not found, skipping runtime test"; \
	fi

# Run eBPF integration test
ebpf-run-test: ebpf-generate
	@echo "Running eBPF integration test..."
	@go run ./cmd/ebpf-integration-test

# Run eBPF demo
ebpf-run-demo: ebpf-generate
	@echo "Running eBPF demo..."
	@go run ./cmd/ebpf-demo

# Docker Targets
#------------------------------------------------------------------------------

DOCKER_REGISTRY ?= docker.io
DOCKER_REPO ?= aria
DOCKER_TAG ?= $(VERSION)

docker-build:
	@echo "Building Docker images..."
	docker build -t $(DOCKER_REGISTRY)/$(DOCKER_REPO)/controller:$(DOCKER_TAG) -f Dockerfile.controller .
	docker build -t $(DOCKER_REGISTRY)/$(DOCKER_REPO)/agent:$(DOCKER_TAG) -f Dockerfile.agent .

docker-push: docker-build
	@echo "Pushing Docker images..."
	docker push $(DOCKER_REGISTRY)/$(DOCKER_REPO)/controller:$(DOCKER_TAG)
	docker push $(DOCKER_REGISTRY)/$(DOCKER_REPO)/agent:$(DOCKER_TAG)

#------------------------------------------------------------------------------
# Development Targets
#------------------------------------------------------------------------------

# Run tests
test:
	@echo "Running tests..."
	go test -v -race -cover ./...

# Run tests with coverage report
test-coverage:
	@echo "Running tests with coverage..."
	go test -v -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

# Build the eBPF disaster recovery demo
build-ebpf-demo:
	@echo ">> Building eBPF disaster recovery demo..."
	@mkdir -p bin
	go build -o bin/ebpf-disaster-recovery-demo ./cmd/ebpf_disaster_recovery_demo
	@echo ">> eBPF disaster recovery demo built successfully!"

# Run the eBPF disaster recovery demo
run-ebpf-demo: build-ebpf-demo
	@echo ">> Running eBPF disaster recovery demo..."
	@sudo bin/ebpf-disaster-recovery-demo

# Format code
fmt:
	@echo "Formatting code..."
	go fmt ./...
	gofmt -s -w .

# Lint code
lint:
	@echo "Linting code..."
	golangci-lint run ./...

# Run controller locally
run-controller: build-aria
	./$(BIN_DIR)/aria controller serve --config=configs/controller.yaml

# Run agent locally
run-agent: build-aria
	sudo ./$(BIN_DIR)/aria up --server=http://localhost:8080 --token=test

#------------------------------------------------------------------------------
# Installation Targets
#------------------------------------------------------------------------------

PREFIX ?= /usr/local
INSTALL_BINDIR ?= $(PREFIX)/bin
INSTALL_SYSCONFDIR ?= $(PREFIX)/etc/aria

install: build
	@echo "Installing to $(PREFIX)..."
	install -D -m 755 $(BIN_DIR)/aria $(INSTALL_BINDIR)/aria
	install -d $(INSTALL_SYSCONFDIR)
	@echo "Installed successfully"

uninstall:
	@echo "Uninstalling..."
	rm -f $(INSTALL_BINDIR)/aria
	@echo "Uninstalled successfully"

#------------------------------------------------------------------------------
# Version Management
#------------------------------------------------------------------------------

# 同步版本号到所有需要的地方
sync-version:
	@echo "Syncing version $(VERSION) to all files..."
	@./scripts/sync-version.sh

# 同步前端文件到部署目录
sync-ui:
	@echo "Syncing UI files to deployment directory..."
	@cp deployments/controller-web/ui-dist/index.html releases/deploy/controller-web/ui-dist/
	@echo "UI files synced."

#------------------------------------------------------------------------------
# Release Targets (永久保留，不会被 clean 删除)
#------------------------------------------------------------------------------

RELEASE_VERSION_DIR := $(RELEASE_DIR)/$(VERSION)

# 清理旧版本，保留最近 KEEP_RELEASES 个（不包括 latest 指向的版本）
clean-old-releases:
	@echo "Cleaning old releases (keeping $(KEEP_RELEASES) + latest)..."
	@LATEST=$$(readlink $(RELEASE_DIR)/latest 2>/dev/null || echo ""); \
	cd $(RELEASE_DIR) && \
	ls -dt */ 2>/dev/null | grep -v "^deploy/$$" | sed 's/\/$$//' | \
	while read dir; do \
		if [ "$$dir" != "$$LATEST" ] && [ "$$dir" != "deploy" ]; then \
			echo "$$dir"; \
		fi; \
	done | tail -n +$$(($(KEEP_RELEASES) + 1)) | \
	while read old; do \
		echo "  Removing old release: $$old"; \
		rm -rf "$$old"; \
	done
	@echo "Cleanup complete."

# 创建正式发布包
release: sync-version sync-ui build-linux-amd64 clean-old-releases
	@echo "Creating release $(VERSION)..."
	@mkdir -p $(RELEASE_VERSION_DIR)
	@cp $(BIN_DIR)/aria-linux-amd64 $(RELEASE_VERSION_DIR)/aria
	@echo "$(VERSION)" > $(RELEASE_VERSION_DIR)/VERSION
	@echo "Build: $(BUILD_TIME)" >> $(RELEASE_VERSION_DIR)/VERSION
	@echo "Commit: $(COMMIT)" >> $(RELEASE_VERSION_DIR)/VERSION
	@rm -f $(RELEASE_DIR)/latest && ln -sf $(VERSION) $(RELEASE_DIR)/latest
	@echo ""
	@echo "Release created: $(RELEASE_VERSION_DIR)/"
	@ls -la $(RELEASE_VERSION_DIR)/
	@echo ""
	@echo "Current releases:"
	@ls -d $(RELEASE_DIR)/*/ 2>/dev/null | grep -v deploy | sed 's/.*\//  /'

# 准备部署包目录
release-deploy: release
	@echo "Preparing deploy package..."
	@mkdir -p $(RELEASE_DIR)/deploy/agent
	@mkdir -p $(RELEASE_DIR)/deploy/controller/images
	@# Agent 部署包
	@cp $(RELEASE_VERSION_DIR)/aria $(RELEASE_DIR)/deploy/agent/aria
	@cp deployments/scripts/agent-deploy.sh $(RELEASE_DIR)/deploy/agent/deploy.sh
	@chmod +x $(RELEASE_DIR)/deploy/agent/deploy.sh
	@# Controller 部署包
	@cp $(RELEASE_VERSION_DIR)/aria $(RELEASE_DIR)/deploy/controller/aria
	@cp deployments/scripts/controller-deploy.sh $(RELEASE_DIR)/deploy/controller/deploy.sh
	@cp deployments/scripts/controller.yaml $(RELEASE_DIR)/deploy/controller/
	@cp deployments/scripts/docker-compose.yml $(RELEASE_DIR)/deploy/controller/
	@chmod +x $(RELEASE_DIR)/deploy/controller/deploy.sh
	@echo "Deploy package ready: $(RELEASE_DIR)/deploy/"

# 构建 Controller Docker 镜像
docker-build-controller:
	@echo "Building Controller Docker image..."
	docker build -t aria-controller:latest -t aria-controller:$(VERSION) -f Dockerfile.controller .
	@echo "Image built: aria-controller:latest"

# 准备 Sidecar 容器化 Web 部署包
release-deploy-web: release docker-build-controller
	@echo "Preparing Sidecar Web deploy package..."
	@mkdir -p $(RELEASE_DIR)/deploy/controller-web/images
	@# 从源码目录复制部署模板
	@cp -r deployments/controller-web/* $(RELEASE_DIR)/deploy/controller-web/
	@# 导出 Docker 镜像
	@echo "Exporting Docker image..."
	docker save aria-controller:latest -o $(RELEASE_DIR)/deploy/controller-web/images/aria-controller.tar
	@# 设置权限
	@chmod +x $(RELEASE_DIR)/deploy/controller-web/deploy.sh
	@echo ""
	@echo "Sidecar Web deploy package ready: $(RELEASE_DIR)/deploy/controller-web/"
	@echo ""
	@echo "部署方法:"
	@echo "  1. 上传到服务器: scp -r $(RELEASE_DIR)/deploy/controller-web root@<server>:/opt/"
	@echo "  2. 执行安装:     ssh root@<server> 'cd /opt/controller-web && ./deploy.sh install'"
	@echo ""

#------------------------------------------------------------------------------
# Docker Deployment (使用 deployments/docker-compose.yaml)
#------------------------------------------------------------------------------

DOCKER_COMPOSE_FILE = releases/deploy/controller-web/docker-compose-full.yml
NETWORK_NAME = aria_shared_net

.PHONY: network-init
network-init:
	@docker network inspect $(NETWORK_NAME) >/dev/null 2>&1 || \
		(echo ">> Creating network $(NETWORK_NAME)..." && docker network create $(NETWORK_NAME))

.PHONY: up
up: network-init
	@echo ">> Deploying services via $(DOCKER_COMPOSE_FILE)..."
	docker-compose -f $(DOCKER_COMPOSE_FILE) up -d --remove-orphans

.PHONY: down
down:
	@echo ">> Stopping services..."
	docker-compose -f $(DOCKER_COMPOSE_FILE) down

.PHONY: logs
logs:
	docker-compose -f $(DOCKER_COMPOSE_FILE) logs -f

.PHONY: ps
ps:
	docker-compose -f $(DOCKER_COMPOSE_FILE) ps

#------------------------------------------------------------------------------
# Image Deployment (跨机器部署)
#------------------------------------------------------------------------------

IMAGE_DIR = bin/images
TAR_FILE = $(IMAGE_DIR)/aria-controller-latest.tar

.PHONY: save-image
save-image:
	@mkdir -p $(IMAGE_DIR)
	@docker save aria-controller:latest -o $(TAR_FILE)
	@echo ">> Image saved to $(TAR_FILE)"

.PHONY: load-and-up
load-and-up: network-init
	@echo ">> 1. Loading image from $(TAR_FILE)..."
	@if [ -f $(TAR_FILE) ]; then \
		docker load -i $(TAR_FILE); \
		docker tag $$(docker images -q aria-controller:latest) aria-controller:latest; \
		echo ">> Image loaded"; \
	else \
		echo ">> No tar file found at $(TAR_FILE), skipping load"; \
	fi
	@echo ">> 2. Restarting services with new image..."
	@cd $$(dirname $$(dirname $(DOCKER_COMPOSE_FILE))) && docker compose -f docker-compose-full.yml up -d --force-recreate

#------------------------------------------------------------------------------
# Cleanup
#------------------------------------------------------------------------------

clean:
	@echo "Cleaning..."
	rm -rf $(BIN_DIR)/
	rm -rf $(DIST_DIR)/
	rm -f *.test
	rm -f coverage.out coverage.html

clean-all: clean
	rm -rf vendor/
	go clean -cache

#------------------------------------------------------------------------------
# Help
#------------------------------------------------------------------------------

help:
	@echo "Aria Build System v$(VERSION)"
	@echo ""
	@echo "Build Targets:"
	@echo "  all                  - Build unified aria binary"
	@echo "  build                - Build aria binary"
	@echo "  build-linux-amd64    - Cross compile for Linux AMD64"
	@echo "  build-linux-arm64    - Cross compile for Linux ARM64"
	@echo "  build-all            - Build for all platforms"
	@echo ""
	@echo "Packaging Targets (requires nfpm):"
	@echo "  package-deb          - Build DEB packages"
	@echo "  package-rpm          - Build RPM packages"
	@echo "  package-all          - Build all packages for all architectures"
	@echo "  package-tarball      - Create tarball package"
	@echo ""
	@echo "Docker Targets:"
	@echo "  docker-build         - Build Docker images"
	@echo "  docker-push          - Push Docker images"
	@echo ""
	@echo "Deployment Targets (使用 deployments/docker-compose.yaml):"
	@echo "  up                   - Deploy services (自动创建 aria-shared-net)"
	@echo "  down                 - Stop services"
	@echo "  logs                 - View service logs"
	@echo "  ps                   - Show running containers"
	@echo "  network-init         - Create aria-shared-net network"
	@echo ""
	@echo "Development Targets:"
	@echo "  test                 - Run tests"
	@echo "  test-coverage        - Run tests with coverage report"
	@echo "  fmt                  - Format code"
	@echo "  lint                 - Lint code"
	@echo ""
	@echo "Installation Targets:"
	@echo "  install              - Install binaries"
	@echo "  uninstall            - Remove installed binaries"
	@echo ""
	@echo "Release Targets:"
	@echo "  release              - Create versioned release in releases/"
	@echo "  release-deploy       - Prepare deploy package in releases/deploy/"
	@echo "  release-deploy-web   - Prepare Sidecar Web deploy package (Nginx+Controller)"
	@echo "  docker-build-controller - Build Controller Docker image"
	@echo "  clean-old-releases   - Remove old releases (keep $(KEEP_RELEASES))"
	@echo ""
	@echo "Cleanup Targets:"
	@echo "  clean                - Clean build artifacts"
	@echo "  clean-all            - Clean everything including cache"
	@echo ""
	@echo "Variables:"
	@echo "  VERSION=$(VERSION)"
	@echo "  ARCH=$(ARCH)"
	@echo "  PREFIX=$(PREFIX)"
	@echo "  DOCKER_REGISTRY=$(DOCKER_REGISTRY)"
	@echo "  DOCKER_REPO=$(DOCKER_REPO)"
