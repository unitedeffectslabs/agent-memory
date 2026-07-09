//go:build localembed

package local

import "github.com/borzou/vecstore/internal/chunker"

// NewChunkerTokenizer builds a chunker.Tokenizer backed by the embedding model's
// own HuggingFace tokenizer, resolving tokenizer.json from the configured or
// bundled assets. This lets main.go (untagged) wire a provider-matched chunker
// without importing the tagged tokenizer implementation directly.
func NewChunkerTokenizer(cfg Config) (chunker.Tokenizer, error) {
	_, tokPath, _, err := resolveAssets(cfg)
	if err != nil {
		return nil, err
	}
	return NewHFChunkerTokenizer(tokPath)
}
