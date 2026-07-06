package chunker

import (
	"strings"
	"testing"
)

func mustNew(t *testing.T, opts ...Option) *TokenChunker {
	t.Helper()
	c, err := New(opts...)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	return c
}

func TestChunkSplittingRespectsSize(t *testing.T) {
	c := mustNew(t, WithChunkSize(10), WithOverlap(2))

	// Generate a string that will produce more than 10 tokens.
	// Each word is typically 1 token.
	words := make([]string, 25)
	for i := range words {
		words[i] = "word"
	}
	content := strings.Join(words, " ")

	results, err := c.ChunkText(content)
	if err != nil {
		t.Fatalf("Chunk() error: %v", err)
	}

	if len(results) < 2 {
		t.Fatalf("expected at least 2 chunks, got %d", len(results))
	}

	for i, r := range results {
		if r.TokenCount > 10 {
			t.Errorf("chunk %d has %d tokens, expected <= 10", i, r.TokenCount)
		}
	}
}

func TestChunkOverlap(t *testing.T) {
	c := mustNew(t, WithChunkSize(10), WithOverlap(3))

	words := make([]string, 25)
	for i := range words {
		words[i] = "hello"
	}
	content := strings.Join(words, " ")

	results, err := c.ChunkText(content)
	if err != nil {
		t.Fatalf("Chunk() error: %v", err)
	}

	if len(results) < 3 {
		t.Fatalf("expected at least 3 chunks with overlap, got %d", len(results))
	}

	// With chunkSize=10 and overlap=3, step = 7 tokens per advance.
	// Verify we get more chunks than we would without overlap.
	cNoOverlap := mustNew(t, WithChunkSize(10), WithOverlap(0))
	noOverlapResults, err := cNoOverlap.ChunkText(content)
	if err != nil {
		t.Fatalf("Chunk() error: %v", err)
	}
	if len(results) <= len(noOverlapResults) {
		t.Errorf("overlap should produce more chunks: got %d with overlap, %d without",
			len(results), len(noOverlapResults))
	}
}

func TestEmptyContentReturnsEmpty(t *testing.T) {
	c := mustNew(t)

	results, err := c.ChunkText("")
	if err != nil {
		t.Fatalf("Chunk() error: %v", err)
	}
	if results != nil {
		t.Errorf("expected nil for empty content, got %v", results)
	}
}

func TestWhitespaceOnlyReturnsEmpty(t *testing.T) {
	c := mustNew(t)

	results, err := c.ChunkText("   \n\t  ")
	if err != nil {
		t.Fatalf("Chunk() error: %v", err)
	}
	if results != nil {
		t.Errorf("expected nil for whitespace-only content, got %v", results)
	}
}

func TestSingleWordContent(t *testing.T) {
	c := mustNew(t, WithChunkSize(512), WithOverlap(50))

	results, err := c.ChunkText("hello")
	if err != nil {
		t.Fatalf("Chunk() error: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 chunk for single word, got %d", len(results))
	}

	r := results[0]
	if r.Index != 0 {
		t.Errorf("expected index 0, got %d", r.Index)
	}
	if r.TokenCount != 1 {
		t.Errorf("expected 1 token, got %d", r.TokenCount)
	}
	if r.Content != "hello" {
		t.Errorf("expected content 'hello', got %q", r.Content)
	}
}

func TestChunkTextDirectly(t *testing.T) {
	c := mustNew(t)

	// ChunkText works on any text regardless of file extension
	results, err := c.ChunkText("some content here")
	if err != nil {
		t.Fatalf("ChunkText() returned error: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected non-empty result from ChunkText")
	}
}

func TestChunkTextEmpty(t *testing.T) {
	c := mustNew(t)

	results, err := c.ChunkText("")
	if err != nil {
		t.Fatalf("ChunkText() returned error: %v", err)
	}
	if results != nil {
		t.Fatal("expected nil for empty content")
	}
}

func TestChunkMetadata(t *testing.T) {
	c := mustNew(t, WithChunkSize(5), WithOverlap(1))

	// Use distinct words so each is a single token.
	content := "alpha beta gamma delta epsilon zeta eta theta iota kappa"

	results, err := c.ChunkText(content)
	if err != nil {
		t.Fatalf("Chunk() error: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("expected at least one chunk")
	}

	for i, r := range results {
		if r.Index != i {
			t.Errorf("chunk %d: expected Index=%d, got %d", i, i, r.Index)
		}
		if r.TokenCount <= 0 {
			t.Errorf("chunk %d: expected positive TokenCount, got %d", i, r.TokenCount)
		}
		if r.TokenCount > 5 {
			t.Errorf("chunk %d: TokenCount %d exceeds chunkSize 5", i, r.TokenCount)
		}
		if r.Content == "" {
			t.Errorf("chunk %d: Content is empty", i)
		}
	}

	// Verify indices are sequential.
	for i := 1; i < len(results); i++ {
		if results[i].Index != results[i-1].Index+1 {
			t.Errorf("chunk indices not sequential: %d followed by %d",
				results[i-1].Index, results[i].Index)
		}
	}
}

func TestContentFitsInOneChunk(t *testing.T) {
	c := mustNew(t, WithChunkSize(512), WithOverlap(50))

	content := "This is a short document with just a few words."
	results, err := c.ChunkText(content)
	if err != nil {
		t.Fatalf("Chunk() error: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(results))
	}

	if results[0].Index != 0 {
		t.Errorf("expected index 0, got %d", results[0].Index)
	}
}

func TestChunkTextSimple(t *testing.T) {
	c := mustNew(t)

	results, err := c.ChunkText("hello world")
	if err != nil {
		t.Fatalf("ChunkText() error: %v", err)
	}
	if len(results) == 0 {
		t.Error("expected non-empty result")
	}
}

// fakeTokenizer is a trivial whitespace tokenizer used to verify that a custom
// tokenizer injected via WithTokenizer is actually used. Each rune becomes a
// token, so token counts are deterministic and independent of tiktoken.
type fakeTokenizer struct {
	encodeCalls int
}

func (f *fakeTokenizer) Encode(text string) ([]uint32, error) {
	f.encodeCalls++
	runes := []rune(text)
	out := make([]uint32, len(runes))
	for i, r := range runes {
		out[i] = uint32(r)
	}
	return out, nil
}

func (f *fakeTokenizer) Decode(tokens []uint32) (string, error) {
	runes := make([]rune, len(tokens))
	for i, t := range tokens {
		runes[i] = rune(t)
	}
	return string(runes), nil
}

func TestWithTokenizerIsHonored(t *testing.T) {
	ft := &fakeTokenizer{}
	c := mustNew(t, WithTokenizer(ft), WithChunkSize(3), WithOverlap(0))

	// "abcdef" -> 6 rune-tokens; with size 3 and no overlap -> 2 chunks.
	results, err := c.ChunkText("abcdef")
	if err != nil {
		t.Fatalf("ChunkText() error: %v", err)
	}
	if ft.encodeCalls == 0 {
		t.Fatal("custom tokenizer Encode was never called")
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(results))
	}
	if results[0].Content != "abc" || results[1].Content != "def" {
		t.Errorf("unexpected chunk contents: %q, %q", results[0].Content, results[1].Content)
	}
}

func TestWithMaxInputTokensClampsChunkSize(t *testing.T) {
	// Deterministic tokenizer so chunk boundaries are exact.
	content := "abcdefghij" // 10 rune-tokens

	// Without clamping: chunkSize 8 -> a single chunk covering all 10? No,
	// 8 < 10 so two chunks (8 + 2). Establish the baseline first.
	base := mustNew(t, WithTokenizer(&fakeTokenizer{}), WithChunkSize(8), WithOverlap(0))
	baseResults, err := base.ChunkText(content)
	if err != nil {
		t.Fatalf("ChunkText() error: %v", err)
	}
	if len(baseResults) == 0 || baseResults[0].TokenCount != 8 {
		t.Fatalf("baseline expected first chunk of 8 tokens, got %+v", baseResults)
	}

	// With clamping to 4: chunks become smaller (max 4 tokens each).
	clamped := mustNew(t, WithTokenizer(&fakeTokenizer{}), WithChunkSize(8), WithOverlap(0), WithMaxInputTokens(4))
	clampedResults, err := clamped.ChunkText(content)
	if err != nil {
		t.Fatalf("ChunkText() error: %v", err)
	}
	for i, r := range clampedResults {
		if r.TokenCount > 4 {
			t.Errorf("chunk %d has %d tokens, expected <= 4 after clamping", i, r.TokenCount)
		}
	}
	if len(clampedResults) <= len(baseResults) {
		t.Errorf("clamping should produce more (smaller) chunks: got %d clamped vs %d base",
			len(clampedResults), len(baseResults))
	}
}
