# Architecture

## Layered Design

Dependencies flow inward only. Outer layers call inner layers, never the reverse.

```
Delivery  ->  Service (Engine)  ->  Domain (interfaces)  ->  Infrastructure (implementations)
```

**Delivery** — translates external requests into engine calls. No business logic.
- `app.go` — Wails GUI adapter. Exposes methods the frontend calls via `window.go.main.App.*`.
- `tray.go` — macOS system tray (Objective-C via CGo).
- `internal/mcp/` — MCP HTTP/SSE server and stdio transport.

**Service** — `internal/engine/engine.go`. Orchestrates use cases by calling injected interfaces. Never implements infrastructure directly.

**Domain** — each package defines its own interface in `iface.go`. No cross-domain imports.

**Infrastructure** — concrete implementations that satisfy domain interfaces: SQLite, OpenAI API, fsnotify, file extractor.

## Composition Root

`main.go` is the only file that imports concrete implementations. It wires everything together and branches to GUI or MCP stdio mode based on the `--mcp` flag. No `init()` side effects, no global state (except `appInstance` for CGo tray callbacks).

## Domain Interfaces

Six interfaces, each in its own package:

| Interface | Package | Defined in | Implemented by |
|-----------|---------|------------|----------------|
| `Store` | `internal/store` | `iface.go` | `sqlite.go` (SQLite + sqlite-vec) |
| `Embedder` | `internal/embeddings` | `iface.go` | `openai.go` (OpenAI API) |
| `Chunker` | `internal/chunker` | `iface.go` | `chunker.go` (tiktoken, cl100k_base) |
| `Watcher` | `internal/watcher` | `iface.go` | `fswatcher.go` (fsnotify) |
| `FileEventHandler` | `internal/watcher` | `iface.go` | `engine.go` (Engine implements OnCreate/OnModify/OnDelete) |
| `Extractor` | `internal/extractor` | `iface.go` | `extractor.go` (text, docx, xlsx, pptx, pdf, metadata) |

The MCP layer defines its own interfaces in `server.go` so it never imports the engine package directly:
- `ReadOnlyEngineService` — search, list, stats, ignore patterns (used by stdio transport)
- `EngineService` — extends `ReadOnlyEngineService` with write operations (used by HTTP/SSE transport)

## Design Rules

1. **Interfaces at domain boundaries.** Each domain package defines its interface. Infrastructure satisfies it. This enables mock-based testing and implementation swapping.

2. **Engine orchestrates, never implements.** The engine coordinates (read -> extract -> chunk -> embed -> store) but delegates all work to injected dependencies.

3. **Delivery layer is thin.** GUI handlers and MCP handlers translate requests into engine calls. `app.go` is a pass-through adapter; the only logic it contains is the embedding model/API key swap flow.

4. **No cross-domain imports.** The chunker doesn't import the store. The embeddings client doesn't import the watcher.

5. **Configuration is injected, not global.** All dependencies wired in `main.go`, passed down explicitly.

6. **Mocks for all interfaces.** `internal/mocks/mocks.go` has mock implementations for every domain interface, used by engine tests.

## File Layout

```
main.go              Composition root (wiring only)
app.go               Wails GUI adapter (App struct, frontend-bound methods)
tray.go              System tray (macOS CGo)
internal/
  domain/types.go    Shared types: Directory, File, Chunk, SearchResult, IndexStats, ActivityLogEntry
  store/             Store interface + SQLite implementation
  embeddings/        Embedder interface + OpenAI implementation
  chunker/           Chunker interface + token-based splitter
  watcher/           Watcher + FileEventHandler interfaces + fsnotify implementation
  extractor/         Extractor interface + multi-format file extraction
  engine/            Service layer orchestrator (Engine + ReadOnlyEngine)
  mcp/               MCP transports (HTTP/SSE server, stdio, shared dispatch)
  mocks/             Mock implementations for all interfaces
frontend/            React 18 + Vite 5 (Wails webview frontend)
```

## Engine Lifecycle

`Start()` creates a cancellation channel, starts the watcher, then launches `initialScan()` as a background goroutine. `Stop()` closes the cancellation channel (halting any in-flight indexing) and stops the watcher. The indexing loops check `eng.stopped()` before each file.

## MCP Transport

Two transports share the same `dispatch()` function in `internal/mcp/dispatch.go`. The dispatch is parameterized with a `toolLister` and `toolHandler` so each transport can expose a different tool set.

**HTTP/SSE** (GUI mode) — full tool set (11 tools). Session-based SSE streaming (GET `/sse` establishes connection, POST `/messages?sessionId=X` sends requests). Bearer token auth. Binds to `127.0.0.1` only. Uses `EngineService` (full engine).

**Stdio** (`--mcp` flag) — read-only tool set (4 tools: search, list_directories, index_status, get_ignore_patterns). Claude Desktop launches the binary as a subprocess. Reads JSON-RPC from stdin, writes to stdout. Uses `ReadOnlyEngineService` backed by `ReadOnlyEngine` (store + embedder only, no watcher/chunker/extractor).

Both transports share the same JSON-RPC types, initialization handshake, and dispatch scaffolding.

## Two Runtime Modes

One binary, two modes controlled by the `--mcp` flag:

- **GUI mode** (default): Full Wails app with watcher, indexer, HTTP/SSE MCP server. Opens DB read-write.
- **Stdio mode** (`--mcp`): Lightweight read-only process for Claude Desktop. Opens DB read-only via `NewReadOnlySQLiteStore`. No watcher, no indexing.

Both share the same SQLite database. WAL mode supports concurrent readers with one writer. The GUI writes, the stdio process reads.

## Embedding Model Change Flow

When the user changes the embedding model in Settings:

1. `app.SetConfig("embedding_model", newModel)` detects the change
2. Persists the new model to the config table
3. Creates a new embedder via the injected `EmbedderFactory` (so `app.go` never imports concrete embeddings)
4. Calls `engine.SetEmbedder()` to swap it
5. Calls `engine.Reset()` which stops the watcher, drops and recreates the vector table with the new dimension, and restarts

The same factory pattern applies when the API key changes — the embedder is swapped so new requests use the updated key immediately.

## Testing

All domain and service packages have `_test.go` files. Tests use mocks from `internal/mocks/`. No test hits the real OpenAI API. Store tests use real SQLite in a temp directory. See `internal/engine/engine_test.go` for the canonical example of interface-based testing.

```bash
make test              # go test ./...
go test -v ./internal/engine/
go test -cover ./...
```
