//go:build filestore

package file

import (
	"context"
	"os"
	"sync"
	"time"

	"charm.land/log/v2"
)

// Watcher watches for changes in user and repo directories.
type Watcher struct {
	store       *FileStore
	interval    time.Duration
	stopCh      chan struct{}
	stoppedCh   chan struct{}
	mu          sync.Mutex
	running     bool
	lastUsers   map[string]os.FileInfo
	lastRepos   map[string]os.FileInfo
}

// NewWatcher creates a new directory watcher.
func NewWatcher(store *FileStore, interval time.Duration) *Watcher {
	if interval == 0 {
		interval = 30 * time.Second
	}
	return &Watcher{
		store:     store,
		interval:  interval,
		stopCh:    make(chan struct{}),
		stoppedCh: make(chan struct{}),
		lastUsers: make(map[string]os.FileInfo),
		lastRepos: make(map[string]os.FileInfo),
	}
}

// Start starts the watcher.
func (w *Watcher) Start(ctx context.Context) {
	w.mu.Lock()
	if w.running {
		w.mu.Unlock()
		return
	}
	w.running = true
	w.mu.Unlock()

	logger := log.FromContext(ctx).WithPrefix("filestore.watcher")
	logger.Info("starting directory watcher", "interval", w.interval)

	// Initial scan
	w.scanDirs(logger)

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	defer close(w.stoppedCh)

	for {
		select {
		case <-ticker.C:
			if w.scanDirs(logger) {
				logger.Info("changes detected, reloading store")
				if err := w.store.Reload(); err != nil {
					logger.Error("failed to reload store", "error", err)
				}
			}
		case <-w.stopCh:
			logger.Info("stopping directory watcher")
			return
		case <-ctx.Done():
			logger.Info("stopping directory watcher (context done)")
			return
		}
	}
}

// Stop stops the watcher.
func (w *Watcher) Stop() {
	w.mu.Lock()
	defer w.mu.Unlock()

	if !w.running {
		return
	}

	close(w.stopCh)
	<-w.stoppedCh
	w.running = false
}

// scanDirs scans user and repo directories for changes.
// Returns true if changes were detected.
func (w *Watcher) scanDirs(logger *log.Logger) bool {
	changed := false

	// Check users directory
	if usersChanged := w.scanDir(w.store.usersPath, w.lastUsers, logger, "users"); usersChanged {
		changed = true
	}

	// Check repos directory
	if reposChanged := w.scanDir(w.store.reposPath, w.lastRepos, logger, "repos"); reposChanged {
		changed = true
	}

	return changed
}

// scanDir scans a directory for changes.
// Returns true if changes were detected.
func (w *Watcher) scanDir(dir string, lastState map[string]os.FileInfo, logger *log.Logger, dirType string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return false
		}
		logger.Warn("failed to read directory", "dir", dir, "error", err)
		return false
	}

	changed := false
	currentState := make(map[string]os.FileInfo)

	for _, entry := range entries {
		name := entry.Name()

		info, err := entry.Info()
		if err != nil {
			continue
		}

		currentState[name] = info

		// Check if this is a new or modified entry
		if lastInfo, exists := lastState[name]; !exists {
			// New entry
			logger.Debug("new entry detected", "type", dirType, "name", name)
			changed = true
		} else if info.ModTime().After(lastInfo.ModTime()) {
			// Modified entry
			logger.Debug("modified entry detected", "type", dirType, "name", name)
			changed = true
		}
	}

	// Check for deleted entries
	for name := range lastState {
		if _, exists := currentState[name]; !exists {
			logger.Debug("deleted entry detected", "type", dirType, "name", name)
			changed = true
		}
	}

	// Update last state
	for k := range lastState {
		delete(lastState, k)
	}
	for k, v := range currentState {
		lastState[k] = v
	}

	return changed
}

// Watch starts a goroutine that watches for directory changes and reloads the store.
// Returns a function that can be called to stop the watcher.
func (s *FileStore) Watch(ctx context.Context, interval time.Duration) (stop func()) {
	watcher := NewWatcher(s, interval)

	ctx, cancel := context.WithCancel(ctx)
	go watcher.Start(ctx)

	return func() {
		cancel()
		watcher.Stop()
	}
}
