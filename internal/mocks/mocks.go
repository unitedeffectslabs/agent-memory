package mocks

import (
	"sync"

	"github.com/borzou/vecstore/internal/chunker"
	"github.com/borzou/vecstore/internal/domain"
	"github.com/borzou/vecstore/internal/extractor"
	"github.com/borzou/vecstore/internal/watcher"
)

// ---------------------------------------------------------------------------
// MockStore implements store.Store
// ---------------------------------------------------------------------------

type MockStore struct {
	GetConfigFn         func(key string) (string, error)
	SetConfigFn         func(key, value string) error
	AddDirectoryFn      func(path string) error
	RemoveDirectoryFn   func(path string) error
	ListDirectoriesFn   func() ([]domain.Directory, error)
	UpsertFileFn        func(f domain.File) error
	RemoveFileFn        func(path string) error
	GetFileByPathFn     func(path string) (*domain.File, error)
	InsertChunksFn      func(fileID int64, chunks []domain.Chunk) error
	RemoveChunksByFileFn func(fileID int64) error
	SearchFn            func(embedding []float32, limit, offset int, threshold float32) ([]domain.SearchResult, error)
	StatsFn             func() (domain.IndexStats, error)
	InsertLogEntryFn    func(entry domain.ActivityLogEntry) error
	ListLogEntriesFn    func(limit, offset int) ([]domain.ActivityLogEntry, int, error)
	ResetFn             func(embeddingDimension int) error
	CloseFn             func() error
}

func (m *MockStore) GetConfig(key string) (string, error) {
	if m.GetConfigFn != nil {
		return m.GetConfigFn(key)
	}
	return "", nil
}

func (m *MockStore) SetConfig(key, value string) error {
	if m.SetConfigFn != nil {
		return m.SetConfigFn(key, value)
	}
	return nil
}

func (m *MockStore) AddDirectory(path string) error {
	if m.AddDirectoryFn != nil {
		return m.AddDirectoryFn(path)
	}
	return nil
}

func (m *MockStore) RemoveDirectory(path string) error {
	if m.RemoveDirectoryFn != nil {
		return m.RemoveDirectoryFn(path)
	}
	return nil
}

func (m *MockStore) ListDirectories() ([]domain.Directory, error) {
	if m.ListDirectoriesFn != nil {
		return m.ListDirectoriesFn()
	}
	return nil, nil
}

func (m *MockStore) UpsertFile(f domain.File) error {
	if m.UpsertFileFn != nil {
		return m.UpsertFileFn(f)
	}
	return nil
}

func (m *MockStore) RemoveFile(path string) error {
	if m.RemoveFileFn != nil {
		return m.RemoveFileFn(path)
	}
	return nil
}

func (m *MockStore) GetFileByPath(path string) (*domain.File, error) {
	if m.GetFileByPathFn != nil {
		return m.GetFileByPathFn(path)
	}
	return nil, nil
}

func (m *MockStore) InsertChunks(fileID int64, chunks []domain.Chunk) error {
	if m.InsertChunksFn != nil {
		return m.InsertChunksFn(fileID, chunks)
	}
	return nil
}

func (m *MockStore) RemoveChunksByFile(fileID int64) error {
	if m.RemoveChunksByFileFn != nil {
		return m.RemoveChunksByFileFn(fileID)
	}
	return nil
}

func (m *MockStore) Search(embedding []float32, limit, offset int, threshold float32) ([]domain.SearchResult, error) {
	if m.SearchFn != nil {
		return m.SearchFn(embedding, limit, offset, threshold)
	}
	return nil, nil
}

func (m *MockStore) Stats() (domain.IndexStats, error) {
	if m.StatsFn != nil {
		return m.StatsFn()
	}
	return domain.IndexStats{}, nil
}

func (m *MockStore) InsertLogEntry(entry domain.ActivityLogEntry) error {
	if m.InsertLogEntryFn != nil {
		return m.InsertLogEntryFn(entry)
	}
	return nil
}

func (m *MockStore) ListLogEntries(limit, offset int) ([]domain.ActivityLogEntry, int, error) {
	if m.ListLogEntriesFn != nil {
		return m.ListLogEntriesFn(limit, offset)
	}
	return nil, 0, nil
}

func (m *MockStore) Reset(embeddingDimension int) error {
	if m.ResetFn != nil {
		return m.ResetFn(embeddingDimension)
	}
	return nil
}

func (m *MockStore) Close() error {
	if m.CloseFn != nil {
		return m.CloseFn()
	}
	return nil
}

// ---------------------------------------------------------------------------
// MockEmbedder implements embeddings.Embedder
// ---------------------------------------------------------------------------

type MockEmbedder struct {
	EmbedFn      func(texts []string) ([][]float32, error)
	DimensionsFn func() int
	ModelNameFn  func() string
}

func (m *MockEmbedder) Embed(texts []string) ([][]float32, error) {
	if m.EmbedFn != nil {
		return m.EmbedFn(texts)
	}
	return nil, nil
}

func (m *MockEmbedder) Dimensions() int {
	if m.DimensionsFn != nil {
		return m.DimensionsFn()
	}
	return 0
}

func (m *MockEmbedder) ModelName() string {
	if m.ModelNameFn != nil {
		return m.ModelNameFn()
	}
	return ""
}

// ---------------------------------------------------------------------------
// MockChunker implements chunker.Chunker
// ---------------------------------------------------------------------------

type MockChunker struct {
	ChunkTextFn func(content string) ([]chunker.ChunkResult, error)
}

func (m *MockChunker) ChunkText(content string) ([]chunker.ChunkResult, error) {
	if m.ChunkTextFn != nil {
		return m.ChunkTextFn(content)
	}
	return nil, nil
}

// ---------------------------------------------------------------------------
// MockWatcher implements watcher.Watcher
// ---------------------------------------------------------------------------

type MockWatcher struct {
	AddFn       func(dir string) error
	RemoveFn    func(dir string) error
	StartFn     func(handler watcher.FileEventHandler) error
	StopFn      func() error
	CloseFn     func() error
	IsRunningFn func() bool
}

func (m *MockWatcher) Add(dir string) error {
	if m.AddFn != nil {
		return m.AddFn(dir)
	}
	return nil
}

func (m *MockWatcher) Remove(dir string) error {
	if m.RemoveFn != nil {
		return m.RemoveFn(dir)
	}
	return nil
}

func (m *MockWatcher) Start(handler watcher.FileEventHandler) error {
	if m.StartFn != nil {
		return m.StartFn(handler)
	}
	return nil
}

func (m *MockWatcher) Stop() error {
	if m.StopFn != nil {
		return m.StopFn()
	}
	return nil
}

func (m *MockWatcher) Close() error {
	if m.CloseFn != nil {
		return m.CloseFn()
	}
	return nil
}

func (m *MockWatcher) IsRunning() bool {
	if m.IsRunningFn != nil {
		return m.IsRunningFn()
	}
	return false
}

// ---------------------------------------------------------------------------
// MockExtractor implements extractor.Extractor
// ---------------------------------------------------------------------------

type MockExtractor struct {
	ExtractFn     func(path string) (extractor.Result, error)
	IsSupportedFn func(path string) bool
}

func (m *MockExtractor) Extract(path string) (extractor.Result, error) {
	if m.ExtractFn != nil {
		return m.ExtractFn(path)
	}
	return extractor.Result{}, nil
}

func (m *MockExtractor) IsSupported(path string) bool {
	if m.IsSupportedFn != nil {
		return m.IsSupportedFn(path)
	}
	return true
}

// ---------------------------------------------------------------------------
// MockFileEventHandler implements watcher.FileEventHandler
// ---------------------------------------------------------------------------

type MockFileEventHandler struct {
	mu       sync.Mutex
	Creates  []string
	Modifies []string
	Deletes  []string

	OnCreateFn func(path string)
	OnModifyFn func(path string)
	OnDeleteFn func(path string)
}

func (m *MockFileEventHandler) OnCreate(path string) {
	m.mu.Lock()
	m.Creates = append(m.Creates, path)
	m.mu.Unlock()
	if m.OnCreateFn != nil {
		m.OnCreateFn(path)
	}
}

func (m *MockFileEventHandler) OnModify(path string) {
	m.mu.Lock()
	m.Modifies = append(m.Modifies, path)
	m.mu.Unlock()
	if m.OnModifyFn != nil {
		m.OnModifyFn(path)
	}
}

func (m *MockFileEventHandler) OnDelete(path string) {
	m.mu.Lock()
	m.Deletes = append(m.Deletes, path)
	m.mu.Unlock()
	if m.OnDeleteFn != nil {
		m.OnDeleteFn(path)
	}
}
