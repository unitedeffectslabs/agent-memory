package embeddings

import "fmt"

// Provider identifiers for the supported embedding backends.
const (
	ProviderLocal  = "local"
	ProviderOpenAI = "openai"
)

// ResolveProvider applies the single provider-resolution policy: an explicit
// configured provider wins; otherwise an existing OpenAI key implies the
// openai provider (preserving pre-provider-config users); otherwise the
// default. Every consumer of a stored provider string (composition root,
// engine, read-only search) must resolve through here — divergent copies of
// this rule are how a legacy DB gets a 'local' threshold and fingerprint
// applied to an OpenAI index.
func ResolveProvider(configuredProvider, apiKey string) string {
	if configuredProvider != "" {
		return configuredProvider
	}
	if apiKey != "" {
		return ProviderOpenAI
	}
	return DefaultProvider()
}

// Fingerprint returns the canonical index-identity string
// (provider:model:dimensions) recorded when an index is built and compared
// before read-only searches. Writer and checker must both use this
// constructor — a hand-built copy that drifts makes every valid index look
// mismatched, or a real mismatch look valid.
func Fingerprint(provider string, e Embedder) string {
	return fmt.Sprintf("%s:%s:%d", provider, e.ModelName(), e.Dimensions())
}

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
// METRIC NOTE (flagged in the PR #2 review round): the chunk_embeddings vec0
// table is declared without distance_metric, so sqlite-vec returns EUCLIDEAN
// (L2) distance, not cosine. Because every vector we store is L2-normalized,
// the two are monotonically equivalent (L2 = sqrt(2·cosine_distance)), so
// ranking is identical either way — but these threshold values are therefore
// L2-scale cutoffs, not the cosine values earlier comments claimed. On the L2
// scale the Phase 0 mE5 ranges map to: related ≈0.51–0.60, cross-lingual
// ≈0.58–0.66, unrelated ≈0.76. The local 0.6 cutoff (field-validated for
// same-language search) truncates part of the cross-lingual band; whether to
// declare distance_metric=cosine (table rebuild) or retune the L2 value
// (~0.66–0.70) is an owner decision recorded in the PR review notes.
func DefaultThreshold(provider string) float32 {
	switch provider {
	case ProviderOpenAI:
		return 1.5
	default:
		// ProviderLocal (and any unrecognized provider). L2-scale cutoff,
		// see METRIC NOTE. Tunable pending real-corpus evaluation.
		return 0.6
	}
}
