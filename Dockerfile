# Build stage
FROM --platform=$BUILDPLATFORM golang:1.24-alpine AS builder

ARG TARGETOS
ARG TARGETARCH

RUN apk add --no-cache git

WORKDIR /src

# Copy go mod files first for caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Use pre-built client binaries if available, otherwise build them
RUN mkdir -p dist/clients && \
    if [ ! -f dist/clients/ib-linux-amd64 ]; then \
        echo "Building client binaries..." && \
        GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o dist/clients/ib-linux-amd64 ./cmd/client && \
        GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o dist/clients/ib-linux-arm64 ./cmd/client && \
        GOOS=darwin GOARCH=amd64 go build -ldflags="-s -w" -o dist/clients/ib-darwin-amd64 ./cmd/client && \
        GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w" -o dist/clients/ib-darwin-arm64 ./cmd/client && \
        GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o dist/clients/ib-windows-amd64.exe ./cmd/client; \
    fi

# Build server for target platform
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -ldflags="-s -w" -o /ib-server ./cmd/server

# Runtime stage
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

COPY --from=builder /ib-server /app/ib-server

# Create data directory for SQLite
RUN mkdir -p /data

# Environment variables for configuration
ENV IB_DB_PATH=/data/ib.db \
    IB_LISTEN_ADDR=:8080 \
    IB_RETENTION_DAYS=90 \
    IB_S3_REGION=us-east-1 \
    IB_IPFS_ENABLED=false \
    IB_IPFS_GATEWAY_ADDR=:8081

# Expose ports
EXPOSE 8080
EXPOSE 9090
EXPOSE 8081
EXPOSE 4001
EXPOSE 4001/udp

# Volume for SQLite database
VOLUME ["/data"]

ENTRYPOINT ["/app/ib-server"]
CMD ["serve"]
