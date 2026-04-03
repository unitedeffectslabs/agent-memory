package store

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/borzou/vecstore/internal/domain"
)

func newTestStore(t *testing.T) *SQLiteStore {
	t.Helper()
	dir := t.TempDir()
	s, err := NewSQLiteStore(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestConfigRoundTrip(t *testing.T) {
	s := newTestStore(t)

	if err := s.SetConfig("key1", "value1"); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetConfig("key1")
	if err != nil {
		t.Fatal(err)
	}
	if got != "value1" {
		t.Fatalf("want %q got %q", "value1", got)
	}

	// Overwrite
	if err := s.SetConfig("key1", "value2"); err != nil {
		t.Fatal(err)
	}
	got, err = s.GetConfig("key1")
	if err != nil {
		t.Fatal(err)
	}
	if got != "value2" {
		t.Fatalf("want %q got %q", "value2", got)
	}

	// Missing key
	got, err = s.GetConfig("missing")
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Fatalf("expected empty string for missing key, got %q", got)
	}
}

func TestDirectories(t *testing.T) {
	s := newTestStore(t)

	if err := s.AddDirectory("/tmp/a"); err != nil {
		t.Fatal(err)
	}
	if err := s.AddDirectory("/tmp/b"); err != nil {
		t.Fatal(err)
	}

	dirs, err := s.ListDirectories()
	if err != nil {
		t.Fatal(err)
	}
	if len(dirs) != 2 {
		t.Fatalf("want 2 dirs, got %d", len(dirs))
	}

	if err := s.RemoveDirectory("/tmp/a"); err != nil {
		t.Fatal(err)
	}
	dirs, err = s.ListDirectories()
	if err != nil {
		t.Fatal(err)
	}
	if len(dirs) != 1 {
		t.Fatalf("want 1 dir, got %d", len(dirs))
	}
	if dirs[0].Path != "/tmp/b" {
		t.Fatalf("want /tmp/b, got %s", dirs[0].Path)
	}
}

func TestUpsertFileGetByPath(t *testing.T) {
	s := newTestStore(t)
	if err := s.AddDirectory("/tmp/a"); err != nil {
		t.Fatal(err)
	}
	dirs, _ := s.ListDirectories()

	f := domain.File{
		DirectoryID: dirs[0].ID,
		Path:        "/tmp/a/file.txt",
		Hash:        "abc123",
		IndexedAt:   time.Now().UTC().Truncate(time.Second),
	}
	if err := s.UpsertFile(f); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetFileByPath("/tmp/a/file.txt")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("expected file, got nil")
	}
	if got.Hash != "abc123" {
		t.Fatalf("want hash abc123, got %s", got.Hash)
	}

	// Update hash
	f.Hash = "def456"
	if err := s.UpsertFile(f); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GetFileByPath("/tmp/a/file.txt")
	if got.Hash != "def456" {
		t.Fatalf("want def456 after upsert, got %s", got.Hash)
	}
}

func TestRemoveFile(t *testing.T) {
	s := newTestStore(t)
	if err := s.AddDirectory("/tmp/a"); err != nil {
		t.Fatal(err)
	}
	dirs, _ := s.ListDirectories()
	f := domain.File{
		DirectoryID: dirs[0].ID,
		Path:        "/tmp/a/file.txt",
		Hash:        "abc",
		IndexedAt:   time.Now().UTC(),
	}
	s.UpsertFile(f)

	if err := s.RemoveFile("/tmp/a/file.txt"); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetFileByPath("/tmp/a/file.txt")
	if got != nil {
		t.Fatal("expected nil after remove")
	}
}

func TestInsertChunksAndRemove(t *testing.T) {
	s := newTestStore(t)
	if err := s.AddDirectory("/tmp/a"); err != nil {
		t.Fatal(err)
	}
	dirs, _ := s.ListDirectories()
	f := domain.File{
		DirectoryID: dirs[0].ID,
		Path:        "/tmp/a/file.txt",
		Hash:        "abc",
		IndexedAt:   time.Now().UTC(),
	}
	s.UpsertFile(f)
	got, _ := s.GetFileByPath("/tmp/a/file.txt")

	emb := make([]float32, 1536)
	emb[0] = 0.1
	chunks := []domain.Chunk{
		{Index: 0, Content: "hello world", TokenCount: 2, Embedding: emb},
		{Index: 1, Content: "foo bar", TokenCount: 2, Embedding: emb},
	}
	if err := s.InsertChunks(got.ID, chunks); err != nil {
		t.Fatal(err)
	}

	stats, err := s.Stats()
	if err != nil {
		t.Fatal(err)
	}
	if stats.TotalChunks != 2 {
		t.Fatalf("want 2 chunks, got %d", stats.TotalChunks)
	}

	if err := s.RemoveChunksByFile(got.ID); err != nil {
		t.Fatal(err)
	}
	stats, _ = s.Stats()
	if stats.TotalChunks != 0 {
		t.Fatalf("want 0 chunks after remove, got %d", stats.TotalChunks)
	}
}

func TestStats(t *testing.T) {
	s := newTestStore(t)

	stats, err := s.Stats()
	if err != nil {
		t.Fatal(err)
	}
	if stats.TotalFiles != 0 || stats.TotalChunks != 0 {
		t.Fatalf("empty store should have 0 files/chunks")
	}

	s.AddDirectory("/tmp/a")
	dirs, _ := s.ListDirectories()
	f := domain.File{
		DirectoryID: dirs[0].ID,
		Path:        "/tmp/a/file.txt",
		Hash:        "abc",
		IndexedAt:   time.Now().UTC(),
	}
	s.UpsertFile(f)
	got, _ := s.GetFileByPath("/tmp/a/file.txt")

	emb := make([]float32, 1536)
	s.InsertChunks(got.ID, []domain.Chunk{
		{Index: 0, Content: "hello", TokenCount: 1, Embedding: emb},
	})

	stats, _ = s.Stats()
	if stats.TotalFiles != 1 {
		t.Fatalf("want 1 file, got %d", stats.TotalFiles)
	}
	if stats.TotalChunks != 1 {
		t.Fatalf("want 1 chunk, got %d", stats.TotalChunks)
	}
}

func TestReset(t *testing.T) {
	s := newTestStore(t)
	s.AddDirectory("/tmp/a")
	s.SetConfig("k", "v")

	if err := s.Reset(1536); err != nil {
		t.Fatal(err)
	}

	// Reset preserves directories and config — only clears indexed data
	dirs, _ := s.ListDirectories()
	if len(dirs) != 1 {
		t.Fatalf("want 1 dir after reset (preserved), got %d", len(dirs))
	}

	val, _ := s.GetConfig("k")
	if val != "v" {
		t.Fatalf("config should be preserved after reset, got %q", val)
	}
}

func TestSearch(t *testing.T) {
	s := newTestStore(t)
	s.AddDirectory("/tmp/a")
	dirs, _ := s.ListDirectories()
	f := domain.File{
		DirectoryID: dirs[0].ID,
		Path:        "/tmp/a/file.txt",
		Hash:        "abc",
		IndexedAt:   time.Now().UTC(),
	}
	s.UpsertFile(f)
	got, _ := s.GetFileByPath("/tmp/a/file.txt")

	// Insert a chunk with a known embedding
	emb := make([]float32, 1536)
	for i := range emb {
		emb[i] = 0.01
	}
	emb[0] = 1.0

	if err := s.InsertChunks(got.ID, []domain.Chunk{
		{Index: 0, Content: "hello world", TokenCount: 2, Embedding: emb},
	}); err != nil {
		t.Fatal(err)
	}

	// Search with the same embedding
	results, err := s.Search(emb, 5, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least one search result")
	}
	if results[0].Content != "hello world" {
		t.Fatalf("unexpected content: %q", results[0].Content)
	}
}
