package watcher

// FileEventHandler handles filesystem events.
type FileEventHandler interface {
	OnCreate(path string)
	OnModify(path string)
	OnDelete(path string)
}

// Watcher monitors directories for file changes.
type Watcher interface {
	Add(dir string) error
	Remove(dir string) error
	Start(handler FileEventHandler) error
	Stop() error
	Close() error
	IsRunning() bool
}
