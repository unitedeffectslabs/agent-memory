//go:build localembed && linux

package local

import "embed"

// Per-platform ONNX Runtime shared library (Linux). One binary can embed only
// one platform's ORT lib (epic Open Concern #8). Both Linux arches ship the
// same versioned file name — the arm64/x64 distinction is which artifact
// `make assets` downloads into embedded/ (see assets/manifest.json), so a
// single build-constrained file covers linux/arm64 and linux/amd64.
//
//go:embed embedded/libonnxruntime.so.1.26.0
var embeddedORTLib embed.FS

// ortLibFile is the base name of this platform's embedded ONNX Runtime shared
// library, both inside embedded/ and after extraction to the runtime dir.
const ortLibFile = "libonnxruntime.so.1.26.0"
