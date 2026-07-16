//go:build localembed && darwin && arm64

package local

import "embed"

// Per-platform ONNX Runtime shared library (macOS arm64). One binary can embed
// only one platform's ORT lib (epic Open Concern #8), so each Phase 5 target
// adds a sibling of this file — the embed directive, the FS var, and the
// ortLibFile constant are the ONLY platform-specific pieces; everything else in
// this package is shared.
//
//go:embed embedded/libonnxruntime.1.26.0.dylib
var embeddedORTLib embed.FS

// ortLibFile is the base name of this platform's embedded ONNX Runtime shared
// library, both inside embedded/ and after extraction to the runtime dir.
const ortLibFile = "libonnxruntime.1.26.0.dylib"
