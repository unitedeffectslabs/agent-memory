package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadOnlyStore_OpensInitializedDB(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	// Create and initialize with the read-write constructor.
	rw, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	rw.Close()

	// Open read-only — should succeed.
	ro, err := NewReadOnlySQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewReadOnlySQLiteStore: %v", err)
	}
	defer ro.Close()

	// Read operations should work.
	dirs, err := ro.ListDirectories()
	if err != nil {
		t.Fatalf("ListDirectories: %v", err)
	}
	if len(dirs) != 0 {
		t.Errorf("expected 0 directories, got %d", len(dirs))
	}

	_, err = ro.Stats()
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}

	_, err = ro.GetConfig("openai_api_key")
	if err != nil {
		t.Fatalf("GetConfig: %v", err)
	}
}

func TestReadOnlyStore_FailsOnMissingDB(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "nonexistent.db")

	_, err := NewReadOnlySQLiteStore(dbPath)
	if err == nil {
		t.Fatal("expected error for missing DB, got nil")
	}
}

func TestReadOnlyStore_FailsOnEmptyDB(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "empty.db")

	// Create an empty file.
	f, err := os.Create(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	_, err = NewReadOnlySQLiteStore(dbPath)
	if err == nil {
		t.Fatal("expected error for empty DB, got nil")
	}
}
