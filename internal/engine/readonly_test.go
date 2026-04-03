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
			if threshold != 1.5 {
				t.Errorf("expected default threshold 1.5, got %f", threshold)
			}
			return expected, nil
		},
	}

	me := &mocks.MockEmbedder{
		EmbedFn: func(texts []string) ([][]float32, error) {
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
		EmbedFn: func(texts []string) ([][]float32, error) {
			return [][]float32{{1.0}}, nil
		},
	}

	ro := NewReadOnly(ms, me)
	_, err := ro.Search(domain.SearchParams{Query: "test", Limit: 5, Offset: 10, Threshold: 1.0})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
}

func TestReadOnlySearch_EmbedderError(t *testing.T) {
	ms := &mocks.MockStore{}
	me := &mocks.MockEmbedder{
		EmbedFn: func(texts []string) ([][]float32, error) {
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
