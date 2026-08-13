//go:build localembed

package local

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// embeddedAssets carries the platform-independent runtime assets compiled into
// the binary (model + tokenizer — identical bytes on every platform). They are
// downloaded and checksum-verified by `make assets` into ./embedded/ before the
// tagged build compiles (the go:embed directive requires the files to exist).
// The platform-specific ONNX Runtime shared library is embedded separately in
// the per-platform assets_embed_<GOOS>_<GOARCH>.go file (embeddedORTLib /
// ortLibFile).
//
// NOTE: go:embed can only reach files inside this package's own directory tree,
// so the runtime assets live under internal/embeddings/local/embedded/ — NOT the
// repo-root assets/ directory. This is a deliberate deviation from the epic's
// documented assets/embedded/ path.
//
// The static tokenizer library (libtokenizers.a) is linked at build time via
// CGO_LDFLAGS and is deliberately NOT embedded here.
//
//go:embed embedded/model_quantized.onnx
//go:embed embedded/tokenizer.json
var embeddedAssets embed.FS

// assetsEmbedded reports whether bundled assets are compiled into this build.
// True here (localembed); the stub sets it false. Tests use it to distinguish
// the "no override configured" outcome across the two builds.
const assetsEmbedded = true

// embeddedAssetSources pairs each runtime asset's embed.FS with its path, in
// extraction order. Each is written to disk under its base name. Model and
// tokenizer come from the shared embeddedAssets; the ONNX Runtime library comes
// from the per-platform embeddedORTLib.
var embeddedAssetSources = []struct {
	fs   *embed.FS
	path string
}{
	{&embeddedAssets, "embedded/" + assetModelFile},
	{&embeddedAssets, "embedded/" + assetTokenizerFile},
	{&embeddedORTLib, "embedded/" + ortLibFile},
}

// extractEmbeddedAssets materializes the go:embed-ed runtime assets into
// ~/.agent-memory/runtime/<fingerprint>/ and returns that directory. Extraction
// is atomic (write-temp-then-rename), checksum-computed and idempotent — reusing
// extractAndVerify from assets.go — so concurrent GUI/stdio processes are safe
// and a warm run costs only stat+hash.
func extractEmbeddedAssets() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("local: locate home dir: %w", err)
	}
	// The directory is namespaced by GOOS-GOARCH in addition to the model
	// fingerprint: the ORT shared library is architecture-specific, and two
	// builds sharing one home directory is a real scenario (an Intel-mac build
	// under Rosetta, or a home directory migrated from an Intel Mac). Without
	// the arch in the path, the first build's extraction poisons the second's
	// dlopen with an incompatible-architecture error.
	destDir := filepath.Join(home, ".agent-memory", "runtime",
		fmt.Sprintf("%s-%s-%s", runtime.GOOS, runtime.GOARCH,
			fingerprint("local", modelName, modelDim)))

	for _, src := range embeddedAssetSources {
		embedPath := src.path
		data, err := src.fs.ReadFile(embedPath)
		if err != nil {
			return "", fmt.Errorf("local: read embedded asset %s: %w", embedPath, err)
		}

		// extractAndVerify copies from a source *file* into destDir under the
		// same base name, so stage the embedded bytes to a temp file named after
		// the asset first.
		base := filepath.Base(embedPath)
		stageDir, err := os.MkdirTemp("", "agent-memory-embed-*")
		if err != nil {
			return "", fmt.Errorf("local: stage dir: %w", err)
		}
		stagePath := filepath.Join(stageDir, base)
		if err := os.WriteFile(stagePath, data, 0o644); err != nil {
			os.RemoveAll(stageDir)
			return "", fmt.Errorf("local: stage embedded asset %s: %w", base, err)
		}

		// wantSHA is empty: the bytes are already trusted (compiled into the
		// binary); extractAndVerify still hashes for its idempotent fast path.
		_, err = extractAndVerify(stagePath, destDir, "")
		os.RemoveAll(stageDir)
		if err != nil {
			return "", err
		}
	}
	return destDir, nil
}
