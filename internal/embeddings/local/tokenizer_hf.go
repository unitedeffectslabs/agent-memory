//go:build localembed

package local

import (
	"fmt"
	"os"

	tok "github.com/daulet/tokenizers"

	"github.com/borzou/vecstore/internal/chunker"
)

// loadTokenizer reads a HuggingFace tokenizer.json and constructs a tokenizer.
func loadTokenizer(path string) (*tok.Tokenizer, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("local: read tokenizer: %w", err)
	}
	t, err := tok.FromBytes(data)
	if err != nil {
		return nil, fmt.Errorf("local: load tokenizer: %w", err)
	}
	return t, nil
}

// hfTokenizer is the real tokenizerBackend for inference. It encodes WITH the
// model's special tokens (<s> ... </s>), which the ONNX model expects.
type hfTokenizer struct {
	t *tok.Tokenizer
}

// newHFTokenizer constructs the inference tokenizer backend.
func newHFTokenizer(tokPath string) (tokenizerBackend, error) {
	t, err := loadTokenizer(tokPath)
	if err != nil {
		return nil, err
	}
	return &hfTokenizer{t: t}, nil
}

func (h *hfTokenizer) encode(text string) ([]uint32, error) {
	ids, _ := h.t.Encode(text, true) // addSpecialTokens = true
	return ids, nil
}

// HFChunkerTokenizer adapts the HuggingFace tokenizer to chunker.Tokenizer so
// the chunker can measure boundaries in the embedding model's own tokens.
// main.go injects it via chunker.WithTokenizer when the provider is local
// (wired in Phase 3). It encodes WITHOUT special tokens so token counts reflect
// content only; the chunk-size clamp accounts for special/prefix tokens
// separately.
type HFChunkerTokenizer struct {
	t *tok.Tokenizer
}

// compile-time assertion that the adapter satisfies the chunker seam.
var _ chunker.Tokenizer = (*HFChunkerTokenizer)(nil)

// NewHFChunkerTokenizer loads a tokenizer.json for use as a chunker.Tokenizer.
func NewHFChunkerTokenizer(tokPath string) (*HFChunkerTokenizer, error) {
	t, err := loadTokenizer(tokPath)
	if err != nil {
		return nil, err
	}
	return &HFChunkerTokenizer{t: t}, nil
}

// Encode returns content token IDs (no special tokens).
func (h *HFChunkerTokenizer) Encode(text string) ([]uint32, error) {
	ids, _ := h.t.Encode(text, false)
	return ids, nil
}

// Decode reconstructs text from token IDs, skipping special tokens.
func (h *HFChunkerTokenizer) Decode(tokens []uint32) (string, error) {
	return h.t.Decode(tokens, true), nil
}

// Close releases the underlying tokenizer.
func (h *HFChunkerTokenizer) Close() error {
	return h.t.Close()
}
