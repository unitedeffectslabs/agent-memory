# Roadmap

## Search Configuration UI

Currently, search parameters (limit, offset, threshold) are controlled per-query by the LLM via MCP tool arguments. Defaults are applied in `engine.go`:

- **limit**: 10
- **offset**: 0
- **threshold**: 1.5 (cosine distance; lower = stricter matching)

**Planned**: A Settings screen where users can configure default values for these parameters. The engine would read defaults from the config table, and MCP callers could still override per-query.

### Distance Metric Selection

sqlite-vec v0.1.6 uses cosine distance by default with `vec0` tables. The distance metric is a table-level property — switching to euclidean distance requires dropping and recreating the `chunk_embeddings` table, which triggers a full re-index.

**Planned**: A setting to choose between cosine and euclidean distance. Changing it would trigger `engine.Reset()` (same flow as changing the embedding model). This should be clearly labeled as a destructive operation in the UI.

## Ignore Patterns UI

Backend methods exist (`GetIgnorePatterns` / `SetIgnorePatterns`). Frontend not built yet.

**Planned**: A Settings panel to list current patterns, add new ones, remove individual patterns, and restore defaults.

## Unit Tests for Watcher File Operations

All six file operations (create, update, delete, rename, move, new directory) were tested live and passed. Formal unit tests still needed. See `internal/watcher/fswatcher_test.go`.

## MCP Transport Verification

HTTP/SSE transport verified via curl. Stdio transport implemented and tested — Claude Desktop connects via `--mcp` flag (subprocess, stdin/stdout). One-click install button added to Settings. Custom Connector UI (HTTPS) was explored but rejected due to self-signed cert restrictions; stdio transport is the production solution.
