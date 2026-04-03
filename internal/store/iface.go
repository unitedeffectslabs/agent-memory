package store

import "github.com/borzou/vecstore/internal/domain"

// Store is the persistence interface for Agent Memory.
type Store interface {
	GetConfig(key string) (string, error)
	SetConfig(key, value string) error
	AddDirectory(path string) error
	RemoveDirectory(path string) error
	ListDirectories() ([]domain.Directory, error)
	UpsertFile(f domain.File) error
	RemoveFile(path string) error
	GetFileByPath(path string) (*domain.File, error)
	InsertChunks(fileID int64, chunks []domain.Chunk) error
	RemoveChunksByFile(fileID int64) error
	Search(embedding []float32, limit, offset int, threshold float32) ([]domain.SearchResult, error)
	Stats() (domain.IndexStats, error)
	InsertLogEntry(entry domain.ActivityLogEntry) error
	ListLogEntries(limit, offset int) ([]domain.ActivityLogEntry, int, error)
	Reset(embeddingDimension int) error
	Close() error
}
