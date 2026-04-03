package embeddings

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func makeEmbedding(dim int) []float32 {
	v := make([]float32, dim)
	for i := range v {
		v[i] = float32(i) * 0.001
	}
	return v
}

func newMockServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *OpenAIEmbedder) {
	t.Helper()
	srv := httptest.NewServer(handler)
	e := NewOpenAIEmbedder("test-key", "text-embedding-3-small")
	e.baseURL = srv.URL
	return srv, e
}

func TestEmbed_Success(t *testing.T) {
	srv, embedder := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Error("missing or wrong Authorization header")
		}

		var req embeddingRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}

		resp := embeddingResponse{}
		for i := range req.Input {
			resp.Data = append(resp.Data, embeddingData{
				Embedding: makeEmbedding(1536),
				Index:     i,
			})
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})
	defer srv.Close()

	results, err := embedder.Embed([]string{"hello", "world"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if len(results[0]) != 1536 {
		t.Errorf("expected 1536 dimensions, got %d", len(results[0]))
	}
}

func TestEmbed_Batching(t *testing.T) {
	var callCount atomic.Int32

	srv, embedder := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)

		var req embeddingRequest
		json.NewDecoder(r.Body).Decode(&req)

		if len(req.Input) > maxBatchSize {
			t.Errorf("batch size %d exceeds max %d", len(req.Input), maxBatchSize)
		}

		resp := embeddingResponse{}
		for i := range req.Input {
			resp.Data = append(resp.Data, embeddingData{
				Embedding: makeEmbedding(1536),
				Index:     i,
			})
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})
	defer srv.Close()

	// Create input larger than one batch
	texts := make([]string, maxBatchSize+10)
	for i := range texts {
		texts[i] = "text"
	}

	results, err := embedder.Embed(texts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != len(texts) {
		t.Fatalf("expected %d results, got %d", len(texts), len(results))
	}
	if callCount.Load() != 2 {
		t.Errorf("expected 2 API calls, got %d", callCount.Load())
	}
}

func TestEmbed_APIError(t *testing.T) {
	srv, embedder := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(embeddingResponse{
			Error: &apiError{
				Message: "invalid model",
				Type:    "invalid_request_error",
			},
		})
	})
	defer srv.Close()

	_, err := embedder.Embed([]string{"hello"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestEmbed_RetryOn429(t *testing.T) {
	var callCount atomic.Int32

	srv, embedder := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		n := callCount.Add(1)
		if n <= 2 {
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"error":{"message":"rate limited","type":"rate_limit_error"}}`))
			return
		}

		var req embeddingRequest
		json.NewDecoder(r.Body).Decode(&req)

		resp := embeddingResponse{}
		for i := range req.Input {
			resp.Data = append(resp.Data, embeddingData{
				Embedding: makeEmbedding(1536),
				Index:     i,
			})
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})
	defer srv.Close()

	results, err := embedder.Embed([]string{"hello"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if callCount.Load() != 3 {
		t.Errorf("expected 3 calls (2 retries + 1 success), got %d", callCount.Load())
	}
}

func TestEmbed_EmptyInput(t *testing.T) {
	embedder := NewOpenAIEmbedder("key", "text-embedding-3-small")
	results, err := embedder.Embed(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if results != nil {
		t.Errorf("expected nil, got %v", results)
	}
}

func TestDimensions(t *testing.T) {
	small := NewOpenAIEmbedder("key", "text-embedding-3-small")
	if small.Dimensions() != 1536 {
		t.Errorf("expected 1536 for small, got %d", small.Dimensions())
	}

	large := NewOpenAIEmbedder("key", "text-embedding-3-large")
	if large.Dimensions() != 3072 {
		t.Errorf("expected 3072 for large, got %d", large.Dimensions())
	}
}

func TestModelName(t *testing.T) {
	e := NewOpenAIEmbedder("key", "text-embedding-3-small")
	if e.ModelName() != "text-embedding-3-small" {
		t.Errorf("expected text-embedding-3-small, got %s", e.ModelName())
	}
}
