package mcp

import (
	"encoding/json"
	"fmt"
)

// --- Shared JSON-RPC types ---

// jsonrpcRequest is a JSON-RPC 2.0 request.
type jsonrpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// jsonrpcResponse is a JSON-RPC 2.0 response.
type jsonrpcResponse struct {
	JSONRPC string        `json:"jsonrpc"`
	ID      interface{}   `json:"id,omitempty"`
	Result  interface{}   `json:"result,omitempty"`
	Error   *jsonrpcError `json:"error,omitempty"`
}

// jsonrpcError is a JSON-RPC 2.0 error object.
type jsonrpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// toolCallParams is the params for a tools/call method.
type toolCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

// contentItem is a single content block in MCP tool results.
type contentItem struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// toolResult wraps content items for tool call responses.
type toolResult struct {
	Content []contentItem `json:"content"`
}

// toolDefinition describes an MCP tool for tools/list.
type toolDefinition struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
}

// --- Shared dispatch ---

// toolHandler processes a tool call by name and returns the result.
type toolHandler func(toolName string, rawArgs json.RawMessage) (interface{}, error)

// toolLister returns the list of available tool definitions.
type toolLister func() []toolDefinition

// dispatch routes a JSON-RPC 2.0 request to the appropriate handler.
// The listTools and callTool functions are injected so that HTTP/SSE (full)
// and stdio (read-only) can use different tool sets.
func dispatch(req jsonrpcRequest, listTools toolLister, callTool toolHandler) jsonrpcResponse {
	switch req.Method {
	case "initialize":
		return jsonrpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]interface{}{
				"protocolVersion": "2024-11-05",
				"capabilities": map[string]interface{}{
					"tools": map[string]interface{}{},
				},
				"serverInfo": map[string]interface{}{
					"name":    "agent-memory",
					"version": "1.0.0",
				},
			},
		}

	case "notifications/initialized":
		// Client acknowledgment — no response required for notifications.
		return jsonrpcResponse{}

	case "tools/list":
		return jsonrpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]interface{}{
				"tools": listTools(),
			},
		}

	case "tools/call":
		var params toolCallParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return jsonrpcResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error:   &jsonrpcError{Code: -32602, Message: "invalid params"},
			}
		}
		result, err := callTool(params.Name, params.Arguments)
		if err != nil {
			return jsonrpcResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error:   &jsonrpcError{Code: -32000, Message: err.Error()},
			}
		}
		return jsonrpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  toolResult{Content: []contentItem{{Type: "text", Text: mustMarshal(result)}}},
		}

	default:
		return jsonrpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &jsonrpcError{Code: -32601, Message: fmt.Sprintf("method not found: %s", req.Method)},
		}
	}
}
