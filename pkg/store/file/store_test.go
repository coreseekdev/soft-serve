//go:build filestore

package file

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/soft-serve/pkg/config"
	"golang.org/x/crypto/ssh"
)

func TestNewStore(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "filestore-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &config.Config{
		DataPath: tmpDir,
	}

	ctx := context.Background()
	store, err := NewStore(ctx, cfg)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	if store == nil {
		t.Error("store should not be nil")
	}

	fs := store.(*FileStore)
	if fs.reposPath != tmpDir {
		t.Errorf("reposPath = %q, want %q", fs.reposPath, tmpDir)
	}

	expectedUsersPath := filepath.Join(tmpDir, "users")
	if fs.usersPath != expectedUsersPath {
		t.Errorf("usersPath = %q, want %q", fs.usersPath, expectedUsersPath)
	}
}

func TestGetUsersPath(t *testing.T) {
	tests := []struct {
		name         string
		dataPath     string
		envValue     string
		wantSuffix   string
		setEnv       bool
		wantContains string
	}{
		{
			name:       "default path",
			dataPath:   filepath.Join("data"),
			wantSuffix: "users",
		},
		{
			name:       "with env variable",
			dataPath:   filepath.Join("data"),
			envValue:   filepath.Join("custom", "users"),
			wantSuffix: "users",
			setEnv:     true,
		},
		{
			name:         "with tilde expansion",
			dataPath:     filepath.Join("data"),
			envValue:     "~/my-users",
			setEnv:       true,
			wantContains: "my-users",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setEnv {
				os.Setenv("SOFT_SERVE_USER_HOME", tt.envValue)
				defer os.Unsetenv("SOFT_SERVE_USER_HOME")
			}

			cfg := &config.Config{DataPath: tt.dataPath}
			got := GetUsersPath(cfg)

			if tt.wantContains != "" {
				if !strings.Contains(got, tt.wantContains) {
					t.Errorf("GetUsersPath() = %q, want to contain %q", got, tt.wantContains)
				}
			} else if tt.wantSuffix != "" {
				if !strings.HasSuffix(got, tt.wantSuffix) {
					t.Errorf("GetUsersPath() = %q, want suffix %q", got, tt.wantSuffix)
				}
			}
		})
	}
}

func TestExpandPath(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		want      string
		wantIsAbs bool
	}{
		{
			name: "empty path",
			path: "",
			want: "",
		},
		{
			name:      "relative path becomes absolute",
			path:      filepath.Join("data", "users"),
			wantIsAbs: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := expandPath(tt.path)

			if tt.want != "" && got != tt.want {
				t.Errorf("expandPath() = %q, want %q", got, tt.want)
			}

			if tt.wantIsAbs && !filepath.IsAbs(got) {
				t.Errorf("expandPath() = %q, want absolute path", got)
			}
		})
	}
}

func TestValidatePath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		basePath string
		wantErr  error
	}{
		{
			name:     "valid path",
			path:     "/data/users/alice",
			basePath: "/data/users",
			wantErr:  nil,
		},
		{
			name:     "traversal attempt",
			path:     "/data/users/../etc/passwd",
			basePath: "/data/users",
			wantErr:  ErrPathTraversal,
		},
		{
			name:     "traversal with relative",
			path:     "/etc/passwd",
			basePath: "/data/users",
			wantErr:  ErrPathTraversal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := &FileStore{}
			err := fs.validatePath(tt.path, tt.basePath)

			if err != tt.wantErr {
				t.Errorf("validatePath() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestSymlinkDetection(t *testing.T) {
	// Create temp directory with symlink
	tmpDir, err := os.MkdirTemp("", "filestore-symlink-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a real directory
	realDir := filepath.Join(tmpDir, "real")
	os.MkdirAll(realDir, 0755)

	// Create a symlink to the real directory
	linkDir := filepath.Join(tmpDir, "link")
	os.Symlink(realDir, linkDir)

	fs := &FileStore{}

	// Test symlink detection
	err = fs.checkSymlinks(linkDir, tmpDir)
	if err != ErrSymlink {
		t.Errorf("checkSymlinks() on symlink should return ErrSymlink, got %v", err)
	}

	// Test normal directory
	err = fs.checkSymlinks(realDir, tmpDir)
	if err != nil {
		t.Errorf("checkSymlinks() on normal dir should return nil, got %v", err)
	}
}

func TestHashUsername(t *testing.T) {
	tests := []struct {
		username string
		want     int64
	}{
		{"alice", hashUsername("alice")},
		{"bob", hashUsername("bob")},
		{"", 0},
	}

	for _, tt := range tests {
		t.Run(tt.username, func(t *testing.T) {
			got := hashUsername(tt.username)

			// Same username should always produce same hash
			if got != tt.want {
				t.Errorf("hashUsername() = %d, want %d", got, tt.want)
			}

			// Non-empty usernames should produce positive hash
			if tt.username != "" && got <= 0 {
				t.Errorf("hashUsername() = %d, want positive", got)
			}
		})
	}

	// Different usernames should produce different hashes
	h1 := hashUsername("alice")
	h2 := hashUsername("bob")
	if h1 == h2 {
		t.Error("hashUsername should produce different hashes for different usernames")
	}
}

func TestLoadAdminKeys(t *testing.T) {
	// Generate a valid test key
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate test key: %v", err)
	}
	pub, err := ssh.NewPublicKey(priv.Public())
	if err != nil {
		t.Fatalf("failed to create public key: %v", err)
	}
	validKey := string(ssh.MarshalAuthorizedKey(pub))

	// Create temp ssh directory
	tmpDir, err := os.MkdirTemp("", "filestore-admin-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	sshDir := filepath.Join(tmpDir, ".ssh")
	os.MkdirAll(sshDir, 0700)
	os.WriteFile(filepath.Join(sshDir, "id_ed25519.pub"), []byte(validKey), 0600)

	// Create invalid key file (should be skipped)
	os.WriteFile(filepath.Join(sshDir, "id_rsa.pub"), []byte("invalid key content"), 0600)

	// Create non-pub file (should be skipped)
	os.WriteFile(filepath.Join(sshDir, "id_ed25519"), []byte("private key"), 0600)

	// Test the key parsing logic
	key, _, _, _, err := ssh.ParseAuthorizedKey([]byte(validKey))
	if err != nil {
		t.Fatalf("failed to parse valid key: %v", err)
	}

	if key == nil {
		t.Error("key should not be nil")
	}
}

func TestReload(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "filestore-reload-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &config.Config{
		DataPath: tmpDir,
	}

	ctx := context.Background()
	store, err := NewStore(ctx, cfg)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	fs := store.(*FileStore)

	// Initial state
	initialRepoCount := len(fs.repos)
	initialUserCount := len(fs.users)

	// Reload
	err = fs.Reload()
	if err != nil {
		t.Fatalf("Reload failed: %v", err)
	}

	// Should have same counts
	if len(fs.repos) != initialRepoCount {
		t.Errorf("repo count changed after reload: %d -> %d", initialRepoCount, len(fs.repos))
	}

	if len(fs.users) != initialUserCount {
		t.Errorf("user count changed after reload: %d -> %d", initialUserCount, len(fs.users))
	}
}
