package extractor

import (
	"archive/zip"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nguyenthenguyen/docx"
	"github.com/xuri/excelize/v2"
)

// Result holds extracted text and whether the file was content-indexed or metadata-only.
type Result struct {
	Text       string
	IsMetadata bool // true if only filesystem/format metadata was extracted
}

// textExtensions are file types where the raw file content is the text to index.
// Weighted toward documents, notes, and knowledge artifacts (agent memory use case)
// plus common code files.
var textExtensions = map[string]bool{
	// Documents & notes
	".md": true, ".txt": true, ".rtf": true, ".org": true, ".rst": true,
	".tex": true, ".latex": true, ".adoc": true, ".wiki": true,
	// Data & config
	".json": true, ".csv": true, ".tsv": true,
	".yml": true, ".yaml": true, ".toml": true, ".xml": true,
	".ini": true, ".cfg": true, ".conf": true, ".properties": true,
	".plist": true,
	// Web
	".html": true, ".htm": true, ".css": true, ".scss": true, ".less": true,
	// Code (common)
	".go": true, ".py": true, ".js": true, ".ts": true, ".jsx": true, ".tsx": true,
	".sql": true, ".sh": true, ".bash": true, ".zsh": true,
	".r": true, ".rs": true, ".java": true, ".c": true, ".h": true,
	".cpp": true, ".hpp": true, ".cs": true, ".rb": true, ".php": true,
	".swift": true, ".kt": true, ".scala": true, ".m": true,
	".svelte": true, ".vue": true,
	// Logs & transcripts
	".log": true, ".srt": true, ".vtt": true,
	// Misc text
	".bib": true, ".ris": true, ".enw": true, // bibliography
}

// binaryExtractors are file types with special extraction logic.
var binaryExtractors = map[string]func(path string) (Result, error){
	".docx": extractDocx,
	".xlsx": extractXlsx,
	".pptx": extractPptx,
	".pdf":  extractPDFPassthrough,
}

// metadataExtensions are file types where we index filesystem + format metadata.
var metadataExtensions = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".gif": true,
	".bmp": true, ".svg": true, ".webp": true, ".tiff": true, ".tif": true,
	".zip": true, ".tar": true, ".gz": true, ".rar": true, ".7z": true,
	".mp4": true, ".mov": true, ".avi": true, ".mkv": true, ".webm": true,
	".mp3": true, ".wav": true, ".flac": true, ".aac": true, ".ogg": true,
}

// Extract returns the text content for a file. For text files it reads the content
// directly. For binary document formats it extracts text. For images/archives/media
// it returns filesystem and format metadata.
// Returns empty Result if the file type is completely unsupported.
func extract(path string) (Result, error) {
	ext := strings.ToLower(filepath.Ext(path))

	// Text files: read directly
	if textExtensions[ext] {
		data, err := os.ReadFile(path)
		if err != nil {
			return Result{}, err
		}
		return Result{Text: string(data)}, nil
	}

	// Binary document formats with extractors
	if fn, ok := binaryExtractors[ext]; ok {
		return fn(path)
	}

	// Metadata-only files
	if metadataExtensions[ext] {
		return extractMetadata(path, ext)
	}

	return Result{}, nil // unsupported
}

// IsSupported reports whether the file extension is handled (text, binary, or metadata).
func isSupported(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return textExtensions[ext] || binaryExtractors[ext] != nil || metadataExtensions[ext]
}

// extractDocx extracts text from a .docx file.
func extractDocx(path string) (Result, error) {
	r, err := docx.ReadDocxFile(path)
	if err != nil {
		return Result{}, fmt.Errorf("read docx: %w", err)
	}
	defer r.Close()

	doc := r.Editable()
	text := doc.GetContent()
	// Strip XML tags that the library sometimes leaves in
	text = stripXMLTags(text)
	return Result{Text: text}, nil
}

// extractXlsx extracts text from all sheets of an .xlsx file.
func extractXlsx(path string) (Result, error) {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return Result{}, fmt.Errorf("open xlsx: %w", err)
	}
	defer f.Close()

	var sb strings.Builder
	for _, sheet := range f.GetSheetList() {
		sb.WriteString(fmt.Sprintf("=== Sheet: %s ===\n", sheet))
		rows, err := f.GetRows(sheet)
		if err != nil {
			continue
		}
		for _, row := range rows {
			sb.WriteString(strings.Join(row, "\t"))
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}
	return Result{Text: sb.String()}, nil
}

// extractPptx extracts text from all slides of a .pptx file.
func extractPptx(path string) (Result, error) {
	r, err := zip.OpenReader(path)
	if err != nil {
		return Result{}, fmt.Errorf("open pptx: %w", err)
	}
	defer r.Close()

	var sb strings.Builder
	slideNum := 0
	for _, f := range r.File {
		if !strings.HasPrefix(f.Name, "ppt/slides/slide") || !strings.HasSuffix(f.Name, ".xml") {
			continue
		}
		slideNum++
		rc, err := f.Open()
		if err != nil {
			continue
		}
		data := make([]byte, f.UncompressedSize64)
		n, _ := rc.Read(data)
		rc.Close()
		text := stripXMLTags(string(data[:n]))
		text = strings.TrimSpace(text)
		if text != "" {
			sb.WriteString(fmt.Sprintf("=== Slide %d ===\n%s\n\n", slideNum, text))
		}
	}
	return Result{Text: sb.String()}, nil
}

// extractPDFPassthrough reads the PDF file as-is. The chunker already handles
// PDF text via the content passed from engine.IndexFile (os.ReadFile).
// This is a passthrough so the extractor pipeline handles it consistently.
func extractPDFPassthrough(path string) (Result, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Result{}, err
	}
	return Result{Text: string(data)}, nil
}

// extractMetadata builds a text description from file metadata and format-specific info.
func extractMetadata(path string, ext string) (Result, error) {
	info, err := os.Stat(path)
	if err != nil {
		return Result{}, err
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("File: %s\n", filepath.Base(path)))
	sb.WriteString(fmt.Sprintf("Path: %s\n", path))
	sb.WriteString(fmt.Sprintf("Type: %s\n", describeType(ext)))
	sb.WriteString(fmt.Sprintf("Size: %s\n", formatSize(info.Size())))
	sb.WriteString(fmt.Sprintf("Modified: %s\n", info.ModTime().Format(time.RFC3339)))
	sb.WriteString(fmt.Sprintf("Directory: %s\n", filepath.Dir(path)))

	// Format-specific metadata
	switch {
	case isImage(ext):
		if dims := imageDimensions(path); dims != "" {
			sb.WriteString(fmt.Sprintf("Dimensions: %s\n", dims))
		}
	case isArchive(ext):
		if contents := zipContents(path); contents != "" {
			sb.WriteString(fmt.Sprintf("Contents:\n%s\n", contents))
		}
	}

	return Result{Text: sb.String(), IsMetadata: true}, nil
}

func isImage(ext string) bool {
	return ext == ".jpg" || ext == ".jpeg" || ext == ".png" || ext == ".gif" ||
		ext == ".bmp" || ext == ".webp" || ext == ".tiff" || ext == ".tif" || ext == ".svg"
}

func isArchive(ext string) bool {
	return ext == ".zip" || ext == ".tar" || ext == ".gz" || ext == ".rar" || ext == ".7z"
}

func imageDimensions(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%d x %d", cfg.Width, cfg.Height)
}

func zipContents(path string) string {
	r, err := zip.OpenReader(path)
	if err != nil {
		return ""
	}
	defer r.Close()
	var lines []string
	for i, f := range r.File {
		if i >= 100 { // cap at 100 entries
			lines = append(lines, fmt.Sprintf("  ... and %d more files", len(r.File)-100))
			break
		}
		lines = append(lines, fmt.Sprintf("  %s (%s)", f.Name, formatSize(int64(f.UncompressedSize64))))
	}
	return strings.Join(lines, "\n")
}

func describeType(ext string) string {
	types := map[string]string{
		".jpg": "JPEG image", ".jpeg": "JPEG image", ".png": "PNG image",
		".gif": "GIF image", ".bmp": "BMP image", ".svg": "SVG image",
		".webp": "WebP image", ".tiff": "TIFF image", ".tif": "TIFF image",
		".zip": "ZIP archive", ".tar": "TAR archive", ".gz": "Gzip archive",
		".rar": "RAR archive", ".7z": "7-Zip archive",
		".mp4": "MP4 video", ".mov": "QuickTime video", ".avi": "AVI video",
		".mkv": "Matroska video", ".webm": "WebM video",
		".mp3": "MP3 audio", ".wav": "WAV audio", ".flac": "FLAC audio",
		".aac": "AAC audio", ".ogg": "Ogg audio",
	}
	if t, ok := types[ext]; ok {
		return t
	}
	return strings.TrimPrefix(ext, ".") + " file"
}

func formatSize(bytes int64) string {
	switch {
	case bytes >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(bytes)/float64(1<<30))
	case bytes >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(1<<20))
	case bytes >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(bytes)/float64(1<<10))
	default:
		return fmt.Sprintf("%d bytes", bytes)
	}
}

func stripXMLTags(s string) string {
	var b strings.Builder
	inTag := false
	for _, r := range s {
		if r == '<' {
			inTag = true
			continue
		}
		if r == '>' {
			inTag = false
			continue
		}
		if !inTag {
			b.WriteRune(r)
		}
	}
	return b.String()
}
