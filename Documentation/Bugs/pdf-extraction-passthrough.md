# Bug: PDF "extraction" embeds raw bytes, not text

**Status:** Fixed on `fix/pdf-extraction` (2026-08; PR follows the #2 merge per single-PR workflow) · **Severity:** Medium (silent quality loss) · **Reported:** 2026-07-06 · **Field-confirmed:** 2026-08-08 by Bo on a real vault (5 PDFs, versions 1.3–1.6, multiple producers)
**Component:** `internal/extractor` · **Track:** separate from the local-embeddings epic (pre-existing)

## Summary

PDF files are treated as "supported," but their text is never actually extracted.
`extractPDFPassthrough` (`internal/extractor/extractor.go:177`) does `os.ReadFile(path)` and returns
the **raw PDF bytes** as the `Text` to chunk and embed. PDFs are binary (compressed content streams,
xref tables, object dictionaries), so what gets embedded is mostly non-text noise, not the
document's readable content.

## Impact

- Semantic search over PDFs is effectively broken — matches are against binary noise, not content.
- The README claims "PDF — text content extraction," which the code does not do.
- Under the local-embeddings epic, the same garbage would burn **local CPU** instead of API dollars.

## Evidence

`internal/extractor/extractor.go`:

```go
func extractPDFPassthrough(path string) (Result, error) {
    data, err := os.ReadFile(path)
    if err != nil { return Result{}, err }
    return Result{Text: string(data)}, nil   // raw PDF bytes, not extracted text
}
```

`.pdf` is registered in `binaryExtractors`, so the file is reported as supported and indexed.

## Proposed fix (separate task)

Replace the passthrough with real PDF text extraction (a pure-Go PDF text library returning
concatenated page text), and add a unit test with a small sample PDF. Keep this **out of the
local-embeddings epic scope** — it is an independent defect.

## Notes

Found during the Phase-0 "get familiar / try to break it" pass. Logged per the bug-report workflow;
not a blocker for the epic.

## Fix (2026-08, `fix/pdf-extraction`)

- Real text extraction via `github.com/ledongthuc/pdf` (pure Go, BSD, rsc.io/pdf lineage),
  page-concatenated, with panic recovery so malformed PDFs surface as visible indexing
  errors instead of crashing the indexer.
- **Re-extraction mechanism** (the trap Bo flagged in the PR #2 review — an extractor fix
  changes no file hashes, so already-indexed PDFs would keep their garbage chunks forever):
  per-type extractor versions. The `files` table gains `extractor_version` (additive
  migration, legacy rows read 0); the engine skips a file only when hash AND version match;
  the PDF extractor is bumped to v1 — so upgrading re-extracts **only PDFs**, leaving every
  other file's vectors untouched.
- Tests: minimal-PDF extraction regression (no raw-bytes leak), corrupt-PDF error path,
  per-type version table, version-bump-forces-reindex (engine), legacy-DB column migration
  round-trip (store). Field-verified against three real-world PDFs (contracts/proposal,
  different producers): 1,045–4,599 words each of clean text, zero raw PDF syntax.
