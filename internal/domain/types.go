package domain

import "time"

type Directory struct {
	ID         int64
	Path       string
	FileCount  int
	ChunkCount int
	Status     string // "watching", "indexing", "error", "stopped"
	AddedAt    time.Time
}

type File struct {
	ID          int64
	DirectoryID int64
	Path        string
	Hash        string // SHA-256 hex
	IndexedAt   time.Time
}

type Chunk struct {
	Index      int
	Content    string
	Embedding  []float32
	TokenCount int
}

type SearchParams struct {
	Query     string
	Limit     int     // Max results. Default: 10.
	Offset    int     // Skip first N results (pagination). Default: 0.
	Threshold float32 // Max distance; results farther than this are excluded. Default: 1.5 (cosine). 0 means no threshold.
}

type SearchResult struct {
	FilePath   string
	ChunkIndex int
	Content    string
	Score      float32
}

// ActivityLogEntry represents a single event in the activity log.
type ActivityLogEntry struct {
	ID        int64
	Timestamp time.Time
	Path      string
	Action    string // "indexed", "ignored", "deleted", "error"
	Detail    string
}

type IndexStats struct {
	TotalFiles     int
	TotalChunks    int
	LastIndexedAt  time.Time
	IsIndexing     bool
	EmbeddingModel string
	// Progress tracking during indexing
	IndexedFiles int // files processed so far in current run
	TotalToIndex int // total files discovered in current run
}
