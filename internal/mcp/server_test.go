package mcp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/borzou/vecstore/internal/domain"
)

// mockEngine implements EngineService with function fields for test control.
type mockEngine struct {
	SearchFn             func(params domain.SearchParams) ([]domain.SearchResult, error)
	AddDirectoryFn       func(path string) error
	RemoveDirectoryFn    func(path string) error
	ListDirectoriesFn    func() ([]domain.Directory, error)
	StatsFn              func() (domain.IndexStats, error)
	StartFn              func() error
	StopFn               func() error
	RestartFn            func() error
	ResetFn              func() error
	GetIgnorePatternsFn  func() ([]string, error)
	SetIgnorePatternsFn  func(patterns []string) error
}

func (m *mockEngine) Search(params domain.SearchParams) ([]domain.SearchResult, error) {
	if m.SearchFn != nil {
		return m.SearchFn(params)
	}
	return nil, nil
}

func (m *mockEngine) AddDirectory(path string) error {
	if m.AddDirectoryFn != nil {
		return m.AddDirectoryFn(path)
	}
	return nil
}

func (m *mockEngine) RemoveDirectory(path string) error {
	if m.RemoveDirectoryFn != nil {
		return m.RemoveDirectoryFn(path)
	}
	return nil
}

func (m *mockEngine) ListDirectories() ([]domain.Directory, error) {
	if m.ListDirectoriesFn != nil {
		return m.ListDirectoriesFn()
	}
	return nil, nil
}

func (m *mockEngine) Stats() (domain.IndexStats, error) {
	if m.StatsFn != nil {
		return m.StatsFn()
	}
	return domain.IndexStats{}, nil
}

func (m *mockEngine) Start() error {
	if m.StartFn != nil {
		return m.StartFn()
	}
	return nil
}

func (m *mockEngine) Stop() error {
	if m.StopFn != nil {
		return m.StopFn()
	}
	return nil
}

func (m *mockEngine) Restart() error {
	if m.RestartFn != nil {
		return m.RestartFn()
	}
	return nil
}

func (m *mockEngine) Reset() error {
	if m.ResetFn != nil {
		return m.ResetFn()
	}
	return nil
}

func (m *mockEngine) GetIgnorePatterns() ([]string, error) {
	if m.GetIgnorePatternsFn != nil {
		return m.GetIgnorePatternsFn()
	}
	return []string{}, nil
}

func (m *mockEngine) SetIgnorePatterns(patterns []string) error {
	if m.SetIgnorePatternsFn != nil {
		return m.SetIgnorePatternsFn(patterns)
	}
	return nil
}

const testToken = "test-secret-token"

func newTestServer(engine EngineService) *httptest.Server {
	srv := NewServer(engine, "0", testToken)
	mux := http.NewServeMux()
	mux.HandleFunc("/sse", srv.handleSSE)
	mux.HandleFunc("/messages", srv.handleMessages)
	return httptest.NewServer(mux)
}

// doMCPRequest performs a complete MCP HTTP/SSE request cycle:
//  1. GET /sse to establish a session and learn the messages endpoint
//  2. POST to that endpoint with the JSON-RPC request body
//  3. Read the JSON-RPC response from the SSE stream
//
// Returns the SSE connect status code and (if connected successfully) the
// parsed JSON-RPC response. The SSE response body is closed before returning.
func doMCPRequest(t *testing.T, serverURL string, body interface{}, token string) (int, *jsonrpcResponse) {
	t.Helper()

	// Step 1: open SSE connection.
	sseReq, err := http.NewRequest(http.MethodGet, serverURL+"/sse", nil)
	if err != nil {
		t.Fatalf("create SSE request: %v", err)
	}
	if token != "" {
		sseReq.Header.Set("Authorization", "Bearer "+token)
	}

	sseResp, err := http.DefaultClient.Do(sseReq)
	if err != nil {
		t.Fatalf("SSE connect: %v", err)
	}
	defer sseResp.Body.Close()

	if sseResp.StatusCode != http.StatusOK {
		return sseResp.StatusCode, nil
	}

	scanner := bufio.NewScanner(sseResp.Body)

	// Step 2: read the "endpoint" event to get the messages path.
	messagesPath := readSSEDataLine(t, scanner)
	// messagesPath is e.g. "/messages?sessionId=abc123"

	// Step 3: POST the JSON-RPC request.
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request body: %v", err)
	}
	postReq, err := http.NewRequest(http.MethodPost, serverURL+messagesPath, bytes.NewReader(b))
	if err != nil {
		t.Fatalf("create POST request: %v", err)
	}
	if token != "" {
		postReq.Header.Set("Authorization", "Bearer "+token)
	}
	postReq.Header.Set("Content-Type", "application/json")

	postResp, err := http.DefaultClient.Do(postReq)
	if err != nil {
		t.Fatalf("POST messages: %v", err)
	}
	postResp.Body.Close()

	if postResp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202 from POST /messages, got %d", postResp.StatusCode)
	}

	// Step 4: read the response from the SSE stream.
	dataLine := readSSEDataLine(t, scanner)
	var result jsonrpcResponse
	if err := json.Unmarshal([]byte(dataLine), &result); err != nil {
		t.Fatalf("unmarshal SSE response: %v", err)
	}

	return http.StatusOK, &result
}

// readSSEDataLine scans forward until it finds a line beginning with "data: "
// and returns the value after the prefix.
func readSSEDataLine(t *testing.T, scanner *bufio.Scanner) string {
	t.Helper()
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			return strings.TrimPrefix(line, "data: ")
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("SSE scanner error: %v", err)
	}
	t.Fatal("SSE stream ended without a data line")
	return ""
}

// mustMarshalRaw marshals v to json.RawMessage for use in test request construction.
func mustMarshalRaw(v interface{}) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return json.RawMessage(b)
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestToolsList(t *testing.T) {
	ts := newTestServer(&mockEngine{})
	defer ts.Close()

	req := jsonrpcRequest{JSONRPC: "2.0", ID: 1, Method: "tools/list"}
	status, resp := doMCPRequest(t, ts.URL, req, testToken)

	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d", status)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %s", resp.Error.Message)
	}

	// Result should be {"tools": [...]}
	resultBytes, err := json.Marshal(resp.Result)
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}

	var listResult struct {
		Tools []toolDefinition `json:"tools"`
	}
	if err := json.Unmarshal(resultBytes, &listResult); err != nil {
		t.Fatalf("unmarshal tools list: %v", err)
	}

	if len(listResult.Tools) != 11 {
		t.Errorf("expected 11 tools, got %d", len(listResult.Tools))
	}

	expected := map[string]bool{
		"search":               false,
		"add_directory":        false,
		"remove_directory":     false,
		"list_directories":     false,
		"index_status":         false,
		"start":                false,
		"stop":                 false,
		"restart":              false,
		"reset":                false,
		"get_ignore_patterns":  false,
		"set_ignore_patterns":  false,
	}
	for _, tool := range listResult.Tools {
		if _, ok := expected[tool.Name]; ok {
			expected[tool.Name] = true
		}
	}
	for name, found := range expected {
		if !found {
			t.Errorf("missing tool: %s", name)
		}
	}
}

func TestSearchTool(t *testing.T) {
	var calledParams domain.SearchParams

	engine := &mockEngine{
		SearchFn: func(params domain.SearchParams) ([]domain.SearchResult, error) {
			calledParams = params
			return []domain.SearchResult{
				{FilePath: "/tmp/test.md", ChunkIndex: 0, Content: "hello world", Score: 0.95},
			}, nil
		},
	}

	ts := newTestServer(engine)
	defer ts.Close()

	req := jsonrpcRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/call",
		Params: mustMarshalRaw(toolCallParams{
			Name:      "search",
			Arguments: mustMarshalRaw(map[string]interface{}{"query": "test query", "limit": 5}),
		}),
	}

	status, resp := doMCPRequest(t, ts.URL, req, testToken)

	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d", status)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %s", resp.Error.Message)
	}
	if calledParams.Query != "test query" {
		t.Errorf("expected query 'test query', got %q", calledParams.Query)
	}
	if calledParams.Limit != 5 {
		t.Errorf("expected limit 5, got %d", calledParams.Limit)
	}
}

func TestAddDirectoryTool(t *testing.T) {
	var calledPath string

	engine := &mockEngine{
		AddDirectoryFn: func(path string) error {
			calledPath = path
			return nil
		},
	}

	ts := newTestServer(engine)
	defer ts.Close()

	req := jsonrpcRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/call",
		Params: mustMarshalRaw(toolCallParams{
			Name:      "add_directory",
			Arguments: mustMarshalRaw(map[string]interface{}{"path": "/tmp/mydir"}),
		}),
	}

	status, resp := doMCPRequest(t, ts.URL, req, testToken)

	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d", status)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %s", resp.Error.Message)
	}
	if calledPath != "/tmp/mydir" {
		t.Errorf("expected path '/tmp/mydir', got %q", calledPath)
	}
}

func TestAuthRequired(t *testing.T) {
	ts := newTestServer(&mockEngine{})
	defer ts.Close()

	// No token — GET /sse should return 401.
	status, _ := doMCPRequest(t, ts.URL, nil, "")
	if status != http.StatusUnauthorized {
		t.Fatalf("expected 401 for missing token, got %d", status)
	}

	// Wrong token — GET /sse should return 401.
	status, _ = doMCPRequest(t, ts.URL, nil, "wrong-token")
	if status != http.StatusUnauthorized {
		t.Fatalf("expected 401 for wrong token, got %d", status)
	}

	// POST /messages without auth should also return 401.
	postReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/messages?sessionId=fake", bytes.NewReader([]byte("{}")))
	postResp, err := http.DefaultClient.Do(postReq)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	postResp.Body.Close()
	if postResp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401 for POST without auth, got %d", postResp.StatusCode)
	}
}

func TestAuthValid(t *testing.T) {
	engine := &mockEngine{
		StatsFn: func() (domain.IndexStats, error) {
			return domain.IndexStats{
				TotalFiles:     42,
				TotalChunks:    100,
				LastIndexedAt:  time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC),
				IsIndexing:     false,
				EmbeddingModel: "text-embedding-3-small",
			}, nil
		},
	}

	ts := newTestServer(engine)
	defer ts.Close()

	req := jsonrpcRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/call",
		Params: mustMarshalRaw(toolCallParams{
			Name:      "index_status",
			Arguments: mustMarshalRaw(map[string]interface{}{}),
		}),
	}

	status, resp := doMCPRequest(t, ts.URL, req, testToken)

	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d", status)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %s", resp.Error.Message)
	}
}

func TestInvalidTool(t *testing.T) {
	ts := newTestServer(&mockEngine{})
	defer ts.Close()

	req := jsonrpcRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/call",
		Params: mustMarshalRaw(toolCallParams{
			Name:      "nonexistent_tool",
			Arguments: mustMarshalRaw(map[string]interface{}{}),
		}),
	}

	status, resp := doMCPRequest(t, ts.URL, req, testToken)

	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d", status)
	}
	if resp.Error == nil {
		t.Fatal("expected error for unknown tool, got nil")
	}
	if resp.Error.Message != "unknown tool: nonexistent_tool" {
		t.Errorf("unexpected error message: %s", resp.Error.Message)
	}
}

func TestGetIgnorePatternsTool(t *testing.T) {
	patterns := []string{"node_modules/**", ".git/**", "*.lock"}

	engine := &mockEngine{
		GetIgnorePatternsFn: func() ([]string, error) {
			return patterns, nil
		},
	}

	ts := newTestServer(engine)
	defer ts.Close()

	req := jsonrpcRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/call",
		Params: mustMarshalRaw(toolCallParams{
			Name:      "get_ignore_patterns",
			Arguments: mustMarshalRaw(map[string]interface{}{}),
		}),
	}

	status, resp := doMCPRequest(t, ts.URL, req, testToken)

	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d", status)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %s", resp.Error.Message)
	}

	// Result is a toolResult; parse the content text to get the patterns.
	resultBytes, err := json.Marshal(resp.Result)
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}
	var tr toolResult
	if err := json.Unmarshal(resultBytes, &tr); err != nil {
		t.Fatalf("unmarshal tool result: %v", err)
	}
	if len(tr.Content) != 1 {
		t.Fatalf("expected 1 content item, got %d", len(tr.Content))
	}

	var got []string
	if err := json.Unmarshal([]byte(tr.Content[0].Text), &got); err != nil {
		t.Fatalf("unmarshal patterns: %v", err)
	}
	if len(got) != len(patterns) {
		t.Errorf("got %d patterns, want %d", len(got), len(patterns))
	}
	for i, p := range patterns {
		if got[i] != p {
			t.Errorf("pattern[%d] = %q, want %q", i, got[i], p)
		}
	}
}

func TestSetIgnorePatternsTool(t *testing.T) {
	var capturedPatterns []string

	engine := &mockEngine{
		SetIgnorePatternsFn: func(patterns []string) error {
			capturedPatterns = patterns
			return nil
		},
	}

	ts := newTestServer(engine)
	defer ts.Close()

	newPatterns := []string{"*.tmp", "build/**"}

	req := jsonrpcRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/call",
		Params: mustMarshalRaw(toolCallParams{
			Name:      "set_ignore_patterns",
			Arguments: mustMarshalRaw(map[string]interface{}{"patterns": newPatterns}),
		}),
	}

	status, resp := doMCPRequest(t, ts.URL, req, testToken)

	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d", status)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %s", resp.Error.Message)
	}
	if len(capturedPatterns) != len(newPatterns) {
		t.Fatalf("got %d patterns, want %d", len(capturedPatterns), len(newPatterns))
	}
	for i, p := range newPatterns {
		if capturedPatterns[i] != p {
			t.Errorf("pattern[%d] = %q, want %q", i, capturedPatterns[i], p)
		}
	}
}
