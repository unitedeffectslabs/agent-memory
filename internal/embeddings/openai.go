package embeddings

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"time"
)

const maxBatchSize = 2048
const maxRetries = 3

// OpenAIEmbedder implements Embedder using the OpenAI embeddings API.
type OpenAIEmbedder struct {
	apiKey  string
	model   string
	baseURL string
	client  *http.Client
}

// NewOpenAIEmbedder creates an OpenAIEmbedder for the given model.
// Model should be "text-embedding-3-small" or "text-embedding-3-large".
func NewOpenAIEmbedder(apiKey, model string) *OpenAIEmbedder {
	return &OpenAIEmbedder{
		apiKey:  apiKey,
		model:   model,
		baseURL: "https://api.openai.com",
		client:  &http.Client{Timeout: 60 * time.Second},
	}
}

type embeddingRequest struct {
	Input []string `json:"input"`
	Model string   `json:"model"`
}

type embeddingData struct {
	Embedding []float32 `json:"embedding"`
	Index     int       `json:"index"`
}

type embeddingResponse struct {
	Data  []embeddingData `json:"data"`
	Error *apiError       `json:"error,omitempty"`
}

type apiError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
}

// Embed generates embeddings for the given texts, batching as needed.
func (e *OpenAIEmbedder) Embed(texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	results := make([][]float32, len(texts))

	for start := 0; start < len(texts); start += maxBatchSize {
		end := start + maxBatchSize
		if end > len(texts) {
			end = len(texts)
		}
		batch := texts[start:end]

		embeddings, err := e.callAPI(batch)
		if err != nil {
			return nil, fmt.Errorf("embedding batch starting at index %d: %w", start, err)
		}

		for _, d := range embeddings {
			results[start+d.Index] = d.Embedding
		}
	}

	return results, nil
}

func (e *OpenAIEmbedder) callAPI(texts []string) ([]embeddingData, error) {
	reqBody := embeddingRequest{
		Input: texts,
		Model: e.model,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(math.Pow(2, float64(attempt-1))) * time.Second
			time.Sleep(backoff)
		}

		req, err := http.NewRequest("POST", e.baseURL+"/v1/embeddings", bytes.NewReader(bodyBytes))
		if err != nil {
			return nil, fmt.Errorf("create request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+e.apiKey)

		resp, err := e.client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("http request: %w", err)
			continue
		}

		respBody, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = fmt.Errorf("read response: %w", err)
			continue
		}

		if resp.StatusCode == http.StatusTooManyRequests {
			lastErr = fmt.Errorf("rate limited (429)")
			continue
		}

		if resp.StatusCode != http.StatusOK {
			var parsed embeddingResponse
			if json.Unmarshal(respBody, &parsed) == nil && parsed.Error != nil {
				return nil, fmt.Errorf("API error (%d): %s", resp.StatusCode, parsed.Error.Message)
			}
			return nil, fmt.Errorf("API error (%d): %s", resp.StatusCode, string(respBody))
		}

		var parsed embeddingResponse
		if err := json.Unmarshal(respBody, &parsed); err != nil {
			return nil, fmt.Errorf("unmarshal response: %w", err)
		}

		return parsed.Data, nil
	}

	return nil, fmt.Errorf("max retries exceeded: %w", lastErr)
}

// Dimensions returns the embedding dimension for the configured model.
func (e *OpenAIEmbedder) Dimensions() int {
	switch e.model {
	case "text-embedding-3-large":
		return 3072
	default:
		return 1536
	}
}

// ModelName returns the model string.
func (e *OpenAIEmbedder) ModelName() string {
	return e.model
}
