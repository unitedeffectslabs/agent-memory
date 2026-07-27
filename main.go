package main

import (
	"embed"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"

	"github.com/borzou/vecstore/internal/embeddings"
	"github.com/borzou/vecstore/internal/embeddings/local"
	"github.com/borzou/vecstore/internal/engine"
	"github.com/borzou/vecstore/internal/extractor"
	"github.com/borzou/vecstore/internal/mcp"
	"github.com/borzou/vecstore/internal/store"
	"github.com/borzou/vecstore/internal/watcher"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

// makeEmbedder is the composition root's EmbedderFactory. Constructors are cheap
// and lazy: local.New does not load the model until the first embed call, so the
// read-only MCP process can build one without forcing native libraries to load.
func makeEmbedder(provider, apiKey, model string) (embeddings.Embedder, error) {
	switch provider {
	case embeddings.ProviderLocal:
		return local.New(local.Config{}), nil
	case embeddings.ProviderOpenAI:
		return embeddings.NewOpenAIEmbedder(apiKey, model), nil
	default:
		return nil, fmt.Errorf("unknown embedding provider %q", provider)
	}
}

// peekConfig reads the provider-selection config from an existing database
// without migrating it. A fresh/uninitialized DB yields empty values, which the
// resolver treats as "brand new install → default provider".
func peekConfig(dbPath string) (provider, apiKey, model string) {
	ro, err := store.NewReadOnlySQLiteStore(dbPath)
	if err != nil {
		return "", "", ""
	}
	defer ro.Close()
	provider, _ = ro.GetConfig("embedding_provider")
	apiKey, _ = ro.GetConfig("openai_api_key")
	model, _ = ro.GetConfig("embedding_model")
	return provider, apiKey, model
}

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

		providerCfg, _ := s.GetConfig("embedding_provider")
		apiKey, _ := s.GetConfig("openai_api_key")
		modelCfg, _ := s.GetConfig("embedding_model")
		provider := resolveProvider(providerCfg, apiKey)
		model := resolveModel(provider, modelCfg)

		embedder, err := makeEmbedder(provider, apiKey, model)
		if err != nil {
			log.Fatalf("create embedder: %v", err)
		}
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

	// 1. Peek existing config (if any) to resolve the provider BEFORE opening
	//    the store, so a fresh DB's vector table is created at the correct
	//    dimension. A fresh/uninitialized DB peeks empty → default provider.
	peekProvider, peekKey, peekModel := peekConfig(*dbPath)
	provider := resolveProvider(peekProvider, peekKey)
	model := resolveModel(provider, peekModel)
	dim := embeddings.DefaultDimension(provider, model)

	// 2. Open store with the resolved dimension.
	s, err := store.NewSQLiteStore(*dbPath, dim)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}

	// 3. Persist the resolved provider/model so Stats and later config changes
	//    reflect reality (idempotent; harmless on re-launch).
	apiKey, _ := s.GetConfig("openai_api_key")
	if err := s.SetConfig("embedding_provider", provider); err != nil {
		log.Fatalf("persist embedding_provider: %v", err)
	}
	if err := s.SetConfig("embedding_model", model); err != nil {
		log.Fatalf("persist embedding_model: %v", err)
	}

	// 3b. Backfill onboarding_complete for pre-existing users. If the flag has
	//     never been set but the DB already has watched directories or an OpenAI
	//     key, this is an upgraded install that should skip onboarding. A truly
	//     fresh DB (no dirs, no key) leaves the flag unset so onboarding runs.
	if complete, _ := s.GetConfig("onboarding_complete"); complete == "" {
		dirs, _ := s.ListDirectories()
		if len(dirs) > 0 || apiKey != "" {
			if err := s.SetConfig("onboarding_complete", "true"); err != nil {
				log.Fatalf("persist onboarding_complete: %v", err)
			}
		}
	}

	// 4. Create the provider-matched embedder and chunker.
	embedder, err := makeEmbedder(provider, apiKey, model)
	if err != nil {
		log.Fatalf("create embedder: %v", err)
	}
	c, err := buildChunker(s, provider, embedder)
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
		newEmbedder: makeEmbedder,
	}
	appInstance = app

	err = wails.Run(&options.App{
		Title:             "Agent Memory",
		Width:             900,
		Height:            700,
		// Hide-on-close pairs with the status-bar tray (Show/Quit menu), which
		// is macOS-only Objective-C (tray.go). On platforms without the tray,
		// hiding would leave an invisible process with no way to surface or
		// quit it — relaunches then stack zombie instances that contend for
		// the single-writer SQLite DB (observed on Windows: six concurrent
		// instances). Close = quit everywhere except macOS.
		HideWindowOnClose: runtime.GOOS == "darwin",
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
