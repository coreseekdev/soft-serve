//go:build filestore

package file

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/soft-serve/pkg/db"
	"github.com/charmbracelet/soft-serve/pkg/db/models"
	"github.com/charmbracelet/soft-serve/pkg/sshutils"
	"github.com/charmbracelet/soft-serve/pkg/utils"
	"golang.org/x/crypto/ssh"
)

// loadUsers loads users from the users directory.
// Directory structure:
// {users_path}/
// ├── alice/
// │   └── .ssh/
// │       ├── id_ed25519.pub
// │       └── id_rsa.pub
// └── bob/
//     └── .ssh/
//         └── id_ed25519.pub
func (s *FileStore) loadUsers() error {
	entries, err := os.ReadDir(s.usersPath)
	if os.IsNotExist(err) {
		// Create the directory if it doesn't exist
		return os.MkdirAll(s.usersPath, dirPerms)
	}
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		username := entry.Name()

		// Validate username
		if err := utils.ValidateUsername(username); err != nil {
			s.logger.Warn("invalid username, skipping", "username", username, "error", err)
			continue
		}

		userDir := filepath.Join(s.usersPath, username)

		// Validate user directory path
		if err := s.validatePath(userDir, s.usersPath); err != nil {
			s.logger.Warn("invalid user directory, skipping", "username", username, "error", err)
			continue
		}

		sshDir := filepath.Join(userDir, ".ssh")

		keys, keyStrs, err := s.loadUserKeys(sshDir)
		if err != nil || len(keys) == 0 {
			continue // Skip users without valid keys
		}

		s.users[username] = &userInfo{
			username: username,
			keys:     keys,
			keyStrs:  keyStrs,
			admin:    false,
		}
	}

	return nil
}

// loadUserKeys loads SSH public keys from a user's .ssh directory.
// Returns parsed keys, raw key strings, and error.
func (s *FileStore) loadUserKeys(sshDir string) ([]ssh.PublicKey, []string, error) {
	entries, err := os.ReadDir(sshDir)
	if err != nil {
		return nil, nil, err
	}

	var keys []ssh.PublicKey
	var keyStrs []string

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !strings.HasSuffix(name, ".pub") {
			continue
		}

		keyPath := filepath.Join(sshDir, name)
		data, err := os.ReadFile(keyPath)
		if err != nil {
			continue
		}

		keyStr := strings.TrimSpace(string(data))

		// Validate and parse the key
		key, _, _, _, err := ssh.ParseAuthorizedKey([]byte(keyStr))
		if err != nil {
			continue
		}

		keys = append(keys, key)
		keyStrs = append(keyStrs, keyStr)
	}

	return keys, keyStrs, nil
}

// GetUserByID returns a user by ID.
// Note: In file store, we use a hash of username as ID.
func (s *FileStore) GetUserByID(ctx context.Context, h db.Handler, id int64) (models.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, u := range s.users {
		if hashUsername(u.username) == id {
			return models.User{
				ID:        id,
				Username:  u.username,
				Admin:     u.admin,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}, nil
		}
	}

	return models.User{}, db.ErrRecordNotFound
}

// FindUserByUsername finds a user by username.
func (s *FileStore) FindUserByUsername(ctx context.Context, h db.Handler, username string) (models.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	u, ok := s.users[username]
	if !ok {
		return models.User{}, db.ErrRecordNotFound
	}

	return models.User{
		ID:        hashUsername(u.username),
		Username:  u.username,
		Admin:     u.admin,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}, nil
}

// FindUserByPublicKey finds a user by public key.
func (s *FileStore) FindUserByPublicKey(ctx context.Context, h db.Handler, pk ssh.PublicKey) (models.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, u := range s.users {
		for _, key := range u.keys {
			if sshutils.KeysEqual(pk, key) {
				return models.User{
					ID:        hashUsername(u.username),
					Username:  u.username,
					Admin:     u.admin,
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
				}, nil
			}
		}
	}

	return models.User{}, db.ErrRecordNotFound
}

// FindUserByAccessToken finds a user by access token.
func (s *FileStore) FindUserByAccessToken(ctx context.Context, h db.Handler, token string) (models.User, error) {
	// Not supported in file store
	return models.User{}, ErrNotSupported
}

// GetAllUsers returns all users.
func (s *FileStore) GetAllUsers(ctx context.Context, h db.Handler) ([]models.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	users := make([]models.User, 0, len(s.users))
	for _, u := range s.users {
		users = append(users, models.User{
			ID:        hashUsername(u.username),
			Username:  u.username,
			Admin:     u.admin,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		})
	}

	return users, nil
}

// CreateUser creates a new user.
func (s *FileStore) CreateUser(ctx context.Context, h db.Handler, username string, isAdmin bool, pks []ssh.PublicKey) error {
	// Validate username
	username = strings.ToLower(username)
	if err := utils.ValidateUsername(username); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.users[username]; exists {
		return errors.New("user already exists")
	}

	// Create user directory structure
	userDir := filepath.Join(s.usersPath, username)
	sshDir := filepath.Join(userDir, ".ssh")

	// Validate paths
	if err := s.validatePath(userDir, s.usersPath); err != nil {
		return fmt.Errorf("invalid user path: %w", err)
	}

	if err := os.MkdirAll(sshDir, sshPerms); err != nil {
		return err
	}

	// Write public keys with secure permissions
	keys := make([]ssh.PublicKey, 0, len(pks))
	keyStrs := make([]string, 0, len(pks))

	for i, pk := range pks {
		pkStr := sshutils.MarshalAuthorizedKey(pk)
		keyFile := filepath.Join(sshDir, fmt.Sprintf("id_key_%d.pub", i))

		if err := os.WriteFile(keyFile, []byte(pkStr+"\n"), keyPerms); err != nil {
			return err
		}

		keys = append(keys, pk)
		keyStrs = append(keyStrs, pkStr)
	}

	s.users[username] = &userInfo{
		username: username,
		keys:     keys,
		keyStrs:  keyStrs,
		admin:    isAdmin,
	}

	return nil
}

// DeleteUserByUsername deletes a user by username.
func (s *FileStore) DeleteUserByUsername(ctx context.Context, h db.Handler, username string) error {
	username = strings.ToLower(username)

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.users[username]; !exists {
		return db.ErrRecordNotFound
	}

	// Validate and remove user directory
	userDir := filepath.Join(s.usersPath, username)
	if err := s.validatePath(userDir, s.usersPath); err != nil {
		return err
	}

	if err := os.RemoveAll(userDir); err != nil {
		return err
	}

	delete(s.users, username)
	return nil
}

// SetUsernameByUsername sets the username of a user.
func (s *FileStore) SetUsernameByUsername(ctx context.Context, h db.Handler, username string, newUsername string) error {
	username = strings.ToLower(username)
	newUsername = strings.ToLower(newUsername)

	// Validate new username
	if err := utils.ValidateUsername(newUsername); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	u, exists := s.users[username]
	if !exists {
		return db.ErrRecordNotFound
	}

	if _, exists := s.users[newUsername]; exists {
		return errors.New("username already exists")
	}

	// Validate paths
	oldDir := filepath.Join(s.usersPath, username)
	newDir := filepath.Join(s.usersPath, newUsername)

	if err := s.validatePath(oldDir, s.usersPath); err != nil {
		return err
	}
	if err := s.validatePath(newDir, s.usersPath); err != nil {
		return err
	}

	// Rename directory
	if err := os.Rename(oldDir, newDir); err != nil {
		return err
	}

	delete(s.users, username)
	u.username = newUsername
	s.users[newUsername] = u

	return nil
}

// SetAdminByUsername sets the admin flag of a user.
func (s *FileStore) SetAdminByUsername(ctx context.Context, h db.Handler, username string, isAdmin bool) error {
	username = strings.ToLower(username)

	s.mu.Lock()
	defer s.mu.Unlock()

	u, exists := s.users[username]
	if !exists {
		return db.ErrRecordNotFound
	}

	u.admin = isAdmin
	return nil
}

// AddPublicKeyByUsername adds a public key to a user.
func (s *FileStore) AddPublicKeyByUsername(ctx context.Context, h db.Handler, username string, pk ssh.PublicKey) error {
	username = strings.ToLower(username)

	s.mu.Lock()
	defer s.mu.Unlock()

	u, exists := s.users[username]
	if !exists {
		return db.ErrRecordNotFound
	}

	pkStr := sshutils.MarshalAuthorizedKey(pk)

	// Check if key already exists
	for _, existingKey := range u.keys {
		if sshutils.KeysEqual(pk, existingKey) {
			return errors.New("public key already exists")
		}
	}

	// Write to file with secure permissions
	sshDir := filepath.Join(s.usersPath, username, ".ssh")
	keyFile := filepath.Join(sshDir, fmt.Sprintf("id_key_%d.pub", len(u.keys)))

	if err := os.WriteFile(keyFile, []byte(pkStr+"\n"), keyPerms); err != nil {
		return err
	}

	u.keys = append(u.keys, pk)
	u.keyStrs = append(u.keyStrs, pkStr)
	return nil
}

// RemovePublicKeyByUsername removes a public key from a user.
func (s *FileStore) RemovePublicKeyByUsername(ctx context.Context, h db.Handler, username string, pk ssh.PublicKey) error {
	username = strings.ToLower(username)

	s.mu.Lock()
	defer s.mu.Unlock()

	u, exists := s.users[username]
	if !exists {
		return db.ErrRecordNotFound
	}

	for i, existingKey := range u.keys {
		if sshutils.KeysEqual(pk, existingKey) {
			// Remove from slices
			u.keys = append(u.keys[:i], u.keys[i+1:]...)
			u.keyStrs = append(u.keyStrs[:i], u.keyStrs[i+1:]...)
			return nil
		}
	}

	return errors.New("public key not found")
}

// ListPublicKeysByUserID lists public keys by user ID.
func (s *FileStore) ListPublicKeysByUserID(ctx context.Context, h db.Handler, id int64) ([]ssh.PublicKey, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, u := range s.users {
		if hashUsername(u.username) == id {
			return u.keys, nil
		}
	}

	return nil, db.ErrRecordNotFound
}

// ListPublicKeysByUsername lists public keys by username.
func (s *FileStore) ListPublicKeysByUsername(ctx context.Context, h db.Handler, username string) ([]ssh.PublicKey, error) {
	username = strings.ToLower(username)

	s.mu.RLock()
	defer s.mu.RUnlock()

	u, exists := s.users[username]
	if !exists {
		return nil, db.ErrRecordNotFound
	}

	return u.keys, nil
}

// SetUserPassword sets the password of a user.
func (s *FileStore) SetUserPassword(ctx context.Context, h db.Handler, userID int64, password string) error {
	// Not supported in file store
	return ErrNotSupported
}

// SetUserPasswordByUsername sets the password of a user by username.
func (s *FileStore) SetUserPasswordByUsername(ctx context.Context, h db.Handler, username string, password string) error {
	// Not supported in file store
	return ErrNotSupported
}

// hashUsername generates a deterministic ID from username.
func hashUsername(username string) int64 {
	// Simple hash for generating consistent IDs
	var h int64
	for _, c := range username {
		h = h*31 + int64(c)
	}
	if h < 0 {
		h = -h
	}
	return h
}
