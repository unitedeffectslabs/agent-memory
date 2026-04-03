package engine

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/borzou/vecstore/internal/chunker"
	"github.com/borzou/vecstore/internal/domain"
	"github.com/borzou/vecstore/internal/embeddings"
	"github.com/borzou/vecstore/internal/extractor"
	"github.com/borzou/vecstore/internal/store"
	"github.com/borzou/vecstore/internal/watcher"

)

// DefaultIgnorePatterns are the patterns seeded into config on first launch.
var DefaultIgnorePatterns = []string{
	"node_modules/**",
	".git/**",
	"__pycache__/**",
	"*.pyc",
	".DS_Store",
	"Thumbs.db",
	"build/**",
	"dist/**",
	".next/**",
	".cache/**",
	"*.lock",
	"package-lock.json",
	"vendor/**",
	".env",
	".env.*",
	"*.env",
}

// Engine orchestrates the indexing pipeline. It depends only on interfaces,
// never on concrete implementations.
type Engine struct {
	store     store.Store
	embedder  embeddings.Embedder
	chunker   chunker.Chunker
	watcher   watcher.Watcher
	extractor extractor.Extractor
	mu        sync.Mutex
	indexing  bool
	stopCh    chan struct{} // closed by Stop() to cancel in-flight indexing
	// Progress tracking
	indexedFiles int
	totalToIndex int
}

// New creates an Engine with the given dependencies.
func New(s store.Store, e embeddings.Embedder, c chunker.Chunker, w watcher.Watcher, ext extractor.Extractor) *Engine {
	return &Engine{
		store:     s,
		embedder:  e,
		chunker:   c,
		watcher:   w,
		extractor: ext,
	}
}

// SetEmbedder swaps the embedder (e.g. after changing the embedding model).
// Must be called while the engine is stopped.
func (eng *Engine) SetEmbedder(e embeddings.Embedder) {
	eng.embedder = e
}

// GetIgnorePatterns returns the current ignore pattern list. If none have been
// configured yet, it seeds and persists the defaults.
func (eng *Engine) GetIgnorePatterns() ([]string, error) {
	val, err := eng.store.GetConfig("ignore_patterns")
	if err != nil {
		return nil, fmt.Errorf("get ignore_patterns config: %w", err)
	}
	if val == "" {
		// First launch — seed defaults.
		if err := eng.SetIgnorePatterns(DefaultIgnorePatterns); err != nil {
			return nil, err
		}
		return DefaultIgnorePatterns, nil
	}
	var patterns []string
	if err := json.Unmarshal([]byte(val), &patterns); err != nil {
		return nil, fmt.Errorf("parse ignore_patterns: %w", err)
	}
	return patterns, nil
}

// SetIgnorePatterns replaces the ignore pattern list in the config store.
func (eng *Engine) SetIgnorePatterns(patterns []string) error {
	b, err := json.Marshal(patterns)
	if err != nil {
		return fmt.Errorf("marshal ignore_patterns: %w", err)
	}
	return eng.store.SetConfig("ignore_patterns", string(b))
}

// shouldIgnore reports whether the file at path should be skipped based on
// the configured ignore patterns. The path is matched relative to the watched
// directory that contains it.
func (eng *Engine) shouldIgnore(path string) (bool, error) {
	patterns, err := eng.GetIgnorePatterns()
	if err != nil {
		return false, err
	}
	if len(patterns) == 0 {
		return false, nil
	}

	dirs, err := eng.store.ListDirectories()
	if err != nil {
		return false, err
	}

	var baseDir string
	for _, d := range dirs {
		if strings.HasPrefix(path, d.Path+string(filepath.Separator)) ||
			strings.HasPrefix(path, d.Path+"/") {
			baseDir = d.Path
			break
		}
	}
	if baseDir == "" {
		return false, nil
	}

	return matchesAnyPattern(patterns, baseDir, path)
}

// matchesAnyPattern checks whether path (relative to baseDir) matches any of
// the given glob patterns using doublestar for ** support.
func matchesAnyPattern(patterns []string, baseDir, path string) (bool, error) {
	relPath, err := filepath.Rel(baseDir, path)
	if err != nil {
		return false, err
	}
	relPath = filepath.ToSlash(relPath)

	for _, pattern := range patterns {
		matched, err := doublestar.Match(pattern, relPath)
		if err != nil {
			continue // invalid pattern — skip
		}
		if matched {
			return true, nil
		}
	}
	return false, nil
}

// shouldSkipDir reports whether the directory at relDir should be skipped
// entirely (i.e., no files underneath it will be indexed). It handles patterns
// like "node_modules/**" by checking whether relDir is the covered prefix.
func shouldSkipDir(patterns []string, relDir string) bool {
	for _, p := range patterns {
		// Direct match (e.g., pattern ".git" matches dir ".git").
		if matched, _ := doublestar.Match(p, relDir); matched {
			return true
		}
		// Pattern covers everything under this dir (e.g., "node_modules/**").
		if strings.HasSuffix(p, "/**") {
			dirPart := strings.TrimSuffix(p, "/**")
			if matched, _ := doublestar.Match(dirPart, relDir); matched {
				return true
			}
		}
	}
	return false
}

// logActivity writes a log entry to the store. Errors are logged but don't propagate.
func (eng *Engine) logActivity(path, action, detail string) {
	entry := domain.ActivityLogEntry{
		Timestamp: time.Now(),
		Path:      path,
		Action:    action,
		Detail:    detail,
	}
	if err := eng.store.InsertLogEntry(entry); err != nil {
		log.Printf("engine: log activity: %v", err)
	}
}

// ListLogEntries delegates to the store.
func (eng *Engine) ListLogEntries(limit, offset int) ([]domain.ActivityLogEntry, int, error) {
	return eng.store.ListLogEntries(limit, offset)
}

// IndexFile extracts text from a file, hashes it, and runs the chunk-embed-store
// pipeline if the content has changed since the last index.
func (eng *Engine) IndexFile(path string) error {
	// 1. Check if the file type is supported at all.
	if !eng.extractor.IsSupported(path) {
		eng.logActivity(path, "ignored", "unsupported file type")
		return nil
	}

	// 2. Hash the raw file for change detection.
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read file %s: %w", path, err)
	}
	hash := fmt.Sprintf("%x", sha256.Sum256(data))

	// 3. Check store for existing file — if hash matches, skip.
	existing, err := eng.store.GetFileByPath(path)
	if err != nil {
		return fmt.Errorf("get file by path %s: %w", path, err)
	}
	if existing != nil && existing.Hash == hash {
		return nil // unchanged
	}

	// 4. Extract text content (handles text, docx, xlsx, pptx, metadata, etc.)
	result, err := eng.extractor.Extract(path)
	if err != nil {
		eng.logActivity(path, "error", fmt.Sprintf("extract: %v", err))
		return fmt.Errorf("extract %s: %w", path, err)
	}
	if result.Text == "" {
		return nil
	}

	// 5. Chunk the extracted text.
	chunks, err := eng.chunker.ChunkText(result.Text)
	if err != nil {
		return fmt.Errorf("chunk %s: %w", path, err)
	}
	if len(chunks) == 0 {
		return nil
	}

	// 6. Embed chunks in batches to stay under API token limits.
	vectors, err := eng.embedBatched(chunks)
	if err != nil {
		return fmt.Errorf("embed %s: %w", path, err)
	}

	// 7. Build domain.Chunk slice with embeddings.
	domainChunks := make([]domain.Chunk, len(chunks))
	for i, c := range chunks {
		domainChunks[i] = domain.Chunk{
			Index:      c.Index,
			Content:    c.Content,
			Embedding:  vectors[i],
			TokenCount: c.TokenCount,
		}
	}

	// 8. Find directory ID from store.
	dirID, err := eng.findDirectoryID(path)
	if err != nil {
		return fmt.Errorf("find directory for %s: %w", path, err)
	}

	// 9. Remove old chunks if file existed.
	if existing != nil {
		if err := eng.store.RemoveChunksByFile(existing.ID); err != nil {
			return fmt.Errorf("remove old chunks for %s: %w", path, err)
		}
	}

	// 10. Upsert file record.
	file := domain.File{
		DirectoryID: dirID,
		Path:        path,
		Hash:        hash,
		IndexedAt:   time.Now(),
	}
	if existing != nil {
		file.ID = existing.ID
	}
	if err := eng.store.UpsertFile(file); err != nil {
		return fmt.Errorf("upsert file %s: %w", path, err)
	}

	// 11. Insert new chunks. Re-fetch the file ID because INSERT OR REPLACE
	// may have assigned a new auto-increment ID.
	stored, err := eng.store.GetFileByPath(path)
	if err != nil {
		return fmt.Errorf("get file after upsert %s: %w", path, err)
	}
	if stored == nil {
		return fmt.Errorf("file not found after upsert: %s", path)
	}
	file.ID = stored.ID
	if err := eng.store.InsertChunks(file.ID, domainChunks); err != nil {
		return fmt.Errorf("insert chunks for %s: %w", path, err)
	}

	eng.logActivity(path, "indexed", fmt.Sprintf("%d chunks", len(domainChunks)))
	return nil
}

// maxTokensPerBatch is the max tokens per OpenAI embedding API call.
const maxTokensPerBatch = 250000 // conservative, API limit is 300K

// embedBatched sends chunks to the embedder in batches that fit within the API
// token limit. Returns all vectors in order.
func (eng *Engine) embedBatched(chunks []chunker.ChunkResult) ([][]float32, error) {
	var allVectors [][]float32
	batchStart := 0
	batchTokens := 0

	for i, c := range chunks {
		if batchTokens+c.TokenCount > maxTokensPerBatch && i > batchStart {
			// Send current batch
			vecs, err := eng.embedSlice(chunks[batchStart:i])
			if err != nil {
				return nil, fmt.Errorf("embedding batch starting at index %d: %w", batchStart, err)
			}
			allVectors = append(allVectors, vecs...)
			batchStart = i
			batchTokens = 0
		}
		batchTokens += c.TokenCount
	}

	// Send remaining batch
	if batchStart < len(chunks) {
		vecs, err := eng.embedSlice(chunks[batchStart:])
		if err != nil {
			return nil, fmt.Errorf("embedding batch starting at index %d: %w", batchStart, err)
		}
		allVectors = append(allVectors, vecs...)
	}

	return allVectors, nil
}

func (eng *Engine) embedSlice(chunks []chunker.ChunkResult) ([][]float32, error) {
	texts := make([]string, len(chunks))
	for i, c := range chunks {
		texts[i] = c.Content
	}
	return eng.embedder.Embed(texts)
}

// findDirectoryID returns the directory ID for the watched directory that
// contains the given file path.
func (eng *Engine) findDirectoryID(path string) (int64, error) {
	dirs, err := eng.store.ListDirectories()
	if err != nil {
		return 0, err
	}
	for _, d := range dirs {
		if strings.HasPrefix(path, d.Path+string(filepath.Separator)) || strings.HasPrefix(path, d.Path+"/") {
			return d.ID, nil
		}
	}
	return 0, fmt.Errorf("no watched directory contains path %s", path)
}

// RemoveFileFromIndex removes a file and its chunks from the index.
func (eng *Engine) RemoveFileFromIndex(path string) error {
	file, err := eng.store.GetFileByPath(path)
	if err != nil {
		return fmt.Errorf("get file %s: %w", path, err)
	}
	if file == nil {
		return nil // not indexed, nothing to do
	}
	if err := eng.store.RemoveChunksByFile(file.ID); err != nil {
		return fmt.Errorf("remove chunks for %s: %w", path, err)
	}
	if err := eng.store.RemoveFile(path); err != nil {
		return fmt.Errorf("remove file %s: %w", path, err)
	}
	return nil
}

// Search embeds the query and delegates to the store's vector search.
func (eng *Engine) Search(params domain.SearchParams) ([]domain.SearchResult, error) {
	if params.Limit <= 0 {
		params.Limit = 10
	}
	if params.Threshold <= 0 {
		params.Threshold = 1.5
	}

	vectors, err := eng.embedder.Embed([]string{params.Query})
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}
	if len(vectors) == 0 {
		return nil, fmt.Errorf("embedder returned no vectors")
	}
	return eng.store.Search(vectors[0], params.Limit, params.Offset, params.Threshold)
}

// AddDirectory adds a directory to the store and watcher, then walks and
// indexes all files within it, skipping those that match ignore patterns.
func (eng *Engine) AddDirectory(path string) error {
	if err := eng.store.AddDirectory(path); err != nil {
		return fmt.Errorf("store add directory %s: %w", path, err)
	}
	if err := eng.watcher.Add(path); err != nil {
		return fmt.Errorf("watcher add %s: %w", path, err)
	}

	patterns, err := eng.GetIgnorePatterns()
	if err != nil {
		return fmt.Errorf("load ignore patterns: %w", err)
	}

	// First pass: count files for progress.
	var filePaths []string
	if walkErr := filepath.Walk(path, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			log.Printf("engine: walk %s: %v", p, err)
			return nil
		}
		relPath, relErr := filepath.Rel(path, p)
		if relErr != nil {
			return nil
		}
		relPath = filepath.ToSlash(relPath)
		if info.IsDir() {
			if p != path && shouldSkipDir(patterns, relPath) {
				return filepath.SkipDir
			}
			return nil
		}
		for _, pattern := range patterns {
			if matched, _ := doublestar.Match(pattern, relPath); matched {
				return nil
			}
		}
		if eng.extractor.IsSupported(p) {
			filePaths = append(filePaths, p)
		}
		return nil
	}); walkErr != nil {
		log.Printf("engine: walk directory %s: %v", path, walkErr)
	}

	eng.mu.Lock()
	eng.indexing = true
	eng.totalToIndex = len(filePaths)
	eng.indexedFiles = 0
	eng.mu.Unlock()
	defer func() {
		eng.mu.Lock()
		eng.indexing = false
		eng.indexedFiles = 0
		eng.totalToIndex = 0
		eng.mu.Unlock()
	}()

	for i, p := range filePaths {
		if eng.stopped() {
			log.Printf("engine: AddDirectory indexing cancelled at %d/%d", i, len(filePaths))
			return nil
		}
		if indexErr := eng.IndexFile(p); indexErr != nil {
			log.Printf("engine: index %s: %v", p, indexErr)
		}
		eng.mu.Lock()
		eng.indexedFiles++
		eng.mu.Unlock()
	}
	return nil
}

// RemoveDirectory removes a directory from the store and watcher.
func (eng *Engine) RemoveDirectory(path string) error {
	if err := eng.store.RemoveDirectory(path); err != nil {
		return fmt.Errorf("store remove directory %s: %w", path, err)
	}
	if err := eng.watcher.Remove(path); err != nil {
		return fmt.Errorf("watcher remove %s: %w", path, err)
	}
	return nil
}

// ListDirectories delegates to the store.
func (eng *Engine) ListDirectories() ([]domain.Directory, error) {
	return eng.store.ListDirectories()
}

// Stats returns index statistics, including the live IsIndexing flag and progress.
// EmbeddingModel is read from the config store (not the embedder) so that a
// model change via Settings is reflected immediately.
func (eng *Engine) Stats() (domain.IndexStats, error) {
	stats, err := eng.store.Stats()
	if err != nil {
		return stats, err
	}
	eng.mu.Lock()
	stats.IsIndexing = eng.indexing
	stats.IndexedFiles = eng.indexedFiles
	stats.TotalToIndex = eng.totalToIndex
	eng.mu.Unlock()
	// stats.EmbeddingModel is already set by store.Stats() from config
	return stats, nil
}

// Start begins the file watcher and performs an initial scan of all registered
// directories. Files that haven't been indexed (or have changed) are embedded.
func (eng *Engine) Start() error {
	eng.mu.Lock()
	eng.stopCh = make(chan struct{})
	eng.mu.Unlock()

	// Start the watcher first so new changes during scan are captured.
	if err := eng.watcher.Start(eng); err != nil {
		return err
	}

	// Persist watcher status so the read-only MCP process can report it.
	eng.store.SetConfig("watcher_running", "true")

	// Initial scan in background so this call doesn't block the UI.
	go eng.initialScan()
	return nil
}

// initialScan walks all registered directories, counts files, then indexes them.
func (eng *Engine) initialScan() {
	log.Printf("engine: initialScan starting")
	dirs, err := eng.store.ListDirectories()
	if err != nil {
		log.Printf("engine: initial scan list dirs: %v", err)
		return
	}
	log.Printf("engine: initialScan found %d directories", len(dirs))
	if len(dirs) == 0 {
		return
	}

	patterns, err := eng.GetIgnorePatterns()
	if err != nil {
		log.Printf("engine: initial scan ignore patterns: %v", err)
		return
	}

	// First pass: count files to index for progress tracking.
	var filePaths []string
	for _, d := range dirs {
		// Register directory with the watcher so we get change events.
		if wErr := eng.watcher.Add(d.Path); wErr != nil {
			log.Printf("engine: watcher add %s: %v", d.Path, wErr)
		}
		if walkErr := filepath.Walk(d.Path, func(p string, info os.FileInfo, err error) error {
			if err != nil {
				log.Printf("engine: walk %s: %v", p, err)
				return nil
			}
			relPath, relErr := filepath.Rel(d.Path, p)
			if relErr != nil {
				return nil
			}
			relPath = filepath.ToSlash(relPath)
			if info.IsDir() {
				if p != d.Path && shouldSkipDir(patterns, relPath) {
					return filepath.SkipDir
				}
				return nil
			}
			for _, pattern := range patterns {
				if matched, _ := doublestar.Match(pattern, relPath); matched {
					return nil
				}
			}
			if eng.extractor.IsSupported(p) {
				filePaths = append(filePaths, p)
			}
			return nil
		}); walkErr != nil {
			log.Printf("engine: walk directory %s: %v", d.Path, walkErr)
		}
	}

	if len(filePaths) == 0 {
		return
	}

	eng.mu.Lock()
	eng.indexing = true
	eng.totalToIndex = len(filePaths)
	eng.indexedFiles = 0
	eng.mu.Unlock()

	defer func() {
		eng.mu.Lock()
		eng.indexing = false
		eng.indexedFiles = 0
		eng.totalToIndex = 0
		eng.mu.Unlock()
	}()

	log.Printf("engine: initialScan indexing %d files", len(filePaths))
	var errored int
	for i, p := range filePaths {
		if eng.stopped() {
			log.Printf("engine: initialScan cancelled at %d/%d", i, len(filePaths))
			return
		}
		if indexErr := eng.IndexFile(p); indexErr != nil {
			errored++
			log.Printf("engine: initial scan index %s: %v", p, indexErr)
		}
		eng.mu.Lock()
		eng.indexedFiles++
		eng.mu.Unlock()
		if (i+1)%50 == 0 {
			log.Printf("engine: initialScan progress %d/%d (errors: %d)", i+1, len(filePaths), errored)
		}
	}
	log.Printf("engine: initial scan complete — %d files processed, %d errors", len(filePaths), errored)
}

// stopped reports whether Stop has been called (i.e. stopCh is closed).
// Returns false if stopCh is nil (engine was never started via Start()).
func (eng *Engine) stopped() bool {
	eng.mu.Lock()
	ch := eng.stopCh
	eng.mu.Unlock()
	if ch == nil {
		return false
	}
	select {
	case <-ch:
		return true
	default:
		return false
	}
}

// Stop stops the file watcher and cancels any in-flight indexing.
func (eng *Engine) Stop() error {
	eng.mu.Lock()
	if eng.stopCh != nil {
		select {
		case <-eng.stopCh:
			// already closed
		default:
			close(eng.stopCh)
		}
	}
	eng.mu.Unlock()

	eng.store.SetConfig("watcher_running", "false")

	return eng.watcher.Stop()
}

// Restart stops and then starts the file watcher.
func (eng *Engine) Restart() error {
	if err := eng.Stop(); err != nil {
		return err
	}
	return eng.Start()
}

// Reset stops the watcher, clears all indexed data (recreating the vector
// table with the current embedder's dimensions), and restarts the watcher.
func (eng *Engine) Reset() error {
	if err := eng.Stop(); err != nil {
		return err
	}
	if err := eng.store.Reset(eng.embedder.Dimensions()); err != nil {
		return err
	}
	return eng.Start()
}

// IsRunning returns whether the watcher is currently running.
func (eng *Engine) IsRunning() bool {
	return eng.watcher.IsRunning()
}

// Close stops the watcher, releases its resources, and closes the store.
func (eng *Engine) Close() error {
	eng.store.SetConfig("watcher_running", "false")
	if err := eng.watcher.Close(); err != nil {
		return err
	}
	return eng.store.Close()
}

// ---------------------------------------------------------------------------
// watcher.FileEventHandler implementation
// ---------------------------------------------------------------------------

// OnCreate handles new file events by indexing the file if not ignored.
func (eng *Engine) OnCreate(path string) {
	ignored, err := eng.shouldIgnore(path)
	if err != nil {
		log.Printf("engine: shouldIgnore %s: %v", path, err)
		return
	}
	if ignored {
		eng.logActivity(path, "ignored", "matched ignore pattern")
		return
	}
	if err := eng.IndexFile(path); err != nil {
		log.Printf("engine: OnCreate %s: %v", path, err)
	}
}

// OnModify handles file modification events by re-indexing the file if not ignored.
func (eng *Engine) OnModify(path string) {
	ignored, err := eng.shouldIgnore(path)
	if err != nil {
		log.Printf("engine: shouldIgnore %s: %v", path, err)
		return
	}
	if ignored {
		eng.logActivity(path, "ignored", "matched ignore pattern")
		return
	}
	if err := eng.IndexFile(path); err != nil {
		log.Printf("engine: OnModify %s: %v", path, err)
	}
}

// OnDelete handles file deletion events by removing the file from the index.
func (eng *Engine) OnDelete(path string) {
	if err := eng.RemoveFileFromIndex(path); err != nil {
		eng.logActivity(path, "error", fmt.Sprintf("delete: %v", err))
		log.Printf("engine: OnDelete %s: %v", path, err)
	} else {
		eng.logActivity(path, "deleted", "")
	}
}
