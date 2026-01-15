.PHONY: all build build-frontend build-clients build-server clean test

# Build configuration
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS := -ldflags "-X main.version=$(VERSION) -X main.buildTime=$(BUILD_TIME)"

# Output directories
DIST_DIR := dist
CLIENT_DIR := $(DIST_DIR)/clients

all: build

# Build everything: frontend -> clients -> server
build: build-frontend build-clients build-server

# Build frontend (Preact)
build-frontend:
	@echo "Building frontend..."
	@mkdir -p frontend/dist
	@if [ -f frontend/package.json ]; then \
		cd frontend && npm install && npm run build; \
	else \
		echo '<!DOCTYPE html><html><head><title>ib</title></head><body><h1>ib Backup Server</h1><p>Web UI coming soon...</p></body></html>' > frontend/dist/index.html; \
	fi

# Build client binaries for all platforms
build-clients:
	@echo "Building client binaries..."
	@mkdir -p $(CLIENT_DIR)
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(CLIENT_DIR)/ib-linux-amd64 ./cmd/client
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o $(CLIENT_DIR)/ib-linux-arm64 ./cmd/client
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o $(CLIENT_DIR)/ib-darwin-amd64 ./cmd/client
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o $(CLIENT_DIR)/ib-darwin-arm64 ./cmd/client
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o $(CLIENT_DIR)/ib-windows-amd64.exe ./cmd/client

# Build server binary (with embedded frontend and client binaries)
build-server:
	@echo "Building server binary..."
	@mkdir -p $(DIST_DIR)
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(DIST_DIR)/ib-server-linux-amd64 ./cmd/server
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o $(DIST_DIR)/ib-server-linux-arm64 ./cmd/server

# Build for current platform only (for development)
dev:
	@echo "Building for development..."
	@mkdir -p $(CLIENT_DIR) frontend/dist
	@echo '<!DOCTYPE html><html><head><title>ib</title></head><body><h1>ib Backup Server</h1></body></html>' > frontend/dist/index.html
	go build $(LDFLAGS) -o $(CLIENT_DIR)/ib-$(shell go env GOOS)-$(shell go env GOARCH) ./cmd/client
	go build $(LDFLAGS) -o $(DIST_DIR)/ib-server ./cmd/server

# Run tests
test:
	go test -v ./...

# Clean build artifacts
clean:
	rm -rf $(DIST_DIR)
	rm -rf frontend/dist
	rm -rf frontend/node_modules

# Install dependencies
deps:
	go mod download
	@if [ -f frontend/package.json ]; then cd frontend && npm install; fi

# Format code
fmt:
	go fmt ./...

# Lint code
lint:
	golangci-lint run

# Run the server locally
run-server: dev
	./$(DIST_DIR)/ib-server serve

# Show help
help:
	@echo "Available targets:"
	@echo "  all           - Build everything (default)"
	@echo "  build         - Build frontend, clients, and server"
	@echo "  build-frontend - Build the Preact frontend"
	@echo "  build-clients  - Build client binaries for all platforms"
	@echo "  build-server   - Build server binary"
	@echo "  dev           - Build for current platform only"
	@echo "  test          - Run tests"
	@echo "  clean         - Remove build artifacts"
	@echo "  deps          - Install dependencies"
	@echo "  fmt           - Format code"
	@echo "  lint          - Lint code"
	@echo "  run-server    - Build and run server locally"
	@echo "  help          - Show this help"
