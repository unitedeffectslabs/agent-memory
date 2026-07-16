.PHONY: build dev test clean assets

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

build: assets
	CGO_LDFLAGS="-L$(PWD)/$(LIB_DIR) -ltokenizers" wails build -skipbindings -tags localembed

dev: assets
	CGO_LDFLAGS="-L$(PWD)/$(LIB_DIR) -ltokenizers" wails dev -tags localembed

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
	    tar xzf "$$tmp" -C "$$xd" "$$member"; \
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
