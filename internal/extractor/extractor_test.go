package extractor

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeMinimalPDF builds a syntactically valid single-page PDF with an
// uncompressed text stream — offsets computed at build time, so the test
// carries no opaque binary fixture.
func writeMinimalPDF(t *testing.T, dir, text string) string {
	t.Helper()
	var buf bytes.Buffer
	offsets := make([]int, 6)
	write := func(s string) { buf.WriteString(s) }
	mark := func(i int) { offsets[i] = buf.Len() }

	write("%PDF-1.4\n")
	mark(1)
	write("1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n")
	mark(2)
	write("2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n")
	mark(3)
	write("3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>\nendobj\n")
	stream := fmt.Sprintf("BT /F1 12 Tf 72 720 Td (%s) Tj ET", text)
	mark(4)
	write(fmt.Sprintf("4 0 obj\n<< /Length %d >>\nstream\n%s\nendstream\nendobj\n", len(stream), stream))
	mark(5)
	write("5 0 obj\n<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>\nendobj\n")

	xref := buf.Len()
	write("xref\n0 6\n0000000000 65535 f \n")
	for i := 1; i <= 5; i++ {
		write(fmt.Sprintf("%010d 00000 n \n", offsets[i]))
	}
	write(fmt.Sprintf("trailer\n<< /Size 6 /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", xref))

	p := filepath.Join(dir, "sample.pdf")
	if err := os.WriteFile(p, buf.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestExtractPDFRealText is the regression test for the passthrough bug: the
// extractor must return the document's readable text, not the raw PDF bytes
// (which begin with %PDF and are full of dictionary/stream syntax).
func TestExtractPDFRealText(t *testing.T) {
	p := writeMinimalPDF(t, t.TempDir(), "Grandmother lasagna secret ingredient")

	res, err := extractPDF(p)
	if err != nil {
		t.Fatalf("extractPDF: %v", err)
	}
	if !strings.Contains(res.Text, "lasagna") {
		t.Fatalf("extracted text does not contain document content; got %q", res.Text)
	}
	for _, garbage := range []string{"%PDF", "/Filter", "endobj", "stream"} {
		if strings.Contains(res.Text, garbage) {
			t.Fatalf("extracted text contains raw PDF syntax %q — passthrough regression; got %q", garbage, res.Text)
		}
	}
}

// TestExtractPDFCorruptFileErrors: a broken PDF must produce an error (which
// the engine surfaces in the activity log), never a panic and never raw bytes.
func TestExtractPDFCorruptFileErrors(t *testing.T) {
	p := filepath.Join(t.TempDir(), "broken.pdf")
	if err := os.WriteFile(p, []byte("%PDF-1.4\nthis is not a valid pdf body"), 0644); err != nil {
		t.Fatal(err)
	}
	res, err := extractPDF(p)
	if err == nil && strings.Contains(res.Text, "%PDF") {
		t.Fatal("corrupt PDF returned raw bytes instead of an error")
	}
	if err == nil && res.Text != "" {
		t.Fatalf("corrupt PDF unexpectedly extracted text: %q", res.Text)
	}
}

// TestExtractorVersionPerType: the PDF extractor is versioned (bumped when its
// logic changed from passthrough to real extraction); everything else is 0, so
// a version bump re-indexes only the affected type.
func TestExtractorVersionPerType(t *testing.T) {
	f := NewFileExtractor()
	if v := f.Version("/x/doc.pdf"); v != 1 {
		t.Fatalf("pdf extractor version = %d, want 1", v)
	}
	if v := f.Version("/x/DOC.PDF"); v != 1 {
		t.Fatalf("pdf version must be case-insensitive on extension, got %d", v)
	}
	for _, p := range []string{"/x/notes.md", "/x/a.docx", "/x/a.txt", "/x/img.png"} {
		if v := f.Version(p); v != 0 {
			t.Fatalf("Version(%s) = %d, want 0 (only pdf is bumped)", p, v)
		}
	}
}
