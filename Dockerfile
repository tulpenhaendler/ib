# Build stage
FROM golang:1.24-alpine AS builder

RUN apk add --no-cache git make

WORKDIR /src

# Copy go mod files first for caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build frontend placeholder (or use pre-built)
RUN mkdir -p frontend/dist dist/clients && \
    if [ ! -f frontend/dist/index.html ]; then \
        echo '<!DOCTYPE html><html><body>ib</body></html>' > frontend/dist/index.html; \
    fi && \
    echo "placeholder" > dist/clients/placeholder.txt

# Build client binaries
RUN GOOS=linux GOARCH=amd64 go build -o dist/clients/ib-linux-amd64 ./cmd/client && \
    GOOS=linux GOARCH=arm64 go build -o dist/clients/ib-linux-arm64 ./cmd/client && \
    GOOS=darwin GOARCH=amd64 go build -o dist/clients/ib-darwin-amd64 ./cmd/client && \
    GOOS=darwin GOARCH=arm64 go build -o dist/clients/ib-darwin-arm64 ./cmd/client && \
    GOOS=windows GOARCH=amd64 go build -o dist/clients/ib-windows-amd64.exe ./cmd/client

# Build server
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /ib-server ./cmd/server

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
    IB_S3_REGION=us-east-1

# Expose ports
EXPOSE 8080
EXPOSE 9090

# Volume for SQLite database
VOLUME ["/data"]

ENTRYPOINT ["/app/ib-server"]
CMD ["serve"]
