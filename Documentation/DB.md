# Database

Agent Memory uses SQLite with the sqlite-vec extension for vector storage. Single file at `~/.agent-memory/agent-memory.db` (override with `--db`).

## Connection Settings

- **WAL mode** — concurrent reads, single writer, crash-safe.
- **MaxOpenConns = 1** — serializes all writes to avoid BUSY errors from concurrent goroutines (initial scan + UI polls + watcher events).
- **Foreign keys enabled** — cascade deletes from directories -> files -> chunks.
- **Busy timeout = 5000ms** — waits up to 5s for locks before failing.

Set in `internal/store/sqlite.go` at `NewSQLiteStore()`.

## Schema

### config

Key-value store for application settings.

| Column | Type | Description |
|--------|------|-------------|
| `key` | TEXT PK | Setting name |
| `value` | TEXT | Setting value |

**Known keys:** `openai_api_key`, `embedding_model`, `chunk_size`, `chunk_overlap`, `auth_token`, `mcp_port`, `ignore_patterns` (JSON array), `onboarding_complete`.

### directories

Watched directory registrations.

| Column | Type | Description |
|--------|------|-------------|
| `id` | INTEGER PK | Auto-increment |
| `path` | TEXT UNIQUE | Absolute directory path |
| `file_count` | INTEGER | Legacy, unused (counts computed from joins) |
| `chunk_count` | INTEGER | Legacy, unused (counts computed from joins) |
| `status` | TEXT | "watching", "indexing", "error", "stopped" |
| `added_at` | DATETIME | When the directory was registered |

File and chunk counts displayed in the UI are computed live via `ListDirectories()` which joins against `files` and `chunks` tables.

### files

Indexed file records. Foreign key to `directories` with cascade delete.

| Column | Type | Description |
|--------|------|-------------|
| `id` | INTEGER PK | Auto-increment |
| `directory_id` | INTEGER FK | -> directories(id) ON DELETE CASCADE |
| `path` | TEXT UNIQUE | Absolute file path |
| `hash` | TEXT | SHA-256 hex of file content (change detection) |
| `indexed_at` | DATETIME | Last successful index time |

### chunks

Text chunks extracted from files. Foreign key to `files` with cascade delete.

| Column | Type | Description |
|--------|------|-------------|
| `id` | INTEGER PK | Auto-increment |
| `file_id` | INTEGER FK | -> files(id) ON DELETE CASCADE |
| `chunk_index` | INTEGER | Position within the file (0-based) |
| `content` | TEXT | Chunk text |
| `token_count` | INTEGER | Token count for this chunk |

### chunk_embeddings

sqlite-vec virtual table for vector similarity search.

| Column | Type | Description |
|--------|------|-------------|
| `chunk_id` | INTEGER PK | Matches chunks(id) |
| `embedding` | FLOAT[N] | Vector, N = model dimension (1536 or 3072) |

Created with `USING vec0`. The dimension is set at table creation time and must match the embedding model. When the model changes, this table is dropped and recreated with the new dimension via `Reset()`.

**Note:** sqlite-vec virtual tables don't support foreign keys. Chunk embedding cleanup is handled manually in `RemoveChunksByFile()` — it queries chunk IDs, deletes from `chunk_embeddings`, then deletes from `chunks`.

### activity_log

Event log for indexing operations.

| Column | Type | Description |
|--------|------|-------------|
| `id` | INTEGER PK | Auto-increment |
| `timestamp` | DATETIME | Event time |
| `path` | TEXT | File path |
| `action` | TEXT | "indexed", "ignored", "deleted", "error" |
| `detail` | TEXT | Additional context (e.g., "3 chunks", error message) |

Indexed on `timestamp DESC` for efficient pagination.

## Vector Search

KNN search uses sqlite-vec's `MATCH` operator:

```sql
SELECT c.chunk_index, c.content, f.path, cev.distance
FROM chunk_embeddings cev
JOIN chunks c ON c.id = cev.chunk_id
JOIN files  f ON f.id = c.file_id
WHERE cev.embedding MATCH ?
  AND K = ?
ORDER BY cev.distance
```

The query embedding is passed as a little-endian float32 blob. `K` sets the number of nearest neighbors. Results are ordered by cosine distance (lower = more similar).

## Reset Flow

`store.Reset(embeddingDimension)`:

1. Deletes all rows from `chunk_embeddings`
2. Drops the `chunk_embeddings` virtual table
3. Deletes all rows from `chunks` and `files`
4. Clears `activity_log`
5. Recreates `chunk_embeddings` with `FLOAT[embeddingDimension]`

Directories and config are preserved — the user doesn't need to re-add directories or re-enter settings.

## Store Interface

The full interface is defined in `internal/store/iface.go`. The engine and MCP layer depend on this interface, never on `SQLiteStore` directly.

## Blob Encoding

Embeddings are stored as raw little-endian float32 byte arrays. Conversion in `float32SliceToBlob()` in `sqlite.go`. Each float32 occupies 4 bytes, so a 1536-dimension vector is 6144 bytes.
