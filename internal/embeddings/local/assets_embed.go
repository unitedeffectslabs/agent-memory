//go:build localembed

package local

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
)

// embeddedAssets carries the runtime assets compiled into the binary. These are
// downloaded and checksum-verified by `make assets` into ./embedded/ before the
// tagged build compiles (the go:embed directive requires the files to exist).
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
//go:embed embedded/libonnxruntime.1.26.0.dylib
var embeddedAssets embed.FS

// assetsEmbedded reports whether bundled assets are compiled into this build.
// True here (localembed); the stub sets it false. Tests use it to distinguish
// the "no override configured" outcome across the two builds.
const assetsEmbedded = true

// embeddedAssetPaths are the embed.FS paths of the runtime assets, in the order
// they are extracted. Each is written to disk under its base name.
var embeddedAssetPaths = []string{
	"embedded/" + assetModelFile,
	"embedded/" + assetTokenizerFile,
	"embedded/" + dylibCandidates[0], // libonnxruntime.1.26.0.dylib
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
	destDir := filepath.Join(home, ".agent-memory", "runtime",
		fingerprint("local", modelName, modelDim))

	for _, embedPath := range embeddedAssetPaths {
		data, err := embeddedAssets.ReadFile(embedPath)
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
