package engine

import (
	"encoding/json"
	"fmt"
	"strconv"

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

	// Resolve the provider through the SAME policy the composition root used
	// to build this process's embedder (config, else key-implies-openai, else
	// default). Reading the config value alone would diverge on legacy DBs
	// where only an API key is set: main.go wires an OpenAI embedder while a
	// config-only read here would resolve 'local' — mis-picking the threshold
	// and mislabeling the fingerprint.
	providerCfg, _ := ro.store.GetConfig("embedding_provider")
	apiKey, _ := ro.store.GetConfig("openai_api_key")
	provider := embeddings.ResolveProvider(providerCfg, apiKey)
	if params.Threshold <= 0 {
		params.Threshold = embeddings.DefaultThreshold(provider)
	}

	// Guard against querying an index built with a different embedding model
	// than the one this read-only process is configured with. The full
	// fingerprint (provider:model:dimensions, per the epic) catches
	// same-dimension model swaps that the bare dimension cannot; the dimension
	// check remains as fallback for DBs written before the fingerprint existed.
	ownFP := embeddings.Fingerprint(provider, ro.embedder)
	if indexFP, _ := ro.store.GetConfig("embedding_fingerprint"); indexFP != "" {
		if indexFP != ownFP {
			return nil, fmt.Errorf("index was built with embedding %q but this process is configured for %q — mixed vectors would return garbage-ranked results; reopen the GUI app to rebuild the index", indexFP, ownFP)
		}
	} else if dimStr, _ := ro.store.GetConfig("embedding_dimension"); dimStr != "" {
		if indexDim, convErr := strconv.Atoi(dimStr); convErr == nil && indexDim != ro.embedder.Dimensions() {
			return nil, fmt.Errorf("index was built with a different embedding model (dim %d) than the active provider (dim %d) — reopen the GUI app to rebuild the index", indexDim, ro.embedder.Dimensions())
		}
	}

	vector, err := ro.embedder.EmbedQuery(params.Query)
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}
	if len(vector) == 0 {
		return nil, fmt.Errorf("embedder returned no vectors")
	}
	return ro.store.Search(vector, params.Limit, params.Offset, params.Threshold)
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
