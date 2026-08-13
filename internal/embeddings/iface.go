package embeddings

// Embedder produces vector embeddings for text.
type Embedder interface {
	EmbedDocuments(texts []string) ([][]float32, error) // indexing path (batched)
	EmbedQuery(text string) ([]float32, error)          // search path
	Dimensions() int
	ModelName() string
	MaxInputTokens() int // 0 = no practical limit (OpenAI); model ctx for local
}
