package watcher

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

const debounceDuration = 500 * time.Millisecond

// FSWatcher implements Watcher using fsnotify.
type FSWatcher struct {
	mu       sync.Mutex
	watcher  *fsnotify.Watcher
	running  bool
	stopCh   chan struct{}
	handler  FileEventHandler
	timers   map[string]*time.Timer
	timersMu sync.Mutex
}

// NewFSWatcher creates a new FSWatcher.
func NewFSWatcher() (*FSWatcher, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("create fsnotify watcher: %w", err)
	}
	return &FSWatcher{
		watcher: w,
		timers:  make(map[string]*time.Timer),
	}, nil
}

// Add watches the given directory and all its subdirectories.
func (fw *FSWatcher) Add(dir string) error {
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if err := fw.watcher.Add(path); err != nil {
				return fmt.Errorf("watch %s: %w", path, err)
			}
		}
		return nil
	})
}

// Remove stops watching the given directory.
func (fw *FSWatcher) Remove(dir string) error {
	return fw.watcher.Remove(dir)
}

// Start begins processing filesystem events in a goroutine.
func (fw *FSWatcher) Start(handler FileEventHandler) error {
	fw.mu.Lock()
	defer fw.mu.Unlock()

	if fw.running {
		return fmt.Errorf("watcher already running")
	}

	fw.handler = handler
	fw.stopCh = make(chan struct{})
	fw.running = true

	go fw.loop()
	return nil
}

func (fw *FSWatcher) loop() {
	for {
		select {
		case <-fw.stopCh:
			return
		case event, ok := <-fw.watcher.Events:
			if !ok {
				return
			}
			fw.handleEvent(event)
		case _, ok := <-fw.watcher.Errors:
			if !ok {
				return
			}
		}
	}
}

func (fw *FSWatcher) handleEvent(event fsnotify.Event) {
	path := event.Name

	// For new directories, start watching them too.
	if event.Has(fsnotify.Create) {
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			fw.watcher.Add(path)
		}
	}

	fw.debounce(path, func() {
		if event.Has(fsnotify.Remove) || event.Has(fsnotify.Rename) {
			fw.handler.OnDelete(path)
		} else if event.Has(fsnotify.Create) {
			fw.handler.OnCreate(path)
		} else if event.Has(fsnotify.Write) || event.Has(fsnotify.Chmod) {
			fw.handler.OnModify(path)
		}
	})
}

func (fw *FSWatcher) debounce(path string, fn func()) {
	fw.timersMu.Lock()
	defer fw.timersMu.Unlock()

	if t, ok := fw.timers[path]; ok {
		t.Stop()
	}

	fw.timers[path] = time.AfterFunc(debounceDuration, func() {
		fn()
		fw.timersMu.Lock()
		delete(fw.timers, path)
		fw.timersMu.Unlock()
	})
}

// Stop halts the event processing goroutine but keeps the fsnotify watcher
// alive so directories can still be added and Start() can resume later.
// Use Close() to release the underlying fsnotify resources.
func (fw *FSWatcher) Stop() error {
	fw.mu.Lock()
	defer fw.mu.Unlock()

	if !fw.running {
		return nil
	}

	close(fw.stopCh)
	fw.running = false

	// Cancel pending debounce timers.
	fw.timersMu.Lock()
	for _, t := range fw.timers {
		t.Stop()
	}
	fw.timers = make(map[string]*time.Timer)
	fw.timersMu.Unlock()

	return nil
}

// Close stops the watcher and releases all fsnotify resources.
// After Close(), the watcher cannot be restarted.
func (fw *FSWatcher) Close() error {
	if err := fw.Stop(); err != nil {
		return err
	}
	return fw.watcher.Close()
}

// IsRunning returns whether the watcher is actively processing events.
func (fw *FSWatcher) IsRunning() bool {
	fw.mu.Lock()
	defer fw.mu.Unlock()
	return fw.running
}
