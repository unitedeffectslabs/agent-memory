// Package local implements an in-process, CPU-based embedding provider backed
// by ONNX Runtime and a HuggingFace tokenizer (multilingual-e5-small).
//
// This file (and assets.go) are pure Go with NO CGo / native-library imports,
// so the default `go build`/`go test` stays green without ONNX Runtime or the
// tokenizer static library present. The native infrastructure lives in files
// guarded by the `//go:build localembed` tag (session_ort.go, tokenizer_hf.go),
// with stubs (session_stub.go, tokenizer_stub.go) for the default build.
//
// The pipeline is: prefix -> tokenize -> pad/build tensors -> ONNX forward ->
// mean-pool over the attention mask -> L2-normalize. The E5 family requires a
// "query: " prefix for search text and a "passage: " prefix for indexed text.
package local

import (
	"errors"
	"fmt"
	"math"
	"sync"

	"github.com/borzou/vecstore/internal/embeddings"
)

// LocalEmbedder implements the embeddings.Embedder seam.
var _ embeddings.Embedder = (*LocalEmbedder)(nil)

// errLocalTagRequired is returned by the native constructors in the default
// build (no `localembed` tag). Defined here in a single untagged file so both
// the stubs and any error-comparison logic can reference it.
var errLocalTagRequired = errors.New("local inference requires the 'localembed' build tag")

// Model constants for multilingual-e5-small.
const (
	modelName      = "multilingual-e5-small"
	modelDim       = 384
	modelMaxTokens = 512

	queryPrefix   = "query: "
	passagePrefix = "passage: "

	defaultBatchSize = 16
)

// EmbedTokenReserve is the token headroom the embedder consumes around each
// input at embed time: the E5 instruction prefix ("passage: " / "query: ")
// plus the tokenizer's special tokens, rounded up generously. Chunkers must
// budget chunks at MaxInputTokens() − EmbedTokenReserve so the prefixed,
// tokenized sequence never exceeds the model's hard limit — the epic's
// "effective chunk size ≈ 480" (512 − 32). Passing MaxInputTokens() straight
// through as the chunk budget overflows the model by the prefix width (the
// "512 by 516" ORT crash that silently dropped every multi-chunk file).
const EmbedTokenReserve = 32

// onnxSession is the seam over the ONNX Runtime session. Implementations take
// padded, batched int64 input tensors (input_ids, attention_mask,
// token_type_ids) and return one mean-pooled vector per input row. Injecting a
// fake here lets the pipeline be unit-tested without native libraries.
type onnxSession interface {
	run(inputIDs, attnMask, typeIDs [][]int64) ([][]float32, error)
	close() error
}

// tokenizerBackend is the seam over the HuggingFace tokenizer. It encodes a
// single string to token IDs (including the model's special tokens).
type tokenizerBackend interface {
	encode(text string) ([]uint32, error)
}

// Config configures a LocalEmbedder.
type Config struct {
	// AssetsDir is the directory holding the model, tokenizer and ONNX Runtime
	// shared library. If empty, the AGENT_MEMORY_LOCAL_ASSETS env var is used.
	AssetsDir string
	// Threads caps ONNX Runtime intra-op parallelism. <= 0 lets the runtime
	// pick a sensible default.
	Threads int
	// BatchSize is the internal sub-batch size for EmbedDocuments. <= 0 uses
	// defaultBatchSize.
	BatchSize int
}

// LocalEmbedder is an in-process ONNX embedder implementing embeddings.Embedder.
type LocalEmbedder struct {
	cfg     Config
	once    sync.Once
	session onnxSession
	tok     tokenizerBackend
	initErr error
}

// New constructs a LocalEmbedder. It is cheap: no model is loaded and no native
// library is touched until the first embed call (lazy session init).
func New(cfg Config) *LocalEmbedder {
	return &LocalEmbedder{cfg: cfg}
}

// Dimensions returns the embedding vector dimensionality.
func (e *LocalEmbedder) Dimensions() int { return modelDim }

// ModelName returns the model identifier.
func (e *LocalEmbedder) ModelName() string { return modelName }

// MaxInputTokens returns the model's context window in tokens.
func (e *LocalEmbedder) MaxInputTokens() int { return modelMaxTokens }

// EmbedDocuments embeds indexed content. Each text is prefixed with "passage: "
// then embedded in sub-batches of cfg.BatchSize.
func (e *LocalEmbedder) EmbedDocuments(texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return [][]float32{}, nil
	}
	if err := e.ensureSession(); err != nil {
		return nil, err
	}
	prefixed := withPrefix(passagePrefix, texts)

	batchSize := e.cfg.BatchSize
	if batchSize <= 0 {
		batchSize = defaultBatchSize
	}

	out := make([][]float32, 0, len(prefixed))
	for start := 0; start < len(prefixed); start += batchSize {
		end := start + batchSize
		if end > len(prefixed) {
			end = len(prefixed)
		}
		vecs, err := e.embedBatch(prefixed[start:end])
		if err != nil {
			return nil, err
		}
		out = append(out, vecs...)
	}
	return out, nil
}

// EmbedQuery embeds a single search query, prefixed with "query: ".
func (e *LocalEmbedder) EmbedQuery(text string) ([]float32, error) {
	if err := e.ensureSession(); err != nil {
		return nil, err
	}
	vecs, err := e.embedBatch(withPrefix(queryPrefix, []string{text}))
	if err != nil {
		return nil, err
	}
	if len(vecs) != 1 {
		return nil, fmt.Errorf("local: expected 1 vector, got %d", len(vecs))
	}
	return vecs[0], nil
}

// embedBatch tokenizes, builds padded tensors, runs the session, and
// L2-normalizes each pooled vector.
func (e *LocalEmbedder) embedBatch(texts []string) ([][]float32, error) {
	tokenIDs := make([][]uint32, len(texts))
	for i, t := range texts {
		ids, err := e.tok.encode(t)
		if err != nil {
			return nil, fmt.Errorf("local: tokenize: %w", err)
		}
		// Defense-in-depth: never hand the model more than its context window.
		// The chunker budgets indexed chunks below the limit, but queries reach
		// here unchunked and any budgeting bug would otherwise crash inference.
		tokenIDs[i] = truncateTokens(ids, modelMaxTokens)
	}

	ids, mask, types := buildInputs(tokenIDs)
	pooled, err := e.session.run(ids, mask, types)
	if err != nil {
		return nil, fmt.Errorf("local: inference: %w", err)
	}
	if len(pooled) != len(texts) {
		return nil, fmt.Errorf("local: expected %d vectors, got %d", len(texts), len(pooled))
	}

	out := make([][]float32, len(pooled))
	for i, v := range pooled {
		out[i] = l2Normalize(v)
	}
	return out, nil
}

// ensureSession lazily constructs the ONNX session and tokenizer exactly once.
// If a session and tokenizer are already set (tests inject fakes), real
// construction is skipped.
func (e *LocalEmbedder) ensureSession() error {
	e.once.Do(func() {
		if e.session != nil && e.tok != nil {
			return
		}
		modelPath, tokPath, dylibPath, err := resolveAssets(e.cfg)
		if err != nil {
			e.initErr = err
			return
		}
		sess, err := newORTSession(dylibPath, modelPath, e.cfg.Threads)
		if err != nil {
			e.initErr = err
			return
		}
		tok, err := newHFTokenizer(tokPath)
		if err != nil {
			_ = sess.close()
			e.initErr = err
			return
		}
		e.session = sess
		e.tok = tok
	})
	return e.initErr
}

// ---------------------------------------------------------------------------
// Pure helpers (unit-tested, no native deps)
// ---------------------------------------------------------------------------

// withPrefix returns a new slice with prefix prepended to each text.
func withPrefix(prefix string, texts []string) []string {
	out := make([]string, len(texts))
	for i, t := range texts {
		out[i] = prefix + t
	}
	return out
}

// truncateTokens caps a token sequence at max tokens. The tokenizer emits the
// model's end-of-sequence special token last; truncation preserves it so an
// over-long sequence stays well-formed (<s> … </s>) instead of ending
// mid-stream.
func truncateTokens(ids []uint32, max int) []uint32 {
	if max <= 0 || len(ids) <= max {
		return ids
	}
	out := make([]uint32, max)
	copy(out, ids[:max-1])
	out[max-1] = ids[len(ids)-1]
	return out
}

// buildInputs converts per-sequence token IDs into padded, batched int64
// tensors. All sequences are right-padded to the batch's max length. The
// attention mask is 1 for real tokens and 0 for padding; token_type_ids are all
// zero (required by the mE5 Xenova export, which takes three INT64 inputs).
func buildInputs(tokenIDs [][]uint32) (ids, mask, types [][]int64) {
	n := len(tokenIDs)
	ids = make([][]int64, n)
	mask = make([][]int64, n)
	types = make([][]int64, n)

	maxLen := 0
	for _, seq := range tokenIDs {
		if len(seq) > maxLen {
			maxLen = len(seq)
		}
	}

	for i, seq := range tokenIDs {
		rowIDs := make([]int64, maxLen)
		rowMask := make([]int64, maxLen)
		rowTypes := make([]int64, maxLen) // all zero
		for j, id := range seq {
			rowIDs[j] = int64(id)
			rowMask[j] = 1
		}
		ids[i] = rowIDs
		mask[i] = rowMask
		types[i] = rowTypes
	}
	return ids, mask, types
}

// meanPool averages the token hidden states of a single sequence, weighted by
// the attention mask (padding positions are ignored). hidden is [seqLen][dim];
// mask is [seqLen]. Returns a [dim] vector. If no positions are active it
// returns a zero vector of the input's dimensionality.
func meanPool(hidden [][]float32, mask []int64) []float32 {
	if len(hidden) == 0 {
		return nil
	}
	dim := len(hidden[0])
	out := make([]float32, dim)
	var count float64
	for i, row := range hidden {
		if i < len(mask) && mask[i] == 0 {
			continue
		}
		count++
		for k := 0; k < dim && k < len(row); k++ {
			out[k] += row[k]
		}
	}
	if count == 0 {
		return out
	}
	for k := range out {
		out[k] = float32(float64(out[k]) / count)
	}
	return out
}

// l2Normalize returns a unit-norm copy of v. A zero vector is returned
// unchanged (as a copy) to avoid division by zero.
func l2Normalize(v []float32) []float32 {
	out := make([]float32, len(v))
	var norm float64
	for _, x := range v {
		norm += float64(x) * float64(x)
	}
	if norm == 0 {
		copy(out, v)
		return out
	}
	norm = math.Sqrt(norm)
	for i, x := range v {
		out[i] = float32(float64(x) / norm)
	}
	return out
}
