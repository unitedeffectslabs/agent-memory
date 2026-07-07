# Epic: Local Embeddings as the Primary Provider

**Date:** 2026-07-02
**Status:** In Progress — Phase 1 complete; entering Phase 2
**Owner:** Bo Motlagh

## Goal

Replace OpenAI as the default embedding provider with a bundled, in-process, CPU-based local
embedding model. The app must work out of the box with **zero API key and zero network calls**.
OpenAI remains available as an explicitly opted-into backup provider (selected in Settings), for
users who want higher retrieval quality and accept the cost.

Decisions made in discussion (2026-07-02):

1. **Cross-platform** in-process inference, with **macOS as the explicit priority** — it is the
   primary development and test environment. All development, spikes, and verification happen on
   macOS arm64 first; a phase is not considered proven until it works there. Linux x64/arm64,
   Windows x64, and macOS x86_64 follow in Phase 5 (see Platform Matrix).
2. **Bundled, in-process** — no external daemon (no Ollama). Model + runtime ship with the app.
3. **Local is primary**, OpenAI is opt-in backup. Onboarding no longer requires an API key.
4. **Multilingual** model preferred — open-source project, global audience.
5. Chunking adapts to the local model's tokenizer and context limit.
6. No time estimates in this document.
7. **OpenAI is never a requirement.** On every platform we ship, local inference works out of
   the box. No platform's users are routed to OpenAI as a forced fallback; OpenAI exists only
   as an explicit opt-in. Where an upstream project stops publishing prebuilt binaries for a
   platform (see macOS x86_64), we build the artifact from source ourselves rather than
   downgrade that platform to API-only.

## Execution Notes (read first if you are the implementing session)

This epic is designed to be executed without access to the conversation that produced it.
Everything you need is in this file plus `CLAUDE.md`. Conventions:

- **⚠ Build environment:** on macOS 26+, Go **1.24.x or older** produces CGo binaries that the
  kernel kills instantly on launch (exit 137 / `Killed: 9`; `dyld` reports missing `LC_UUID`).
  This breaks `make build`, the `wails` CLI itself, and the built app — with no useful error.
  Before starting, ensure Go ≥ 1.25.1 is in effect: `go env -w GOTOOLCHAIN=go1.26.4` (or
  upgrade the system Go), then reinstall the wails CLI so it's built with the good toolchain:
  `go install github.com/wailsapp/wails/v2/cmd/wails@v2.11.0`. Verified working with 1.25.1 on
  2026-07-02. If a freshly built binary dies instantly with no output, this is why.
- **Phase discipline:** phases are ordered by dependency; do not start a phase before the prior
  phase's **Verify** gate passes. Phase 0 is a throwaway spike — keep it out of `main`
  (scratch branch or scratchpad), but **record its results in this file** (model decision,
  measured load time / per-chunk latency / RSS, typical cosine-distance ranges for threshold
  tuning) under a new "Phase 0 Results" section.
- **Status tracking:** update the `Status:` field at the top as work proceeds
  (Proposed → In Progress (Phase N) → Complete). Check off or annotate plan items as they land.
- **Never commit:** model weights, tokenizer files, or ORT libraries (`assets/embedded/` is
  gitignored; only `assets/manifest.json` with URLs + SHA-256 checksums belongs in git).
- **Architecture rules are binding:** dependency flow, wiring-in-main.go, thin delivery layer,
  mocks for all interfaces — see `Documentation/ARCHITECTURE.md`. The Modified/New files tables
  below are the authoritative scope; if implementation reveals a needed deviation, update the
  tables rather than silently diverging.
- **User-set constraints (do not relitigate):** local is the default provider; OpenAI is
  opt-in only and never required on any platform; in-process bundled inference (no external
  daemon); macOS arm64 is the priority environment; no time estimates.

## Research Summary (verified 2026-07-02)

Full sources at the end. The chosen stack:

| Component | Choice | Version | License | Notes |
|---|---|---|---|---|
| Inference runtime | ONNX Runtime (CPU) | 1.26.0 | MIT | Pinned to what `onnxruntime_go` targets; shared lib loaded at runtime via `SetSharedLibraryPath` |
| Go bindings | `github.com/yalue/onnxruntime_go` | v1.31.0 | MIT | Active (June 2026); no compile-time link to ORT — ideal for bundling |
| Tokenizer | `github.com/daulet/tokenizers` | v1.27.0 | MIT | CGo → HF Rust tokenizers; handles XLM-R Unigram `tokenizer.json` correctly |
| Model (default) | `intfloat/multilingual-e5-small` | int8 ONNX (Xenova export) | MIT | 384-dim, 512-token ctx, ~100 languages, ~113 MB + 16 MB tokenizer |
| Model (spike candidate) | `ibm-granite/granite-embedding-97m-multilingual-r2` | int8 ONNX (official) | Apache-2.0 | 384-dim, 32k ctx, +9.4 pts multilingual retrieval vs mE5-small, 98 MB — see Phase 0 |

**Rejected:**

- `knights-analytics/hugot` — heavy dependency tree (GoMLX ecosystem), Linux-amd64-centric,
  server-oriented. We need one pipeline (~100 lines of glue); its source is a good *reference*
  for pooling/attention-mask math.
- `sugarme/tokenizer` (pure Go) — **no Unigram/sentencepiece support**; would be silently wrong
  for multilingual-e5. Hard disqualifier.
- Pure-Go inference (GoMLX backend, cybertron) — slow or stale.
- Ollama/llama.cpp sidecar — violates the bundled in-process decision.
- `google/embeddinggemma-308m` (Gemma license — not permissive), `jina-embeddings-v3` (CC-BY-NC),
  `Qwen3-Embedding-0.6B` (too large). License hygiene matters: this project is BUSL/commercial.

### Model selection

**Committed default: `intfloat/multilingual-e5-small`.** MIT, battle-tested, 384-dim (identical
vector storage cost to English-only bge-small), 512-token context, generic dynamic-int8 ONNX
export available (portable across x64 and arm64). English quality gap vs the best English-only
small model (bge-small-en-v1.5) is ~4 MTEB points — accepted in discussion as the price of
working for a global audience.

**Critical model behavior:** every input must be prefixed — `"query: "` for search queries,
`"passage: "` for indexed content, *even for non-English text*. Omitting prefixes degrades
quality (confirmed by the model card FAQ). Outputs must be mean-pooled over the attention mask
and L2-normalized.

**Phase 0 spike: `granite-embedding-97m-multilingual-r2`** (released April 2026) beats
mE5-small significantly on multilingual retrieval (60.3 vs 50.9), is Apache-2.0 with an
IBM-official int8 ONNX export (98 MB), and has a 32k context. It is *not* the committed default
because three things are unverified: (a) ModernBERT architecture compatibility with ORT 1.26 CPU
on all targets, (b) the official int8 file is AVX2-targeted — arm64 behavior needs testing or a
re-quantization, (c) prefix requirements undocumented. The design below is model-agnostic
(model + tokenizer are data files), so if the spike passes, swapping the default is a
configuration change, not a redesign.

### Platform matrix

| Platform | ORT prebuilt | libtokenizers prebuilt | Plan |
|---|---|---|---|
| macOS arm64 | ✅ 1.26 (dylib 38 MB) | ✅ | **Phase 1 target** |
| Linux x64 | ✅ (so 24 MB) | ✅ | Phase 5 |
| Linux arm64 | ✅ | ✅ | Phase 5 |
| Windows x64 | ✅ | ⚠ buildable since v1.27, no published binary — we own a Rust build step in CI | Phase 5 |
| macOS x86_64 | ⚠ official prebuilts stopped at 1.23.0 — **we source-build ORT 1.26.0 ourselves** | ✅ | Phase 5 |

**macOS x86_64 note:** ONNX Runtime is MIT open source; only its *published binaries* dropped
mac-Intel. We compile the CPU-only dylib for x86_64 ourselves (CMake
`--osx_arch x86_64 --build_shared_lib`, cross-compilable from an arm64 Mac — no Intel hardware
required), targeting **exactly ORT 1.26.0** so the `onnxruntime_go` header version matches
perfectly (strictly safer than pinning the old 1.23 official binary, which would create a
newer-headers/older-runtime mismatch). The artifact is hosted in this repo's GitHub releases and
pinned by SHA-256 in `assets/manifest.json` like every other artifact — `make assets` treats it
identically to officially-published binaries.

## Phase 0 Results

**Date:** 2026-07-06 · **Outcome: GATE PASSED** — local embedding stack proven end-to-end on
macOS arm64 (throwaway spike, scratchpad only; no repo code touched).

**Verified stack (all pinned versions worked):** ONNX Runtime 1.26.0 CPU via `onnxruntime_go`
v1.31.0; tokenizer via `daulet/tokenizers` v1.27.0 (prebuilt `libtokenizers.darwin-arm64`);
models `Xenova/multilingual-e5-small` int8 (118 MB) and
`ibm-granite/granite-embedding-97m-multilingual-r2` int8 (98 MB). Pipeline
tokenizer → ORT → mean-pool → L2-normalize produced a valid **384-dim, L2-norm = 1.0** vector;
multilingual tokenization (EN/ES/ZH/AR) sane.

| Metric | mE5-small | Granite-97m |
|---|---|---|
| Cold load | 129 ms | 145 ms |
| Per-chunk @ batch 16 (~300 tok) | ~34 ms (~30/s) | ~32 ms |
| Peak RSS (upper bound) | ~1.3 GB | ~1.5 GB |
| Disk | 118 MB | 98 MB |

**Quality (cosine; distance = 1 − cos):** ordering correct — related 0.13–0.18 <
cross-lingual 0.17–0.22 < unrelated ~0.29 (mE5). Cross-lingual (EN↔ES) retrieval works well.

**Model decision:** **`multilingual-e5-small` is the committed default.** **Granite-97m logged as
a future upgrade candidate** — it loaded and ran at full speed on arm64 (the AVX2-int8 concern did
not materialize), smaller file, crisper related/unrelated separation, but marginally weaker
cross-lingual; needs a broader recall eval + prefix-convention decision before promotion. Upside,
not a dependency (model = data file).

**Findings that adjust the plan:**

1. **mE5 requires `token_type_ids`.** The Xenova export takes **three** INT64 inputs
   (`input_ids`, `attention_mask`, `token_type_ids`), not two — pass a zero tensor for
   `token_type_ids` or ORT errors. Affects the Phase 2 `LocalEmbedder` inference code. (Granite
   takes the two-input form.)
2. **Default search threshold `1.5` is miscalibrated for local vectors** (real related-vs-unrelated
   separation is ~0.25 cosine distance; 1.5 admits nearly everything). Make the default
   **provider-aware**, and confirm which distance metric sqlite-vec's `vec0` table is configured for
   before tuning.
3. **Peak RSS ~1 GB** (ORT memory arena + batch activations) — constrain ORT arena / batch size in
   `LocalEmbedder` for the desktop memory budget.
4. **Build flags:** minimal `CGO_LDFLAGS="-L<dir> -ltokenizers"` suffices on darwin arm64; no
   `-framework` flags needed (the lib embeds `-ldl -lm`).

## Architecture

### Design principles (unchanged)

The `Embedder` interface is the seam — this was always the intended extension point. The local
embedder is a new **infrastructure implementation** in `internal/embeddings/`; the engine, store,
watcher, and MCP layers are provider-agnostic. All wiring stays in `main.go`.

### Runtime model

```
GUI mode (full engine)                    Stdio --mcp mode (read-only)
  LocalEmbedder (default)                   LocalEmbedder (default)
    ONNX session, loaded on Start()           ONNX session, LAZY-loaded on first search
    embeds passages during indexing           embeds only the query text
  — or OpenAIEmbedder if opted in           — or OpenAIEmbedder if opted in
            \                                      /
             same SQLite DB (WAL, 1 writer / N readers)
```

- Both processes load their own copy of the model (~150–200 MB RSS each when active). Accepted.
- Stdio must lazy-load: the MCP `initialize` handshake stays instant; the model loads on the
  first `search` call and stays warm for the life of the subprocess (Claude Desktop keeps it
  alive per session).
- The stdio process never phones the GUI — unchanged principle from the stdio refactor.

### Interface changes

**`internal/embeddings/iface.go`** — the one deliberate breaking change. The current `Embed()`
cannot express the query/passage asymmetry that E5-family models require:

```go
type Embedder interface {
    EmbedDocuments(texts []string) ([][]float32, error) // indexing path ("passage: " prefix for local)
    EmbedQuery(text string) ([]float32, error)          // search path ("query: " prefix for local)
    Dimensions() int
    ModelName() string
    MaxInputTokens() int // 0 = no practical limit (OpenAI); 512 for mE5-small
}
```

- `OpenAIEmbedder`: `EmbedDocuments` = current `Embed`; `EmbedQuery` wraps it for one text;
  `MaxInputTokens` returns 0 (chunk sizes stay well under the API's 8191 limit).
- Prefixing is an implementation detail of `LocalEmbedder` — callers never see prefixes, and
  prefixes are added *after* chunking (chunk budget accounts for them, below).
- `MockEmbedder` in `internal/mocks/mocks.go` grows the new methods; all engine/mcp tests that
  stub `EmbedFn` update mechanically.

**`internal/chunker/iface.go`** — the chunker gains a tokenizer seam (no cross-domain import;
the interface lives in the chunker package, satisfied by adapters):

```go
type Tokenizer interface {
    Encode(text string) ([]uint32, error)
    Decode(tokens []uint32) (string, error)
}
```

- `TokenChunker` accepts `WithTokenizer(t Tokenizer)`; the existing tiktoken codec becomes the
  default adapter (used when provider = openai).
- A new adapter wraps the HF tokenizer (used when provider = local) so chunk boundaries and
  `TokenCount` are measured in the *model's* tokens — otherwise inputs silently truncate.
- `main.go` wires embedder ↔ chunker consistency (composition root rule).

**Effective chunk size** for the local provider:
`min(configured chunk_size, MaxInputTokens − prefix_tokens − special_tokens)` ≈
`512 − 4 ("passage: ") − 2 ([CLS]/[SEP])` → default **480 tokens**, overlap unchanged (50).
The user-configured `chunk_size` is clamped, never exceeded.

### New package: `internal/embeddings/local`

```
internal/embeddings/local/
  local.go        LocalEmbedder: lazy session init, tokenize → run ONNX → mean-pool → L2-normalize
  assets.go       go:embed of model/tokenizer/ORT lib; extraction to disk (see Bundling)
  local_test.go   unit tests (mock session boundary) + integration test behind a build tag
```

`LocalEmbedder` responsibilities:

- **Lazy init** (`sync.Once`): extract assets if needed, `SetSharedLibraryPath`,
  `InitializeEnvironment`, create session. First call pays ~1–3s; subsequent calls are warm.
- **Inference**: batch inputs internally (batch size ~16–32), build `input_ids` /
  `attention_mask` tensors (padded per batch), mean-pool token embeddings weighted by attention
  mask, L2-normalize. (Normalization keeps cosine/L2 distance semantics consistent —
  OpenAI vectors are already unit-norm, so `store.Search` and the threshold logic are unaffected
  structurally; see Risks for threshold *tuning*.)
- **Thread control**: cap ORT intra-op threads (e.g. `min(4, NumCPU/2)`) so background indexing
  doesn't peg the machine.
- **Clear errors**: if assets are missing/corrupt → actionable message; never panic into Wails.

### Bundling & distribution

"Bundled" = the shipped app works offline with no downloads, but **model weights do not live in
git**:

1. `make assets` (new) downloads pinned artifacts into `assets/embedded/` (gitignored) and
   verifies SHA-256 checksums committed in a small manifest file (`assets/manifest.json`, in git):
   - `model.int8.onnx` (~113 MB, Xenova export of multilingual-e5-small)
   - `tokenizer.json` (~16 MB)
   - `libonnxruntime.{dylib,so}` / `onnxruntime.dll` for the target platform (~24–38 MB)
2. The Go binary embeds them via `go:embed`. `make build` depends on `make assets`.
3. On first use, assets are extracted to `~/.agent-memory/runtime/<fingerprint>/` (atomic
   write-then-rename, checksum-verified, idempotent) because ORT's shared library must be
   `dlopen`-ed from a real file path. Extraction is safe from either GUI or stdio mode.
4. `daulet/tokenizers` links `libtokenizers.a` **statically** at build time via `CGO_LDFLAGS` —
   nothing extra to ship, but the Makefile/docs must pin its download too.

Binary grows from ~26 MB to ~180 MB. Accepted in discussion (the whole point is no API costs).

### Configuration & provider switching

New/changed config keys (config table):

| Key | Values | Notes |
|---|---|---|
| `embedding_provider` | `local` (default) \| `openai` | new |
| `embedding_model` | provider-scoped model name | existing key; `multilingual-e5-small` when local |
| `embedding_fingerprint` | `provider:model:dimensions` | new — written on every successful index run; validated at startup |
| `onboarding_complete` | `"true"` | new — replaces "has API key" as the onboarding gate |

**Provider switch flow** (reuses the existing model-change machinery in `app.SetConfig` — do not
build a parallel path):
set `embedding_provider` → build new embedder via factory → `engine.SetEmbedder()` →
`engine.Reset()` (drops/recreates `chunk_embeddings` at the new dimension, re-indexes). The
destructive re-index warning UX already exists for model changes; extend it to provider changes.

**Fingerprint check** at GUI startup: if `embedding_fingerprint` disagrees with the active
provider/model/dimensions (e.g. DB written by OpenAI vectors but provider is now local), surface
a "re-index required" state instead of silently returning garbage matches.

**`EmbedderFactory`** in `app.go` generalizes from `func(apiKey, model string)` to:

```go
type EmbedderFactory func(provider, apiKey, model string) (embeddings.Embedder, error)
```

`main.go` implements it: `local` → `local.New(...)`; `openai` → `NewOpenAIEmbedder(...)`.

## Codebase Integration — exact touch points (anti-orphan audit)

Verified against the code on 2026-07-02. **Reuse, don't duplicate**: the reset flow
(`app.SetConfig` → `SetEmbedder` → `engine.Reset`), the dispatch parameterization in `mcp/`,
and the indexing-progress UX all already exist and are the extension points.

### Modified files

| File | Change |
|---|---|
| `internal/embeddings/iface.go` | New interface (EmbedDocuments/EmbedQuery/MaxInputTokens) |
| `internal/embeddings/openai.go` | Implement new interface; `Embed` → `EmbedDocuments`; add `EmbedQuery`, `MaxInputTokens() → 0` |
| `internal/embeddings/openai_test.go` | Update for renamed/new methods |
| `internal/chunker/iface.go` | Add `Tokenizer` interface |
| `internal/chunker/chunker.go` | `WithTokenizer` option; tiktoken becomes the default adapter; clamp to max tokens |
| `internal/chunker/chunker_test.go` | Cover both tokenizer adapters + clamping |
| `internal/engine/engine.go` | `Search` → `EmbedQuery`; `embedSlice` → `EmbedDocuments`; comment that `maxTokensPerBatch=250000` is OpenAI-API-specific (harmless for local — the local embedder sub-batches internally; do not add a second batching layer) |
| `internal/engine/readonly.go` | `Search` → `EmbedQuery` |
| `internal/engine/engine_test.go`, `readonly_test.go` | Update mock stubs |
| `internal/mocks/mocks.go` | `MockEmbedder` implements the new interface |
| `internal/store/sqlite.go` | `migrate()`: create `chunk_embeddings` at the **local default dimension (384)** for fresh DBs; `Reset()` default dim likewise; **fix layering leak** — `Stats()` must stop hardcoding `"text-embedding-3-small"` and instead report `embedding_provider` + `embedding_model` from config |
| `main.go` | Both branches: read `embedding_provider`, construct via factory; stdio branch must NOT eagerly load the local model (constructor is cheap, session is lazy); GUI branch wires embedder-matched tokenizer into chunker |
| `app.go` | Generalized `EmbedderFactory`; `SetConfig` cases for `embedding_provider` (swap + Reset) and provider-aware `embedding_model` / `openai_api_key` handling (key change only swaps embedder when provider = openai); startup fingerprint check |
| `internal/mcp/server.go` | `index_status` response gains provider field (flows from Stats) |
| `frontend/src/App.jsx` | Onboarding gate: `onboarding_complete` flag, **not** `openai_api_key` presence |
| `frontend/src/pages/Onboarding.jsx` | Remove required API-key step; flow = welcome → add dirs → done (local default); optional "use OpenAI instead" affordance |
| `frontend/src/pages/Settings.jsx` | New "Embedding Provider" section: local (default) vs OpenAI (key + model picker only shown when OpenAI selected); destructive-change warning on provider switch; update "Outbound: api.openai.com only" → provider-aware (local = "none") |
| `frontend/src/pages/Dashboard.jsx` | Model display shows provider-qualified name |
| `Makefile` | `make assets` target (download + checksum); `build` depends on it; `CGO_LDFLAGS` for libtokenizers |
| `.gitignore` | `assets/embedded/` |
| `go.mod` | `+ yalue/onnxruntime_go v1.31.0`, `+ daulet/tokenizers v1.27.0` |

### New files

| File | Purpose |
|---|---|
| `internal/embeddings/local/local.go` | `LocalEmbedder` (lazy ONNX session, tokenize→infer→pool→normalize, prefixes) |
| `internal/embeddings/local/assets.go` | `go:embed` + extract-to-`~/.agent-memory/runtime/` with checksums |
| `internal/embeddings/local/local_test.go` | Unit tests + build-tagged integration test |
| `internal/chunker/hf_tokenizer.go` (or similar) | HF tokenizer adapter satisfying `chunker.Tokenizer` |
| `assets/manifest.json` | Pinned artifact URLs + SHA-256 (in git) |

### Explicitly unchanged

`internal/watcher/`, `internal/extractor/`, `internal/mcp/dispatch.go`, `internal/mcp/stdio.go`
(interface types unchanged — `ReadOnlyEngineService` signature is stable), `tray.go`,
`internal/store/sqlite_readonly.go`, all search/KNN logic in `store.Search`.

### Known dead-code risks to check at the end

- No leftover callers of the old `Embed()` name anywhere (grep).
- tiktoken dependency stays (OpenAI provider still needs it) — do not remove.
- The `"query: "`/`"passage: "` prefixes must never leak into stored chunk `Content` (prefix at
  embed time only), or search-result display and re-embedding both corrupt.

## Implementation Plan

### Phase 0 — Spike: validate the stack on macOS arm64 (throwaway branch)

1. Tiny Go program: `daulet/tokenizers` loads mE5-small `tokenizer.json`; verify Unigram output
   against known token counts.
2. `onnxruntime_go` + ORT 1.26 dylib + Xenova int8 model: embed "query: hello world", verify
   384-dim unit-norm vector; cosine-compare a few sentence pairs for sanity.
3. **Granite spike**: run `granite-embedding-97m-multilingual-r2` int8 through the same harness
   on arm64. If it loads on ORT 1.26 CPU, quality-check vs mE5-small on a handful of
   multilingual pairs and check inference speed. Decide default model; record decision here.
4. Measure: model load time, per-chunk embed latency at batch 16, RSS.

**Gate:** stack proven end-to-end before any production code changes.

### Phase 1 — Interface change (mechanical, keeps all tests green)

1. New `Embedder` interface; update `OpenAIEmbedder`, `MockEmbedder`, engine, readonly, all tests.
2. `Tokenizer` seam in chunker; tiktoken adapter; wire in `main.go`.
3. **Verify:** `go test ./...` green; behavior identical (OpenAI path unchanged).

### Phase 2 — Local embedder package

1. `internal/embeddings/local`: assets extraction (checksummed, atomic, idempotent), lazy
   session, tokenize → infer → mean-pool → normalize, prefixes, internal batching, thread cap.
2. HF tokenizer adapter for chunker.
3. Unit tests (session boundary mocked); integration test behind `//go:build localembed` tag.
4. **Verify:** integration test embeds and round-trips search-quality sanity checks locally.

### Phase 3 — Wiring, config, provider switching

1. `make assets` + manifest + gitignore; `go:embed` hookup.
2. `main.go` both branches provider-aware (stdio lazy); generalized factory in `app.go`;
   `SetConfig` provider handling; fingerprint write/check; store migrate/Reset dimension changes;
   Stats layering fix.
3. **Verify:** fresh DB → GUI indexes a test directory fully offline (network disabled);
   `--mcp` process answers a search offline; provider switch local↔openai triggers reset +
   re-index; existing OpenAI-vectored DB triggers the fingerprint mismatch path, not garbage
   results.

### Phase 4 — Frontend + docs

1. Onboarding rework (no key required), Settings provider section, Dashboard display, gate flag.
2. Update README (privacy story: **no outbound network by default** — this is now a headline
   feature), FEATURES, ARCHITECTURE (new infra implementation + asset pipeline), CLAUDE.md
   (build prerequisites: `make assets`), ROADMAP.
3. **Verify:** clean-machine walkthrough: build → onboard with no key → index → search from
   Claude Desktop via stdio → switch to OpenAI in Settings → re-index → switch back.

### Phase 5 — Cross-platform builds

1. Linux x64/arm64: assets manifest entries, CI build, smoke test.
2. Windows x64: CI job builds `libtokenizers.a` with Rust toolchain (no published binary);
   ORT DLL from official release; smoke test.
3. macOS x86_64: build ORT 1.26.0 CPU dylib from source for x86_64 (cross-compile on arm64 or
   a mac-Intel CI runner); publish to this repo's releases; add manifest entry; smoke test
   (Rosetta on an arm64 Mac can execute the x86_64 test binary if no Intel hardware is at hand).
4. **Verify:** per-platform: binary runs offline, indexes, searches — **local provider working
   on every shipped platform, no OpenAI required anywhere.**

### Phase 6 — Cleanup & verification

1. Grep for orphaned symbols (`Embed(` callers, unused OpenAI defaults, stale
   `"text-embedding-3-small"` fallbacks outside the OpenAI provider path).
2. Architecture rule check per `Documentation/ARCHITECTURE.md` (inward deps, wiring in main.go,
   mocks for all interfaces, thin delivery layer).
3. `go vet ./...`, `go test ./...`, `make build`, full manual test both modes.
4. Update this epic's Status to Complete; record the chosen default model and measured numbers.

## Risks

| Risk | Severity | Mitigation |
|---|---|---|
| ORT 1.26 pin vs future `onnxruntime_go` header bumps | Low | Pin both in manifest/go.mod; upgrade deliberately |
| Windows `libtokenizers.a` self-build (Rust + CGo toolchain matching) | Medium | Own it in CI (Phase 5); Windows local provider ships only when green |
| macOS Intel: we own an ORT source build (no official prebuilt ≥1.24) | Medium | Build exact-match 1.26.0 from source (eliminates header/runtime mismatch); checksummed in manifest; one-time CI cost, refreshed only on deliberate ORT upgrades |
| Search `threshold` default (1.5) tuned for OpenAI distance distribution; mE5 cosine distances distribute differently | Medium | Phase 0 measures typical distances; make the default provider-aware if needed (engine already centralizes the default) |
| Granite spike fails (ModernBERT/ORT-1.26 or arm64-quant issues) | Low | mE5-small is the committed default; Granite is upside, not dependency |
| Two processes × model RSS (~400 MB combined worst case) | Low | Lazy load in stdio; document; small model keeps it bounded |
| Prefixes leaking into stored content | Medium | Prefix strictly inside `LocalEmbedder`; test asserts stored chunks are prefix-free |
| Binary size ~180 MB | Accepted | Decision made 2026-07-02; the trade for zero API cost |
| int8 quantization quality drop vs fp32 | Low | Phase 0 sanity comparison; fp32 fallback possible at 449 MB if unacceptable |

## Open Concerns & Plan Adjustments (from review + Phase 0)

Captured 2026-07-06 during pre-implementation review and the Phase 0 spike. Each item is tagged to
the phase that must address it, so nothing is lost mid-epic.

| # | Concern | Fix in | Note |
|---|---|---|---|
| 1 | **Chunker not swapped on runtime provider switch.** `engine` has `SetEmbedder` but no `SetChunker`; `app.SetConfig` swaps only the embedder — switching OpenAI↔local at runtime would leave the wrong tokenizer/clamp. | **Phase 3** | Add `engine.SetChunker` (or a combined provider swap); `SetConfig` provider case swaps embedder + chunker atomically. |
| 2 | **Read-only stdio dimension mismatch unhandled.** `readonly.Search` embeds the query with a config-derived embedder; if its dim disagrees with the stored vectors, sqlite-vec errors and the read-only process cannot re-index. | **Phase 3** | stdio compares embedder `Dimensions()`/fingerprint to the stored table; return an actionable error ("rebuild in the GUI"), not a raw failure. |
| 3 | **Existing-user onboarding regression.** Switching the gate to `onboarding_complete` re-onboards current users and defaults them to local, mismatching their OpenAI vectors. | **Phase 4** | Migration: if the DB has watched dirs or a saved key, back-fill `onboarding_complete=true` and preserve `provider=openai` for existing OpenAI DBs. |
| 4 | **Scattered default-model/dimension hardcodes** (`"text-embedding-3-small"` ×5, `1536` ×2). | **Phase 3** (seed in Phase 1) | Centralize `defaultModel(provider)` / `defaultDimension(provider,model)`; stop trading a `1536`→`384` hardcode. |
| 5 | **Threshold `1.5` duplicated (~4 sites) and miscalibrated** for local (Phase 0: real separation ~0.25 cosine distance). | **Phase 3** | Centralize the default; make it provider-aware; confirm sqlite-vec's configured distance metric. |
| 6 | **mE5 needs `token_type_ids`** (3 INT64 inputs, not 2). | **Phase 2** | Pass a zero tensor in `LocalEmbedder`. |
| 7 | **Peak RSS ~1 GB** (ORT arena + batch activations). | **Phase 2** | Constrain ORT arena / batch size. |
| 8 | **`go:embed` must be build-tagged per platform** (one binary can embed only one platform's ORT lib). | **Phase 5** | Build constraints on the embed directives. |
| 9 | **`OpenAIEmbedder.MaxInputTokens()==0` ("no limit")** — footgun if `chunk_size` ever exceeds OpenAI's 8191. | Low priority | Documented trade-off; add a comment. |

## Out of Scope (this epic)

- Ollama / external-runtime provider option
- Per-directory or per-file-type model selection
- Re-ranking, hybrid (keyword+vector) search
- Distance-metric selection UI (existing roadmap item; interacts with normalization — note kept there)

## Sources

- onnxruntime_go: https://github.com/yalue/onnxruntime_go (MIT, v1.31.0, ORT 1.26 headers)
- daulet/tokenizers: https://github.com/daulet/tokenizers (MIT, v1.27.0; Windows buildable, no prebuilt)
- sugarme/tokenizer (rejected — no Unigram): https://github.com/sugarme/tokenizer
- hugot (rejected — heavy/Linux-centric; reference for pooling): https://github.com/knights-analytics/hugot
- multilingual-e5-small: https://huggingface.co/intfloat/multilingual-e5-small (MIT; prefixes required)
- Xenova int8 ONNX export: https://huggingface.co/Xenova/multilingual-e5-small (~113 MB)
- E5 technical report: https://arxiv.org/abs/2402.05672
- bge-small-en-v1.5 (English baseline): https://huggingface.co/BAAI/bge-small-en-v1.5
- granite-embedding-97m-multilingual-r2: https://huggingface.co/ibm-granite/granite-embedding-97m-multilingual-r2 (Apache-2.0)
- ONNX Runtime releases (macOS x64 dropped after 1.23.0): https://github.com/microsoft/onnxruntime/releases
