package main

import (
	"embed"
	"flag"
	"log"
	"os"
	"path/filepath"
	"strconv"

	"github.com/borzou/vecstore/internal/chunker"
	"github.com/borzou/vecstore/internal/embeddings"
	"github.com/borzou/vecstore/internal/engine"
	"github.com/borzou/vecstore/internal/extractor"
	"github.com/borzou/vecstore/internal/mcp"
	"github.com/borzou/vecstore/internal/store"
	"github.com/borzou/vecstore/internal/watcher"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	mcpMode := flag.Bool("mcp", false, "run as MCP stdio server")
	dbPath := flag.String("db", "", "database path (default ~/.agent-memory/agent-memory.db)")
	flag.Parse()

	// Resolve default DB path.
	if *dbPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			log.Fatalf("cannot determine home directory: %v", err)
		}
		*dbPath = filepath.Join(home, ".agent-memory", "agent-memory.db")
	}

	// --- MCP stdio mode (read-only, launched by Claude Desktop) ---
	if *mcpMode {
		s, err := store.NewReadOnlySQLiteStore(*dbPath)
		if err != nil {
			log.Fatalf("open store (read-only): %v", err)
		}
		defer s.Close()

		apiKey, _ := s.GetConfig("openai_api_key")
		model, _ := s.GetConfig("embedding_model")
		if model == "" {
			model = "text-embedding-3-small"
		}

		embedder := embeddings.NewOpenAIEmbedder(apiKey, model)
		roEngine := engine.NewReadOnly(s, embedder)

		stdio := mcp.NewStdioServer(roEngine)
		if err := stdio.Run(); err != nil {
			log.Fatalf("mcp stdio: %v", err)
		}
		return
	}

	// --- GUI mode ---

	// Ensure parent directory exists.
	if err := os.MkdirAll(filepath.Dir(*dbPath), 0755); err != nil {
		log.Fatalf("create data directory: %v", err)
	}

	// 1. Open store.
	s, err := store.NewSQLiteStore(*dbPath)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}

	// 2. Read config from store.
	apiKey, _ := s.GetConfig("openai_api_key")
	model, _ := s.GetConfig("embedding_model")
	if model == "" {
		model = "text-embedding-3-small"
	}

	// 3. Create embedder (may have empty API key on first run).
	embedder := embeddings.NewOpenAIEmbedder(apiKey, model)

	// 4. Create chunker with stored config.
	var chunkOpts []chunker.Option
	if sizeStr, _ := s.GetConfig("chunk_size"); sizeStr != "" {
		if size, err := strconv.Atoi(sizeStr); err == nil {
			chunkOpts = append(chunkOpts, chunker.WithChunkSize(size))
		}
	}
	if overlapStr, _ := s.GetConfig("chunk_overlap"); overlapStr != "" {
		if overlap, err := strconv.Atoi(overlapStr); err == nil {
			chunkOpts = append(chunkOpts, chunker.WithOverlap(overlap))
		}
	}
	c, err := chunker.New(chunkOpts...)
	if err != nil {
		log.Fatalf("create chunker: %v", err)
	}

	// 5. Create watcher.
	w, err := watcher.NewFSWatcher()
	if err != nil {
		log.Fatalf("create watcher: %v", err)
	}

	// 6. Create extractor and engine.
	ext := extractor.NewFileExtractor()
	eng := engine.New(s, embedder, c, w, ext)

	// Ensure auth token exists.
	authToken, _ := s.GetConfig("auth_token")
	if authToken == "" {
		authToken, err = generateToken()
		if err != nil {
			log.Fatalf("generate auth token: %v", err)
		}
		if err := s.SetConfig("auth_token", authToken); err != nil {
			log.Fatalf("store auth token: %v", err)
		}
	}

	// MCP HTTP port.
	port, _ := s.GetConfig("mcp_port")
	if port == "" {
		port = "9847"
	}

	mcpServer := mcp.NewServer(eng, port, authToken)

	app := &App{
		engine:      eng,
		store:       s,
		mcpServer:   mcpServer,
		dbPath:      *dbPath,
		newEmbedder: func(apiKey, model string) embeddings.Embedder {
			return embeddings.NewOpenAIEmbedder(apiKey, model)
		},
	}
	appInstance = app

	err = wails.Run(&options.App{
		Title:             "Agent Memory",
		Width:             900,
		Height:            700,
		HideWindowOnClose: true,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup:  app.startup,
		OnShutdown: app.shutdown,
		Bind:       []interface{}{app},
	})
	if err != nil {
		log.Fatalf("wails: %v", err)
	}
}
