# Bug: local embedder overflows the model context — every multi-chunk file silently dropped

**Date found:** 2026-07-17
**Found during:** Phase 5 Windows GUI field test (first real-world vault ever indexed)
**Status:** Fixed — branch `fix/local-embed-token-budget` (stacks on the epic PR chain)
**Severity:** Critical for the local provider — affects ALL platforms, latent since Phase 3

## Symptom

Indexing a real vault (109 files, D:\Brain) silently indexed only 26 files. No errors
anywhere user-visible (see the companion visibility bug, PR #8). The 26 survivors were
exactly the files ≤ 1.6 KB; every file ≥ 1.7 KB — i.e., anything needing more than one
chunk — was dropped. All previous platform verifications used tiny single-chunk test
files, so five platforms' smokes all passed while the pipeline was broken for real
workloads everywhere.

## Root cause (the "512 by 516" error)

ONNX Runtime fails hard when a sequence exceeds mE5-small's 512-token context:

```
BroadcastIterator::Append ... Attempting to broadcast an axis by a dimension other than 1. 512 by 516
```

`buildChunker` passed `embedder.MaxInputTokens()` (512) straight through as the chunk
budget. But the embedder prepends the E5 instruction prefix ("passage: ") and the
tokenizer adds special tokens **after** chunking, so a chunk cut at exactly 512 tokens
reached the model at 516. The epic specified the defense precisely ("effective chunk
size = min(chunk_size, MaxInputTokens − prefix − special) → default 480"); the
implementation skipped the reservation.

The unchunked **query path** was also exposed: `EmbedQuery` has no chunker, so a search
query longer than ~500 tokens crashed inference on every platform.

## Fix (two layers)

1. `app.go buildChunker`: chunk budget = `MaxInputTokens() − local.EmbedTokenReserve`
   (32 → effective 480, matching the epic).
2. `LocalEmbedder.embedBatch`: defensively truncate any tokenized input to the model
   limit, preserving the trailing EOS token — protects queries and any future caller
   regardless of chunking correctness.

## Verification

- Unit: truncation table test + fake-session proof the model never receives >512 tokens.
- Integration (permanent): `TestIntegrationLargeDocument` — real pipeline, multi-chunk
  document + over-long query. Failed before the fix on macOS AND Windows; passes on both.
- Field: clean re-index of the same vault on Windows → **105 files / 932 chunks, all 5
  dossier files present, 79 multi-chunk files, zero errors** (was 26/26/0).

## Lesson recorded

Platform verifications used single-chunk corpora only; a large-document case is now a
permanent integration test, and the Phase 6 checklist gained a real-vault field test.
