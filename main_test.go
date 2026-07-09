package main

import (
	"path/filepath"
	"testing"

	"github.com/borzou/vecstore/internal/embeddings"
	"github.com/borzou/vecstore/internal/engine"
	"github.com/borzou/vecstore/internal/mocks"
	"github.com/borzou/vecstore/internal/store"
)

func TestResolveProvider(t *testing.T) {
	cases := []struct {
		name       string
		configured string
		apiKey     string
		want       string
	}{
		{"default when nothing set", "", "", embeddings.ProviderLocal},
		{"openai when key present", "", "sk-abc", embeddings.ProviderOpenAI},
		{"explicit provider wins over key", embeddings.ProviderLocal, "sk-abc", embeddings.ProviderLocal},
		{"explicit openai with no key", embeddings.ProviderOpenAI, "", embeddings.ProviderOpenAI},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := resolveProvider(tc.configured, tc.apiKey); got != tc.want {
				t.Fatalf("resolveProvider(%q,%q)=%q want %q", tc.configured, tc.apiKey, got, tc.want)
			}
		})
	}
}

func TestResolveModel(t *testing.T) {
	if got := resolveModel(embeddings.ProviderLocal, ""); got != "multilingual-e5-small" {
		t.Fatalf("local default model = %q", got)
	}
	if got := resolveModel(embeddings.ProviderOpenAI, ""); got != "text-embedding-3-small" {
		t.Fatalf("openai default model = %q", got)
	}
	if got := resolveModel(embeddings.ProviderOpenAI, "text-embedding-3-large"); got != "text-embedding-3-large" {
		t.Fatalf("stored model should win, got %q", got)
	}
}

// TestSetConfigSwitchProvider proves that switching the provider swaps both the
// embedder and the chunker and resets the index. The app's config store is a
// real SQLite DB; the engine is built over mocks so the Reset call is observable.
// Switching to OpenAI (tiktoken chunker) keeps this test native-lib-free.
func TestSetConfigSwitchProvider(t *testing.T) {
	dir := t.TempDir()
	s, err := store.NewSQLiteStore(filepath.Join(dir, "cfg.db"), 384)
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	defer s.Close()

	// The engine runs over mocks so Reset is observable. initialScan (fired by
	// Reset→Start) lists directories; the default mock returns none, so it exits.
	var resetCalled bool
	engStore := &mocks.MockStore{
		ResetFn: func(dim int) error { resetCalled = true; return nil },
	}

	newEmb := &mocks.MockEmbedder{
		DimensionsFn:     func() int { return 1536 },
		MaxInputTokensFn: func() int { return 0 },
	}
	var factoryProvider, factoryModel string
	eng := engine.New(engStore, &mocks.MockEmbedder{}, &mocks.MockChunker{}, &mocks.MockWatcher{}, &mocks.MockExtractor{})

	app := &App{
		engine: eng,
		store:  s,
		newEmbedder: func(provider, apiKey, model string) (embeddings.Embedder, error) {
			factoryProvider = provider
			factoryModel = model
			return newEmb, nil
		},
	}

	// Start from the default (local) provider; switch to openai.
	if err := app.SetConfig("embedding_provider", embeddings.ProviderOpenAI); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}

	if factoryProvider != embeddings.ProviderOpenAI {
		t.Fatalf("factory called with provider %q, want openai", factoryProvider)
	}
	if factoryModel != "text-embedding-3-small" {
		t.Fatalf("factory model = %q, want text-embedding-3-small", factoryModel)
	}
	if !resetCalled {
		t.Fatal("expected engine.Reset() to hit the store")
	}
	if p, _ := s.GetConfig("embedding_provider"); p != embeddings.ProviderOpenAI {
		t.Fatalf("persisted provider = %q", p)
	}
	if m, _ := s.GetConfig("embedding_model"); m != "text-embedding-3-small" {
		t.Fatalf("persisted model = %q", m)
	}
}
