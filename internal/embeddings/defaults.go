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
