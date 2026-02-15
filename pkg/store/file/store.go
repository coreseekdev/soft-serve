//go:build filestore

package file

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"charm.land/log/v2"
	"github.com/charmbracelet/soft-serve/pkg/config"
	"github.com/charmbracelet/soft-serve/pkg/sshutils"
	"github.com/charmbracelet/soft-serve/pkg/store"
	"golang.org/x/crypto/ssh"
)

var (
	// ErrNotSupported is returned when a feature is not supported by the file store.
	ErrNotSupported = errors.New("feature not supported by file store backend")

	// ErrPathTraversal is returned when a path attempts to escape the allowed directory.
	ErrPathTraversal = errors.New("path traversal attempt detected")

	// ErrSymlink is returned when a symlink is detected in an unsafe location.
	ErrSymlink = errors.New("symlinks not allowed in user paths")
)

// Default directory and file permissions
const (
	dirPerms  = 0755
	filePerms = 0644
	sshPerms  = 0700
	keyPerms  = 0600
)

// FileStore is a file-based store implementation.
type FileStore struct {
	cfg       *config.Config
	logger    *log.Logger
	usersPath string
	reposPath string
	lfsPath   string

	// In-memory caches
	users     map[string]*userInfo
	repos     map[string]*repoInfo
	adminKeys []ssh.PublicKey

	mu sync.RWMutex
}

type userInfo struct {
	username string
	keys     []ssh.PublicKey
	keyStrs  []string // Raw key strings for comparison
	admin    bool
}

type repoInfo struct {
	name        string
	path        string
	description string
	private     bool
	hidden      bool
	mirror      bool
	projectName string
	collabs     map[string]string // username -> access level
	webhooks    []webhookInfo
}

type webhookInfo struct {
	url    string
	secret string
	events []int
	active bool
}

// NewStore creates a new file-based store.
func NewStore(ctx context.Context, cfg *config.Config) (store.Store, error) {
	logger := log.FromContext(ctx).WithPrefix("filestore")

	// In filestore mode, use current directory as default DataPath
	// unless explicitly set via SOFT_SERVE_DATA_PATH env or config file
	if os.Getenv("SOFT_SERVE_DATA_PATH") == "" {
		// Check if DataPath is the default "data" or its absolute path form
		absData, err := filepath.Abs("data")
		if err == nil && (cfg.DataPath == "data" || cfg.DataPath == absData) {
			cfg.DataPath, err = os.Getwd()
			if err != nil {
				return nil, fmt.Errorf("get working directory: %w", err)
			}
		}
	}

	// Ensure data path is absolute and clean
	cfg.DataPath = filepath.Clean(cfg.DataPath)
	if !filepath.IsAbs(cfg.DataPath) {
		abs, err := filepath.Abs(cfg.DataPath)
		if err != nil {
			return nil, fmt.Errorf("get absolute path: %w", err)
		}
		cfg.DataPath = abs
	}

	fs := &FileStore{
		cfg:       cfg,
		logger:    logger,
		usersPath: GetUsersPath(cfg),
		reposPath: cfg.DataPath,
		lfsPath:   filepath.Join(cfg.DataPath, ".lfs"),
		users:     make(map[string]*userInfo),
		repos:     make(map[string]*repoInfo),
		adminKeys: make([]ssh.PublicKey, 0),
	}

	// Create necessary directories
	if err := fs.createDirectories(); err != nil {
		return nil, fmt.Errorf("create directories: %w", err)
	}

	// Load admin keys
	if err := fs.loadAdminKeys(); err != nil {
		logger.Warn("failed to load admin keys", "error", err)
	}

	// Load users from directory
	if err := fs.loadUsers(); err != nil {
		logger.Warn("failed to load users", "error", err)
	}

	// Discover repositories
	if err := fs.discoverRepos(); err != nil {
		logger.Warn("failed to discover repos", "error", err)
	}

	logger.Info("file store initialized",
		"users_path", fs.usersPath,
		"repos_path", fs.reposPath,
		"users_count", len(fs.users),
		"repos_count", len(fs.repos),
		"admin_keys_count", len(fs.adminKeys))

	return fs, nil
}

// createDirectories creates necessary directories with proper permissions.
func (s *FileStore) createDirectories() error {
	dirs := []struct {
		path string
		perm os.FileMode
	}{
		{s.reposPath, dirPerms},
		{s.usersPath, dirPerms},
		{s.lfsPath, dirPerms},
		{filepath.Join(s.reposPath, "ssh"), sshPerms},
		{filepath.Join(s.reposPath, "log"), dirPerms},
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir.path, dir.perm); err != nil {
			return fmt.Errorf("create directory %s: %w", dir.path, err)
		}
	}

	return nil
}

// GetUsersPath returns the path to the users directory.
// It first checks the SOFT_SERVE_USER_HOME environment variable,
// then falls back to {DATA_PATH}/users.
func GetUsersPath(cfg *config.Config) string {
	if path := os.Getenv("SOFT_SERVE_USER_HOME"); path != "" {
		return expandPath(path)
	}
	return filepath.Join(cfg.DataPath, "users")
}

// expandPath expands ~ and environment variables in a path.
func expandPath(path string) string {
	if path == "" {
		return path
	}

	// Expand ~ to home directory
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		path = filepath.Join(home, path[2:])
	}

	// Expand environment variables
	path = os.ExpandEnv(path)

	// Convert to absolute path
	if !filepath.IsAbs(path) {
		abs, err := filepath.Abs(path)
		if err == nil {
			path = abs
		}
	}

	return filepath.Clean(path)
}

// validatePath checks if a path is safe (no traversal, no symlinks).
func (s *FileStore) validatePath(path string, basePath string) error {
	// Clean and get absolute path
	path = filepath.Clean(path)
	if !filepath.IsAbs(path) {
		abs, err := filepath.Abs(path)
		if err != nil {
			return err
		}
		path = abs
	}

	// Get absolute base path
	basePath = filepath.Clean(basePath)
	if !filepath.IsAbs(basePath) {
		abs, err := filepath.Abs(basePath)
		if err != nil {
			return err
		}
		basePath = abs
	}

	// Check for path traversal
	rel, err := filepath.Rel(basePath, path)
	if err != nil {
		return err
	}

	if strings.HasPrefix(rel, "..") || strings.HasPrefix(rel, "../") {
		return ErrPathTraversal
	}

	// Check for symlinks in path components
	return s.checkSymlinks(path, basePath)
}

// checkSymlinks checks if any component in the path is a symlink.
func (s *FileStore) checkSymlinks(path, basePath string) error {
	current := basePath
	rel, _ := filepath.Rel(basePath, path)
	parts := strings.Split(rel, string(filepath.Separator))

	for _, part := range parts {
		if part == "" || part == "." {
			continue
		}
		current = filepath.Join(current, part)

		// Check if this component is a symlink
		info, err := os.Lstat(current)
		if err != nil {
			if os.IsNotExist(err) {
				continue // Path doesn't exist yet
			}
			return err
		}

		if info.Mode()&os.ModeSymlink != 0 {
			return ErrSymlink
		}
	}

	return nil
}

// loadAdminKeys loads admin public keys from ~/.ssh directory.
func (s *FileStore) loadAdminKeys() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	sshDir := filepath.Join(home, ".ssh")

	// Validate the ssh directory path
	if err := s.validatePath(sshDir, home); err != nil {
		return fmt.Errorf("validate ssh directory: %w", err)
	}

	entries, err := os.ReadDir(sshDir)
	if err != nil {
		return err
	}

	s.adminKeys = make([]ssh.PublicKey, 0)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		// Only read id_*.pub files
		if !strings.HasPrefix(name, "id_") || !strings.HasSuffix(name, ".pub") {
			continue
		}

		keyPath := filepath.Join(sshDir, name)
		data, err := os.ReadFile(keyPath)
		if err != nil {
			continue
		}

		// Parse the public key
		key, _, _, _, err := ssh.ParseAuthorizedKey(data)
		if err != nil {
			s.logger.Warn("failed to parse admin key", "file", name, "error", err)
			continue
		}

		s.adminKeys = append(s.adminKeys, key)
	}

	return nil
}

// isAdminKey checks if a public key matches any admin key.
func (s *FileStore) isAdminKey(key ssh.PublicKey) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, adminKey := range s.adminKeys {
		if sshutils.KeysEqual(key, adminKey) {
			return true
		}
	}
	return false
}

// Reload reloads users and repositories from disk.
func (s *FileStore) Reload() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Clear existing data
	s.users = make(map[string]*userInfo)
	s.repos = make(map[string]*repoInfo)

	// Reload
	if err := s.loadAdminKeys(); err != nil {
		s.logger.Warn("failed to reload admin keys", "error", err)
	}

	if err := s.loadUsers(); err != nil {
		s.logger.Warn("failed to reload users", "error", err)
	}

	if err := s.discoverRepos(); err != nil {
		s.logger.Warn("failed to rediscover repos", "error", err)
	}

	s.logger.Info("file store reloaded",
		"users_count", len(s.users),
		"repos_count", len(s.repos))

	return nil
}
