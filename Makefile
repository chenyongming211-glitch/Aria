# Aria SD-WAN Makefile
# Deployment is handled by Ansible (deployments/ansible/).
# This Makefile covers build, test, Docker image, and local dev only.

.PHONY: all build build-controller build-ariactl clean test \
        build-linux-amd64 build-linux-arm64 build-all \
        docker-build-controller save-image \
        lint fmt run-controller install uninstall help

# Variables
VERSION ?= $(shell cat VERSION 2>/dev/null || echo "0.1.0")
BUILD_TIME := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
LDFLAGS := -X aria/internal/cli.Version=$(VERSION) -X main.buildTime=$(BUILD_TIME) -X main.commit=$(COMMIT)

BIN_DIR := bin
ARCH ?= amd64

#------------------------------------------------------------------------------
# Build
#------------------------------------------------------------------------------

all: build

build: build-controller

build-controller:
	@echo "Building aria-controller $(VERSION)..."
	@mkdir -p $(BIN_DIR)
	go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/aria-controller ./cmd

build-ariactl:
	@echo "Building ariactl $(VERSION)..."
	@mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/ariactl ./cmd/ariactl

build-linux-amd64:
	@echo "Building aria-controller for Linux AMD64..."
	@mkdir -p $(BIN_DIR)
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/aria-controller-linux-amd64 ./cmd

build-linux-arm64:
	@echo "Building aria-controller for Linux ARM64..."
	@mkdir -p $(BIN_DIR)
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/aria-controller-linux-arm64 ./cmd

build-linux-amd64-cli:
	@echo "Building ariactl for Linux AMD64..."
	@mkdir -p $(BIN_DIR)
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o $(BIN_DIR)/ariactl-linux-amd64 ./cmd/ariactl

build-all: build-linux-amd64 build-linux-arm64

#------------------------------------------------------------------------------
# Docker
#------------------------------------------------------------------------------

docker-build-controller:
	@echo "Building Controller Docker image..."
	docker build -t aria-controller:latest -t aria-controller:$(VERSION) \
		--build-arg VERSION=$(VERSION) -f Dockerfile.controller .
	@echo "Image built: aria-controller:$(VERSION)"

save-image:
	@mkdir -p $(BIN_DIR)/images
	@docker save aria-controller:latest -o $(BIN_DIR)/images/aria-controller-latest.tar
	@echo ">> Image saved to $(BIN_DIR)/images/aria-controller-latest.tar"

#------------------------------------------------------------------------------
# Development
#------------------------------------------------------------------------------

test:
	@echo "Running tests..."
	go test -v -race -cover ./...

test-coverage:
	@echo "Running tests with coverage..."
	go test -v -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

fmt:
	@echo "Formatting code..."
	go fmt ./...
	gofmt -s -w .

lint:
	@echo "Linting code..."
	golangci-lint run ./...

run-controller: build-controller
	./$(BIN_DIR)/aria-controller serve --config=configs/controller.yaml

#------------------------------------------------------------------------------
# Install
#------------------------------------------------------------------------------

PREFIX ?= /usr/local

install: build
	@echo "Installing to $(PREFIX)..."
	install -D -m 755 $(BIN_DIR)/aria-controller $(PREFIX)/bin/aria-controller
	@echo "Installed successfully"

uninstall:
	rm -f $(PREFIX)/bin/aria-controller

#------------------------------------------------------------------------------
# Version
#------------------------------------------------------------------------------

sync-version:
	@echo "Syncing version $(VERSION)..."
	@./scripts/sync-version.sh

#------------------------------------------------------------------------------
# Cleanup
#------------------------------------------------------------------------------

clean:
	@echo "Cleaning..."
	rm -rf $(BIN_DIR)/
	rm -f *.test coverage.out coverage.html

clean-all: clean
	rm -rf vendor/
	go clean -cache

#------------------------------------------------------------------------------
# Help
#------------------------------------------------------------------------------

help:
	@echo "Aria Build System v$(VERSION)"
	@echo ""
	@echo "Build:"
	@echo "  build                - Build controller binary"
	@echo "  build-ariactl        - Build ariactl CLI"
	@echo "  build-linux-amd64    - Cross compile for Linux AMD64"
	@echo "  build-linux-arm64    - Cross compile for Linux ARM64"
	@echo "  build-all            - Build for all platforms"
	@echo ""
	@echo "Docker:"
	@echo "  docker-build-controller - Build Controller Docker image"
	@echo "  save-image              - Export image to bin/images/"
	@echo ""
	@echo "Development:"
	@echo "  test                 - Run tests"
	@echo "  test-coverage        - Run tests with coverage"
	@echo "  fmt                  - Format code"
	@echo "  lint                 - Lint code"
	@echo "  run-controller       - Run controller locally"
	@echo ""
	@echo "Other:"
	@echo "  install              - Install binaries"
	@echo "  uninstall            - Remove installed binaries"
	@echo "  sync-version         - Sync VERSION to all files"
	@echo "  clean                - Clean build artifacts"
	@echo "  clean-all            - Clean everything including cache"
	@echo ""
	@echo "Deployment: see deployments/ansible/"
	@echo ""
	@echo "VERSION=$(VERSION)  ARCH=$(ARCH)"
