# Epic: MCP Search Filters (filesystem-tier metadata)

**Date:** 2026-08-12
**Status:** Proposed — definition requested in PR #2 review; queued after `background-presence`
**Owner:** Bo Motlagh (definition scoped by Bo; drafted by Nestor per review)

## Goal

The MCP `search` tool (stdio and HTTP) gains **optional filter parameters** that constrain
results by file metadata, so a caller can ask "search only my markdown notes," "only files
under this directory," or "only files modified in the last month" — instead of over-fetching
and filtering client-side after the fact.

## User-set constraints (settled in the PR #2 review — do not relitigate)

- **Filesystem-tier metadata only**: path/glob, file type/extension, watched directory,
  file modified time, file size, indexed-at time. **Document-embedded properties**
  (title/author/created from docx/pdf/etc.) are explicitly **out of scope** for this epic —
  parking-lot note only.
- **MCP surface only**: both transports' `search` tool schemas. **No GUI search work.**
- Definition merged in PR #2; implementation sequenced after `background-presence` unless
  Bo reorders.

## Current state (verified against the PR #2 branch)

- The `files` table carries only `path`, `hash`, `indexed_at`, `directory_id` — **no mtime,
  size, or type column**. Filters on modified-time/size need columns captured at index time:
  a schema migration plus a backfill decision. Backfill is cheap — an `os.Stat` per file at
  the next scan populates the columns **without re-embedding** (the content hash is
  unchanged, so the pipeline is skipped; only the file row updates).
- Filter params must flow through `domain.SearchParams` so engine / readonly / store stay
  interface-clean; the tool schemas in `internal/mcp/server.go` are the only delivery-layer
  touch (both transports dispatch through the same search path).
- `store.Search` today over-fetches (`limit+offset`, ×2 under a threshold) and filters
  distance in Go — there is already a post-KNN filtering stage to build on.

## The key design question (the epic's real research content)

**How filtering composes with the KNN query** in `store.Search`: sqlite-vec `vec0` KNN does
not trivially JOIN-filter. Research and **measure** both options, recording numbers in this
doc the way local-embeddings recorded its Phase 0 results:

1. **Over-fetch then filter** (in SQL over the joined rows, or in Go): simple, no schema
   coupling to sqlite-vec — but `limit`/`offset`/`threshold` semantics must hold
   **post-filter**. A filter that excludes 90% of neighbors must not return 1 result because
   the over-fetch was too small: the over-fetch factor needs to adapt (e.g. iterative
   re-query with growing K until `limit` post-filter results or the index is exhausted).
2. **sqlite-vec metadata columns / partition keys** in the `vec0` table (newer sqlite-vec
   feature): filters pushed into the KNN itself. Verify version availability in our pinned
   sqlite-vec, migration cost for existing tables, and which of our filter types it can
   express (globs almost certainly not — likely a hybrid: partition/metadata for cheap
   equality filters, post-filter for globs/ranges).

**Design-once note (from the review):** result post-processing (over-fetch → dedupe/filter →
limit/offset/threshold) is the **same machinery `multi-representation-indexing` needs** for
its per-chunk best-score dedupe. Whichever epic lands first should design `store.Search`'s
post-KNN stage to serve both.

## Execution Notes (read first if you are the implementing session)

- Epic conventions per `local-embeddings.md`: phase gates with Verify, record deviations here.
- Schema migration must be additive (`ALTER TABLE ADD COLUMN`, nullable) — existing DBs keep
  working un-backfilled; NULL metadata simply doesn't match metadata filters until the next
  scan backfills (document this in the tool description).
- Tool schema: filters land as optional properties (e.g. `path_glob`, `extensions[]`,
  `directory`, `modified_after/before`, `min/max_size`, `indexed_after/before`) — absent
  filters must behave byte-for-byte like today's search (regression tests on that).

## Phases (outline)

- **Phase 0 — Measure the composition options** on a realistic index (thousands of chunks):
  over-fetch-adaptive vs metadata-column KNN, latency + correctness under selective filters.
  **Gate:** numbers recorded here; approach chosen.
- **Phase 1 — Schema + backfill:** mtime/size (+type derived from path) columns, migration,
  os.Stat backfill on scan without re-embed. **Verify:** upgraded DB backfills on next scan;
  no re-embedding occurs (chunk counts and vectors unchanged).
- **Phase 2 — `domain.SearchParams` + store filtering** per the chosen approach, with the
  post-filter limit/offset/threshold semantics pinned by tests (including the
  filter-eats-90%-of-neighbors case).
- **Phase 3 — MCP tool schemas** (both transports) + docs. **Verify:** filtered searches via
  a live MCP client; unfiltered search unchanged.

## Out of scope / parking lot

- Document-embedded properties (docx/pdf titles, authors, created dates) — future epic.
- GUI search/filter UI.
- Saved filters, filter persistence, per-directory defaults.
