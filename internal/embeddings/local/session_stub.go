//go:build !localembed

package local

// newORTSession is the default-build stub. Building with the `localembed` tag
// (and linking ONNX Runtime) provides the real implementation.
func newORTSession(dylibPath, modelPath string, threads int) (onnxSession, error) {
	return nil, errLocalTagRequired
}
