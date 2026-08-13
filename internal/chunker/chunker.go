package chunker

import (
	"strings"

	"github.com/tiktoken-go/tokenizer"
)

// DefaultChunkSize is the default number of tokens per chunk.
const DefaultChunkSize = 512

// DefaultChunkOverlap is the default number of overlapping tokens between consecutive chunks.
const DefaultChunkOverlap = 50

// tiktokenAdapter wraps a tokenizer.Codec to satisfy the Tokenizer interface,
// converting between the codec's []uint tokens and the interface's []uint32.
type tiktokenAdapter struct {
	codec tokenizer.Codec
}

func (a tiktokenAdapter) Encode(text string) ([]uint32, error) {
	ids, _, err := a.codec.Encode(text)
	if err != nil {
		return nil, err
	}
	out := make([]uint32, len(ids))
	for i, id := range ids {
		out[i] = uint32(id)
	}
	return out, nil
}

func (a tiktokenAdapter) Decode(tokens []uint32) (string, error) {
	ids := make([]uint, len(tokens))
	for i, t := range tokens {
		ids[i] = uint(t)
	}
	return a.codec.Decode(ids)
}

// TokenChunker implements the Chunker interface using tiktoken-based token counting.
type TokenChunker struct {
	chunkSize int
	overlap   int
	tokenizer Tokenizer
	maxTokens int
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

// WithTokenizer sets a custom tokenizer, overriding the default tiktoken codec.
func WithTokenizer(t Tokenizer) Option {
	return func(c *TokenChunker) {
		if t != nil {
			c.tokenizer = t
		}
	}
}

// WithMaxInputTokens clamps the effective chunk size so no chunk exceeds the
// tokenizer/model's maximum input length. A value of 0 disables clamping.
func WithMaxInputTokens(n int) Option {
	return func(c *TokenChunker) {
		if n > 0 {
			c.maxTokens = n
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
		tokenizer: tiktokenAdapter{codec: codec},
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

	tokens, err := c.tokenizer.Encode(content)
	if err != nil {
		return nil, err
	}

	totalTokens := len(tokens)
	if totalTokens == 0 {
		return nil, nil
	}

	// Effective chunk size: clamp to maxTokens when it is smaller than the
	// configured chunk size. With the default (maxTokens==0) this is a no-op.
	size := c.chunkSize
	if c.maxTokens > 0 && c.maxTokens < size {
		size = c.maxTokens
	}

	var results []ChunkResult
	idx := 0
	start := 0

	for start < totalTokens {
		end := start + size
		if end > totalTokens {
			end = totalTokens
		}

		chunkTokens := tokens[start:end]
		chunkText, err := c.tokenizer.Decode(chunkTokens)
		if err != nil {
			return nil, err
		}

		results = append(results, ChunkResult{
			Content:    chunkText,
			Index:      idx,
			TokenCount: len(chunkTokens),
		})

		idx++

		step := size - c.overlap
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

