package chunker

import (
	"strings"

	"github.com/tiktoken-go/tokenizer"
)

// DefaultChunkSize is the default number of tokens per chunk.
const DefaultChunkSize = 512

// DefaultChunkOverlap is the default number of overlapping tokens between consecutive chunks.
const DefaultChunkOverlap = 50

// TokenChunker implements the Chunker interface using tiktoken-based token counting.
type TokenChunker struct {
	chunkSize int
	overlap   int
	codec     tokenizer.Codec
}

// Option configures a TokenChunker.
type Option func(*TokenChunker)

// WithChunkSize sets the maximum number of tokens per chunk.
func WithChunkSize(size int) Option {
	return func(c *TokenChunker) {
		if size > 0 {
			c.chunkSize = size
		}
	}
}

// WithOverlap sets the number of overlapping tokens between consecutive chunks.
func WithOverlap(overlap int) Option {
	return func(c *TokenChunker) {
		if overlap >= 0 {
			c.overlap = overlap
		}
	}
}

// New creates a new TokenChunker with the given options.
// It uses the cl100k_base encoding for token counting.
func New(opts ...Option) (*TokenChunker, error) {
	codec, err := tokenizer.Get(tokenizer.Cl100kBase)
	if err != nil {
		return nil, err
	}

	c := &TokenChunker{
		chunkSize: DefaultChunkSize,
		overlap:   DefaultChunkOverlap,
		codec:     codec,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c, nil
}

// ChunkText splits pre-extracted text into overlapping chunks based on token count.
// Returns an empty slice for empty content.
func (c *TokenChunker) ChunkText(content string) ([]ChunkResult, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, nil
	}

	tokens, _, err := c.codec.Encode(content)
	if err != nil {
		return nil, err
	}

	totalTokens := len(tokens)
	if totalTokens == 0 {
		return nil, nil
	}

	var results []ChunkResult
	idx := 0
	start := 0

	for start < totalTokens {
		end := start + c.chunkSize
		if end > totalTokens {
			end = totalTokens
		}

		chunkTokens := tokens[start:end]
		chunkText, err := c.codec.Decode(chunkTokens)
		if err != nil {
			return nil, err
		}

		results = append(results, ChunkResult{
			Content:    chunkText,
			Index:      idx,
			TokenCount: len(chunkTokens),
		})

		idx++

		step := c.chunkSize - c.overlap
		if step < 1 {
			step = 1
		}
		start += step

		if end == totalTokens {
			break
		}
	}

	return results, nil
}

