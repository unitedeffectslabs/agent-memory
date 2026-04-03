package watcher

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
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
