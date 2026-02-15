package chatfile

import (
	"bufio"
	"os"
	"sync"
)

// LockManager manages locks for inboxes.
type LockManager struct {
	mu    sync.Mutex
	locks map[string]*sync.RWMutex
}

// NewLockManager creates a new lock manager.
func NewLockManager() *LockManager {
	return &LockManager{
		locks: make(map[string]*sync.RWMutex),
	}
}

// Lock acquires a write lock for an inbox.
func (lm *LockManager) Lock(inbox string) {
	lm.mu.Lock()
	lock, ok := lm.locks[inbox]
	if !ok {
		lock = &sync.RWMutex{}
		lm.locks[inbox] = lock
	}
	lm.mu.Unlock()
	lock.Lock()
}

// Unlock releases a write lock for an inbox.
func (lm *LockManager) Unlock(inbox string) {
	lm.mu.Lock()
	lock, ok := lm.locks[inbox]
	lm.mu.Unlock()
	if ok {
		lock.Unlock()
	}
}

// RLock acquires a read lock for an inbox.
func (lm *LockManager) RLock(inbox string) {
	lm.mu.Lock()
	lock, ok := lm.locks[inbox]
	if !ok {
		lock = &sync.RWMutex{}
		lm.locks[inbox] = lock
	}
	lm.mu.Unlock()
	lock.RLock()
}

// RUnlock releases a read lock for an inbox.
func (lm *LockManager) RUnlock(inbox string) {
	lm.mu.Lock()
	lock, ok := lm.locks[inbox]
	lm.mu.Unlock()
	if ok {
		lock.RUnlock()
	}
}

// readFileLines reads all lines from a file.
func readFileLines(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if line != "" {
			lines = append(lines, line)
		}
	}

	return lines, scanner.Err()
}

// readFileLinesReverse reads lines from a file in reverse order.
func readFileLinesReverse(path string, limit int) ([]string, error) {
	lines, err := readFileLines(path)
	if err != nil {
		return nil, err
	}

	// Reverse the lines
	for i, j := 0, len(lines)-1; i < j; i, j = i+1, j-1 {
		lines[i], lines[j] = lines[j], lines[i]
	}

	if limit > 0 && len(lines) > limit {
		lines = lines[:limit]
	}

	return lines, nil
}
