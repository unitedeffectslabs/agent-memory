package store

import (
	"database/sql"
	"encoding/binary"
	"fmt"
	"math"
	"time"

	sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
	_ "github.com/mattn/go-sqlite3"

	"github.com/borzou/vecstore/internal/domain"
)

// SQLiteStore implements Store using SQLite + sqlite-vec.
type SQLiteStore struct {
	db *sql.DB
}

// NewSQLiteStore opens (or creates) a SQLite database at dbPath and initializes the schema.
func NewSQLiteStore(dbPath string) (*SQLiteStore, error) {
	sqlite_vec.Auto()
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_foreign_keys=on&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	// SQLite performs best with serialized access. Limit to one connection
	// so concurrent goroutines (initial scan + UI polls) don't hit BUSY errors.
	db.SetMaxOpenConns(1)

	s := &SQLiteStore{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

func (s *SQLiteStore) migrate() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS config (
			key   TEXT PRIMARY KEY,
			value TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS directories (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			path        TEXT UNIQUE NOT NULL,
			file_count  INTEGER NOT NULL DEFAULT 0,
			chunk_count INTEGER NOT NULL DEFAULT 0,
			status      TEXT NOT NULL DEFAULT 'watching',
			added_at    DATETIME NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS files (
			id           INTEGER PRIMARY KEY AUTOINCREMENT,
			directory_id INTEGER NOT NULL,
			path         TEXT UNIQUE NOT NULL,
			hash         TEXT NOT NULL,
			indexed_at   DATETIME NOT NULL,
			FOREIGN KEY(directory_id) REFERENCES directories(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS chunks (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			file_id     INTEGER NOT NULL,
			chunk_index INTEGER NOT NULL,
			content     TEXT NOT NULL,
			token_count INTEGER NOT NULL DEFAULT 0,
			FOREIGN KEY(file_id) REFERENCES files(id) ON DELETE CASCADE
		)`,
		`CREATE VIRTUAL TABLE IF NOT EXISTS chunk_embeddings USING vec0(
			chunk_id  INTEGER PRIMARY KEY,
			embedding FLOAT[1536]
		)`,
		`CREATE TABLE IF NOT EXISTS activity_log (
			id        INTEGER PRIMARY KEY AUTOINCREMENT,
			timestamp DATETIME NOT NULL,
			path      TEXT NOT NULL,
			action    TEXT NOT NULL,
			detail    TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE INDEX IF NOT EXISTS idx_activity_log_timestamp ON activity_log(timestamp DESC)`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("exec %q: %w", stmt[:min(40, len(stmt))], err)
		}
	}
	return nil
}

// --- Config ---

func (s *SQLiteStore) GetConfig(key string) (string, error) {
	var val string
	err := s.db.QueryRow(`SELECT value FROM config WHERE key = ?`, key).Scan(&val)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return val, err
}

func (s *SQLiteStore) SetConfig(key, value string) error {
	_, err := s.db.Exec(`INSERT OR REPLACE INTO config(key, value) VALUES(?, ?)`, key, value)
	return err
}

// --- Directories ---

func (s *SQLiteStore) AddDirectory(path string) error {
	_, err := s.db.Exec(
		`INSERT OR IGNORE INTO directories(path, added_at) VALUES(?, ?)`,
		path, time.Now().UTC(),
	)
	return err
}

func (s *SQLiteStore) RemoveDirectory(path string) error {
	_, err := s.db.Exec(`DELETE FROM directories WHERE path = ?`, path)
	return err
}

func (s *SQLiteStore) ListDirectories() ([]domain.Directory, error) {
	rows, err := s.db.Query(`
		SELECT d.id, d.path,
			COALESCE(fc.cnt, 0) AS file_count,
			COALESCE(cc.cnt, 0) AS chunk_count,
			d.status, d.added_at
		FROM directories d
		LEFT JOIN (SELECT directory_id, COUNT(*) AS cnt FROM files GROUP BY directory_id) fc
			ON fc.directory_id = d.id
		LEFT JOIN (SELECT f.directory_id, COUNT(*) AS cnt FROM chunks c JOIN files f ON f.id = c.file_id GROUP BY f.directory_id) cc
			ON cc.directory_id = d.id
		ORDER BY d.added_at
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var dirs []domain.Directory
	for rows.Next() {
		var d domain.Directory
		if err := rows.Scan(&d.ID, &d.Path, &d.FileCount, &d.ChunkCount, &d.Status, &d.AddedAt); err != nil {
			return nil, err
		}
		dirs = append(dirs, d)
	}
	return dirs, rows.Err()
}

// --- Files ---

func (s *SQLiteStore) UpsertFile(f domain.File) error {
	_, err := s.db.Exec(
		`INSERT OR REPLACE INTO files(directory_id, path, hash, indexed_at) VALUES(?, ?, ?, ?)`,
		f.DirectoryID, f.Path, f.Hash, f.IndexedAt.UTC(),
	)
	return err
}

func (s *SQLiteStore) RemoveFile(path string) error {
	// Chunks and embeddings are cascade-deleted via FK.
	_, err := s.db.Exec(`DELETE FROM files WHERE path = ?`, path)
	return err
}

func (s *SQLiteStore) GetFileByPath(path string) (*domain.File, error) {
	var f domain.File
	err := s.db.QueryRow(
		`SELECT id, directory_id, path, hash, indexed_at FROM files WHERE path = ?`, path,
	).Scan(&f.ID, &f.DirectoryID, &f.Path, &f.Hash, &f.IndexedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &f, nil
}

// --- Chunks ---

func (s *SQLiteStore) InsertChunks(fileID int64, chunks []domain.Chunk) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmtChunk, err := tx.Prepare(`INSERT INTO chunks(file_id, chunk_index, content, token_count) VALUES(?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmtChunk.Close()

	stmtVec, err := tx.Prepare(`INSERT INTO chunk_embeddings(chunk_id, embedding) VALUES(?, ?)`)
	if err != nil {
		return err
	}
	defer stmtVec.Close()

	for _, c := range chunks {
		res, err := stmtChunk.Exec(fileID, c.Index, c.Content, c.TokenCount)
		if err != nil {
			return fmt.Errorf("insert chunk: %w", err)
		}
		chunkID, err := res.LastInsertId()
		if err != nil {
			return err
		}

		if len(c.Embedding) > 0 {
			blob := float32SliceToBlob(c.Embedding)
			if _, err := stmtVec.Exec(chunkID, blob); err != nil {
				return fmt.Errorf("insert embedding: %w", err)
			}
		}
	}

	return tx.Commit()
}

func (s *SQLiteStore) RemoveChunksByFile(fileID int64) error {
	// Get chunk IDs first to remove from vec table.
	rows, err := s.db.Query(`SELECT id FROM chunks WHERE file_id = ?`, fileID)
	if err != nil {
		return err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, id := range ids {
		if _, err := tx.Exec(`DELETE FROM chunk_embeddings WHERE chunk_id = ?`, id); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`DELETE FROM chunks WHERE file_id = ?`, fileID); err != nil {
		return err
	}

	return tx.Commit()
}

// --- Search ---

func (s *SQLiteStore) Search(embedding []float32, limit, offset int, threshold float32) ([]domain.SearchResult, error) {
	blob := float32SliceToBlob(embedding)

	// Fetch extra rows to account for offset and threshold filtering.
	// sqlite-vec's K is applied before we can filter by distance, so we
	// over-fetch and filter in Go.
	fetchLimit := limit + offset
	if threshold > 0 {
		// Over-fetch to compensate for rows that will be filtered out.
		fetchLimit = fetchLimit * 2
	}

	query := `
		SELECT c.chunk_index, c.content, f.path, cev.distance
		FROM chunk_embeddings cev
		JOIN chunks c ON c.id = cev.chunk_id
		JOIN files  f ON f.id = c.file_id
		WHERE cev.embedding MATCH ?
		  AND K = ?
		ORDER BY cev.distance
	`
	rows, err := s.db.Query(query, blob, fetchLimit)
	if err != nil {
		return nil, fmt.Errorf("search query: %w", err)
	}
	defer rows.Close()

	var all []domain.SearchResult
	for rows.Next() {
		var r domain.SearchResult
		if err := rows.Scan(&r.ChunkIndex, &r.Content, &r.FilePath, &r.Score); err != nil {
			return nil, err
		}
		if threshold > 0 && r.Score > threshold {
			continue
		}
		all = append(all, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Apply offset.
	if offset > 0 && offset < len(all) {
		all = all[offset:]
	} else if offset >= len(all) {
		return nil, nil
	}

	// Apply limit.
	if limit > 0 && limit < len(all) {
		all = all[:limit]
	}

	return all, nil
}

// --- Stats ---

func (s *SQLiteStore) Stats() (domain.IndexStats, error) {
	var stats domain.IndexStats

	if err := s.db.QueryRow(`SELECT COUNT(*) FROM files`).Scan(&stats.TotalFiles); err != nil {
		return stats, err
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM chunks`).Scan(&stats.TotalChunks); err != nil {
		return stats, err
	}

	var lastIndexed sql.NullString
	if err := s.db.QueryRow(`SELECT MAX(indexed_at) FROM files`).Scan(&lastIndexed); err != nil {
		return stats, err
	}
	if lastIndexed.Valid {
		stats.LastIndexedAt = parseTimestamp(lastIndexed.String)
	}

	model, _ := s.GetConfig("embedding_model")
	if model == "" {
		model = "text-embedding-3-small"
	}
	stats.EmbeddingModel = model

	return stats, nil
}

// --- Activity Log ---

func (s *SQLiteStore) InsertLogEntry(entry domain.ActivityLogEntry) error {
	_, err := s.db.Exec(
		`INSERT INTO activity_log(timestamp, path, action, detail) VALUES(?, ?, ?, ?)`,
		entry.Timestamp.UTC(), entry.Path, entry.Action, entry.Detail,
	)
	return err
}

func (s *SQLiteStore) ListLogEntries(limit, offset int) ([]domain.ActivityLogEntry, int, error) {
	var total int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM activity_log`).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := s.db.Query(
		`SELECT id, timestamp, path, action, detail FROM activity_log ORDER BY timestamp DESC LIMIT ? OFFSET ?`,
		limit, offset,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var entries []domain.ActivityLogEntry
	for rows.Next() {
		var e domain.ActivityLogEntry
		var ts string
		if err := rows.Scan(&e.ID, &ts, &e.Path, &e.Action, &e.Detail); err != nil {
			return nil, 0, err
		}
		e.Timestamp = parseTimestamp(ts)
		entries = append(entries, e)
	}
	return entries, total, rows.Err()
}

// --- Reset ---

// Reset clears all indexed data (embeddings, chunks, files) and recreates the
// vector table with the given embedding dimension. Configuration and watched
// directories are preserved so the user doesn't have to re-onboard.
func (s *SQLiteStore) Reset(embeddingDimension int) error {
	if embeddingDimension <= 0 {
		embeddingDimension = 1536
	}

	stmts := []string{
		`DELETE FROM chunk_embeddings`,
		`DROP TABLE IF EXISTS chunk_embeddings`,
		`DELETE FROM chunks`,
		`DELETE FROM files`,
		`DELETE FROM activity_log`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("reset %q: %w", stmt, err)
		}
	}

	// Recreate the vector table with the correct dimension for the current model.
	createVT := fmt.Sprintf(`CREATE VIRTUAL TABLE IF NOT EXISTS chunk_embeddings USING vec0(
		chunk_id  INTEGER PRIMARY KEY,
		embedding FLOAT[%d]
	)`, embeddingDimension)
	if _, err := s.db.Exec(createVT); err != nil {
		return fmt.Errorf("recreate chunk_embeddings: %w", err)
	}

	return nil
}

// --- Close ---

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

// --- helpers ---

// parseTimestamp tries multiple time formats that SQLite may produce.
func parseTimestamp(s string) time.Time {
	for _, layout := range []string{
		"2006-01-02 15:04:05-07:00",
		"2006-01-02T15:04:05Z",
		time.RFC3339,
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

func float32SliceToBlob(fs []float32) []byte {
	b := make([]byte, len(fs)*4)
	for i, f := range fs {
		binary.LittleEndian.PutUint32(b[i*4:], math.Float32bits(f))
	}
	return b
}
