package store

import (
	"database/sql"
	"fmt"

	sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
	_ "github.com/mattn/go-sqlite3"
)

// NewReadOnlySQLiteStore opens an existing SQLite database in read-only mode.
// It skips schema migrations — the GUI must have initialized the DB first.
// Returns a clear error if the DB does not exist or the schema is missing.
func NewReadOnlySQLiteStore(dbPath string) (*SQLiteStore, error) {
	sqlite_vec.Auto()
	db, err := sql.Open("sqlite3", dbPath+"?mode=ro&_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open sqlite (read-only): %w", err)
	}

	db.SetMaxOpenConns(1)

	// Verify the schema exists by probing a core table.
	if _, err := db.Exec("SELECT 1 FROM files LIMIT 1"); err != nil {
		db.Close()
		return nil, fmt.Errorf("database not initialized — launch the GUI app first")
	}

	return &SQLiteStore{db: db}, nil
}
