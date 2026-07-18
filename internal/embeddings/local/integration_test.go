//go:build localembed

package local

import (
	"math"
	"strings"
	"testing"

	"github.com/borzou/vecstore/internal/chunker"
)

// TestIntegrationEmbed exercises the real ONNX Runtime + HF tokenizer pipeline
// against the mE5-small assets. It requires the native libraries and the assets
// directory (AGENT_MEMORY_LOCAL_ASSETS), so it only builds under the
// `localembed` tag and skips if assets are absent.
func TestIntegrationEmbed(t *testing.T) {
	// Assets come from either the dev override (AGENT_MEMORY_LOCAL_ASSETS) or the
	// go:embed-ed set (present in every localembed build). Skip only if neither
	// resolves — proving the embed->extract path when run with no override set.
	if _, _, _, err := resolveAssets(Config{}); err != nil {
		t.Skipf("no local assets available: %v", err)
	}

	e := New(Config{Threads: 2, BatchSize: 8})

	// dimensionality + unit norm
	q, err := e.EmbedQuery("hello world")
	if err != nil {
		t.Fatalf("EmbedQuery: %v", err)
	}
	if len(q) != 384 {
		t.Fatalf("dim = %d, want 384", len(q))
	}
	if n := vecNorm(q); math.Abs(n-1) > 1e-4 {
		t.Fatalf("query vector norm = %v, want ~1", n)
	}

	// Phase 0 sanity ordering:
	//   related < cross-lingual < unrelated  (cosine distance)
	query := "how do I reset my password"
	related := "steps to recover a forgotten account password"
	crossES := "pasos para recuperar una contraseña de cuenta olvidada"
	unrelated := "the recipe calls for two cups of flour"

	qv, err := e.EmbedQuery(query)
	if err != nil {
		t.Fatal(err)
	}
	docs, err := e.EmbedDocuments([]string{related, crossES, unrelated})
	if err != nil {
		t.Fatal(err)
	}
	for i, d := range docs {
		if len(d) != 384 {
			t.Fatalf("doc %d dim = %d, want 384", i, len(d))
		}
		if n := vecNorm(d); math.Abs(n-1) > 1e-4 {
			t.Fatalf("doc %d norm = %v, want ~1", i, n)
		}
	}

	distRelated := 1 - cosine(qv, docs[0])
	distCross := 1 - cosine(qv, docs[1])
	distUnrelated := 1 - cosine(qv, docs[2])

	t.Logf("cosine distances: related=%.4f cross-lingual=%.4f unrelated=%.4f",
		distRelated, distCross, distUnrelated)

	if !(distRelated < distCross) {
		t.Errorf("expected related (%.4f) < cross-lingual (%.4f)", distRelated, distCross)
	}
	if !(distCross < distUnrelated) {
		t.Errorf("expected cross-lingual (%.4f) < unrelated (%.4f)", distCross, distUnrelated)
	}
}

func cosine(a, b []float32) float64 {
	var dot float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
	}
	return dot
}

// TestOrtLibFileIsKnownCandidate is a tripwire for future platform files: the
// per-platform ortLibFile must be a name findDylib recognizes, so a developer
// override directory populated with the same artifacts always resolves.
func TestOrtLibFileIsKnownCandidate(t *testing.T) {
	for _, name := range dylibCandidates {
		if name == ortLibFile {
			return
		}
	}
	t.Fatalf("ortLibFile %q is not in dylibCandidates %v", ortLibFile, dylibCandidates)
}

// TestIntegrationLargeDocument covers the scenario every platform smoke missed:
// a document big enough to need multiple chunks, through the REAL wired
// pipeline (HF tokenizer + reserved chunk budget + ONNX inference), plus an
// over-long query through the unchunked query path. Regression for the
// "512 by 516" failure that silently dropped every >1-chunk file.
func TestIntegrationLargeDocument(t *testing.T) {
	if _, _, _, err := resolveAssets(Config{}); err != nil {
		t.Skipf("no local assets available: %v", err)
	}
	e := New(Config{Threads: 2, BatchSize: 8})
	tok, err := NewChunkerTokenizer(Config{})
	if err != nil {
		t.Fatalf("chunker tokenizer: %v", err)
	}
	c, err := chunker.New(
		chunker.WithTokenizer(tok),
		chunker.WithMaxInputTokens(e.MaxInputTokens()-EmbedTokenReserve),
	)
	if err != nil {
		t.Fatalf("chunker: %v", err)
	}

	doc := strings.Repeat("Session notes: the demo plan needs a gap execution review and an architecture pivot before the milestone. ", 300) // well beyond one chunk
	chunks, err := c.ChunkText(doc)
	if err != nil {
		t.Fatalf("chunk: %v", err)
	}
	if len(chunks) < 2 {
		t.Fatalf("test needs a multi-chunk doc, got %d chunks", len(chunks))
	}
	texts := make([]string, len(chunks))
	for i, ch := range chunks {
		texts[i] = ch.Content
	}
	vecs, err := e.EmbedDocuments(texts)
	if err != nil {
		t.Fatalf("EmbedDocuments over %d real chunks: %v", len(chunks), err)
	}
	if len(vecs) != len(chunks) {
		t.Fatalf("got %d vectors for %d chunks", len(vecs), len(chunks))
	}
	for i, v := range vecs {
		if len(v) != 384 {
			t.Fatalf("chunk %d dim = %d, want 384", i, len(v))
		}
	}

	// The unchunked query path: a query far beyond the context window must
	// embed (truncated) rather than crash inference.
	longQuery := strings.Repeat("what was the demo plan and architecture pivot for the dossier project ", 60)
	qv, err := e.EmbedQuery(longQuery)
	if err != nil {
		t.Fatalf("EmbedQuery over-long query: %v", err)
	}
	if len(qv) != 384 {
		t.Fatalf("query dim = %d, want 384", len(qv))
	}
	t.Logf("large-doc pipeline OK: %d chunks embedded, over-long query embedded", len(chunks))
}
