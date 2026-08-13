//go:build !localembed

package local

// newHFTokenizer is the default-build stub. Building with the `localembed` tag
// (and linking libtokenizers) provides the real implementation.
func newHFTokenizer(tokPath string) (tokenizerBackend, error) {
	return nil, errLocalTagRequired
}
