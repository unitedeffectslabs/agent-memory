package mcp

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/borzou/vecstore/internal/domain"
)

// ReadOnlyEngineService defines the read-only operations available via stdio MCP.
type ReadOnlyEngineService interface {
	Search(params domain.SearchParams) ([]domain.SearchResult, error)
	ListDirectories() ([]domain.Directory, error)
	Stats() (domain.IndexStats, error)
	GetIgnorePatterns() ([]string, error)
}

// EngineService defines the full engine operations the HTTP/SSE MCP server needs.
// It extends ReadOnlyEngineService with write operations.
type EngineService interface {
	ReadOnlyEngineService
	AddDirectory(path string) error
	RemoveDirectory(path string) error
	Start() error
	Stop() error
	Restart() error
	Reset() error
	SetIgnorePatterns(patterns []string) error
}

// sseSession holds the response channel for an active SSE client connection.
type sseSession struct {
	ch   chan []byte    // buffered channel; server sends serialised responses here
	done chan struct{}  // closed when the SSE connection is dropped
}

// Server is the MCP HTTP/SSE server.
//
// Transport (per MCP spec):
//   - GET  /sse          — client establishes a long-lived SSE connection.
//     The server immediately emits an "endpoint" event containing the URL
//     the client should POST requests to.
//   - POST /messages?sessionId=<id> — client sends a JSON-RPC 2.0 request.
//     The server returns HTTP 202 and pushes the JSON-RPC response back over
//     the matching SSE connection.
type Server struct {
	engine   EngineService
	port     string
	httpSrv  *http.Server
	sessions sync.Map // sessionID(string) -> *sseSession

	authMu    sync.RWMutex
	authToken string
}

// NewServer creates a new MCP HTTP/SSE server.
func NewServer(engine EngineService, port string, authToken string) *Server {
	return &Server{
		engine:    engine,
		authToken: authToken,
		port:      port,
	}
}

// SetAuthToken updates the bearer token used to authenticate MCP requests.
func (s *Server) SetAuthToken(token string) {
	s.authMu.Lock()
	defer s.authMu.Unlock()
	s.authToken = token
}

// Start begins listening on 127.0.0.1:port.
func (s *Server) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/sse", s.handleSSE)
	mux.HandleFunc("/messages", s.handleMessages)

	s.httpSrv = &http.Server{
		Addr:    net.JoinHostPort("127.0.0.1", s.port),
		Handler: mux,
	}

	return s.httpSrv.ListenAndServe()
}

// Stop gracefully shuts down the HTTP server.
func (s *Server) Stop() error {
	if s.httpSrv == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return s.httpSrv.Shutdown(ctx)
}

// handleSSE handles GET /sse. It establishes a long-lived SSE connection,
// immediately sends the session's message endpoint URL, and then streams
// JSON-RPC responses back as they arrive.
func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.checkAuth(r) {
		http.Error(w, "unauthorized: invalid or missing bearer token", http.StatusUnauthorized)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	sessionID := newSessionID()
	sess := &sseSession{
		ch:   make(chan []byte, 64),
		done: make(chan struct{}),
	}
	s.sessions.Store(sessionID, sess)
	defer func() {
		s.sessions.Delete(sessionID)
		close(sess.done)
	}()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	// Send the endpoint event so the client knows where to POST requests.
	fmt.Fprintf(w, "event: endpoint\ndata: /messages?sessionId=%s\n\n", sessionID)
	flusher.Flush()

	for {
		select {
		case msg := <-sess.ch:
			fmt.Fprintf(w, "event: message\ndata: %s\n\n", msg)
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

// handleMessages handles POST /messages?sessionId=<id>. It parses the
// JSON-RPC 2.0 request, dispatches it, and pushes the response to the
// client's SSE connection. Returns HTTP 202 Accepted immediately.
func (s *Server) handleMessages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.checkAuth(r) {
		http.Error(w, "unauthorized: invalid or missing bearer token", http.StatusUnauthorized)
		return
	}

	sessionID := r.URL.Query().Get("sessionId")
	val, ok := s.sessions.Load(sessionID)
	if !ok {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	sess := val.(*sseSession)

	var req jsonrpcRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Dispatch asynchronously so we can return 202 immediately.
	go func() {
		resp := dispatch(req,
			getToolDefinitions,
			func(name string, args json.RawMessage) (interface{}, error) {
				return handleToolCall(s.engine, name, args)
			},
		)
		// Notifications (no ID) don't produce a response.
		if resp.JSONRPC == "" && resp.ID == nil {
			return
		}
		b, err := json.Marshal(resp)
		if err != nil {
			return
		}
		select {
		case sess.ch <- b:
		case <-sess.done:
		}
	}()

	w.WriteHeader(http.StatusAccepted)
}

func (s *Server) checkAuth(r *http.Request) bool {
	s.authMu.RLock()
	defer s.authMu.RUnlock()
	return r.Header.Get("Authorization") == "Bearer "+s.authToken
}

// handleToolCall is the shared dispatch logic used by both HTTP/SSE and stdio transports.
func handleToolCall(engine EngineService, toolName string, rawArgs json.RawMessage) (interface{}, error) {
	switch toolName {
	case "search":
		var args struct {
			Query     string  `json:"query"`
			Limit     int     `json:"limit"`
			Offset    int     `json:"offset"`
			Threshold float32 `json:"threshold"`
		}
		if err := json.Unmarshal(rawArgs, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments for search: %w", err)
		}
		return engine.Search(domain.SearchParams{
			Query:     args.Query,
			Limit:     args.Limit,
			Offset:    args.Offset,
			Threshold: args.Threshold,
		})

	case "add_directory":
		var args struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal(rawArgs, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments for add_directory: %w", err)
		}
		if args.Path == "" {
			return nil, fmt.Errorf("path is required")
		}
		if err := engine.AddDirectory(args.Path); err != nil {
			return nil, err
		}
		return map[string]string{"status": "ok"}, nil

	case "remove_directory":
		var args struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal(rawArgs, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments for remove_directory: %w", err)
		}
		if args.Path == "" {
			return nil, fmt.Errorf("path is required")
		}
		if err := engine.RemoveDirectory(args.Path); err != nil {
			return nil, err
		}
		return map[string]string{"status": "ok"}, nil

	case "list_directories":
		return engine.ListDirectories()

	case "index_status":
		return engine.Stats()

	case "start":
		if err := engine.Start(); err != nil {
			return nil, err
		}
		return map[string]string{"status": "ok"}, nil

	case "stop":
		if err := engine.Stop(); err != nil {
			return nil, err
		}
		return map[string]string{"status": "ok"}, nil

	case "restart":
		if err := engine.Restart(); err != nil {
			return nil, err
		}
		return map[string]string{"status": "ok"}, nil

	case "reset":
		if err := engine.Reset(); err != nil {
			return nil, err
		}
		return map[string]string{"status": "ok"}, nil

	case "get_ignore_patterns":
		return engine.GetIgnorePatterns()

	case "set_ignore_patterns":
		var args struct {
			Patterns []string `json:"patterns"`
		}
		if err := json.Unmarshal(rawArgs, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments for set_ignore_patterns: %w", err)
		}
		if err := engine.SetIgnorePatterns(args.Patterns); err != nil {
			return nil, err
		}
		return map[string]string{"status": "ok"}, nil

	default:
		return nil, fmt.Errorf("unknown tool: %s", toolName)
	}
}

// getToolDefinitions returns the list of all MCP tool definitions.
func getToolDefinitions() []toolDefinition {
	return []toolDefinition{
		{
			Name:        "search",
			Description: "Semantic search across all indexed content. Returns ranked results with file path, chunk text, and cosine distance score (lower = more similar).",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"query": map[string]interface{}{
						"type":        "string",
						"description": "The search query text.",
					},
					"limit": map[string]interface{}{
						"type":        "integer",
						"description": "Maximum number of results to return. Default: 10.",
					},
					"offset": map[string]interface{}{
						"type":        "integer",
						"description": "Number of results to skip for pagination. Default: 0.",
					},
					"threshold": map[string]interface{}{
						"type":        "number",
						"description": "Maximum cosine distance for results. Lower values mean stricter matching. Default: 1.5. Set to 0 to disable filtering.",
					},
				},
				"required": []string{"query"},
			},
		},
		{
			Name:        "add_directory",
			Description: "Add a directory to watch and trigger initial indexing.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"path": map[string]interface{}{
						"type":        "string",
						"description": "Absolute path to the directory to watch.",
					},
				},
				"required": []string{"path"},
			},
		},
		{
			Name:        "remove_directory",
			Description: "Stop watching a directory and remove its indexed data.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"path": map[string]interface{}{
						"type":        "string",
						"description": "Absolute path to the directory to remove.",
					},
				},
				"required": []string{"path"},
			},
		},
		{
			Name:        "list_directories",
			Description: "List all watched directories with file count, chunk count, and status.",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
		{
			Name:        "index_status",
			Description: "Returns total files, total chunks, last indexed timestamp, currently indexing flag, and embedding model in use.",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
		{
			Name:        "start",
			Description: "Start the file watcher if it is stopped.",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
		{
			Name:        "stop",
			Description: "Stop the file watcher.",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
		{
			Name:        "restart",
			Description: "Stop then start the file watcher.",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
		{
			Name:        "reset",
			Description: "Clear all embeddings and re-index everything from scratch.",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
		{
			Name:        "get_ignore_patterns",
			Description: "Returns the current list of glob patterns used to ignore files and directories during indexing.",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
		{
			Name:        "set_ignore_patterns",
			Description: "Replace the ignore pattern list. Patterns use glob syntax with ** support (e.g. node_modules/**).",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"patterns": map[string]interface{}{
						"type":        "array",
						"items":       map[string]interface{}{"type": "string"},
						"description": "Array of glob patterns to ignore.",
					},
				},
				"required": []string{"patterns"},
			},
		},
	}
}

func mustMarshal(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf(`{"error": "marshal failed: %s"}`, err.Error())
	}
	return string(b)
}

// handleReadOnlyToolCall handles tool calls for the read-only stdio transport.
func handleReadOnlyToolCall(engine ReadOnlyEngineService, toolName string, rawArgs json.RawMessage) (interface{}, error) {
	switch toolName {
	case "search":
		var args struct {
			Query     string  `json:"query"`
			Limit     int     `json:"limit"`
			Offset    int     `json:"offset"`
			Threshold float32 `json:"threshold"`
		}
		if err := json.Unmarshal(rawArgs, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments for search: %w", err)
		}
		return engine.Search(domain.SearchParams{
			Query:     args.Query,
			Limit:     args.Limit,
			Offset:    args.Offset,
			Threshold: args.Threshold,
		})

	case "list_directories":
		return engine.ListDirectories()

	case "index_status":
		return engine.Stats()

	case "get_ignore_patterns":
		return engine.GetIgnorePatterns()

	default:
		return nil, fmt.Errorf("unknown tool: %s", toolName)
	}
}

// getReadOnlyToolDefinitions returns the tool definitions for the read-only stdio transport.
func getReadOnlyToolDefinitions() []toolDefinition {
	return []toolDefinition{
		{
			Name:        "search",
			Description: "Semantic search across all indexed content. Returns ranked results with file path, chunk text, and cosine distance score (lower = more similar).",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"query": map[string]interface{}{
						"type":        "string",
						"description": "The search query text.",
					},
					"limit": map[string]interface{}{
						"type":        "integer",
						"description": "Maximum number of results to return. Default: 10.",
					},
					"offset": map[string]interface{}{
						"type":        "integer",
						"description": "Number of results to skip for pagination. Default: 0.",
					},
					"threshold": map[string]interface{}{
						"type":        "number",
						"description": "Maximum cosine distance for results. Lower values mean stricter matching. Default: 1.5. Set to 0 to disable filtering.",
					},
				},
				"required": []string{"query"},
			},
		},
		{
			Name:        "list_directories",
			Description: "List all watched directories with file count, chunk count, and status.",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
		{
			Name:        "index_status",
			Description: "Returns total files, total chunks, last indexed timestamp, currently indexing flag, and embedding model in use.",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
		{
			Name:        "get_ignore_patterns",
			Description: "Returns the current list of glob patterns used to ignore files and directories during indexing.",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
	}
}

func newSessionID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

