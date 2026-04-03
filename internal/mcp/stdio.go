package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// StdioServer implements MCP over stdin/stdout using JSON-RPC style messages.
// Each line on stdin is a JSON request; each response is written as a single
// JSON line to stdout.
type StdioServer struct {
	engine ReadOnlyEngineService
	reader io.Reader
	writer io.Writer
}

// NewStdioServer creates a new stdio MCP server backed by a read-only engine.
func NewStdioServer(engine ReadOnlyEngineService) *StdioServer {
	return &StdioServer{
		engine: engine,
		reader: os.Stdin,
		writer: os.Stdout,
	}
}

// Run reads JSON-RPC requests from stdin line by line and writes responses to stdout.
// It blocks until stdin is closed or an unrecoverable error occurs.
func (s *StdioServer) Run() error {
	scanner := bufio.NewScanner(s.reader)
	// Allow up to 1MB per line for large requests.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	encoder := json.NewEncoder(s.writer)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var req jsonrpcRequest
		if err := json.Unmarshal(line, &req); err != nil {
			resp := jsonrpcResponse{
				JSONRPC: "2.0",
				Error: &jsonrpcError{
					Code:    -32700,
					Message: "parse error",
				},
			}
			encoder.Encode(resp)
			continue
		}

		resp := dispatch(req,
			getReadOnlyToolDefinitions,
			func(name string, args json.RawMessage) (interface{}, error) {
				return handleReadOnlyToolCall(s.engine, name, args)
			},
		)
		// Notifications (no ID) don't get responses.
		if resp.JSONRPC == "" && resp.ID == nil {
			continue
		}
		encoder.Encode(resp)
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("stdin read error: %w", err)
	}

	return nil
}
