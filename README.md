# ib - Incremental Backup

A fast, deduplicated backup tool with content-addressed storage. Built for backing up large datasets like blockchain nodes.

## Features

- **Content-addressed storage** - Blocks are identified by IPFS CIDv1 hashes (SHA-256)
- **Deduplication** - Identical blocks are stored once, even across different backups
- **Incremental backups** - Only changed files are re-chunked and uploaded
- **LZ4 compression** - Fast compression with good ratios
- **S3 storage** - Blocks stored in any S3-compatible storage (AWS, MinIO, etc.)
- **Web UI** - Browse and download backups from the browser
- **Streaming downloads** - Download as .tar.gz or .zip without server-side buffering
- **Tag-based organization** - Filter backups by custom tags (project, version, node, etc.)
- **Auto-pruning** - Configurable retention policy with automatic cleanup

## Quick Start

### Server Setup

```bash
# Download the server binary
curl -LO https://your-server/cli/linux/amd64
chmod +x ib-server-linux-amd64

# Generate a token and configure
./ib-server-linux-amd64 token show

# Edit ~/.config/ib/server.json with your S3 credentials

# Start the server
./ib-server-linux-amd64 serve --title "My Backups"
```

### Client Usage

```bash
# Download the client
curl -LO https://your-server/cli/linux/amd64
chmod +x ib-linux-amd64

# Login to server
./ib-linux-amd64 login http://your-server:8080

# Create a backup with tags
./ib-linux-amd64 backup create /data/node \
  --tag name="Ethereum Node" \
  --tag network=mainnet \
  --tag version=1.0

# List backups
./ib-linux-amd64 backup list

# Restore a backup
./ib-linux-amd64 backup restore --id 20260115-142855-289518bf ./restore-dir
```

## Docker Deployment

```yaml
# docker-compose.yml
services:
  ib-server:
    image: ib-server
    ports:
      - "8080:8080"
    volumes:
      - ib-data:/data
    environment:
      IB_TOKEN: "your-secret-token"
      IB_S3_BUCKET: "ib-backups"
      IB_S3_ENDPOINT: "http://minio:9000"
      IB_S3_ACCESS_KEY: "minioadmin"
      IB_S3_SECRET_KEY: "minioadmin"
      IB_TITLE: "My Backups"
      IB_RETENTION_DAYS: "90"
```

## Configuration

### Server Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `IB_TOKEN` | Authentication token for uploads | Required |
| `IB_S3_BUCKET` | S3 bucket name | Required |
| `IB_S3_ENDPOINT` | S3 endpoint URL | AWS default |
| `IB_S3_ACCESS_KEY` | S3 access key | Required |
| `IB_S3_SECRET_KEY` | S3 secret key | Required |
| `IB_S3_REGION` | S3 region | `us-east-1` |
| `IB_DB_PATH` | SQLite database path | `/data/ib.db` |
| `IB_LISTEN_ADDR` | Server listen address | `:8080` |
| `IB_TITLE` | Web UI title | `ib Backup` |
| `IB_RETENTION_DAYS` | Days to keep backups | `90` |
| `IB_METRICS_PORT` | Prometheus metrics port | Disabled |

## Architecture

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│   Client    │────▶│   Server    │────▶│     S3      │
│  (ib CLI)   │     │ (ib-server) │     │   Storage   │
└─────────────┘     └─────────────┘     └─────────────┘
                           │
                           ▼
                    ┌─────────────┐
                    │   SQLite    │
                    │  (metadata) │
                    └─────────────┘
```

- **Blocks < 256KB**: Stored inline in SQLite
- **Blocks >= 256KB**: Stored in S3, referenced by CID
- **Manifests**: Compressed JSON stored in SQLite
- **Chunking**: 128MB fixed-size blocks

## API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/health` | GET | Health check |
| `/api/manifests` | GET | List manifests (filter with `?tag.key=value`) |
| `/api/manifests/:id` | GET | Get manifest details |
| `/api/manifests/latest` | GET | Get latest manifest matching tags |
| `/api/manifests` | POST | Create manifest (auth required) |
| `/api/blocks/:cid` | GET | Download block |
| `/api/blocks` | POST | Upload block (auth required) |
| `/api/download/:id.tar.gz` | GET | Download backup as tar.gz |
| `/api/download/:id.zip` | GET | Download backup as zip |
| `/cli/:os/:arch` | GET | Download CLI binary |

## Building from Source

```bash
# Build everything
make build

# Or build individually
go build -o dist/clients/ib-linux-amd64 ./cmd/client
go build -o dist/ib-server-linux-amd64 ./cmd/server
```

## License

MIT
