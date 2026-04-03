package mcp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"testing"

	"github.com/borzou/vecstore/internal/domain"
)

// mockReadOnlyEngine implements ReadOnlyEngineService for testing.
type mockReadOnlyEngine struct {
	SearchFn            func(params domain.SearchParams) ([]domain.SearchResult, error)
	ListDirectoriesFn   func() ([]domain.Directory, error)
	StatsFn             func() (domain.IndexStats, error)
	GetIgnorePatternsFn func() ([]string, error)
}

func (m *mockReadOnlyEngine) Search(params domain.SearchParams) ([]domain.SearchResult, error) {
	if m.SearchFn != nil {
		return m.SearchFn(params)
	}
	return nil, nil
}

func (m *mockReadOnlyEngine) ListDirectories() ([]domain.Directory, error) {
	if m.ListDirectoriesFn != nil {
		return m.ListDirectoriesFn()
	}
	return nil, nil
}

func (m *mockReadOnlyEngine) Stats() (domain.IndexStats, error) {
	if m.StatsFn != nil {
		return m.StatsFn()
	}
	return domain.IndexStats{}, nil
}

func (m *mockReadOnlyEngine) GetIgnorePatterns() ([]string, error) {
	if m.GetIgnorePatternsFn != nil {
		return m.GetIgnorePatternsFn()
	}
	return nil, nil
}

func TestStdio_Initialize(t *testing.T) {
	var in, out bytes.Buffer

	req := jsonrpcRequest{JSONRPC: "2.0", ID: 1, Method: "initialize"}
	b, _ := json.Marshal(req)
	in.Write(b)
	in.WriteByte('\n')

	srv := &StdioServer{engine: &mockReadOnlyEngine{}, reader: &in, writer: &out}
	if err := srv.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}

	var resp jsonrpcResponse
	if err := json.NewDecoder(&out).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %s", resp.Error.Message)
	}
}

func TestStdio_ToolsList_ReadOnly(t *testing.T) {
	var in, out bytes.Buffer

	req := jsonrpcRequest{JSONRPC: "2.0", ID: 1, Method: "tools/list"}
	b, _ := json.Marshal(req)
	in.Write(b)
	in.WriteByte('\n')

	srv := &StdioServer{engine: &mockReadOnlyEngine{}, reader: &in, writer: &out}
	srv.Run()

	var resp jsonrpcResponse
	json.NewDecoder(&out).Decode(&resp)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %s", resp.Error.Message)
	}

	// Extract tool names from the result.
	resultBytes, _ := json.Marshal(resp.Result)
	var result struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}
	json.Unmarshal(resultBytes, &result)

	toolNames := make(map[string]bool)
	for _, tool := range result.Tools {
		toolNames[tool.Name] = true
	}

	// Should have exactly 4 read-only tools.
	if len(result.Tools) != 4 {
		t.Fatalf("expected 4 tools, got %d: %v", len(result.Tools), toolNames)
	}

	expected := []string{"search", "list_directories", "index_status", "get_ignore_patterns"}
	for _, name := range expected {
		if !toolNames[name] {
			t.Errorf("missing expected tool: %s", name)
		}
	}

	// Should NOT have write tools.
	forbidden := []string{"add_directory", "remove_directory", "start", "stop", "restart", "reset", "set_ignore_patterns"}
	for _, name := range forbidden {
		if toolNames[name] {
			t.Errorf("unexpected write tool present: %s", name)
		}
	}
}

func TestStdio_Search(t *testing.T) {
	var in, out bytes.Buffer

	engine := &mockReadOnlyEngine{
		SearchFn: func(params domain.SearchParams) ([]domain.SearchResult, error) {
			return []domain.SearchResult{
				{FilePath: "/test.md", ChunkIndex: 0, Content: "hello", Score: 0.5},
			}, nil
		},
	}

	req := jsonrpcRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/call",
		Params:  mustMarshalJSON(toolCallParams{Name: "search", Arguments: mustMarshalJSON(map[string]interface{}{"query": "test"})}),
	}
	b, _ := json.Marshal(req)
	in.Write(b)
	in.WriteByte('\n')

	srv := &StdioServer{engine: engine, reader: &in, writer: &out}
	srv.Run()

	var resp jsonrpcResponse
	json.NewDecoder(&out).Decode(&resp)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %s", resp.Error.Message)
	}
}

func TestStdio_WriteToolRejected(t *testing.T) {
	var in, out bytes.Buffer

	req := jsonrpcRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/call",
		Params:  mustMarshalJSON(toolCallParams{Name: "add_directory", Arguments: mustMarshalJSON(map[string]interface{}{"path": "/tmp"})}),
	}
	b, _ := json.Marshal(req)
	in.Write(b)
	in.WriteByte('\n')

	srv := &StdioServer{engine: &mockReadOnlyEngine{}, reader: &in, writer: &out}
	srv.Run()

	scanner := bufio.NewScanner(&out)
	scanner.Scan()
	var resp jsonrpcResponse
	json.Unmarshal(scanner.Bytes(), &resp)
	if resp.Error == nil {
		t.Fatal("expected error for write tool, got nil")
	}
	if resp.Error.Message != "unknown tool: add_directory" {
		t.Errorf("unexpected error message: %s", resp.Error.Message)
	}
}

func mustMarshalJSON(v interface{}) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return json.RawMessage(b)
}
