package embeddings

// Provider identifiers for the supported embedding backends.
const (
	ProviderLocal  = "local"
	ProviderOpenAI = "openai"
)

// Default model identifiers per provider.
const (
	defaultLocalModel  = "multilingual-e5-small"
	defaultOpenAIModel = "text-embedding-3-small"
)

// Embedding dimensions per model.
const (
	localDimension       = 384
	openAISmallDimension = 1536
	openAILargeDimension = 3072
)

// DefaultProvider returns the provider used when none is configured. The
// local, in-process model is the default so the app works offline out of the box.
func DefaultProvider() string { return ProviderLocal }

// DefaultModel returns the default model identifier for a provider. Any
// unrecognized provider falls back to the local model (the default provider).
func DefaultModel(provider string) string {
	if provider == ProviderOpenAI {
		return defaultOpenAIModel
	}
	return defaultLocalModel
}

// DefaultDimension returns the embedding vector dimension for a provider/model
// pair. It centralizes the dimension knowledge that would otherwise be scattered
// as magic numbers across the store and composition root.
func DefaultDimension(provider, model string) int {
	switch provider {
	case ProviderOpenAI:
		if model == "text-embedding-3-large" {
			return openAILargeDimension
		}
		return openAISmallDimension
	default: // ProviderLocal and any unrecognized provider
		return localDimension
	}
}

// DefaultThreshold returns the default maximum distance for search results for a
// given provider. Results farther than this are excluded when a caller does not
// supply an explicit threshold.
//
// sqlite-vec's vec0 tables use cosine distance by default, and our embedding
// vectors are L2-normalized, so cosine distance is the correct metric to
// threshold on for both providers.
func DefaultThreshold(provider string) float32 {
	switch provider {
	case ProviderOpenAI:
		return 1.5
	default:
		// ProviderLocal (and any unrecognized provider). This is an initial
		// value derived from the Phase 0 spike's cosine-distance ranges for the
		// multilingual-e5-small model (related ~0.13–0.18, cross-lingual
		// ~0.17–0.22, unrelated ~0.29). It is intentionally conservative and is
		// tunable pending real-corpus evaluation.
		return 0.6
	}
}
