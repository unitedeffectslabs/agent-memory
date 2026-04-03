package extractor

// Extractor extracts text content from files for indexing.
type Extractor interface {
	Extract(path string) (Result, error)
	IsSupported(path string) bool
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
