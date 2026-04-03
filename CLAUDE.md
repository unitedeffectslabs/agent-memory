# Agent Memory

Local-first desktop app + MCP server for semantic file search. Watches directories, embeds file contents via OpenAI, stores vectors in SQLite (sqlite-vec), and provides KNN semantic search.

## Build & Run

```bash
make build          # wails build -skipbindings (DO NOT use plain `wails build` — hangs on binding generation due to CGo)
make dev            # hot-reload dev mode
make test           # go test ./...
make clean          # rm -rf build/bin
```

Run modes:
- `./agent-memory` — GUI mode (default)
- `./agent-memory --mcp` — MCP stdio mode (for Claude Desktop)
- `./agent-memory --db /path/to/db.db` — custom database path

Frontend: `cd frontend && npm install` (React 18 + Vite 5, required before first build)

## Architecture

Layered design with inward-only dependency flow:

```
Delivery (main.go, mcp/, tray.go, frontend/)
  → Service (engine/)
    → Domain interfaces (store/iface.go, embeddings/iface.go, chunker/iface.go, watcher/iface.go)
      → Infrastructure (store/sqlite.go, embeddings/openai.go, chunker/chunker.go, watcher/fswatcher.go)
```

- All wiring in `main.go` — no global state, no `init()` side effects
- Engine depends only on interfaces, never concrete implementations
- Domain types live in `internal/domain/types.go`
- Mocks for all interfaces in `internal/mocks/mocks.go`

## Key Packages

| Package | Purpose |
|---------|---------|
| `internal/engine` | Core orchestrator: scan → extract → chunk → embed → store |
| `internal/store` | SQLite + sqlite-vec persistence (config, directories, files, chunks, vectors) |
| `internal/embeddings` | OpenAI embedding client with batching (2048/req) and retry |
| `internal/chunker` | Token-based text splitting (tiktoken, cl100k_base) |
| `internal/watcher` | fsnotify recursive directory watcher with 500ms debounce |
| `internal/extractor` | Multi-format content extraction (text, docx, xlsx, pptx, pdf, images) |
| `internal/mcp` | MCP HTTP/SSE server (localhost:9847) + stdio transport |

## Testing

Tests use mocks from `internal/mocks/`. Each domain package has its own `_test.go` file.

```bash
go test ./...                      # all tests
go test -v ./internal/engine/      # verbose, specific package
go test -cover ./...               # with coverage
```

## Constraints

- SQLite: `MaxOpenConns=1`, WAL mode — single writer, safe concurrent reads
- Tray integration (`tray.go`) is macOS-only (Objective-C via CGo)
- DB default path: `~/.agent-memory/agent-memory.db`
- MCP HTTP binds to `127.0.0.1` only, requires bearer token auth
