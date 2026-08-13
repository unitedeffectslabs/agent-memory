package engine

import (
	"fmt"
	"testing"

	"github.com/borzou/vecstore/internal/domain"
	"github.com/borzou/vecstore/internal/mocks"
)

func TestReadOnlySearch(t *testing.T) {
	expected := []domain.SearchResult{
		{FilePath: "/a.txt", ChunkIndex: 0, Content: "result", Score: 0.95},
	}

	ms := &mocks.MockStore{
		SearchFn: func(embedding []float32, limit, offset int, threshold float32) ([]domain.SearchResult, error) {
			if limit != 10 {
				t.Errorf("expected default limit 10, got %d", limit)
			}
			// No provider configured → falls back to the local default provider,
			// whose default threshold is 0.6.
			if threshold != 0.6 {
				t.Errorf("expected default threshold 0.6, got %f", threshold)
			}
			return expected, nil
		},
	}

	me := &mocks.MockEmbedder{
		EmbedDocumentsFn: func(texts []string) ([][]float32, error) {
			return [][]float32{{1.0, 2.0}}, nil
		},
	}

	ro := NewReadOnly(ms, me)

	results, err := ro.Search(domain.SearchParams{Query: "test"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 || results[0].FilePath != "/a.txt" {
		t.Errorf("unexpected results: %v", results)
	}
}

func TestReadOnlySearch_ExplicitParams(t *testing.T) {
	ms := &mocks.MockStore{
		SearchFn: func(embedding []float32, limit, offset int, threshold float32) ([]domain.SearchResult, error) {
			if limit != 5 {
				t.Errorf("expected limit 5, got %d", limit)
			}
			if offset != 10 {
				t.Errorf("expected offset 10, got %d", offset)
			}
			if threshold != 1.0 {
				t.Errorf("expected threshold 1.0, got %f", threshold)
			}
			return nil, nil
		},
	}

	me := &mocks.MockEmbedder{
		EmbedDocumentsFn: func(texts []string) ([][]float32, error) {
			return [][]float32{{1.0}}, nil
		},
	}

	ro := NewReadOnly(ms, me)
	_, err := ro.Search(domain.SearchParams{Query: "test", Limit: 5, Offset: 10, Threshold: 1.0})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
}

func TestReadOnlySearch_ProviderAwareThreshold(t *testing.T) {
	ms := &mocks.MockStore{
		GetConfigFn: func(key string) (string, error) {
			if key == "embedding_provider" {
				return "openai", nil
			}
			return "", nil
		},
		SearchFn: func(embedding []float32, limit, offset int, threshold float32) ([]domain.SearchResult, error) {
			// OpenAI provider default threshold is 1.5.
			if threshold != 1.5 {
				t.Errorf("expected openai default threshold 1.5, got %f", threshold)
			}
			return nil, nil
		},
	}
	me := &mocks.MockEmbedder{
		EmbedDocumentsFn: func(texts []string) ([][]float32, error) {
			return [][]float32{{1.0}}, nil
		},
	}

	ro := NewReadOnly(ms, me)
	if _, err := ro.Search(domain.SearchParams{Query: "test"}); err != nil {
		t.Fatalf("Search: %v", err)
	}
}

func TestReadOnlySearch_DimensionMismatch(t *testing.T) {
	searchCalled := false
	ms := &mocks.MockStore{
		GetConfigFn: func(key string) (string, error) {
			if key == "embedding_dimension" {
				return "1536", nil // index built with a 1536-dim model
			}
			return "", nil
		},
		SearchFn: func(embedding []float32, limit, offset int, threshold float32) ([]domain.SearchResult, error) {
			searchCalled = true
			return nil, nil
		},
	}
	me := &mocks.MockEmbedder{
		DimensionsFn: func() int { return 384 }, // active provider is 384-dim
		EmbedDocumentsFn: func(texts []string) ([][]float32, error) {
			return [][]float32{{1.0}}, nil
		},
	}

	ro := NewReadOnly(ms, me)
	_, err := ro.Search(domain.SearchParams{Query: "test"})
	if err == nil {
		t.Fatal("expected dimension-mismatch error, got nil")
	}
	if searchCalled {
		t.Error("store.Search must not be called on a dimension mismatch")
	}
}

func TestReadOnlySearch_DimensionMatch(t *testing.T) {
	ms := &mocks.MockStore{
		GetConfigFn: func(key string) (string, error) {
			if key == "embedding_dimension" {
				return "384", nil
			}
			return "", nil
		},
		SearchFn: func(embedding []float32, limit, offset int, threshold float32) ([]domain.SearchResult, error) {
			return nil, nil
		},
	}
	me := &mocks.MockEmbedder{
		DimensionsFn: func() int { return 384 },
		EmbedDocumentsFn: func(texts []string) ([][]float32, error) {
			return [][]float32{{1.0}}, nil
		},
	}

	ro := NewReadOnly(ms, me)
	if _, err := ro.Search(domain.SearchParams{Query: "test"}); err != nil {
		t.Fatalf("Search with matching dimension should succeed: %v", err)
	}
}

func TestReadOnlySearch_EmbedderError(t *testing.T) {
	ms := &mocks.MockStore{}
	me := &mocks.MockEmbedder{
		EmbedDocumentsFn: func(texts []string) ([][]float32, error) {
			return nil, fmt.Errorf("no API key configured")
		},
	}

	ro := NewReadOnly(ms, me)
	_, err := ro.Search(domain.SearchParams{Query: "test"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestReadOnlyListDirectories(t *testing.T) {
	expected := []domain.Directory{{ID: 1, Path: "/tmp/test"}}
	ms := &mocks.MockStore{
		ListDirectoriesFn: func() ([]domain.Directory, error) {
			return expected, nil
		},
	}

	ro := NewReadOnly(ms, &mocks.MockEmbedder{})
	dirs, err := ro.ListDirectories()
	if err != nil {
		t.Fatalf("ListDirectories: %v", err)
	}
	if len(dirs) != 1 || dirs[0].Path != "/tmp/test" {
		t.Errorf("unexpected dirs: %v", dirs)
	}
}

func TestReadOnlyStats(t *testing.T) {
	ms := &mocks.MockStore{
		StatsFn: func() (domain.IndexStats, error) {
			return domain.IndexStats{TotalFiles: 42, TotalChunks: 100}, nil
		},
		GetConfigFn: func(key string) (string, error) {
			if key == "watcher_running" {
				return "true", nil
			}
			return "", nil
		},
	}

	ro := NewReadOnly(ms, &mocks.MockEmbedder{})
	stats, err := ro.Stats()
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats.TotalFiles != 42 || stats.TotalChunks != 100 {
		t.Errorf("unexpected stats: %+v", stats)
	}
}

func TestReadOnlyGetIgnorePatterns(t *testing.T) {
	ms := &mocks.MockStore{
		GetConfigFn: func(key string) (string, error) {
			if key == "ignore_patterns" {
				return `["*.log","build/**"]`, nil
			}
			return "", nil
		},
	}

	ro := NewReadOnly(ms, &mocks.MockEmbedder{})
	patterns, err := ro.GetIgnorePatterns()
	if err != nil {
		t.Fatalf("GetIgnorePatterns: %v", err)
	}
	if len(patterns) != 2 || patterns[0] != "*.log" {
		t.Errorf("unexpected patterns: %v", patterns)
	}
}

func TestReadOnlyGetIgnorePatterns_Defaults(t *testing.T) {
	ms := &mocks.MockStore{
		GetConfigFn: func(key string) (string, error) {
			return "", nil
		},
	}

	ro := NewReadOnly(ms, &mocks.MockEmbedder{})
	patterns, err := ro.GetIgnorePatterns()
	if err != nil {
		t.Fatalf("GetIgnorePatterns: %v", err)
	}
	if len(patterns) != len(DefaultIgnorePatterns) {
		t.Errorf("expected default patterns, got %d", len(patterns))
	}
}

// TestReadOnlySearch_FingerprintMismatchSameDimension pins the case the bare
// dimension guard is blind to: a same-width model swap (e.g. the epic's named
// upgrade candidate granite-97m is also 384-dim). Mixed vectors would return
// garbage-ranked results silently.
func TestReadOnlySearch_FingerprintMismatchSameDimension(t *testing.T) {
	searchCalled := false
	ms := &mocks.MockStore{
		GetConfigFn: func(key string) (string, error) {
			switch key {
			case "embedding_fingerprint":
				return "local:multilingual-e5-small:384", nil // index identity
			case "embedding_provider":
				return "local", nil
			}
			return "", nil
		},
		SearchFn: func(embedding []float32, limit, offset int, threshold float32) ([]domain.SearchResult, error) {
			searchCalled = true
			return nil, nil
		},
	}
	me := &mocks.MockEmbedder{
		DimensionsFn: func() int { return 384 },                          // SAME dimension...
		ModelNameFn:  func() string { return "granite-embedding-97m" },   // ...different model
		EmbedDocumentsFn: func(texts []string) ([][]float32, error) {
			return [][]float32{{1.0}}, nil
		},
	}

	ro := NewReadOnly(ms, me)
	_, err := ro.Search(domain.SearchParams{Query: "test"})
	if err == nil {
		t.Fatal("expected fingerprint-mismatch error for same-dimension model swap, got nil")
	}
	if searchCalled {
		t.Error("store.Search must not be called on a fingerprint mismatch")
	}
}

// TestReadOnlySearch_FingerprintMatch: matching fingerprints search normally.
func TestReadOnlySearch_FingerprintMatch(t *testing.T) {
	ms := &mocks.MockStore{
		GetConfigFn: func(key string) (string, error) {
			switch key {
			case "embedding_fingerprint":
				return "local:multilingual-e5-small:384", nil
			case "embedding_provider":
				return "local", nil
			}
			return "", nil
		},
		SearchFn: func(embedding []float32, limit, offset int, threshold float32) ([]domain.SearchResult, error) {
			return []domain.SearchResult{}, nil
		},
	}
	me := &mocks.MockEmbedder{
		DimensionsFn: func() int { return 384 },
		ModelNameFn:  func() string { return "multilingual-e5-small" },
		EmbedDocumentsFn: func(texts []string) ([][]float32, error) {
			return [][]float32{{1.0}}, nil
		},
	}
	ro := NewReadOnly(ms, me)
	if _, err := ro.Search(domain.SearchParams{Query: "test"}); err != nil {
		t.Fatalf("matching fingerprint should search cleanly: %v", err)
	}
}
