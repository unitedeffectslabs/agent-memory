package chunker

// ChunkResult holds a single chunk's data.
type ChunkResult struct {
	Content    string
	Index      int
	TokenCount int
}

// Chunker splits text content into overlapping chunks.
type Chunker interface {
	ChunkText(content string) ([]ChunkResult, error)
}
