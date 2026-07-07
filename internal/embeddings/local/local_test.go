package local

import (
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// ---------------------------------------------------------------------------
// Fakes (no native libraries)
// ---------------------------------------------------------------------------

// fakeTokenizer maps a string to token IDs by length so different inputs pad
// differently. Deterministic and native-free.
type fakeTokenizer struct {
	calls []string
}

func (f *fakeTokenizer) encode(text string) ([]uint32, error) {
	f.calls = append(f.calls, text)
	ids := make([]uint32, len(text))
	for i := range ids {
		ids[i] = uint32(i + 1)
	}
	return ids, nil
}

// fakeSession records the tensors it received and returns a fixed pooled vector
// per input row (un-normalized, so l2Normalize is observable downstream).
type fakeSession struct {
	lastIDs   [][]int64
	lastMask  [][]int64
	lastTypes [][]int64
	vec       []float32 // returned for every row
	closed    bool
}

func (s *fakeSession) run(inputIDs, attnMask, typeIDs [][]int64) ([][]float32, error) {
	s.lastIDs = inputIDs
	s.lastMask = attnMask
	s.lastTypes = typeIDs
	out := make([][]float32, len(inputIDs))
	for i := range out {
		cp := make([]float32, len(s.vec))
		copy(cp, s.vec)
		out[i] = cp
	}
	return out, nil
}

func (s *fakeSession) close() error { s.closed = true; return nil }

func newFakeEmbedder(vec []float32) (*LocalEmbedder, *fakeSession, *fakeTokenizer) {
	sess := &fakeSession{vec: vec}
	tk := &fakeTokenizer{}
	e := New(Config{})
	e.session = sess
	e.tok = tk
	return e, sess, tk
}

// ---------------------------------------------------------------------------
// Pure helpers
// ---------------------------------------------------------------------------

func TestWithPrefix(t *testing.T) {
	got := withPrefix("passage: ", []string{"a", "b"})
	want := []string{"passage: a", "passage: b"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("withPrefix = %v, want %v", got, want)
	}
	// original inputs unchanged
	orig := []string{"x"}
	_ = withPrefix("query: ", orig)
	if orig[0] != "x" {
		t.Fatalf("withPrefix mutated input: %v", orig)
	}
}

func TestBuildInputs(t *testing.T) {
	tokenIDs := [][]uint32{
		{10, 11, 12}, // len 3
		{20, 21},     // len 2 -> padded to 3
	}
	ids, mask, types := buildInputs(tokenIDs)

	// three parallel inputs, padded to max len 3
	wantIDs := [][]int64{{10, 11, 12}, {20, 21, 0}}
	wantMask := [][]int64{{1, 1, 1}, {1, 1, 0}}
	wantTypes := [][]int64{{0, 0, 0}, {0, 0, 0}}

	if !reflect.DeepEqual(ids, wantIDs) {
		t.Errorf("ids = %v, want %v", ids, wantIDs)
	}
	if !reflect.DeepEqual(mask, wantMask) {
		t.Errorf("mask = %v, want %v", mask, wantMask)
	}
	if !reflect.DeepEqual(types, wantTypes) {
		t.Errorf("token_type_ids = %v, want %v (must be all zero)", types, wantTypes)
	}

	// exactly three input tensors are produced, all same shape
	if len(ids) != len(mask) || len(mask) != len(types) {
		t.Fatalf("input row counts differ: ids=%d mask=%d types=%d", len(ids), len(mask), len(types))
	}
	for i := range ids {
		if len(ids[i]) != 3 || len(mask[i]) != 3 || len(types[i]) != 3 {
			t.Fatalf("row %d not padded to 3: ids=%v mask=%v types=%v", i, ids[i], mask[i], types[i])
		}
	}
}

func TestBuildInputsTokenTypeIDsAllZero(t *testing.T) {
	_, _, types := buildInputs([][]uint32{{1, 2, 3, 4}, {5}})
	for i, row := range types {
		for j, v := range row {
			if v != 0 {
				t.Fatalf("token_type_ids[%d][%d] = %d, want 0", i, j, v)
			}
		}
	}
}

func TestMeanPool(t *testing.T) {
	// two real tokens, one padded (mask 0). Padded row must be ignored.
	hidden := [][]float32{
		{1, 2},
		{3, 4},
		{100, 100}, // padding — ignored
	}
	mask := []int64{1, 1, 0}
	got := meanPool(hidden, mask)
	want := []float32{2, 3} // (1+3)/2, (2+4)/2
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("meanPool = %v, want %v", got, want)
	}
}

func TestMeanPoolAllMasked(t *testing.T) {
	got := meanPool([][]float32{{5, 5}}, []int64{0})
	want := []float32{0, 0}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("meanPool all-masked = %v, want %v", got, want)
	}
}

func TestL2Normalize(t *testing.T) {
	got := l2Normalize([]float32{3, 4})
	want := []float32{0.6, 0.8}
	for i := range want {
		if math.Abs(float64(got[i]-want[i])) > 1e-6 {
			t.Fatalf("l2Normalize = %v, want %v", got, want)
		}
	}
	// resulting norm ~= 1
	if n := vecNorm(got); math.Abs(n-1) > 1e-6 {
		t.Fatalf("norm = %v, want ~1", n)
	}
}

func TestL2NormalizeZero(t *testing.T) {
	got := l2Normalize([]float32{0, 0, 0})
	if vecNorm(got) != 0 {
		t.Fatalf("zero vector should stay zero, got %v", got)
	}
}

// ---------------------------------------------------------------------------
// Pipeline via fakes
// ---------------------------------------------------------------------------

func TestEmbedQueryPrefixAndShape(t *testing.T) {
	e, sess, tk := newFakeEmbedder([]float32{3, 4}) // un-normalized

	vec, err := e.EmbedQuery("hello")
	if err != nil {
		t.Fatal(err)
	}

	// query prefix applied to the tokenizer input
	if len(tk.calls) != 1 || tk.calls[0] != "query: hello" {
		t.Fatalf("tokenizer calls = %v, want [\"query: hello\"]", tk.calls)
	}
	// result is L2-normalized (3,4 -> 0.6,0.8)
	if math.Abs(float64(vec[0]-0.6)) > 1e-6 || math.Abs(float64(vec[1]-0.8)) > 1e-6 {
		t.Fatalf("EmbedQuery vec = %v, want normalized [0.6 0.8]", vec)
	}
	if n := vecNorm(vec); math.Abs(n-1) > 1e-6 {
		t.Fatalf("EmbedQuery norm = %v, want ~1", n)
	}
	// three padded inputs reached the session, token_type_ids all zero
	assertThreeZeroTypeInputs(t, sess)
}

func TestEmbedDocumentsPrefixAndBatching(t *testing.T) {
	e, sess, tk := newFakeEmbedder([]float32{0, 3})
	e.cfg.BatchSize = 2 // force multiple sub-batches over 5 inputs

	texts := []string{"a", "b", "c", "d", "e"}
	vecs, err := e.EmbedDocuments(texts)
	if err != nil {
		t.Fatal(err)
	}

	if len(vecs) != len(texts) {
		t.Fatalf("got %d vectors, want %d", len(vecs), len(texts))
	}
	// passage prefix applied to every input
	for i, c := range tk.calls {
		want := "passage: " + texts[i]
		if c != want {
			t.Fatalf("tokenizer call %d = %q, want %q", i, c, want)
		}
	}
	// every returned vector is unit-norm and 2-dim
	for i, v := range vecs {
		if len(v) != 2 {
			t.Fatalf("vec %d has dim %d, want 2", i, len(v))
		}
		if n := vecNorm(v); math.Abs(n-1) > 1e-6 {
			t.Fatalf("vec %d norm = %v, want ~1", i, n)
		}
	}
	// 5 inputs at batch size 2 -> sub-batches of 2,2,1; last one has 1 row
	if len(sess.lastIDs) != 1 {
		t.Fatalf("last sub-batch rows = %d, want 1", len(sess.lastIDs))
	}
	assertThreeZeroTypeInputs(t, sess)
}

func TestEmbedDocumentsEmpty(t *testing.T) {
	e, _, _ := newFakeEmbedder([]float32{1})
	vecs, err := e.EmbedDocuments(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(vecs) != 0 {
		t.Fatalf("want empty result, got %v", vecs)
	}
}

func TestMetadata(t *testing.T) {
	e := New(Config{})
	if e.Dimensions() != 384 {
		t.Errorf("Dimensions = %d, want 384", e.Dimensions())
	}
	if e.ModelName() != "multilingual-e5-small" {
		t.Errorf("ModelName = %q", e.ModelName())
	}
	if e.MaxInputTokens() != 512 {
		t.Errorf("MaxInputTokens = %d, want 512", e.MaxInputTokens())
	}
}

// ---------------------------------------------------------------------------
// Assets helpers
// ---------------------------------------------------------------------------

func TestFingerprintStable(t *testing.T) {
	a := fingerprint("local", "multilingual-e5-small", 384)
	b := fingerprint("local", "multilingual-e5-small", 384)
	if a != b {
		t.Fatalf("fingerprint not deterministic: %q vs %q", a, b)
	}
	if a == fingerprint("openai", "text-embedding-3-small", 1536) {
		t.Fatalf("fingerprint collision across providers")
	}
	if len(a) != 16 {
		t.Fatalf("fingerprint len = %d, want 16", len(a))
	}
}

func TestResolveAssetsMissing(t *testing.T) {
	if _, _, _, err := resolveAssets(Config{AssetsDir: t.TempDir()}); err == nil {
		t.Fatal("expected error for empty assets dir")
	}
	// no dir and no env
	t.Setenv(assetsDirEnv, "")
	if _, _, _, err := resolveAssets(Config{}); err == nil {
		t.Fatal("expected error when no assets dir configured")
	}
}

func TestExtractAndVerify(t *testing.T) {
	src := filepath.Join(t.TempDir(), assetModelFile)
	content := []byte("weights")
	if err := os.WriteFile(src, content, 0o644); err != nil {
		t.Fatal(err)
	}
	want, err := sha256File(src)
	if err != nil {
		t.Fatal(err)
	}
	dest := t.TempDir()

	got, err := extractAndVerify(src, dest, want)
	if err != nil {
		t.Fatal(err)
	}
	out, err := os.ReadFile(got)
	if err != nil || string(out) != string(content) {
		t.Fatalf("extracted content mismatch: %q err=%v", out, err)
	}

	// idempotent second call
	if _, err := extractAndVerify(src, dest, want); err != nil {
		t.Fatalf("second extract failed: %v", err)
	}
	// checksum mismatch is rejected
	if _, err := extractAndVerify(src, t.TempDir(), "deadbeef"); err == nil {
		t.Fatal("expected checksum mismatch error")
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func assertThreeZeroTypeInputs(t *testing.T, s *fakeSession) {
	t.Helper()
	if s.lastIDs == nil || s.lastMask == nil || s.lastTypes == nil {
		t.Fatal("expected all three inputs (ids, mask, types) to reach session")
	}
	if len(s.lastIDs) != len(s.lastMask) || len(s.lastMask) != len(s.lastTypes) {
		t.Fatalf("input row counts differ: ids=%d mask=%d types=%d",
			len(s.lastIDs), len(s.lastMask), len(s.lastTypes))
	}
	for i, row := range s.lastTypes {
		for j, v := range row {
			if v != 0 {
				t.Fatalf("token_type_ids[%d][%d] = %d, want 0", i, j, v)
			}
		}
	}
}

func vecNorm(v []float32) float64 {
	var s float64
	for _, x := range v {
		s += float64(x) * float64(x)
	}
	return math.Sqrt(s)
}
