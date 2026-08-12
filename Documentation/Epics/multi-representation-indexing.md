# Epic: Multi-Representation Indexing (dual-vector chunks)

**Date:** 2026-08-12
**Status:** Proposed — definition requested in PR #2 review; queued after `mcp-search-filters`
**Owner:** Bo Motlagh (definition + design input scoped by Bo; drafted by Nestor per review)

## Problem (verified against the PR #2 branch)

A chunk's single `content` currently serves two conflicting jobs: **the text we embed** and
**the text we return**. `.html`/`.htm` are in the extractor's `textExtensions` (raw
passthrough), so an HTML chunk's vector substantially describes class names and tags
(`text-slate-800`, `border-collapse`) rather than what the page says — findable only by
markup-shaped queries. Stripping tags before embedding is wrong the other way: for source
files the markup *is* the artifact, and a code-shaped query should match raw and get raw
back. Any per-file "document or code?" classification has no right answer.

## Proposed shape (Bo's design input — to validate in this epic, not settled)

Decouple embedded text from stored text: **up to two vectors per chunk**, both pointing at
the same stored raw payload.

- **Vector A** = embed(raw chunk) — matches code/markup-shaped queries.
- **Vector B** = embed(extracted text of that chunk) — matches prose queries about the
  rendered content.
- **Emission is measurable, not taxonomic** — at index time compare extracted length to raw
  length:
  - Ratio ≈ 1 (prose: md/txt/docx): one vector from raw — exactly today's behavior, no
    regression.
  - Extracted ≪ raw (html/svg/ipynb): both vectors.
  - Extraction ≈ nothing (minified bundles, base64 blobs): one raw vector **plus a
    file-level summary chunk** (path, type, title/headings outline) — the same shape as the
    ZIP metadata stub the extractor already emits.
- Extraction stays **deterministic and free**: for HTML, walk text nodes + structural
  landmarks (`<title>`, headings, `alt`, `aria-label`, link/button labels) via
  `golang.org/x/net/html`. No LLM, no per-file cost.

## Binding constraints (settled — do not relitigate)

- Files on disk are **never modified**.
- `search` returns **raw `content` by default** — existing callers unchanged; an optional
  `include_extracted` response field is fine.
- **No LLM** anywhere in the pipeline.
- The extra embed/vector cost lands on local compute and is accepted (~10–15% vector growth
  in a mostly-prose corpus).
- **Hard prerequisite: the PDF extraction fix** (see
  `Documentation/Bugs/pdf-extraction-passthrough.md` + its scheduled fix) — same mechanism,
  and PDFs become "embed extracted, store extracted" with **no raw vector** (nobody queries
  for `/Filter /FlateDecode`).

## Design questions this epic must work out (the real content)

1. **Schema.** `chunk_embeddings` is `vec0(chunk_id INTEGER PRIMARY KEY, embedding
   FLOAT[dim])` — strictly one vector per chunk today. Two vectors per chunk means a
   synthetic embedding id with a (chunk_id, representation) mapping — via an aux table or
   sqlite-vec's metadata/auxiliary columns — plus migration and `Reset()` changes.
2. **Search semantics.** KNN now returns *embedding* rows, not chunks — results must dedupe
   to one row per chunk keeping the best score, which forces over-fetch before
   `limit`/`offset`/`threshold` apply. This is the **same post-KNN machinery
   `mcp-search-filters` needs**; whichever epic lands second inherits the first's design —
   design `store.Search`'s post-processing once for both.
3. **Pairing granularity — the subtlest question.** Chunk boundaries are computed on raw
   text, so "the extracted text of chunk 42" means extracting from a raw HTML *fragment*
   (`x/net/html` tolerates fragments, but landmark quality mid-document needs verification)
   — versus the alternative of two parallel chunkings of the file with looser file-level
   pairing. **Spike this first; it decides the data model.**
4. **Threshold interplay.** Raw-vector and extracted-vector distances for the same chunk
   will distribute differently; confirm the provider-aware default threshold behaves
   sensibly when both representations compete in one result set (measure, per the
   local-embeddings Phase 0 pattern).

## Execution Notes (read first if you are the implementing session)

- Epic conventions per `local-embeddings.md`: phase gates with Verify, record deviations and
  measured numbers in this doc.
- Sequenced **after** `mcp-search-filters` (unless Bo reorders) — and note design question 2
  is shared with it either way.
- The emission ratio thresholds (what counts as "≈1", "≪") must be recorded here once
  measured against a real corpus — they are product behavior, not implementation detail.

## Phases (outline)

- **Phase 0 — Pairing spike (throwaway):** fragment-extraction quality on real HTML
  mid-document vs parallel-chunking alternative. **Gate:** data model decided, recorded here.
- **Phase 1 — Schema migration** ((chunk_id, representation) mapping, Reset, fingerprint
  interaction). **Verify:** existing single-vector DBs migrate or trigger the re-index path
  cleanly — never silently mixed.
- **Phase 2 — Extraction + emission rules** (ratio-based; html first, svg/ipynb after).
  **Verify:** prose corpus produces byte-identical behavior to today (no regression class).
- **Phase 3 — Search dedupe + threshold calibration** (shared machinery with
  mcp-search-filters). **Verify:** prose-shaped query finds the HTML page's content; code-
  shaped query still finds raw markup; measured distances recorded.
- **Phase 4 — MCP `include_extracted` + docs.**

## Out of scope / parking lot

- LLM-generated summaries or descriptions of any kind.
- Per-file-type user configuration of representations.
- OCR / image content extraction.
