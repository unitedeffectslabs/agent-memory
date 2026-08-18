package store

import (
	"database/sql"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/borzou/vecstore/internal/domain"
)

func newTestStore(t *testing.T) *SQLiteStore {
	t.Helper()
	dir := t.TempDir()
	s, err := NewSQLiteStore(filepath.Join(dir, "test.db"), 1536)
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

	// Provider/model are reported straight from config; empty when unset.
	if stats.Provider != "" || stats.EmbeddingModel != "" {
		t.Fatalf("want empty provider/model when unconfigured, got %q/%q", stats.Provider, stats.EmbeddingModel)
	}
	s.SetConfig("embedding_provider", "local")
	s.SetConfig("embedding_model", "multilingual-e5-small")
	stats, _ = s.Stats()
	if stats.Provider != "local" {
		t.Fatalf("want provider 'local', got %q", stats.Provider)
	}
	if stats.EmbeddingModel != "multilingual-e5-small" {
		t.Fatalf("want model 'multilingual-e5-small', got %q", stats.EmbeddingModel)
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

func TestNewSQLiteStoreDimension(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "dim.db")

	// Fresh DB created at 384 dims should accept a 384-wide embedding.
	s, err := NewSQLiteStore(dbPath, 384)
	if err != nil {
		t.Fatalf("NewSQLiteStore(384): %v", err)
	}
	s.AddDirectory("/tmp/a")
	dirs, _ := s.ListDirectories()
	f := domain.File{DirectoryID: dirs[0].ID, Path: "/tmp/a/f.txt", Hash: "h", IndexedAt: time.Now().UTC()}
	s.UpsertFile(f)
	got, _ := s.GetFileByPath("/tmp/a/f.txt")

	emb := make([]float32, 384)
	emb[0] = 1
	if err := s.InsertChunks(got.ID, []domain.Chunk{{Index: 0, Content: "x", TokenCount: 1, Embedding: emb}}); err != nil {
		t.Fatalf("InsertChunks(384): %v", err)
	}
	s.Close()

	// Reopening with a different dim must NOT change the existing table.
	s2, err := NewSQLiteStore(dbPath, 1536)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer s2.Close()
	stats, _ := s2.Stats()
	if stats.TotalChunks != 1 {
		t.Fatalf("existing 384-dim data should survive reopen, got %d chunks", stats.TotalChunks)
	}
	// A 384-wide query still matches — table width was preserved as 384.
	res, err := s2.Search(emb, 5, 0, 0)
	if err != nil {
		t.Fatalf("Search on preserved 384 table: %v", err)
	}
	if len(res) == 0 {
		t.Fatal("expected a result from the preserved 384-dim table")
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

// TestUpsertFileWithChunksAtomic pins the crash-safety contract: the
// remove-old-chunks / upsert-file / insert-chunks sequence is one transaction,
// so a failure partway through leaves the previous index entry fully intact —
// never the file recorded at the new hash with missing chunks (which the hash
// short-circuit would then skip forever).
func TestUpsertFileWithChunksAtomic(t *testing.T) {
	s := newTestStore(t)
	if err := s.AddDirectory("/tmp/a"); err != nil {
		t.Fatal(err)
	}
	dirs, _ := s.ListDirectories()

	emb := make([]float32, 1536)
	emb[0] = 0.5

	// Index v1 successfully.
	v1 := domain.File{DirectoryID: dirs[0].ID, Path: "/tmp/a/doc.txt", Hash: "hash-v1", IndexedAt: time.Now().UTC()}
	if err := s.UpsertFileWithChunks(v1, []domain.Chunk{
		{Index: 0, Content: "v1 chunk zero", TokenCount: 3, Embedding: emb},
		{Index: 1, Content: "v1 chunk one", TokenCount: 3, Embedding: emb},
	}); err != nil {
		t.Fatalf("v1 index: %v", err)
	}
	stored, _ := s.GetFileByPath("/tmp/a/doc.txt")
	if stored == nil || stored.Hash != "hash-v1" {
		t.Fatalf("v1 not stored correctly: %+v", stored)
	}

	// Attempt v2 with a chunk whose embedding has the WRONG dimension — the
	// vec0 insert fails mid-transaction. Everything must roll back.
	v2 := domain.File{ID: stored.ID, DirectoryID: dirs[0].ID, Path: "/tmp/a/doc.txt", Hash: "hash-v2", IndexedAt: time.Now().UTC()}
	badEmb := []float32{1, 2, 3} // store was created with dim 1536
	if err := s.UpsertFileWithChunks(v2, []domain.Chunk{
		{Index: 0, Content: "v2 chunk zero", TokenCount: 3, Embedding: emb},
		{Index: 1, Content: "v2 chunk one", TokenCount: 3, Embedding: badEmb},
	}); err == nil {
		t.Fatal("expected wrong-dimension embedding to fail the transaction")
	}

	// The file must still be recorded at hash-v1 with BOTH v1 chunks intact.
	after, _ := s.GetFileByPath("/tmp/a/doc.txt")
	if after == nil {
		t.Fatal("file row vanished after failed update")
	}
	if after.Hash != "hash-v1" {
		t.Fatalf("hash = %q after failed update, want hash-v1 (partial write leaked!)", after.Hash)
	}
	results, err := s.Search(emb, 10, 0, 0)
	if err != nil {
		t.Fatalf("search after rollback: %v", err)
	}
	v1Chunks := 0
	for _, r := range results {
		if r.FilePath == "/tmp/a/doc.txt" && strings.HasPrefix(r.Content, "v1 ") {
			v1Chunks++
		}
	}
	if v1Chunks != 2 {
		t.Fatalf("searchable v1 chunks after rollback = %d, want 2", v1Chunks)
	}

	// A good v2 then replaces v1 completely.
	if err := s.UpsertFileWithChunks(v2, []domain.Chunk{
		{Index: 0, Content: "v2 only chunk", TokenCount: 3, Embedding: emb},
	}); err != nil {
		t.Fatalf("good v2 index: %v", err)
	}
	final, _ := s.GetFileByPath("/tmp/a/doc.txt")
	if final.Hash != "hash-v2" {
		t.Fatalf("hash = %q, want hash-v2", final.Hash)
	}
	results, _ = s.Search(emb, 10, 0, 0)
	for _, r := range results {
		if r.FilePath == "/tmp/a/doc.txt" && strings.HasPrefix(r.Content, "v1 ") {
			t.Fatalf("stale v1 chunk still searchable after replace: %q", r.Content)
		}
	}
}

// TestUpsertLogEntryDedupes pins the one-living-row-per-(path,action)
// contract: repeat failures update the existing row instead of appending.
func TestUpsertLogEntryDedupes(t *testing.T) {
	s := newTestStore(t)
	e := domain.ActivityLogEntry{Timestamp: time.Now(), Path: "/a/b.pdf", Action: "error", Detail: "index: boom v1"}
	if err := s.UpsertLogEntry(e); err != nil {
		t.Fatal(err)
	}
	e.Detail = "index: boom v2"
	e.Timestamp = time.Now().Add(time.Minute)
	if err := s.UpsertLogEntry(e); err != nil {
		t.Fatal(err)
	}
	entries, total, err := s.ListLogEntries(10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(entries) != 1 {
		t.Fatalf("total = %d entries = %d, want exactly 1 row", total, len(entries))
	}
	if entries[0].Detail != "index: boom v2" {
		t.Fatalf("detail = %q, want the updated v2 detail", entries[0].Detail)
	}
	// A different path appends normally.
	e2 := domain.ActivityLogEntry{Timestamp: time.Now(), Path: "/a/c.pdf", Action: "error", Detail: "index: other"}
	if err := s.UpsertLogEntry(e2); err != nil {
		t.Fatal(err)
	}
	if _, total, _ = s.ListLogEntries(10, 0); total != 2 {
		t.Fatalf("total = %d, want 2 after a second distinct path", total)
	}
}

// TestActivityLogRetention pins the TTL prune at store open.
func TestActivityLogRetention(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "retention.db")
	s, err := NewSQLiteStore(dbPath, 8)
	if err != nil {
		t.Fatal(err)
	}
	old := domain.ActivityLogEntry{Timestamp: time.Now().AddDate(0, 0, -60), Path: "/old.txt", Action: "indexed", Detail: "1 chunks"}
	fresh := domain.ActivityLogEntry{Timestamp: time.Now(), Path: "/fresh.txt", Action: "indexed", Detail: "1 chunks"}
	if err := s.InsertLogEntry(old); err != nil {
		t.Fatal(err)
	}
	if err := s.InsertLogEntry(fresh); err != nil {
		t.Fatal(err)
	}
	s.Close()

	// Re-open: the 60-day-old row must be pruned, the fresh one kept.
	s2, err := NewSQLiteStore(dbPath, 8)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	entries, total, err := s2.ListLogEntries(10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || entries[0].Path != "/fresh.txt" {
		t.Fatalf("after reopen: total = %d first = %+v, want only /fresh.txt", total, entries)
	}
}

// TestSearchEmptyResultsMarshalsToArray pins the wire contract: an empty
// result set must serialize as [] (not null) — strict JSON clients call
// .length on it. Covers both the no-matches and offset-past-results branches.
func TestSearchEmptyResultsMarshalsToArray(t *testing.T) {
	s := newTestStore(t)
	q := make([]float32, 1536)
	q[0] = 1

	for name, fn := range map[string]func() ([]domain.SearchResult, error){
		"no matches":          func() ([]domain.SearchResult, error) { return s.Search(q, 5, 0, 0) },
		"offset past results": func() ([]domain.SearchResult, error) { return s.Search(q, 5, 100, 0) },
	} {
		results, err := fn()
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if results == nil {
			t.Fatalf("%s: results is nil, must be an empty slice", name)
		}
		b, err := json.Marshal(results)
		if err != nil {
			t.Fatal(err)
		}
		if string(b) != "[]" {
			t.Fatalf("%s: marshals to %s, want []", name, b)
		}
	}
}

// TestExtractorVersionMigrationAndRoundtrip: DBs created before the
// extractor_version column existed must gain it on open (additive ALTER), old
// rows read as 0, and the field round-trips through upsert/get.
func TestExtractorVersionMigrationAndRoundtrip(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "legacy.db")

	// Build a legacy DB by hand: the pre-column files schema + one row.
	legacy, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	stmts := []string{
		`CREATE TABLE directories (id INTEGER PRIMARY KEY AUTOINCREMENT, path TEXT UNIQUE NOT NULL, file_count INTEGER NOT NULL DEFAULT 0, chunk_count INTEGER NOT NULL DEFAULT 0, status TEXT NOT NULL DEFAULT 'watching', added_at DATETIME NOT NULL)`,
		`CREATE TABLE files (id INTEGER PRIMARY KEY AUTOINCREMENT, directory_id INTEGER NOT NULL, path TEXT UNIQUE NOT NULL, hash TEXT NOT NULL, indexed_at DATETIME NOT NULL)`,
		`INSERT INTO directories(path, added_at) VALUES('/tmp/a', '2026-01-01')`,
		`INSERT INTO files(directory_id, path, hash, indexed_at) VALUES(1, '/tmp/a/old.pdf', 'oldhash', '2026-01-01')`,
	}
	for _, s := range stmts {
		if _, err := legacy.Exec(s); err != nil {
			t.Fatalf("legacy setup %q: %v", s[:30], err)
		}
	}
	legacy.Close()

	// Opening through the store must ALTER the table in place.
	s, err := NewSQLiteStore(dbPath, 8)
	if err != nil {
		t.Fatalf("open legacy DB: %v", err)
	}
	defer s.Close()

	got, err := s.GetFileByPath("/tmp/a/old.pdf")
	if err != nil {
		t.Fatalf("get legacy row: %v", err)
	}
	if got == nil || got.ExtractorVersion != 0 {
		t.Fatalf("legacy row extractor_version = %+v, want 0", got)
	}

	// Round-trip a bumped version.
	got.ExtractorVersion = 1
	got.Hash = "newhash"
	if err := s.UpsertFileWithChunks(*got, nil); err != nil {
		t.Fatalf("upsert with version: %v", err)
	}
	after, _ := s.GetFileByPath("/tmp/a/old.pdf")
	if after.ExtractorVersion != 1 {
		t.Fatalf("extractor_version after upsert = %d, want 1", after.ExtractorVersion)
	}
}
