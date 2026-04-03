package engine

import (
	"encoding/json"
	"fmt"

	"github.com/borzou/vecstore/internal/domain"
	"github.com/borzou/vecstore/internal/embeddings"
	"github.com/borzou/vecstore/internal/store"
)

// ReadOnlyEngine provides read-only access to the indexed data.
// It is used by the stdio MCP transport — no watcher, no indexing.
type ReadOnlyEngine struct {
	store    store.Store
	embedder embeddings.Embedder
}

// NewReadOnly creates a ReadOnlyEngine with just a store and embedder.
func NewReadOnly(s store.Store, e embeddings.Embedder) *ReadOnlyEngine {
	return &ReadOnlyEngine{store: s, embedder: e}
}

// Search embeds the query and runs vector search against the store.
func (ro *ReadOnlyEngine) Search(params domain.SearchParams) ([]domain.SearchResult, error) {
	if params.Limit <= 0 {
		params.Limit = 10
	}
	if params.Threshold <= 0 {
		params.Threshold = 1.5
	}

	vectors, err := ro.embedder.Embed([]string{params.Query})
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}
	if len(vectors) == 0 {
		return nil, fmt.Errorf("embedder returned no vectors")
	}
	return ro.store.Search(vectors[0], params.Limit, params.Offset, params.Threshold)
}

// ListDirectories returns all watched directories from the store.
func (ro *ReadOnlyEngine) ListDirectories() ([]domain.Directory, error) {
	return ro.store.ListDirectories()
}

// Stats returns index statistics and watcher status.
func (ro *ReadOnlyEngine) Stats() (domain.IndexStats, error) {
	stats, err := ro.store.Stats()
	if err != nil {
		return stats, err
	}

	// Read watcher status persisted by the full engine.
	if running, _ := ro.store.GetConfig("watcher_running"); running == "true" {
		stats.IsIndexing = false // not indexing from this process, but watcher is active
	}

	return stats, nil
}

// GetIgnorePatterns returns the configured ignore patterns.
func (ro *ReadOnlyEngine) GetIgnorePatterns() ([]string, error) {
	val, err := ro.store.GetConfig("ignore_patterns")
	if err != nil {
		return nil, err
	}
	if val == "" {
		return DefaultIgnorePatterns, nil
	}
	var patterns []string
	if err := json.Unmarshal([]byte(val), &patterns); err != nil {
		return nil, fmt.Errorf("unmarshal ignore_patterns: %w", err)
	}
	return patterns, nil
}
