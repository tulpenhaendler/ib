we are making a incremental backup tool called "ib"

it is written in go, uses common golang libs like sqlite, corba, gin, etc.
it is one binary that serves two purposes, server and client.


clients can upload/download backups, server manages the actual data.
a backup is always a complete folder, in practice this will be used for blockchain backups.

a example of creating a backup cmd:

$ ib backup create --node=tezos --version=24.0 --network=mainnet ./

where node version etc can be any key value pairs, and at the end there is the path.
then we traverse that path, all folders recusively, etc and create the backup.
during the traversal, we respect gitignore files, and also have a .ibignore file with same syntax as gitignore that we also respect,
and these files are also part of the backup / what is uploaded / restored.

when creating a backup: we split large files in chunks of up to 128MB and then hash each block, then we ask the server if this hash exists,
if note we upload it (as lz4 compressed with highest compression factor), then the server stores it.
at the end we upload a backup manifest that includes all files and how to recreatee them from the original blocks, for block hashes we use the IPFS CID format.

directory traversal:
- use a streaming approach to handle large directories without exhausting memory
- do not follow symlinks (store symlink itself, not target)
- do not upload special files (sockets, device files, named pipes)

manifests include:
- last modified timestamp (mtime) for all files and directories
- file permissions/mode

incremental backup optimization:
- before creating a backup, client can request the previous manifest with matching tags from the server
- compare local file mtime against manifest mtime
- if unchanged, copy block references from old manifest instead of re-hashing/uploading
- only process files that have changed since last backup

the server saves each manifest with our tags and also a date, we can have multiple backups with the same tags etc, in this case the date is what makes them different.
the server keeps records for manifests and blocks in a sqlite db, small blocks (<256kb) are inlined in the table, larger blocks are offloaded to S3 compatible storage.

server auto-pruning:
- automatically prune manifests older than 3 months (default, configurable via server config)
- data blocks are only pruned when no manifest references them anymore
- pruning runs periodically (e.g., daily) and also after manual backup deletion
- when pruning blocks: remove from SQLite (inline blocks) and from S3 (offloaded blocks)

the server also has a web UI (use a simple preact website) where we can explore the backups and download them via a http link, or the server also offers to download the ib cli,
so we have to build the ib command for linux,mac,windows in all archs and then include download links in the web UI.

$ ib server serve
- starts the server (API + web UI)
- web UI static files are embedded in the binary using go:embed
- single binary serves both API endpoints and HTML/JS/CSS content
- optional: --metrics-port <port> to expose Prometheus metrics on a separate port

prometheus metrics (when --metrics-port is set):
- ib_blocks_total - total number of blocks stored
- ib_manifests_total - total number of manifests
- ib_storage_bytes - total storage used (inline + S3)
- ib_bandwidth_upload_bytes_total - total bytes uploaded
- ib_bandwidth_download_bytes_total - total bytes downloaded

project structure:
- ./main.go - entrypoint
- ./cmd/ - cobra commands (e.g., ./cmd/server/, ./cmd/backup/, ./cmd/login/, etc.)
- ./frontend/ - preact web UI source


everyone can download.

for creating backups one must have a auth token,

server:
ib server token show
<token>

token show generates a token if not exists in the config, and shows it.

client:
ib login server.address.com --token <token>

when ib login is called wihtout the auth we just store the server location in the
config file, for users who like to dowload via the cli but dont have permissions to upload.



for uploading we have a default concurrency of 16 upload workers,
for downloading we have a concurrency of 4.

challenges:
- config writeable / auto create config in the correct place in all OS
- efficient uploading with 16 concurrent workers
- efficient downloading, recreate directory structure with correct permissions/mtime
- multi-part/block file reassembly during restore
- cross-platform path handling (Windows vs Unix)
- SQLite concurrency (use WAL mode, connection pooling)
- atomicity of backup creation (cleanup orphaned blocks on failure)
- race conditions when multiple clients upload same block
- retry logic with exponential backoff for transient network failures
- progress reporting for large operations


