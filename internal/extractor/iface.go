package extractor

// Extractor extracts text content from files for indexing.
type Extractor interface {
	Extract(path string) (Result, error)
	IsSupported(path string) bool
	// Version reports the version of the extraction logic that applies to this
	// file's type. The engine stores it beside the content hash and re-indexes
	// when it changes — an extractor fix is invisible to the content hash, so
	// without this, files already indexed with a buggy extractor would keep
	// their bad chunks forever (the hash short-circuit skips them).
	Version(path string) int
}

// FileExtractor is the concrete implementation of Extractor using the
// package-level extraction functions.
type FileExtractor struct{}

// NewFileExtractor creates a new FileExtractor.
func NewFileExtractor() *FileExtractor {
	return &FileExtractor{}
}

func (f *FileExtractor) Extract(path string) (Result, error) {
	return extract(path)
}

func (f *FileExtractor) IsSupported(path string) bool {
	return isSupported(path)
}

func (f *FileExtractor) Version(path string) int {
	return extractorVersion(path)
}
