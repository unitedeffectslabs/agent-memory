//go:build !localembed

package local

import "errors"

// assetsEmbedded reports whether bundled assets are compiled into this build.
// False here (default/untagged build); the localembed build sets it true.
const assetsEmbedded = false

// extractEmbeddedAssets is the default-build stub. No assets are embedded in the
// untagged build (no go:embed, so `go build ./...` needs no downloaded files and
// stays native-lib-free), so resolving assets without a developer override
// (Config.AssetsDir / AGENT_MEMORY_LOCAL_ASSETS) is an error here.
func extractEmbeddedAssets() (string, error) {
	return "", errors.New(
		"local: bundled assets available only in the 'localembed' build " +
			"(build with -tags localembed, or set " + assetsDirEnv + ")")
}
