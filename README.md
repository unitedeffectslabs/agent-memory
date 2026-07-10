# Agent Memory

A desktop app that makes your local files searchable by meaning, not just keywords. Point it at your folders, and it indexes everything — documents, code, notes, spreadsheets — into a local SQLite vector database. Then ask questions in natural language and get relevant results ranked by semantic similarity.

Works as a standalone file tracker/indexer and as an MCP server that gives Claude Desktop (or any MCP client) direct access to semantic search across your files.

## How It Works

1. **Launch the app.** It works out of the box with a bundled local embedding model — no API key, no account, no network calls. Onboarding is just: welcome → add folders → done. (You *can* opt into OpenAI embeddings later in Settings if you prefer; see below.)
2. **Add directories** you want indexed. The app watches them in real time — new files, edits, and deletions are picked up automatically.
3. **Search your files**, connect Claude Desktop so it can search your files during conversations.

### Connecting to Claude Desktop

Open Settings in the app and click **"Install to Claude Desktop"**. Restart Claude Desktop. That's it — Claude can now search your indexed files using the `search` tool.

Under the hood, Claude Desktop launches the app as a subprocess (`--mcp` flag) that communicates over stdin/stdout. The subprocess opens the same database in read-only mode. Your GUI app handles all indexing; Claude just reads.

### What Gets Indexed

- **Text files** — markdown, code, config, plain text (read directly)
- **Office documents** — .docx, .xlsx, .pptx (structured text extraction)
- **PDFs** — text content extraction
- **Media & archives** — metadata only (file name, size, dimensions, contents list)

Files are chunked into ~512-token segments (configurable), embedded by the active provider (local by default), and stored as vectors. Unchanged files (matched by SHA-256 hash) are skipped on re-scan.

### Supported Embedding Models

**Local (default, opt-out):**

- `multilingual-e5-small` (384 dimensions) — bundled into the app, runs in-process on the CPU, ~100 languages. Works fully offline: no API key, no account, no network calls.

**OpenAI (opt-in):** enable in Settings by adding an API key. Higher retrieval quality, at a per-request cost; sends text to `api.openai.com`.

- `text-embedding-3-small` (1536 dimensions) — faster, cheaper
- `text-embedding-3-large` (3072 dimensions) — higher quality, more expensive

Switching provider or model in Settings re-indexes everything (vectors from different models are incompatible).

### Privacy & Security

- **No outbound network calls by default** — the local embedding model runs entirely on your machine, so with the default provider nothing ever leaves your computer.
- All data stays local in `~/.agent-memory/agent-memory.db`
- The only outbound network call is to `api.openai.com` for embeddings — and only if you opt into the OpenAI provider in Settings.
- The MCP HTTP server binds to `127.0.0.1` only (never exposed to the network)
- The stdio transport (Claude Desktop) uses process-level isolation — no network at all

---

## Building from Source

### Prerequisites

- Go 1.24+
- Wails v2 CLI (`go install github.com/wailsapp/wails/v2/cmd/wails@latest`)
- Node.js 18+ and npm (for the frontend build)
- Network access on the first build — `make build` runs `make assets`, which downloads the pinned local model, tokenizer, and ONNX Runtime library (~150 MB, checksummed against `assets/manifest.json`) into the gitignored `assets/embedded/`. Cached after the first run.
- OpenAI API key — optional, only needed if you opt into the OpenAI provider at runtime
- macOS, Linux, or Windows
- CGo enabled (required for SQLite + sqlite-vec, and for the local embedder's native libraries; default on macOS/Linux, may need `CC=gcc` on Windows)

### Local Development

```bash
git clone <repo-url>
cd agent-memory
go mod download
cd frontend && npm install && cd ..
make dev        # or: wails dev
```

- Hot-reloads frontend changes
- Go backend rebuilds on save
- SQLite database created at `~/.agent-memory/agent-memory.db` (default)

### Building for Production

```bash
make build      # recommended — skips slow binding generation
make test       # run all Go tests
make clean      # remove build artifacts
```

`make build` depends on `make assets` (downloads the local model/tokenizer/ORT libraries, ~150 MB, on the first run) and builds with the `localembed` build tag plus the `CGO_LDFLAGS` needed to link the native libraries. The resulting binary is ~180 MB (it bundles the embedding model). `make test` runs the default, native-lib-free build — the `localembed`-tagged code is isolated so plain `go test ./...` needs no assets.

Or manually:

```bash
wails build -skipbindings
```

> **Note:** `wails build` (without `-skipbindings`) hangs during binding generation due to the CGO sqlite-vec dependency. The frontend calls Go methods via `window.go.main.App.*` at runtime, so generated bindings are not needed.

Output: `build/bin/agent-memory.app` on macOS, `build/bin/agent-memory` on Linux, `build/bin/agent-memory.exe` on Windows.

### Running

- **GUI mode:** `./agent-memory` (default)
- **MCP stdio mode:** `./agent-memory --mcp` (used by Claude Desktop)
- **Custom DB path:** `./agent-memory --db /path/to/data.db`

To manually configure Claude Desktop (instead of using the Install button), add to `~/Library/Application Support/Claude/claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "agent-memory": {
      "command": "/path/to/agent-memory",
      "args": ["--mcp"]
    }
  }
}
```

### Testing

```bash
make test                           # all tests
go test -cover ./...                # with coverage
go test -v ./internal/engine/       # verbose, specific package
```

## Contributing

Contributions are welcome! By submitting a pull request or otherwise contributing code to this project, you agree that your contributions become the intellectual property of United Effects Ventures, LLC, licensed under the same terms as the rest of the project (see [LICENSE](LICENSE)). You represent that you have the right to make such contributions and that no other party has claims to the contributed work.

## License

This project is licensed under the [Business Source License 1.1](LICENSE). You may use it for personal, non-commercial purposes. Commercial use requires a separate license from United Effects Ventures, LLC. On April 6, 2031, the license converts to Apache 2.0.

See [TRADEMARK.md](TRADEMARK.md) for copyright and trademark details.

## Documentation

See `Documentation/` for detailed technical docs:

- [Architecture](Documentation/ARCHITECTURE.md) — layered design, domain interfaces, runtime modes
- [Features](Documentation/FEATURES.md) — full feature reference, MCP tools, file extraction details
- [Roadmap](Documentation/ROADMAP.md) — planned work
