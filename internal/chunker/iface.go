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

// Tokenizer encodes text to tokens and back. It abstracts the underlying
// tokenization backend (e.g. tiktoken) so alternate tokenizers can be injected.
type Tokenizer interface {
	Encode(text string) ([]uint32, error)
	Decode(tokens []uint32) (string, error)
}
