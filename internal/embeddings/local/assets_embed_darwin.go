//go:build localembed && darwin

package local

import "embed"

// Per-platform ONNX Runtime shared library (macOS). One binary can embed only
// one platform's ORT lib (epic Open Concern #8), so each Phase 5 target adds a
// sibling of this file — the embed directive, the FS var, and the ortLibFile
// constant are the ONLY platform-specific pieces; everything else in this
// package is shared. Both mac arches ship the same dylib file name — arm64 from
// the official ORT release, x86_64 from our own source build (official mac-Intel
// prebuilts stopped at 1.23) hosted in this repo's GitHub releases; the arch
// difference is which artifact `make assets` downloads (assets/manifest.json).
//
//go:embed embedded/libonnxruntime.1.26.0.dylib
var embeddedORTLib embed.FS

// ortLibFile is the base name of this platform's embedded ONNX Runtime shared
// library, both inside embedded/ and after extraction to the runtime dir.
const ortLibFile = "libonnxruntime.1.26.0.dylib"
