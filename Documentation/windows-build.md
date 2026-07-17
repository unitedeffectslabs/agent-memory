# Windows x64 Build & Artifact Provenance (local-embeddings, Phase 5)

How the Windows local-embedding build is produced and how to reproduce its one
source-built artifact. Records what was actually run (versions below are what
shipped), so a future maintainer can rebuild after a dependency bump.

## Why Windows needs a build step at all

Every other platform gets both native libraries as official prebuilts. Windows
is the exception on one of them:

| Artifact | Windows source |
|---|---|
| ONNX Runtime DLL | **Official** Microsoft release (`onnxruntime-win-x64-1.26.0.zip`) — pinned directly in `assets/manifest.json`. |
| `libtokenizers.a` (static) | **No upstream Windows prebuilt** — `daulet/tokenizers` publishes macOS/Linux only. We build it from source and host it in this repo's releases (`tokenizers-1.27.0-windows-x64`). |

## Toolchain (installed via scoop)

```
scoop install go mingw rustup-gnu make
rustup default stable-x86_64-pc-windows-gnu
```

Verified versions: Go 1.26.5, MinGW gcc 16.1.0, rustc/cargo 1.97.1 (GNU),
GNU make 4.4.1.

**The GNU pairing is load-bearing.** CGo links the Go binary with MinGW `gcc`,
so the Rust static lib must come from the `*-windows-gnu` toolchain. Building it
with the default MSVC toolchain yields a `.lib` in a format MinGW cannot link.

## Reproducing `libtokenizers.a`

```
git clone --depth 1 --branch v1.27.0 https://github.com/daulet/tokenizers
cd tokenizers
cargo build --release
# Output: target/release/libtokenizers_ffi.a  (~40 MB)
```

Note the **name change**: this repo layout (v1.27.0) emits `libtokenizers_ffi.a`
from the `tokenizers-ffi` crate. The macOS/Linux prebuilts — and therefore the
`-ltokenizers` link flag and the manifest's `member: libtokenizers.a` — expect
`libtokenizers.a`. Same archive, so we simply rename on packaging:

```
cp target/release/libtokenizers_ffi.a libtokenizers.a
tar czf libtokenizers.windows-x86_64.tar.gz libtokenizers.a
# publish as a repo release, pin archive + member SHA-256 in the manifest
```

## Building the app

```
git clone https://github.com/unitedeffectslabs/agent-memory
cd agent-memory && git checkout feat/xplat-windows
cd frontend && npm install && cd ..
make assets      # downloads DLL + model + tokenizer + our published static lib; verifies all checksums
make build       # wails build -skipbindings -tags localembed  →  build/bin/agent-memory.exe
```

`make assets` extracts the `.zip` ORT archive via `unzip` if present, else falls
back to Windows' bundled `C:\Windows\System32\tar.exe` (Git Bash's GNU tar
cannot read zips). Checksums use `sha256sum` if present, else `shasum -a 256`
(both provided by Git for Windows).

## Verification (the Phase 5 gate)

1. Integration test — real tokenizer + ORT + model, as a native Windows binary:
   ```
   CGO_LDFLAGS="-L$PWD/internal/embeddings/local/lib -ltokenizers" \
     go test -tags localembed -count=1 -run TestIntegrationEmbed -v ./internal/embeddings/local/
   ```
   Expect PASS, cosine ordering ≈ related 0.14 < cross-lingual 0.17 < unrelated
   0.29 (matches macOS arm64/x86_64 and Linux).
2. Offline app run: disconnect network, `agent-memory.exe --db test.db`, onboard
   with **no API key**, index a small folder, confirm search returns matches.

## Troubleshooting (observed / anticipated)

- **Missing Windows system symbols at link** (`undefined reference to Nt*` /
  `Rtl*`, `ws2_32`, `bcrypt`, `userenv`): the Rust static lib pulls in NT/Winsock/
  crypto syscalls MinGW doesn't link by default. **Handled** — the Makefile's
  `LINK_LIBS` appends `-lntdll -lws2_32 -lbcrypt -luserenv -ladvapi32 -lkernel32
  -lncrypt` when `GOOS=windows` (empty elsewhere). Observed and fixed during the
  first Windows build; listed here in case a tokenizers/Rust bump adds more.
- **`onnxruntime_providers_shared.dll` load error**: the official zip ships this
  second DLL; CPU-only use normally doesn't need it, but if ORT fails to load,
  pin it as an extra `embedded/` artifact beside the main DLL.
- **Defender quarantines the fresh unsigned exe**: add a temp exclusion for the
  repo folder while testing.
