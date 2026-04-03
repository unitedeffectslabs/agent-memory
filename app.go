package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/borzou/vecstore/internal/domain"
	"github.com/borzou/vecstore/internal/embeddings"
	"github.com/borzou/vecstore/internal/engine"
	"github.com/borzou/vecstore/internal/mcp"
	"github.com/borzou/vecstore/internal/store"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// appInstance is a package-level reference so CGo tray callbacks can reach the app.
var appInstance *App

// EmbedderFactory creates an Embedder for the given API key and model.
// Injected by main.go so app.go doesn't depend on concrete embedder packages.
type EmbedderFactory func(apiKey, model string) embeddings.Embedder

// App exposes methods to the Wails frontend.
type App struct {
	ctx             context.Context
	engine          *engine.Engine
	store           *store.SQLiteStore
	mcpServer       *mcp.Server
	dbPath          string
	stopTray        func() // cleanup function for systray
	newEmbedder     EmbedderFactory
}

// startup is called by Wails when the app starts.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	// Start the MCP HTTP server in the background.
	// The engine/watcher is NOT auto-started — the user controls that
	// from the Dashboard so they can configure the embedding model first.
	go func() {
		if err := a.mcpServer.Start(); err != nil {
			log.Printf("mcp server: %v", err)
		}
	}()

	// Start system tray icon.
	a.stopTray = a.setupTray()
}

// shutdown is called by Wails when the app is closing.
func (a *App) shutdown(ctx context.Context) {
	if a.stopTray != nil {
		a.stopTray()
	}
	if err := a.mcpServer.Stop(); err != nil {
		log.Printf("mcp server stop: %v", err)
	}
	if err := a.engine.Close(); err != nil {
		log.Printf("engine close: %v", err)
	}
}

// GetStats returns current index statistics.
func (a *App) GetStats() (domain.IndexStats, error) {
	stats, err := a.engine.Stats()
	if err != nil {
		log.Printf("GetStats error: %v", err)
		return stats, err
	}
	return stats, nil
}

// ListDirectories returns all watched directories.
func (a *App) ListDirectories() ([]domain.Directory, error) {
	dirs, err := a.engine.ListDirectories()
	if err != nil {
		log.Printf("ListDirectories error: %v", err)
		return nil, err
	}
	return dirs, nil
}

// AddDirectory adds a directory to be watched and indexed.
func (a *App) AddDirectory(path string) error {
	log.Printf("[AddDirectory] path=%q", path)
	err := a.engine.AddDirectory(path)
	if err != nil {
		log.Printf("[AddDirectory] error: %v", err)
	} else {
		log.Printf("[AddDirectory] success")
	}
	return err
}

// RegisterDirectory saves a directory to the DB without triggering indexing.
// Used during onboarding so the UI doesn't block. Indexing starts when Start() is called.
func (a *App) RegisterDirectory(path string) error {
	log.Printf("[RegisterDirectory] path=%q", path)
	return a.store.AddDirectory(path)
}

// RemoveRegisteredDirectory removes a directory from the DB only (no engine/watcher cleanup).
// Used during onboarding before the engine is running.
func (a *App) RemoveRegisteredDirectory(path string) error {
	log.Printf("[RemoveRegisteredDirectory] path=%q", path)
	return a.store.RemoveDirectory(path)
}

// RemoveDirectory stops watching a directory and removes its data.
func (a *App) RemoveDirectory(path string) error {
	return a.engine.RemoveDirectory(path)
}

// Start starts the file watcher.
func (a *App) Start() error {
	return a.engine.Start()
}

// Stop stops the file watcher.
func (a *App) Stop() error {
	return a.engine.Stop()
}

// Restart stops then starts the file watcher.
func (a *App) Restart() error {
	return a.engine.Restart()
}

// Reset clears all embeddings and re-indexes from scratch.
func (a *App) Reset() error {
	return a.engine.Reset()
}

// IsRunning returns whether the file watcher is currently running.
func (a *App) IsRunning() bool {
	return a.engine.IsRunning()
}

// GetConfig reads a configuration value from the store.
func (a *App) GetConfig(key string) string {
	val, err := a.store.GetConfig(key)
	if err != nil {
		log.Printf("get config %s: %v", key, err)
	}
	return val
}

// SetConfig writes a configuration value to the store.
// If the embedding model changes, the embedder is swapped and the index is
// reset — the virtual table must be recreated with the new dimension.
// If the API key changes, the embedder is swapped so new requests use it.
func (a *App) SetConfig(key, value string) error {
	switch key {
	case "embedding_model":
		oldModel, _ := a.store.GetConfig("embedding_model")
		if oldModel == "" {
			oldModel = "text-embedding-3-small"
		}
		if oldModel != value {
			log.Printf("[SetConfig] embedding model changed %s → %s — resetting index", oldModel, value)
			if err := a.store.SetConfig(key, value); err != nil {
				return err
			}
			apiKey, _ := a.store.GetConfig("openai_api_key")
			a.engine.SetEmbedder(a.newEmbedder(apiKey, value))
			return a.engine.Reset()
		}

	case "openai_api_key":
		if err := a.store.SetConfig(key, value); err != nil {
			return err
		}
		model, _ := a.store.GetConfig("embedding_model")
		if model == "" {
			model = "text-embedding-3-small"
		}
		a.engine.SetEmbedder(a.newEmbedder(value, model))
		return nil
	}

	return a.store.SetConfig(key, value)
}

// RotateAuthToken generates a new cryptographically random bearer token,
// stores it in the database, and returns it.
func (a *App) RotateAuthToken() (string, error) {
	token, err := generateToken()
	if err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	if err := a.store.SetConfig("auth_token", token); err != nil {
		return "", fmt.Errorf("store token: %w", err)
	}
	a.mcpServer.SetAuthToken(token)
	return token, nil
}

// GetIgnorePatterns returns the current list of ignore glob patterns.
func (a *App) GetIgnorePatterns() []string {
	patterns, err := a.engine.GetIgnorePatterns()
	if err != nil {
		log.Printf("get ignore patterns: %v", err)
		return nil
	}
	return patterns
}

// SetIgnorePatterns replaces the ignore pattern list.
func (a *App) SetIgnorePatterns(patterns []string) error {
	return a.engine.SetIgnorePatterns(patterns)
}

// ActivityLogResponse wraps log entries with a total count for pagination.
type ActivityLogResponse struct {
	Entries []domain.ActivityLogEntry
	Total   int
}

// GetActivityLog returns paginated activity log entries.
func (a *App) GetActivityLog(limit, offset int) (ActivityLogResponse, error) {
	entries, total, err := a.engine.ListLogEntries(limit, offset)
	if err != nil {
		log.Printf("GetActivityLog error: %v", err)
		return ActivityLogResponse{}, err
	}
	return ActivityLogResponse{Entries: entries, Total: total}, nil
}

// DeleteDatabaseAndQuit removes the database file (and WAL/SHM) then quits the app.
func (a *App) DeleteDatabaseAndQuit() error {
	// Close the store so the DB file is released.
	if err := a.store.Close(); err != nil {
		log.Printf("close store: %v", err)
	}

	// Remove the database and its WAL/SHM companions.
	for _, suffix := range []string{"", "-wal", "-shm"} {
		p := a.dbPath + suffix
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove %s: %w", p, err)
		}
	}

	// Try to remove the parent directory if it's now empty.
	_ = os.Remove(filepath.Dir(a.dbPath))

	// Quit the app.
	runtime.Quit(a.ctx)
	return nil
}

// SelectDirectory opens a native folder picker and returns the selected path.
func (a *App) SelectDirectory() string {
	home, _ := os.UserHomeDir()
	log.Printf("[SelectDirectory] opening dialog, default=%s", home)
	dir, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title:            "Select a Directory to Watch",
		DefaultDirectory: home,
	})
	if err != nil {
		log.Printf("[SelectDirectory] error: %v", err)
		return ""
	}
	log.Printf("[SelectDirectory] user picked: %q", dir)
	return dir
}

// generateToken returns a 32-byte random hex string.
func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("read random bytes: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// defaultClaudeDesktopConfigPath returns the standard Claude Desktop config location.
func defaultClaudeDesktopConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "Application Support", "Claude", "claude_desktop_config.json")
}

// GetClaudeDesktopConfigPath returns the current config path (custom or default).
func (a *App) GetClaudeDesktopConfigPath() string {
	custom, _ := a.store.GetConfig("claude_desktop_config_path")
	if custom != "" {
		return custom
	}
	return defaultClaudeDesktopConfigPath()
}

// SetClaudeDesktopConfigPath stores a custom config file path.
func (a *App) SetClaudeDesktopConfigPath(path string) error {
	return a.store.SetConfig("claude_desktop_config_path", path)
}

// InstallToClaudeDesktop writes the agent-memory MCP server entry into
// Claude Desktop's config file. Creates a backup before modifying.
func (a *App) InstallToClaudeDesktop() (string, error) {
	configPath := a.GetClaudeDesktopConfigPath()

	// Resolve our binary path.
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve executable path: %w", err)
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return "", fmt.Errorf("resolve symlinks: %w", err)
	}

	// Read existing config or start fresh.
	config := map[string]interface{}{}
	if data, err := os.ReadFile(configPath); err == nil {
		// Backup the original before modifying.
		backupPath := configPath + ".backup"
		if err := copyFile(configPath, backupPath); err != nil {
			return "", fmt.Errorf("create backup: %w", err)
		}

		if err := json.Unmarshal(data, &config); err != nil {
			return "", fmt.Errorf("parse existing config: %w", err)
		}
	}

	// Ensure mcpServers map exists.
	servers, ok := config["mcpServers"].(map[string]interface{})
	if !ok {
		servers = map[string]interface{}{}
		config["mcpServers"] = servers
	}

	// Add/update our entry.
	servers["agent-memory"] = map[string]interface{}{
		"command": exe,
		"args":    []string{"--mcp"},
	}

	// Ensure parent directory exists.
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		return "", fmt.Errorf("create config directory: %w", err)
	}

	// Write back with pretty formatting.
	out, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal config: %w", err)
	}
	if err := os.WriteFile(configPath, out, 0644); err != nil {
		return "", fmt.Errorf("write config: %w", err)
	}

	return "Installed. Restart Claude Desktop to activate.", nil
}

// copyFile copies src to dst, preserving content.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
