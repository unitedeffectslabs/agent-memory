package local

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Asset file names expected inside the assets directory.
const (
	assetModelFile     = "model_quantized.onnx"
	assetTokenizerFile = "tokenizer.json"

	// assetsDirEnv is the environment variable naming the directory that holds
	// the model, tokenizer and ONNX Runtime shared library during development
	// (Phase 2). Phase 3 replaces this with go:embed + checksummed extraction.
	assetsDirEnv = "AGENT_MEMORY_LOCAL_ASSETS"
)

// dylibCandidates are the ONNX Runtime shared-library file names we look for,
// most specific first. Used only for developer-override directories
// (Config.AssetsDir / AGENT_MEMORY_LOCAL_ASSETS); the bundled path resolves the
// per-platform ortLibFile directly. The Linux release tarball ships the
// versioned name (libonnxruntime.so.1.26.0), hence both .so forms.
var dylibCandidates = []string{
	"libonnxruntime.1.26.0.dylib",
	"libonnxruntime.dylib",
	"libonnxruntime.so.1.26.0",
	"libonnxruntime.so",
	"onnxruntime.dll",
}

// resolveAssets locates the model, tokenizer and ONNX Runtime shared library.
//
// Priority:
//  1. A developer override — cfg.AssetsDir, falling back to the
//     AGENT_MEMORY_LOCAL_ASSETS environment variable — points at a directory
//     that already holds the three files.
//  2. Otherwise, the assets bundled into the binary via go:embed (localembed
//     build only) are extracted to ~/.agent-memory/runtime/<fingerprint>/ and
//     resolved from there. In the default (untagged) build no assets are
//     embedded, so extractEmbeddedAssets returns an error and resolveAssets
//     fails with an actionable message.
func resolveAssets(cfg Config) (modelPath, tokPath, dylibPath string, err error) {
	base := cfg.AssetsDir
	if base == "" {
		base = os.Getenv(assetsDirEnv)
	}
	if base == "" {
		// No dev override: fall back to the go:embed-ed, extracted assets.
		extracted, eerr := extractEmbeddedAssets()
		if eerr != nil {
			return "", "", "", eerr
		}
		base = extracted
	}
	return resolveFromDir(base)
}

// resolveFromDir resolves the three asset paths from a directory, requiring the
// model and tokenizer files to be present and at least one ONNX Runtime shared
// library to be found.
func resolveFromDir(base string) (modelPath, tokPath, dylibPath string, err error) {
	modelPath = filepath.Join(base, assetModelFile)
	tokPath = filepath.Join(base, assetTokenizerFile)

	if err := mustExist(modelPath); err != nil {
		return "", "", "", err
	}
	if err := mustExist(tokPath); err != nil {
		return "", "", "", err
	}

	dylibPath, err = findDylib(base)
	if err != nil {
		return "", "", "", err
	}
	return modelPath, tokPath, dylibPath, nil
}

// findDylib returns the first ONNX Runtime shared library found under base.
func findDylib(base string) (string, error) {
	for _, name := range dylibCandidates {
		p := filepath.Join(base, name)
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("local: no ONNX Runtime shared library found in %s (looked for %v)",
		base, dylibCandidates)
}

func mustExist(path string) error {
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("local: required asset missing: %s: %w", path, err)
	}
	return nil
}

// fingerprint returns a short, stable identifier for a provider/model/dimension
// triple, used to namespace the on-disk runtime extraction directory.
func fingerprint(provider, model string, dim int) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s:%s:%d", provider, model, dim)))
	return hex.EncodeToString(sum[:])[:16]
}

// extractAndVerify copies srcPath into destDir atomically (write to a temp file
// then rename), verifying the SHA-256 checksum. If wantSHA is empty the checksum
// is computed and not enforced. The operation is idempotent: if the destination
// already exists with a matching checksum it is left untouched. Returns the
// final destination path.
//
// Phase 2 sources assets from a developer directory and does not require
// extraction; this helper exists for Phase 3's go:embed-backed extraction and is
// unit-tested here so the behavior is pinned early.
func extractAndVerify(srcPath, destDir, wantSHA string) (string, error) {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", fmt.Errorf("local: create runtime dir: %w", err)
	}
	destPath := filepath.Join(destDir, filepath.Base(srcPath))

	// Idempotent fast path: destination already present and (if requested)
	// matches the wanted checksum.
	if existing, err := sha256File(destPath); err == nil {
		if wantSHA == "" || existing == wantSHA {
			return destPath, nil
		}
	}

	srcSum, err := sha256File(srcPath)
	if err != nil {
		return "", fmt.Errorf("local: hash source: %w", err)
	}
	if wantSHA != "" && srcSum != wantSHA {
		return "", fmt.Errorf("local: checksum mismatch for %s: got %s want %s",
			srcPath, srcSum, wantSHA)
	}

	tmp, err := os.CreateTemp(destDir, ".tmp-*")
	if err != nil {
		return "", fmt.Errorf("local: temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op after successful rename

	src, err := os.Open(srcPath)
	if err != nil {
		tmp.Close()
		return "", fmt.Errorf("local: open source: %w", err)
	}
	if _, err := io.Copy(tmp, src); err != nil {
		src.Close()
		tmp.Close()
		return "", fmt.Errorf("local: copy asset: %w", err)
	}
	src.Close()
	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("local: close temp: %w", err)
	}

	if err := os.Rename(tmpName, destPath); err != nil {
		return "", fmt.Errorf("local: rename into place: %w", err)
	}
	return destPath, nil
}

// sha256File returns the hex-encoded SHA-256 of the file at path.
func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
