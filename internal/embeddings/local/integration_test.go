//go:build localembed

package local

import (
	"math"
	"os"
	"testing"
)

// TestIntegrationEmbed exercises the real ONNX Runtime + HF tokenizer pipeline
// against the mE5-small assets. It requires the native libraries and the assets
// directory (AGENT_MEMORY_LOCAL_ASSETS), so it only builds under the
// `localembed` tag and skips if assets are absent.
func TestIntegrationEmbed(t *testing.T) {
	if os.Getenv(assetsDirEnv) == "" {
		t.Skipf("set %s to the assets dir to run the integration test", assetsDirEnv)
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
