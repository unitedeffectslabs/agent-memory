//go:build !localembed

package local

import (
	"errors"

	"github.com/borzou/vecstore/internal/chunker"
)

// NewChunkerTokenizer is the default-build stub. The real, tokenizer-backed
// implementation is provided by the `localembed` build.
func NewChunkerTokenizer(cfg Config) (chunker.Tokenizer, error) {
	return nil, errors.New("local tokenizer requires the localembed build")
}
