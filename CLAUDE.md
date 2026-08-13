# Agent Memory

Local-first desktop app + MCP server for semantic file search. Watches directories, embeds file contents (a bundled local model by default; OpenAI opt-in), stores vectors in SQLite (sqlite-vec), and provides KNN semantic search.

## Build & Run

```bash
make assets         # download pinned local model/tokenizer/ORT libs into assets/embedded/ (~150 MB, checksummed via assets/manifest.json)
make build          # wails build -skipbindings, builds with -tags localembed (DO NOT use plain `wails build` — hangs on binding generation due to CGo)
make build-darwin-amd64  # cross-build the Intel-mac app from an arm64 Mac (swaps in darwin-amd64 assets)
make dev            # hot-reload dev mode
make test           # go test ./... (default build, no localembed tag — stays native-lib-free)
make clean          # rm -rf build/bin
```

> `make build` depends on `make assets`, which downloads ~150 MB of native artifacts (model,
> tokenizer, ONNX Runtime lib) the first time — needs network access. The build sets
> `-tags localembed` plus `CGO_LDFLAGS` to link the tokenizer library; the resulting binary is
> ~180 MB (it bundles the model). The native code is build-tag-isolated, so plain
> `go test ./...` and CI need no assets.

> **macOS 26+ gotcha:** Go ≤ 1.24 produces CGo binaries the kernel kills instantly on launch
> (exit 137, `dyld: missing LC_UUID`) — this breaks `make build`, the `wails` CLI, and the built
> app. Fix: `go env -w GOTOOLCHAIN=go1.26.4`, then `go install github.com/wailsapp/wails/v2/cmd/wails@v2.11.0`.

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
| `internal/embeddings` | Embedder interface + OpenAI client (opt-in) with batching (2048/req) and retry |
| `internal/embeddings/local` | Bundled in-process local embedder (default): ONNX Runtime + HF tokenizer, `multilingual-e5-small` (384-dim), offline |
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

## Active Work

Planned/in-progress work is defined in epics under `Documentation/Epics/` — each has a `Status:`
field and, if executable, an "Execution Notes" section addressed to the implementing session.
Read the epic's Execution Notes before starting; its stated user constraints are binding.
Current: [local-embeddings](Documentation/Epics/local-embeddings.md) (local model replaces
OpenAI as the default embedding provider).

## Constraints

- SQLite: `MaxOpenConns=1`, WAL mode — single writer, safe concurrent reads
- Tray integration (`tray.go`) is macOS-only (Objective-C via CGo)
- DB default path: `~/.agent-memory/agent-memory.db`
- MCP HTTP binds to `127.0.0.1` only, requires bearer token auth
