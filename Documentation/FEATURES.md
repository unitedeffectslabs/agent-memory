# Features

## Core Pipeline

**Directory watching and indexing.** Add directories via the GUI or MCP. The engine walks each directory, filters by ignore patterns and supported file types, then runs the pipeline: extract text -> chunk -> embed (via the active provider — local by default) -> store vectors in SQLite. File content is hashed (SHA-256) so unchanged files are skipped on subsequent scans.

**Semantic search.** Query text is embedded, then matched against stored vectors using KNN (sqlite-vec cosine distance). Returns ranked results with file path, chunk text, and similarity score. Available via MCP `search` tool.

**Live file watching.** fsnotify monitors all watched directories. File creates and modifications trigger re-indexing (with ignore pattern and hash checks). File deletions remove the file and its chunks from the index. Events are debounced at 500ms to handle rapid saves.

**Cancellable indexing.** Clicking Stop (or calling the `stop` MCP tool) immediately halts any in-flight indexing. The engine checks a cancellation channel between each file in both initial scans and directory additions.

## File Extraction

The extractor handles three categories of files (see `internal/extractor/extractor.go` for the full extension lists):

- **Text files** — read directly. Covers documents (.md, .txt, .rst), code (.go, .py, .js, .ts, etc.), config (.json, .yaml, .toml), web (.html, .css), and more.
- **Binary documents** — structured text extraction. Supports .docx (via docx library), .xlsx (via excelize, all sheets), .pptx (zip/XML slide parsing), .pdf (passthrough).
- **Metadata-only** — images, archives, audio, video. Indexes file name, path, size, modified date, and format-specific metadata (image dimensions, zip contents).

## Chunking

Token-based splitting using tiktoken (cl100k_base encoding). Configurable chunk size (default 512 tokens) and overlap (default 50 tokens). Each chunk records its index and token count. The overlap creates sliding-window continuity between chunks.

## Embedding

Two providers, selectable in Settings. Both satisfy the same `Embedder` interface, so the rest of the pipeline is provider-agnostic.

**Local (default).** A bundled `multilingual-e5-small` model (384 dimensions, ~100 languages) runs in-process on the CPU via ONNX Runtime. Works fully offline — no API key, no network calls. The model, tokenizer, and runtime library are bundled into the app; onboarding requires no key.

**OpenAI (opt-in).** OpenAI API client supporting `text-embedding-3-small` (1536 dimensions) and `text-embedding-3-large` (3072 dimensions). Enabled by adding an API key in Settings. Batches up to 2048 inputs per API call. Retries with exponential backoff on rate limits (max 3 retries). The engine further batches by token count (250K tokens per batch) to stay within API limits.

**Provider & model switching.** Changing the provider or model via Settings drops the vector table, recreates it with the new dimension, and re-indexes everything (vectors from different models are incompatible). The embedder is hot-swapped so no restart is needed.

**API key changes.** Updating the OpenAI API key (while the OpenAI provider is active) immediately swaps the embedder so new requests use the updated key.

## Ignore Patterns

Glob patterns that skip files and directories during indexing. Stored as JSON in the config table. Supports `**` via the doublestar library. Checked during both initial scans and live watcher events.

Default patterns ship with the app (node_modules, .git, __pycache__, build artifacts, lock files, etc.). Full list in `internal/engine/engine.go` at `DefaultIgnorePatterns`.

Manageable via GUI Settings page or MCP tools (`get_ignore_patterns` / `set_ignore_patterns`).

## MCP Server

Two transports, shared dispatch logic (parameterized via `toolLister` and `toolHandler` in `internal/mcp/dispatch.go`):

**HTTP/SSE** (always-on in GUI mode) — binds to `127.0.0.1` on configurable port (default 9847). Bearer token auth. GET `/sse` opens an SSE stream, POST `/messages?sessionId=X` sends JSON-RPC requests. Responses pushed back over SSE. Full 11-tool set.

**Stdio** (`--mcp` flag) — for Claude Desktop. Reads JSON-RPC from stdin, writes responses to stdout. No auth needed (process-level isolation). Read-only 4-tool set. Uses `ReadOnlyEngine` (store + embedder only, no watcher/chunker/extractor).

Both implement the MCP protocol: `initialize` handshake, `tools/list`, and `tools/call`.

### MCP Tools

| Tool | Params | HTTP/SSE | Stdio | Description |
|------|--------|----------|-------|-------------|
| `search` | `query`, `limit`, `offset`, `threshold` | Yes | Yes | Semantic search across indexed content |
| `list_directories` | — | Yes | Yes | List watched directories with counts |
| `index_status` | — | Yes | Yes | Stats: files, chunks, indexing status, model |
| `get_ignore_patterns` | — | Yes | Yes | Current ignore glob list |
| `add_directory` | `path` | Yes | No | Add directory to watch, trigger indexing |
| `remove_directory` | `path` | Yes | No | Stop watching, remove indexed data |
| `start` | — | Yes | No | Start file watcher |
| `stop` | — | Yes | No | Stop watcher and cancel in-flight indexing |
| `restart` | — | Yes | No | Stop then start |
| `reset` | — | Yes | No | Clear all vectors, re-index from scratch |
| `set_ignore_patterns` | `patterns` | Yes | No | Replace ignore list |

## Desktop GUI

Wails v2 with React frontend. Native webview, no browser required.

**Onboarding** — first-launch flow: welcome, directory picker, then dashboard. No API key required — the local provider works out of the box. Opting into OpenAI is done later in Settings.

**Dashboard** — total files, chunks, last indexed time, indexing progress bar, embedding model display.

**Directories** — list watched directories with per-directory file/chunk counts. Add (native folder picker) and remove with confirmation.

**Controls** — Start/Stop/Restart/Reset buttons. Reset clears all vectors and re-indexes (with confirmation dialog).

**Settings** — embedding provider selector (local default vs OpenAI); when OpenAI is selected, an API key field (masked) and model picker appear. Plus chunk size/overlap, MCP port, and auth token display with rotate button. Switching provider re-indexes everything.

**Activity Log** — paginated log of indexing events (indexed, ignored, deleted, errors).

**System Tray** — macOS menu bar icon with Show/Hide and Quit options.

## Claude Desktop Integration

One-click install from the Settings page. The app writes its MCP server entry into Claude Desktop's config file (`~/Library/Application Support/Claude/claude_desktop_config.json`).

**Install flow:**
1. Click "Install to Claude Desktop" in Settings
2. The app resolves its own binary path, backs up the existing config file (`.backup`), and merges an `agent-memory` entry into `mcpServers`
3. Restart Claude Desktop to activate

The config path is customizable for non-standard installs. Claude Desktop launches the binary as a subprocess with `--mcp`, communicating over stdin/stdout. The subprocess opens the same SQLite DB in read-only mode — no watcher, no indexing, just search and status queries.

## Security

- HTTP server binds to `127.0.0.1` only — no network exposure.
- Bearer token auth on all MCP HTTP requests. Token auto-generated, stored in SQLite, rotatable via GUI.
- No outbound traffic by default — the local embedding provider runs in-process. Outbound HTTPS to `api.openai.com` occurs only if the OpenAI provider is opted into.
- GUI uses native webview IPC — no localhost web server for the UI.
- SQLite file uses standard filesystem permissions.

## Configuration

All settings are stored in the SQLite `config` table and manageable from the GUI Settings page.

**Database location:** `~/.agent-memory/agent-memory.db` by default. Override with `--db /path/to/file.db`.

**Auth token:** A random bearer token is generated on first launch. Required on all MCP HTTP/SSE requests. View and rotate it in Settings.

**MCP server port:** Default `9847`. Binds to `127.0.0.1` only.

**Embedding provider & model:** local `multilingual-e5-small` (384 dimensions) by default. Switch to the OpenAI provider in Settings to use `text-embedding-3-small` (1536 dimensions) or `text-embedding-3-large` (3072 dimensions), which require an API key. Changing the provider or model re-indexes everything.

**Chunk size:** 512 tokens default, configurable. Overlap: 50 tokens default, configurable.

**Ignore patterns:** Glob patterns using `**` syntax (via the doublestar library). Defaults seeded on first launch:

```
node_modules/**   .git/**   __pycache__/**   *.pyc   .DS_Store   Thumbs.db
build/**   dist/**   .next/**   .cache/**   *.lock   package-lock.json   vendor/**
```

Manageable via Settings or the `get_ignore_patterns` / `set_ignore_patterns` MCP tools.

## Activity Log

Every indexing event (file indexed, ignored, deleted, error) is recorded in the `activity_log` table with timestamp, path, action, and detail. Viewable in the GUI Log page with pagination. Useful for debugging watcher behavior.
