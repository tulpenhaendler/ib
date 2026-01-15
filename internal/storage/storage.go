package storage

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	"github.com/johann/ib/internal/backup"
	"github.com/johann/ib/internal/config"
	_ "modernc.org/sqlite"
)

const (
	// InlineThreshold is the max size for inline storage in SQLite
	InlineThreshold = 256 * 1024 // 256KB
)

// Storage handles manifest and block persistence
type Storage struct {
	db      *sql.DB
	s3      *S3Client
	cfg     *config.ServerConfig
	writeMu sync.Mutex // Serialize write operations
}

// New creates a new storage instance
func New(cfg *config.ServerConfig) (*Storage, error) {
	// Add busy_timeout and WAL mode via connection string
	dsn := cfg.DBPath + "?_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Verify pragmas are set
	if _, err := db.Exec("PRAGMA busy_timeout=10000"); err != nil {
		return nil, fmt.Errorf("failed to set busy_timeout: %w", err)
	}
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		return nil, fmt.Errorf("failed to enable WAL mode: %w", err)
	}

	s := &Storage{
		db:  db,
		cfg: cfg,
	}

	// Initialize S3 client
	s3Client, err := NewS3Client(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize S3 client: %w", err)
	}
	s.s3 = s3Client

	// Create tables
	if err := s.migrate(); err != nil {
		return nil, fmt.Errorf("failed to migrate database: %w", err)
	}

	return s, nil
}

func (s *Storage) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS blocks (
		cid TEXT PRIMARY KEY,
		size INTEGER NOT NULL,
		original_size INTEGER NOT NULL,
		inline_data BLOB,
		s3_key TEXT,
		created_at INTEGER NOT NULL
	);

	CREATE TABLE IF NOT EXISTS manifests (
		id TEXT PRIMARY KEY,
		tags TEXT NOT NULL,
		created_at INTEGER NOT NULL,
		data BLOB NOT NULL
	);

	CREATE TABLE IF NOT EXISTS block_refs (
		manifest_id TEXT NOT NULL,
		cid TEXT NOT NULL,
		PRIMARY KEY (manifest_id, cid),
		FOREIGN KEY (manifest_id) REFERENCES manifests(id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_manifests_created_at ON manifests(created_at);
	CREATE INDEX IF NOT EXISTS idx_block_refs_cid ON block_refs(cid);
	`

	_, err := s.db.Exec(schema)
	return err
}

// Close closes the storage
func (s *Storage) Close() error {
	return s.db.Close()
}

// SaveBlock saves a block to storage
func (s *Storage) SaveBlock(ctx context.Context, cid string, data []byte, originalSize int64) error {
	var inlineData []byte
	var s3Key string

	if len(data) < InlineThreshold {
		inlineData = data
	} else {
		s3Key = cid
		if err := s.s3.Put(ctx, s3Key, data); err != nil {
			return fmt.Errorf("failed to upload to S3: %w", err)
		}
	}

	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	_, err := s.db.ExecContext(ctx, `
		INSERT OR IGNORE INTO blocks (cid, size, original_size, inline_data, s3_key, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, cid, len(data), originalSize, inlineData, s3Key, time.Now().Unix())

	return err
}

// GetBlock retrieves a block from storage
func (s *Storage) GetBlock(ctx context.Context, cid string) ([]byte, error) {
	var inlineData []byte
	var s3Key sql.NullString

	err := s.db.QueryRowContext(ctx, `
		SELECT inline_data, s3_key FROM blocks WHERE cid = ?
	`, cid).Scan(&inlineData, &s3Key)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("block not found: %s", cid)
	}
	if err != nil {
		return nil, err
	}

	if inlineData != nil {
		return inlineData, nil
	}

	if s3Key.Valid {
		return s.s3.Get(ctx, s3Key.String)
	}

	return nil, fmt.Errorf("block has no data: %s", cid)
}

// BlockExists checks if a block exists
func (s *Storage) BlockExists(ctx context.Context, cid string) (bool, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM blocks WHERE cid = ?`, cid).Scan(&count)
	return count > 0, err
}

// SaveManifest saves a manifest
func (s *Storage) SaveManifest(ctx context.Context, manifest *backup.Manifest, data []byte) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Serialize tags as JSON
	tagsJSON, err := serializeTags(manifest.Tags)
	if err != nil {
		return err
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO manifests (id, tags, created_at, data)
		VALUES (?, ?, ?, ?)
	`, manifest.ID, tagsJSON, manifest.CreatedAt.Unix(), data)
	if err != nil {
		return err
	}

	// Save block references
	for _, entry := range manifest.Entries {
		for _, cid := range entry.Blocks {
			_, err = tx.ExecContext(ctx, `
				INSERT OR IGNORE INTO block_refs (manifest_id, cid)
				VALUES (?, ?)
			`, manifest.ID, cid)
			if err != nil {
				return err
			}
		}
	}

	return tx.Commit()
}

// GetManifest retrieves a manifest by ID
func (s *Storage) GetManifest(ctx context.Context, id string) ([]byte, error) {
	var data []byte
	err := s.db.QueryRowContext(ctx, `SELECT data FROM manifests WHERE id = ?`, id).Scan(&data)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("manifest not found: %s", id)
	}
	return data, err
}

// ListManifests lists manifests, optionally filtered by tags
func (s *Storage) ListManifests(ctx context.Context, tags map[string]string) ([]ManifestInfo, error) {
	query := `SELECT id, tags, created_at FROM manifests ORDER BY created_at DESC`
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []ManifestInfo
	for rows.Next() {
		var info ManifestInfo
		var tagsJSON string
		var createdAt int64

		if err := rows.Scan(&info.ID, &tagsJSON, &createdAt); err != nil {
			return nil, err
		}

		info.Tags, _ = deserializeTags(tagsJSON)
		info.CreatedAt = time.Unix(createdAt, 0)

		// Filter by tags if provided
		if matchesTags(info.Tags, tags) {
			result = append(result, info)
		}
	}

	return result, rows.Err()
}

// GetLatestManifest gets the latest manifest matching the given tags
func (s *Storage) GetLatestManifest(ctx context.Context, tags map[string]string) ([]byte, error) {
	manifests, err := s.ListManifests(ctx, tags)
	if err != nil {
		return nil, err
	}

	if len(manifests) == 0 {
		return nil, fmt.Errorf("no manifests found matching tags")
	}

	return s.GetManifest(ctx, manifests[0].ID)
}

// DeleteManifest deletes a manifest
func (s *Storage) DeleteManifest(ctx context.Context, id string) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	_, err := s.db.ExecContext(ctx, `DELETE FROM manifests WHERE id = ?`, id)
	return err
}

// PruneManifests deletes manifests older than the cutoff and cleans up orphaned blocks
func (s *Storage) PruneManifests(ctx context.Context, cutoff time.Time) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	// Delete old manifests (block_refs will cascade delete)
	_, err := s.db.ExecContext(ctx, `
		DELETE FROM manifests WHERE created_at < ?
	`, cutoff.Unix())
	if err != nil {
		return err
	}

	// Find and delete orphaned blocks
	return s.pruneOrphanedBlocksLocked(ctx)
}

// pruneOrphanedBlocksLocked must be called with writeMu held
func (s *Storage) pruneOrphanedBlocksLocked(ctx context.Context) error {
	// Find blocks with no references
	rows, err := s.db.QueryContext(ctx, `
		SELECT b.cid, b.s3_key FROM blocks b
		LEFT JOIN block_refs br ON b.cid = br.cid
		WHERE br.cid IS NULL
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	var toDelete []string
	var s3Keys []string

	for rows.Next() {
		var cid string
		var s3Key sql.NullString
		if err := rows.Scan(&cid, &s3Key); err != nil {
			return err
		}
		toDelete = append(toDelete, cid)
		if s3Key.Valid && s3Key.String != "" {
			s3Keys = append(s3Keys, s3Key.String)
		}
	}

	// Delete from S3
	for _, key := range s3Keys {
		if err := s.s3.Delete(ctx, key); err != nil {
			fmt.Printf("Warning: failed to delete S3 object %s: %v\n", key, err)
		}
	}

	// Delete from SQLite
	for _, cid := range toDelete {
		if _, err := s.db.ExecContext(ctx, `DELETE FROM blocks WHERE cid = ?`, cid); err != nil {
			return err
		}
	}

	if len(toDelete) > 0 {
		fmt.Printf("Pruned %d orphaned blocks\n", len(toDelete))
	}

	return nil
}

// ManifestInfo contains basic manifest information
type ManifestInfo struct {
	ID        string
	Tags      map[string]string
	CreatedAt time.Time
}

func matchesTags(manifestTags, filterTags map[string]string) bool {
	for k, v := range filterTags {
		if manifestTags[k] != v {
			return false
		}
	}
	return true
}
