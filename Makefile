.PHONY: build build-darwin-amd64 dev test clean assets winhdr

# --- Local-embedding asset bundling -----------------------------------------
# Artifacts (model, tokenizer, ONNX Runtime dylib, static tokenizer lib) are
# pinned by URL + SHA-256 in assets/manifest.json and downloaded by `make
# assets` into the local embedding package (gitignored). They are never
# committed. `make build` links the tokenizer static lib via CGO_LDFLAGS and
# compiles with the `localembed` tag (which go:embed's the runtime assets).
MANIFEST  := assets/manifest.json
LOCAL_DIR := internal/embeddings/local
EMBED_DIR := $(LOCAL_DIR)/embedded
LIB_DIR   := $(LOCAL_DIR)/lib
PLATFORM  := $(shell go env GOOS)-$(shell go env GOARCH)
# Portable SHA-256: macOS ships shasum, Linux ships sha256sum. Both print
# "<hash>  <file>", so the awk '{print $1}' callers work with either.
SHA256    := $(shell command -v sha256sum >/dev/null 2>&1 && echo sha256sum || echo shasum -a 256)

# Windows-only: the source-built libtokenizers.a (Rust std, GNU toolchain) pulls
# in low-level NT/Winsock/crypto syscalls that MinGW does not link by default.
# Naming them here resolves "undefined reference to Nt*/Rtl*" at link time.
# Empty on macOS/Linux, so those builds are unaffected.
LINK_LIBS := -ltokenizers
CGO_EXTRA_CFLAGS :=
WIN_PREREQ :=
ifeq ($(shell go env GOOS),windows)
LINK_LIBS += -lntdll -lws2_32 -lbcrypt -luserenv -ladvapi32 -lkernel32 -lncrypt
# sqlite-vec's cgo build #includes sqlite3.h / sqlite3ext.h, which macOS and
# Linux supply from the system but Windows does not. Stage the exact headers
# mattn/go-sqlite3 bundles (its sqlite3-binding.h IS the amalgamation sqlite3.h),
# so they match the SQLite that mattn compiles in — see the winhdr target.
CGO_EXTRA_CFLAGS := -I$(PWD)/build/winhdr
WIN_PREREQ := winhdr
endif

# Stage sqlite headers for the Windows build from the mattn/go-sqlite3 module
# (Windows has no system sqlite3.h). No-op / unused on macOS and Linux.
winhdr:
	@mkdir -p "$(PWD)/build/winhdr"
	@d=$$(go list -m -f '{{.Dir}}' github.com/mattn/go-sqlite3); \
	  d=$$(cygpath -u "$$d" 2>/dev/null || echo "$$d"); \
	  cp "$$d/sqlite3-binding.h" "$(PWD)/build/winhdr/sqlite3.h"; \
	  cp "$$d/sqlite3ext.h" "$(PWD)/build/winhdr/sqlite3ext.h"; \
	  echo ">> staged Windows sqlite headers from $$d"

build: assets $(WIN_PREREQ)
	CGO_CFLAGS="$(CGO_EXTRA_CFLAGS)" CGO_LDFLAGS="-L$(PWD)/$(LIB_DIR) $(LINK_LIBS)" wails build -skipbindings -tags localembed

# Cross-build the Intel-mac app from an arm64 Mac. GOARCH=amd64 makes the
# assets target fetch the darwin-amd64 artifacts (the embedded/ and lib/ dirs
# hold ONE platform at a time — the checksum check refetches on arch switch,
# so alternating with `make build` is safe, just re-downloads).
build-darwin-amd64:
	GOARCH=amd64 $(MAKE) assets
	CGO_LDFLAGS="-L$(PWD)/$(LIB_DIR) -ltokenizers" wails build -skipbindings -tags localembed -platform darwin/amd64

dev: assets $(WIN_PREREQ)
	CGO_CFLAGS="$(CGO_EXTRA_CFLAGS)" CGO_LDFLAGS="-L$(PWD)/$(LIB_DIR) $(LINK_LIBS)" wails dev -tags localembed

test:
	go test ./...

clean:
	rm -rf build/bin

# Download + checksum-verify each manifest artifact for the current platform.
# Idempotent: files already present with a matching checksum are skipped.
assets:
	@echo ">> fetching local-embedding assets for $(PLATFORM)"
	@command -v jq >/dev/null || { echo "ERROR: jq is required"; exit 1; }
	@test -f $(MANIFEST) || { echo "ERROR: $(MANIFEST) not found"; exit 1; }
	@jq -e '.platforms["$(PLATFORM)"]' $(MANIFEST) >/dev/null 2>&1 || \
		{ echo "ERROR: no manifest entry for platform $(PLATFORM)"; exit 1; }
	@mkdir -p $(EMBED_DIR) $(LIB_DIR)
	@set -e; \
	for key in $$(jq -r '.platforms["$(PLATFORM)"] | keys[]' $(MANIFEST)); do \
	  sel() { jq -r ".platforms[\"$(PLATFORM)\"].$$key.$$1 // empty" $(MANIFEST); }; \
	  url=$$(sel url); sha=$$(sel sha256); dest=$$(sel dest); \
	  member=$$(sel member); membersha=$$(sel member_sha256); \
	  destpath=$(LOCAL_DIR)/$$dest; \
	  want=$$sha; [ -n "$$membersha" ] && want=$$membersha; \
	  if [ -f "$$destpath" ]; then \
	    have=$$($(SHA256) "$$destpath" | awk '{print $$1}'); \
	    if [ "$$have" = "$$want" ]; then echo "   ok (cached)  $$dest"; continue; fi; \
	    echo "   stale, refetching  $$dest"; \
	  fi; \
	  echo "   downloading  $$key -> $$dest"; \
	  tmp=$$(mktemp); \
	  curl -fsSL -o "$$tmp" "$$url"; \
	  got=$$($(SHA256) "$$tmp" | awk '{print $$1}'); \
	  if [ "$$got" != "$$sha" ]; then \
	    echo "ERROR: archive checksum mismatch for $$key: got $$got want $$sha"; rm -f "$$tmp"; exit 1; \
	  fi; \
	  mkdir -p "$$(dirname "$$destpath")"; \
	  if [ -n "$$member" ]; then \
	    xd=$$(mktemp -d); \
	    case "$$url" in \
	      *.zip) if command -v unzip >/dev/null 2>&1; then unzip -q "$$tmp" "$$member" -d "$$xd"; \
	             elif [ -x /c/Windows/System32/tar.exe ]; then /c/Windows/System32/tar.exe xf "$$tmp" -C "$$xd" "$$member"; \
	             else tar xf "$$tmp" -C "$$xd" "$$member"; fi ;; \
	      *) tar xzf "$$tmp" -C "$$xd" "$$member" ;; \
	    esac; \
	    cp "$$xd/$$member" "$$destpath"; \
	    rm -rf "$$xd"; \
	    got2=$$($(SHA256) "$$destpath" | awk '{print $$1}'); \
	    if [ "$$got2" != "$$membersha" ]; then \
	      echo "ERROR: member checksum mismatch for $$key: got $$got2 want $$membersha"; rm -f "$$tmp"; exit 1; \
	    fi; \
	  else \
	    cp "$$tmp" "$$destpath"; \
	  fi; \
	  rm -f "$$tmp"; \
	  echo "   verified     $$dest"; \
	done
	@echo ">> assets ready in $(EMBED_DIR) and $(LIB_DIR)"
