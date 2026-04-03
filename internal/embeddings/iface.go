package embeddings

// Embedder produces vector embeddings for text.
type Embedder interface {
	Embed(texts []string) ([][]float32, error)
	Dimensions() int
	ModelName() string
}
