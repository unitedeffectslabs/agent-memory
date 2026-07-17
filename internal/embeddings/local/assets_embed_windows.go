//go:build localembed && windows

package local

import "embed"

// Per-platform ONNX Runtime shared library (Windows x64). One binary can embed
// only one platform's ORT lib (epic Open Concern #8); the official Microsoft
// release ships the DLL — see assets/manifest.json (windows-amd64) and
// Documentation/windows-build.md for the full Windows build procedure
// (the static tokenizer lib has no upstream prebuilt and is built from Rust
// source there).
//
//go:embed embedded/onnxruntime.dll
var embeddedORTLib embed.FS

// ortLibFile is the base name of this platform's embedded ONNX Runtime shared
// library, both inside embedded/ and after extraction to the runtime dir.
const ortLibFile = "onnxruntime.dll"
