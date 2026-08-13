package watcher

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
)

// mockHandler records filesystem events for assertions.
type mockHandler struct {
	mu       sync.Mutex
	creates  []string
	modifies []string
	deletes  []string
}

func (m *mockHandler) OnCreate(path string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.creates = append(m.creates, path)
}

func (m *mockHandler) OnModify(path string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.modifies = append(m.modifies, path)
}

func (m *mockHandler) OnDelete(path string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.deletes = append(m.deletes, path)
}

func (m *mockHandler) getCreates() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]string, len(m.creates))
	copy(cp, m.creates)
	return cp
}

func (m *mockHandler) getModifies() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]string, len(m.modifies))
	copy(cp, m.modifies)
	return cp
}

func (m *mockHandler) getDeletes() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]string, len(m.deletes))
	copy(cp, m.deletes)
	return cp
}

func TestStartStop(t *testing.T) {
	fw, err := NewFSWatcher()
	if err != nil {
		t.Fatalf("NewFSWatcher: %v", err)
	}

	if fw.IsRunning() {
		t.Error("expected not running before Start")
	}

	handler := &mockHandler{}
	if err := fw.Start(handler); err != nil {
		t.Fatalf("Start: %v", err)
	}

	if !fw.IsRunning() {
		t.Error("expected running after Start")
	}

	// Starting again should error.
	if err := fw.Start(handler); err == nil {
		t.Error("expected error on double Start")
	}

	if err := fw.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	if fw.IsRunning() {
		t.Error("expected not running after Stop")
	}
}

func TestOnCreate(t *testing.T) {
	dir := t.TempDir()
	fw, err := NewFSWatcher()
	if err != nil {
		t.Fatalf("NewFSWatcher: %v", err)
	}
	defer fw.Stop()

	if err := fw.Add(dir); err != nil {
		t.Fatalf("Add: %v", err)
	}

	handler := &mockHandler{}
	if err := fw.Start(handler); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Create a file.
	filePath := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(filePath, []byte("hello"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	// Wait for debounce + processing.
	if !waitFor(t, 2*time.Second, func() bool {
		return len(handler.getCreates()) > 0
	}) {
		t.Error("expected OnCreate to be called")
	}
}

func TestOnModify(t *testing.T) {
	dir := t.TempDir()

	// Create file before watching.
	filePath := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(filePath, []byte("hello"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	fw, err := NewFSWatcher()
	if err != nil {
		t.Fatalf("NewFSWatcher: %v", err)
	}
	defer fw.Stop()

	if err := fw.Add(dir); err != nil {
		t.Fatalf("Add: %v", err)
	}

	handler := &mockHandler{}
	if err := fw.Start(handler); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Modify the file.
	if err := os.WriteFile(filePath, []byte("world"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	if !waitFor(t, 2*time.Second, func() bool {
		return len(handler.getModifies()) > 0
	}) {
		t.Error("expected OnModify to be called")
	}
}

func TestOnDelete(t *testing.T) {
	dir := t.TempDir()

	// Create file before watching.
	filePath := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(filePath, []byte("hello"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	fw, err := NewFSWatcher()
	if err != nil {
		t.Fatalf("NewFSWatcher: %v", err)
	}
	defer fw.Stop()

	if err := fw.Add(dir); err != nil {
		t.Fatalf("Add: %v", err)
	}

	handler := &mockHandler{}
	if err := fw.Start(handler); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Delete the file.
	if err := os.Remove(filePath); err != nil {
		t.Fatalf("remove file: %v", err)
	}

	if !waitFor(t, 2*time.Second, func() bool {
		return len(handler.getDeletes()) > 0
	}) {
		t.Error("expected OnDelete to be called")
	}
}

// waitFor polls the condition every 50ms until it returns true or timeout.
func waitFor(t *testing.T, timeout time.Duration, cond func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
}

// TestDebounceMergesOps drives the debouncer directly — no real filesystem
// events — so the Create+Write merge behavior is pinned deterministically on
// EVERY platform. The end-to-end TestOnCreate only catches a merge regression
// on OSes that emit the CREATE-then-WRITE double event (Linux/Windows); on
// macOS it passes either way, which would let the primary dev environment
// ship a regression.
func TestDebounceMergesOps(t *testing.T) {
	newFW := func(t *testing.T) (*FSWatcher, *mockHandler) {
		t.Helper()
		fw, err := NewFSWatcher()
		if err != nil {
			t.Fatalf("NewFSWatcher: %v", err)
		}
		t.Cleanup(func() { fw.Close() })
		h := &mockHandler{}
		fw.handler = h
		return fw, h
	}
	wait := func() { time.Sleep(debounceDuration + 200*time.Millisecond) }

	t.Run("create then write fires OnCreate", func(t *testing.T) {
		fw, h := newFW(t)
		fw.debounce("/p/new.txt", fsnotify.Create)
		fw.debounce("/p/new.txt", fsnotify.Write) // the Linux/Windows double event
		wait()
		if got := h.getCreates(); len(got) != 1 || got[0] != "/p/new.txt" {
			t.Fatalf("OnCreate calls = %v, want exactly [/p/new.txt]", got)
		}
		if got := h.getModifies(); len(got) != 0 {
			t.Fatalf("OnModify calls = %v, want none (Create outranks Write)", got)
		}
	})

	t.Run("write alone fires OnModify", func(t *testing.T) {
		fw, h := newFW(t)
		fw.debounce("/p/existing.txt", fsnotify.Write)
		wait()
		if got := h.getModifies(); len(got) != 1 {
			t.Fatalf("OnModify calls = %v, want exactly one", got)
		}
		if got := h.getCreates(); len(got) != 0 {
			t.Fatalf("OnCreate calls = %v, want none", got)
		}
	})

	t.Run("delete wins over create and write", func(t *testing.T) {
		fw, h := newFW(t)
		fw.debounce("/p/gone.txt", fsnotify.Create)
		fw.debounce("/p/gone.txt", fsnotify.Write)
		fw.debounce("/p/gone.txt", fsnotify.Remove)
		wait()
		if got := h.getDeletes(); len(got) != 1 {
			t.Fatalf("OnDelete calls = %v, want exactly one (removal ends the story)", got)
		}
		if len(h.getCreates()) != 0 || len(h.getModifies()) != 0 {
			t.Fatalf("create/modify fired alongside delete: creates=%v modifies=%v", h.getCreates(), h.getModifies())
		}
	})
}

// TestDispatchAtomicSaveRename pins the atomic-save pattern: editors like vim
// (backupcopy=no) RENAME the file away then CREATE it fresh within one
// debounce window. The merged ops carry Rename, but the path still exists —
// it must dispatch as a create, not silently vanish from the index. A Rename
// with the path truly gone still dispatches as delete.
func TestDispatchAtomicSaveRename(t *testing.T) {
	t.Run("rename then create, file exists -> OnCreate", func(t *testing.T) {
		fw, h := func() (*FSWatcher, *mockHandler) {
			fw, err := NewFSWatcher()
			if err != nil {
				t.Fatalf("NewFSWatcher: %v", err)
			}
			t.Cleanup(func() { fw.Close() })
			h := &mockHandler{}
			fw.handler = h
			return fw, h
		}()
		real := tempFileInDirW(t, t.TempDir(), "saved.md", "new content")
		fw.dispatch(real, fsnotify.Rename|fsnotify.Create)
		if got := h.getCreates(); len(got) != 1 {
			t.Fatalf("OnCreate calls = %v, want exactly one", got)
		}
		if got := h.getDeletes(); len(got) != 0 {
			t.Fatalf("OnDelete fired for a file that still exists: %v", got)
		}
	})

	t.Run("rename, file gone -> OnDelete", func(t *testing.T) {
		fw, err := NewFSWatcher()
		if err != nil {
			t.Fatalf("NewFSWatcher: %v", err)
		}
		t.Cleanup(func() { fw.Close() })
		h := &mockHandler{}
		fw.handler = h
		fw.dispatch("/definitely/not/a/real/path.md", fsnotify.Rename)
		if got := h.getDeletes(); len(got) != 1 {
			t.Fatalf("OnDelete calls = %v, want exactly one", got)
		}
	})
}

func tempFileInDirW(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return p
}
