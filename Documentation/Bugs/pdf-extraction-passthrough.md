# Bug: PDF "extraction" embeds raw bytes, not text

**Status:** Open · **Severity:** Medium (silent quality loss) · **Reported:** 2026-07-06
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
