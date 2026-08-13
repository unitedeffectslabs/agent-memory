package store

import "github.com/borzou/vecstore/internal/domain"

// Store is the persistence interface for Agent Memory.
type Store interface {
	GetConfig(key string) (string, error)
	SetConfig(key, value string) error
	AddDirectory(path string) error
	RemoveDirectory(path string) error
	ListDirectories() ([]domain.Directory, error)
	RemoveFile(path string) error
	GetFileByPath(path string) (*domain.File, error)
	RemoveChunksByFile(fileID int64) error
	// UpsertFileWithChunks atomically replaces a file's index entry: old chunks
	// (and their vectors) are removed, the file row is upserted, and the new
	// chunks are inserted — all in one transaction, so a crash mid-index leaves
	// the file either fully indexed or untouched-and-retryable, never recorded
	// at the new hash with missing chunks. (The former separate UpsertFile /
	// InsertChunks steps live on as concrete SQLiteStore methods for tests but
	// are no longer part of the engine's contract.)
	UpsertFileWithChunks(f domain.File, chunks []domain.Chunk) error
	Search(embedding []float32, limit, offset int, threshold float32) ([]domain.SearchResult, error)
	Stats() (domain.IndexStats, error)
	InsertLogEntry(entry domain.ActivityLogEntry) error
	// UpsertLogEntry keeps at most one row per (path, action): if one exists
	// its timestamp and detail are updated in place, otherwise the entry is
	// inserted. Used for error rows so a persistently-failing file yields one
	// living row instead of an identical append on every launch.
	UpsertLogEntry(entry domain.ActivityLogEntry) error
	ListLogEntries(limit, offset int) ([]domain.ActivityLogEntry, int, error)
	Reset(embeddingDimension int) error
	Close() error
}
