package engine

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/borzou/vecstore/internal/chunker"
	"github.com/borzou/vecstore/internal/domain"
	"github.com/borzou/vecstore/internal/extractor"
	"github.com/borzou/vecstore/internal/mocks"
	"github.com/borzou/vecstore/internal/watcher"
)

// helpers

// defaultMockExtractor returns a MockExtractor that reads files from disk
// (like the real extractor does for .txt files).
func defaultMockExtractor() *mocks.MockExtractor {
	return &mocks.MockExtractor{
		IsSupportedFn: func(path string) bool {
			return strings.HasSuffix(path, ".txt") || strings.HasSuffix(path, ".go") || strings.HasSuffix(path, ".js")
		},
		ExtractFn: func(path string) (extractor.Result, error) {
			data, err := os.ReadFile(path)
			if err != nil {
				return extractor.Result{}, err
			}
			return extractor.Result{Text: string(data)}, nil
		},
	}
}

func tempFileInDir(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestIndexFile(t *testing.T) {
	dir := t.TempDir()
	content := "hello world"
	filePath := tempFileInDir(t, dir, "test.txt", content)
	expectedHash := fmt.Sprintf("%x", sha256.Sum256([]byte(content)))

	var (
		upsertedFile   domain.File
		insertedChunks []domain.Chunk
		insertedFileID int64
	)

	ms := &mocks.MockStore{
		GetFileByPathFn: func(path string) (*domain.File, error) {
			return nil, nil // new file
		},
		ListDirectoriesFn: func() ([]domain.Directory, error) {
			return []domain.Directory{{ID: 42, Path: dir}}, nil
		},
		UpsertFileFn: func(f domain.File) error {
			upsertedFile = f
			return nil
		},
		InsertChunksFn: func(fileID int64, chunks []domain.Chunk) error {
			insertedFileID = fileID
			insertedChunks = chunks
			return nil
		},
	}

	// After upsert, GetFileByPath should return the file with an ID.
	upsertCalled := false
	ms.UpsertFileFn = func(f domain.File) error {
		upsertedFile = f
		upsertCalled = true
		// Simulate that after upsert, store returns the file with ID.
		ms.GetFileByPathFn = func(path string) (*domain.File, error) {
			return &domain.File{ID: 7, DirectoryID: 42, Path: path, Hash: expectedHash, IndexedAt: time.Now()}, nil
		}
		return nil
	}

	mc := &mocks.MockChunker{
		ChunkTextFn: func(c string) ([]chunker.ChunkResult, error) {
			return []chunker.ChunkResult{
				{Content: "hello", Index: 0, TokenCount: 1},
				{Content: "world", Index: 1, TokenCount: 1},
			}, nil
		},
	}

	me := &mocks.MockEmbedder{
		EmbedFn: func(texts []string) ([][]float32, error) {
			vecs := make([][]float32, len(texts))
			for i := range texts {
				vecs[i] = []float32{float32(i), 0.5}
			}
			return vecs, nil
		},
	}

	mw := &mocks.MockWatcher{}

	eng := New(ms, me, mc, mw, defaultMockExtractor())

	if err := eng.IndexFile(filePath); err != nil {
		t.Fatalf("IndexFile: %v", err)
	}

	if !upsertCalled {
		t.Fatal("UpsertFile was not called")
	}
	if upsertedFile.Hash != expectedHash {
		t.Errorf("hash = %s, want %s", upsertedFile.Hash, expectedHash)
	}
	if upsertedFile.DirectoryID != 42 {
		t.Errorf("directoryID = %d, want 42", upsertedFile.DirectoryID)
	}
	if insertedFileID != 7 {
		t.Errorf("insertedFileID = %d, want 7", insertedFileID)
	}
	if len(insertedChunks) != 2 {
		t.Fatalf("len(chunks) = %d, want 2", len(insertedChunks))
	}
	if insertedChunks[0].Content != "hello" {
		t.Errorf("chunk[0].Content = %q, want %q", insertedChunks[0].Content, "hello")
	}
	if insertedChunks[1].Embedding[0] != 1.0 {
		t.Errorf("chunk[1].Embedding[0] = %f, want 1.0", insertedChunks[1].Embedding[0])
	}
}

func TestIndexFileSkipsUnchanged(t *testing.T) {
	dir := t.TempDir()
	content := "same content"
	filePath := tempFileInDir(t, dir, "unchanged.txt", content)
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(content)))

	chunkerCalled := false

	ms := &mocks.MockStore{
		GetFileByPathFn: func(path string) (*domain.File, error) {
			return &domain.File{ID: 1, Path: path, Hash: hash}, nil
		},
	}

	mc := &mocks.MockChunker{
		ChunkTextFn: func(c string) ([]chunker.ChunkResult, error) {
			chunkerCalled = true
			return nil, nil
		},
	}

	me := &mocks.MockEmbedder{}
	mw := &mocks.MockWatcher{}

	eng := New(ms, me, mc, mw, defaultMockExtractor())

	if err := eng.IndexFile(filePath); err != nil {
		t.Fatalf("IndexFile: %v", err)
	}

	if chunkerCalled {
		t.Error("chunker should not have been called for unchanged file")
	}
}

func TestSearch(t *testing.T) {
	expected := []domain.SearchResult{
		{FilePath: "/a.txt", ChunkIndex: 0, Content: "result", Score: 0.95},
	}
	queryVec := []float32{1.0, 2.0}

	ms := &mocks.MockStore{
		SearchFn: func(embedding []float32, limit, offset int, threshold float32) ([]domain.SearchResult, error) {
			if limit != 5 {
				t.Errorf("limit = %d, want 5", limit)
			}
			return expected, nil
		},
	}

	me := &mocks.MockEmbedder{
		EmbedFn: func(texts []string) ([][]float32, error) {
			if len(texts) != 1 || texts[0] != "test query" {
				t.Errorf("unexpected texts: %v", texts)
			}
			return [][]float32{queryVec}, nil
		},
	}

	eng := New(ms, me, &mocks.MockChunker{}, &mocks.MockWatcher{}, defaultMockExtractor())

	results, err := eng.Search(domain.SearchParams{Query: "test query", Limit: 5})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	if results[0].FilePath != "/a.txt" {
		t.Errorf("FilePath = %s, want /a.txt", results[0].FilePath)
	}
}

func TestAddDirectory(t *testing.T) {
	addDirCalled := false
	watcherAddCalled := false

	ms := &mocks.MockStore{
		AddDirectoryFn: func(path string) error {
			addDirCalled = true
			return nil
		},
		ListDirectoriesFn: func() ([]domain.Directory, error) {
			return nil, nil
		},
	}

	mw := &mocks.MockWatcher{
		AddFn: func(dir string) error {
			watcherAddCalled = true
			return nil
		},
	}

	eng := New(ms, &mocks.MockEmbedder{}, &mocks.MockChunker{}, mw, defaultMockExtractor())

	// Use a temp dir so filepath.Walk works.
	dir := t.TempDir()
	if err := eng.AddDirectory(dir); err != nil {
		t.Fatalf("AddDirectory: %v", err)
	}

	if !addDirCalled {
		t.Error("store.AddDirectory was not called")
	}
	if !watcherAddCalled {
		t.Error("watcher.Add was not called")
	}
}

func TestRemoveDirectory(t *testing.T) {
	storeCalled := false
	watcherCalled := false

	ms := &mocks.MockStore{
		RemoveDirectoryFn: func(path string) error {
			storeCalled = true
			return nil
		},
	}

	mw := &mocks.MockWatcher{
		RemoveFn: func(dir string) error {
			watcherCalled = true
			return nil
		},
	}

	eng := New(ms, &mocks.MockEmbedder{}, &mocks.MockChunker{}, mw, defaultMockExtractor())

	if err := eng.RemoveDirectory("/some/dir"); err != nil {
		t.Fatalf("RemoveDirectory: %v", err)
	}

	if !storeCalled {
		t.Error("store.RemoveDirectory was not called")
	}
	if !watcherCalled {
		t.Error("watcher.Remove was not called")
	}
}

func TestStartStop(t *testing.T) {
	startCalled := false
	stopCalled := false

	var capturedHandler watcher.FileEventHandler

	mw := &mocks.MockWatcher{
		StartFn: func(handler watcher.FileEventHandler) error {
			startCalled = true
			capturedHandler = handler
			return nil
		},
		StopFn: func() error {
			stopCalled = true
			return nil
		},
	}

	eng := New(&mocks.MockStore{}, &mocks.MockEmbedder{}, &mocks.MockChunker{}, mw, defaultMockExtractor())

	if err := eng.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if !startCalled {
		t.Error("watcher.Start was not called")
	}
	if capturedHandler != eng {
		t.Error("engine should be passed as the FileEventHandler")
	}

	if err := eng.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if !stopCalled {
		t.Error("watcher.Stop was not called")
	}
}

// ---------------------------------------------------------------------------
// Ignore pattern tests
// ---------------------------------------------------------------------------

func TestShouldSkipDir(t *testing.T) {
	cases := []struct {
		pattern string
		relDir  string
		want    bool
	}{
		{"node_modules/**", "node_modules", true},
		{"node_modules/**", "node_modules/lodash", true}, // subdir also matches the pattern
		{".git/**", ".git", true},
		{"vendor/**", "vendor", true},
		{"vendor/**", "src", false},
		{".git", ".git", true},    // exact match
		{"*.log", "logs", false},  // file pattern doesn't skip dirs
	}

	for _, tc := range cases {
		got := shouldSkipDir([]string{tc.pattern}, tc.relDir)
		if got != tc.want {
			t.Errorf("shouldSkipDir(%q, %q) = %v, want %v", tc.pattern, tc.relDir, got, tc.want)
		}
	}
}

func TestMatchesAnyPattern(t *testing.T) {
	baseDir := "/project"
	cases := []struct {
		patterns []string
		path     string
		want     bool
	}{
		{[]string{"*.pyc"}, "/project/main.pyc", true},
		{[]string{"*.pyc"}, "/project/main.py", false},
		{[]string{"node_modules/**"}, "/project/node_modules/lodash/index.js", true},
		{[]string{"node_modules/**"}, "/project/src/index.js", false},
		{[]string{".DS_Store"}, "/project/.DS_Store", true},
		{[]string{".DS_Store"}, "/project/src/.DS_Store", false}, // not recursive
		{[]string{"**/.DS_Store"}, "/project/src/.DS_Store", true},
		{[]string{"*.lock"}, "/project/go.sum", false},
		{[]string{"*.lock"}, "/project/yarn.lock", true},
		{[]string{"build/**", "dist/**"}, "/project/dist/main.js", true},
		{[]string{"build/**", "dist/**"}, "/project/src/main.js", false},
	}

	for _, tc := range cases {
		got, err := matchesAnyPattern(tc.patterns, baseDir, tc.path)
		if err != nil {
			t.Errorf("matchesAnyPattern(%v, %q, %q): unexpected error: %v", tc.patterns, baseDir, tc.path, err)
			continue
		}
		if got != tc.want {
			t.Errorf("matchesAnyPattern(%v, %q, %q) = %v, want %v", tc.patterns, baseDir, tc.path, got, tc.want)
		}
	}
}

func TestGetIgnorePatternsSeededDefaults(t *testing.T) {
	// When no ignore_patterns key exists, GetIgnorePatterns should seed defaults.
	var storedKey, storedVal string

	ms := &mocks.MockStore{
		GetConfigFn: func(key string) (string, error) {
			if key == "ignore_patterns" && storedVal == "" {
				return "", nil // not set yet
			}
			if key == "ignore_patterns" {
				return storedVal, nil
			}
			return "", nil
		},
		SetConfigFn: func(key, value string) error {
			storedKey = key
			storedVal = value
			return nil
		},
	}

	eng := New(ms, &mocks.MockEmbedder{}, &mocks.MockChunker{}, &mocks.MockWatcher{}, defaultMockExtractor())

	patterns, err := eng.GetIgnorePatterns()
	if err != nil {
		t.Fatalf("GetIgnorePatterns: %v", err)
	}

	if storedKey != "ignore_patterns" {
		t.Error("SetConfig was not called with ignore_patterns key")
	}
	if len(patterns) != len(DefaultIgnorePatterns) {
		t.Errorf("got %d patterns, want %d", len(patterns), len(DefaultIgnorePatterns))
	}
	if patterns[0] != DefaultIgnorePatterns[0] {
		t.Errorf("first pattern = %q, want %q", patterns[0], DefaultIgnorePatterns[0])
	}
}

func TestSetIgnorePatterns(t *testing.T) {
	var storedVal string

	ms := &mocks.MockStore{
		SetConfigFn: func(key, value string) error {
			if key == "ignore_patterns" {
				storedVal = value
			}
			return nil
		},
	}

	eng := New(ms, &mocks.MockEmbedder{}, &mocks.MockChunker{}, &mocks.MockWatcher{}, defaultMockExtractor())

	patterns := []string{"*.tmp", "build/**"}
	if err := eng.SetIgnorePatterns(patterns); err != nil {
		t.Fatalf("SetIgnorePatterns: %v", err)
	}

	var got []string
	if err := json.Unmarshal([]byte(storedVal), &got); err != nil {
		t.Fatalf("unmarshal stored value: %v", err)
	}
	if len(got) != 2 || got[0] != "*.tmp" || got[1] != "build/**" {
		t.Errorf("stored patterns = %v, want [*.tmp build/**]", got)
	}
}

func TestAddDirectorySkipsIgnoredFiles(t *testing.T) {
	dir := t.TempDir()

	// Create files: one normal, one ignored.
	tempFileInDir(t, dir, "main.go", "package main")
	nodeDir := filepath.Join(dir, "node_modules")
	if err := os.Mkdir(nodeDir, 0755); err != nil {
		t.Fatal(err)
	}
	tempFileInDir(t, nodeDir, "index.js", "const x = 1")
	tempFileInDir(t, dir, "yarn.lock", "lockfile content")

	patternsJSON, _ := json.Marshal([]string{"node_modules/**", "*.lock"})

	var indexedPaths []string

	ms := &mocks.MockStore{
		AddDirectoryFn: func(path string) error { return nil },
		GetConfigFn: func(key string) (string, error) {
			if key == "ignore_patterns" {
				return string(patternsJSON), nil
			}
			return "", nil
		},
		ListDirectoriesFn: func() ([]domain.Directory, error) {
			return []domain.Directory{{ID: 1, Path: dir}}, nil
		},
		GetFileByPathFn: func(path string) (*domain.File, error) {
			return nil, nil
		},
		InsertChunksFn: func(fileID int64, chunks []domain.Chunk) error { return nil },
	}

	// Track indexed paths via UpsertFile, since IndexFile now uses ChunkText (no filePath arg).
	ms.UpsertFileFn = func(f domain.File) error {
		indexedPaths = append(indexedPaths, f.Path)
		ms.GetFileByPathFn = func(path string) (*domain.File, error) {
			return &domain.File{ID: 1, Path: path}, nil
		}
		return nil
	}

	mc := &mocks.MockChunker{
		ChunkTextFn: func(content string) ([]chunker.ChunkResult, error) {
			return []chunker.ChunkResult{{Content: content, Index: 0, TokenCount: 1}}, nil
		},
	}

	me := &mocks.MockEmbedder{
		EmbedFn: func(texts []string) ([][]float32, error) {
			vecs := make([][]float32, len(texts))
			for i := range texts {
				vecs[i] = []float32{0.1}
			}
			return vecs, nil
		},
	}

	mw := &mocks.MockWatcher{
		AddFn: func(dir string) error { return nil },
	}

	eng := New(ms, me, mc, mw, defaultMockExtractor())

	if err := eng.AddDirectory(dir); err != nil {
		t.Fatalf("AddDirectory: %v", err)
	}

	// Only main.go should have been indexed; node_modules/index.js and yarn.lock skipped.
	for _, p := range indexedPaths {
		if strings.Contains(p, "node_modules") {
			t.Errorf("node_modules file was indexed: %s", p)
		}
		if strings.HasSuffix(p, ".lock") {
			t.Errorf("*.lock file was indexed: %s", p)
		}
	}

	found := false
	for _, p := range indexedPaths {
		if strings.HasSuffix(p, "main.go") {
			found = true
		}
	}
	if !found {
		t.Error("main.go was not indexed but should have been")
	}
}

func TestOnCreateSkipsIgnoredFiles(t *testing.T) {
	dir := t.TempDir()
	// Create a real file so IndexFile can read it.
	lockFile := tempFileInDir(t, dir, "yarn.lock", "lockfile content")

	patternsJSON, _ := json.Marshal([]string{"*.lock"})

	chunkerCalled := false

	ms := &mocks.MockStore{
		GetConfigFn: func(key string) (string, error) {
			if key == "ignore_patterns" {
				return string(patternsJSON), nil
			}
			return "", nil
		},
		ListDirectoriesFn: func() ([]domain.Directory, error) {
			return []domain.Directory{{ID: 1, Path: dir}}, nil
		},
	}

	mc := &mocks.MockChunker{
		ChunkTextFn: func(content string) ([]chunker.ChunkResult, error) {
			chunkerCalled = true
			return nil, nil
		},
	}

	eng := New(ms, &mocks.MockEmbedder{}, mc, &mocks.MockWatcher{}, defaultMockExtractor())

	eng.OnCreate(lockFile)

	if chunkerCalled {
		t.Error("chunker was called for an ignored file")
	}
}

func TestReset(t *testing.T) {
	resetCalled := false
	stopCalled := false
	startCalled := false
	var resetDim int

	mw := &mocks.MockWatcher{
		StopFn: func() error {
			stopCalled = true
			return nil
		},
		StartFn: func(handler watcher.FileEventHandler) error {
			startCalled = true
			return nil
		},
	}

	ms := &mocks.MockStore{
		ResetFn: func(dim int) error {
			resetCalled = true
			resetDim = dim
			return nil
		},
	}

	me := &mocks.MockEmbedder{
		DimensionsFn: func() int { return 1536 },
	}

	eng := New(ms, me, &mocks.MockChunker{}, mw, defaultMockExtractor())

	if err := eng.Reset(); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	if !stopCalled {
		t.Error("watcher.Stop was not called")
	}
	if !resetCalled {
		t.Error("store.Reset was not called")
	}
	if !startCalled {
		t.Error("watcher.Start was not called after reset")
	}
	if resetDim != 1536 {
		t.Errorf("store.Reset dimension = %d, want 1536", resetDim)
	}
}
