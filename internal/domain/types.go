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
	// ExtractorVersion is the extraction-logic version this file was indexed
	// with. A file is skipped as unchanged only when its hash AND this version
	// match — bumping a type's extractor version re-indexes that type even
	// though file contents (and hashes) are unchanged.
	ExtractorVersion int
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
	Threshold float32 // Max distance; results farther than this are excluded. 0 means "use the provider-aware default" (openai 1.5, local 0.6 — see embeddings.DefaultThreshold).
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
	Provider       string
	EmbeddingModel string
	// Progress tracking during indexing
	IndexedFiles int // files processed so far in current run
	TotalToIndex int // total files discovered in current run
}
