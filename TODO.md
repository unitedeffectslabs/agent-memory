# Agent Memory — Outstanding Work

## 1. Watcher / File Operations

- [x] **Debug watcher not picking up new files** — Root cause: macOS fires CHMOD on file copy; debounce replaced CREATE timer with CHMOD timer which wasn't handled. Fixed by adding CHMOD to the Modify branch.
- [x] **File deletion prunes DB** — Tested live: OnDelete removes file + chunks + embeddings. Needs unit test.
- [x] **File update triggers reindex** — Tested live: OnModify re-hashes, re-embeds. Fixed FK constraint bug (INSERT OR REPLACE generates new ID; now always re-fetches ID after upsert). Needs unit test.
- [x] **New file creation indexes correctly** — Tested live: OnCreate -> IndexFile, file/chunk counts increment on Dashboard. Needs unit test.
- [x] **File rename = delete + create** — Tested live: fsnotify.Rename -> OnDelete, fsnotify.Create -> OnCreate. Needs unit test.
- [x] **File move handled correctly** — Tested live: move from customers/ to customers/curoshift/ detected correctly. Needs unit test.
- [ ] **Unit tests for watcher file operations** — All six operations above were tested live and passed. Still need formal unit tests.

## 2. MCP Transport

- [ ] **Confirm MCP HTTP/SSE transport works** — Implementation exists (GET /sse + POST /messages). Needs live test with curl or Claude Desktop.

## 3. UI

- [ ] **Ignore patterns GUI in Settings** — List current patterns, add new, remove individual, restore defaults button. Backend methods exist (GetIgnorePatterns/SetIgnorePatterns), frontend not built yet.
- [x] **Directories page counts confirmed updating** — Verified live after watcher fix.
- [x] **Dashboard counts** — Verified live: counts update correctly after create, update, delete, and move operations.

## 4. Final

- [ ] **Run full test suite** — All tests pass after all fixes.
- [ ] **Rebuild app** — `make build` (uses `wails build -skipbindings`)
