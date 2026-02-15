//go:build filestore

package file

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"os"
	"path/filepath"
	"testing"

	"github.com/charmbracelet/soft-serve/pkg/config"
	"github.com/charmbracelet/soft-serve/pkg/db"
	"golang.org/x/crypto/ssh"
)

func setupTestStore(t *testing.T) (*FileStore, func()) {
	tmpDir, err := os.MkdirTemp("", "filestore-user-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	cfg := &config.Config{
		DataPath: tmpDir,
	}

	ctx := context.Background()
	store, err := NewStore(ctx, cfg)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("NewStore failed: %v", err)
	}

	cleanup := func() {
		os.RemoveAll(tmpDir)
	}

	return store.(*FileStore), cleanup
}

func TestUserCreateAndFind(t *testing.T) {
	fs, cleanup := setupTestStore(t)
	defer cleanup()

	// Generate test key
	testKey := generateTestKey(t)

	// Create user
	err := fs.CreateUser(context.Background(), nil, "testuser", false, []ssh.PublicKey{testKey})
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	// Find user by username
	user, err := fs.FindUserByUsername(context.Background(), nil, "testuser")
	if err != nil {
		t.Fatalf("FindUserByUsername failed: %v", err)
	}

	if user.Username != "testuser" {
		t.Errorf("username = %q, want %q", user.Username, "testuser")
	}

	if user.Admin {
		t.Error("user should not be admin")
	}
}

func TestUserCreateInvalidUsername(t *testing.T) {
	fs, cleanup := setupTestStore(t)
	defer cleanup()

	testKey := generateTestKey(t)

	// Test various invalid usernames
	invalidNames := []string{
		"",           // empty
		"user/name",  // contains slash
		"user\\name", // contains backslash
		"user..name", // contains double dot
		".user",      // starts with dot
		"user.",      // ends with dot
		"-user",      // starts with dash
		"user@name",  // contains @
	}

	for _, name := range invalidNames {
		err := fs.CreateUser(context.Background(), nil, name, false, []ssh.PublicKey{testKey})
		if err == nil {
			t.Errorf("CreateUser(%q) should fail", name)
		}
	}
}

func TestUserDuplicate(t *testing.T) {
	fs, cleanup := setupTestStore(t)
	defer cleanup()

	testKey := generateTestKey(t)

	// Create user
	err := fs.CreateUser(context.Background(), nil, "testuser", false, []ssh.PublicKey{testKey})
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	// Try to create again
	err = fs.CreateUser(context.Background(), nil, "testuser", false, []ssh.PublicKey{testKey})
	if err == nil {
		t.Error("CreateUser should fail for duplicate user")
	}
}

func TestUserDelete(t *testing.T) {
	fs, cleanup := setupTestStore(t)
	defer cleanup()

	testKey := generateTestKey(t)

	// Create user
	fs.CreateUser(context.Background(), nil, "testuser", false, []ssh.PublicKey{testKey})

	// Delete user
	err := fs.DeleteUserByUsername(context.Background(), nil, "testuser")
	if err != nil {
		t.Fatalf("DeleteUserByUsername failed: %v", err)
	}

	// Should not find user
	_, err = fs.FindUserByUsername(context.Background(), nil, "testuser")
	if err != db.ErrRecordNotFound {
		t.Errorf("error = %v, want %v", err, db.ErrRecordNotFound)
	}
}

func TestUserDeleteNonExistent(t *testing.T) {
	fs, cleanup := setupTestStore(t)
	defer cleanup()

	err := fs.DeleteUserByUsername(context.Background(), nil, "nonexistent")
	if err != db.ErrRecordNotFound {
		t.Errorf("error = %v, want %v", err, db.ErrRecordNotFound)
	}
}

func TestUserSetAdmin(t *testing.T) {
	fs, cleanup := setupTestStore(t)
	defer cleanup()

	testKey := generateTestKey(t)
	fs.CreateUser(context.Background(), nil, "testuser", false, []ssh.PublicKey{testKey})

	// Set admin
	err := fs.SetAdminByUsername(context.Background(), nil, "testuser", true)
	if err != nil {
		t.Fatalf("SetAdminByUsername failed: %v", err)
	}

	// Check admin status
	user, _ := fs.FindUserByUsername(context.Background(), nil, "testuser")
	if !user.Admin {
		t.Error("user should be admin")
	}

	// Remove admin
	fs.SetAdminByUsername(context.Background(), nil, "testuser", false)
	user, _ = fs.FindUserByUsername(context.Background(), nil, "testuser")
	if user.Admin {
		t.Error("user should not be admin")
	}
}

func TestUserAddKey(t *testing.T) {
	fs, cleanup := setupTestStore(t)
	defer cleanup()

	testKey1 := generateTestKey(t)
	testKey2 := generateTestKeyWithComment(t, "key2@test")

	// Create user with one key
	fs.CreateUser(context.Background(), nil, "testuser", false, []ssh.PublicKey{testKey1})

	// Add another key
	err := fs.AddPublicKeyByUsername(context.Background(), nil, "testuser", testKey2)
	if err != nil {
		t.Fatalf("AddPublicKeyByUsername failed: %v", err)
	}

	// Check keys count
	keys, err := fs.ListPublicKeysByUsername(context.Background(), nil, "testuser")
	if err != nil {
		t.Fatalf("ListPublicKeysByUsername failed: %v", err)
	}

	if len(keys) != 2 {
		t.Errorf("keys count = %d, want 2", len(keys))
	}
}

func TestUserAddDuplicateKey(t *testing.T) {
	fs, cleanup := setupTestStore(t)
	defer cleanup()

	testKey := generateTestKey(t)
	fs.CreateUser(context.Background(), nil, "testuser", false, []ssh.PublicKey{testKey})

	// Try to add same key
	err := fs.AddPublicKeyByUsername(context.Background(), nil, "testuser", testKey)
	if err == nil {
		t.Error("AddPublicKeyByUsername should fail for duplicate key")
	}
}

func TestUserRemoveKey(t *testing.T) {
	fs, cleanup := setupTestStore(t)
	defer cleanup()

	testKey1 := generateTestKey(t)
	testKey2 := generateTestKeyWithComment(t, "key2@test")

	fs.CreateUser(context.Background(), nil, "testuser", false, []ssh.PublicKey{testKey1, testKey2})

	// Remove first key
	err := fs.RemovePublicKeyByUsername(context.Background(), nil, "testuser", testKey1)
	if err != nil {
		t.Fatalf("RemovePublicKeyByUsername failed: %v", err)
	}

	// Check keys count
	keys, _ := fs.ListPublicKeysByUsername(context.Background(), nil, "testuser")
	if len(keys) != 1 {
		t.Errorf("keys count = %d, want 1", len(keys))
	}
}

func TestUserRename(t *testing.T) {
	fs, cleanup := setupTestStore(t)
	defer cleanup()

	testKey := generateTestKey(t)
	fs.CreateUser(context.Background(), nil, "oldname", false, []ssh.PublicKey{testKey})

	// Rename user
	err := fs.SetUsernameByUsername(context.Background(), nil, "oldname", "newname")
	if err != nil {
		t.Fatalf("SetUsernameByUsername failed: %v", err)
	}

	// Old name should not exist
	_, err = fs.FindUserByUsername(context.Background(), nil, "oldname")
	if err != db.ErrRecordNotFound {
		t.Error("old username should not exist")
	}

	// New name should exist
	user, err := fs.FindUserByUsername(context.Background(), nil, "newname")
	if err != nil {
		t.Fatalf("FindUserByUsername failed: %v", err)
	}

	if user.Username != "newname" {
		t.Errorf("username = %q, want %q", user.Username, "newname")
	}

	// Check directory was renamed
	if _, err := os.Stat(filepath.Join(fs.usersPath, "oldname")); !os.IsNotExist(err) {
		t.Error("old directory should not exist")
	}

	if _, err := os.Stat(filepath.Join(fs.usersPath, "newname")); os.IsNotExist(err) {
		t.Error("new directory should exist")
	}
}

func TestUserFindByPublicKey(t *testing.T) {
	fs, cleanup := setupTestStore(t)
	defer cleanup()

	testKey := generateTestKey(t)
	fs.CreateUser(context.Background(), nil, "testuser", false, []ssh.PublicKey{testKey})

	// Find by public key
	user, err := fs.FindUserByPublicKey(context.Background(), nil, testKey)
	if err != nil {
		t.Fatalf("FindUserByPublicKey failed: %v", err)
	}

	if user.Username != "testuser" {
		t.Errorf("username = %q, want %q", user.Username, "testuser")
	}

	// Try with different key
	otherKey := generateTestKeyWithComment(t, "other@test")
	_, err = fs.FindUserByPublicKey(context.Background(), nil, otherKey)
	if err != db.ErrRecordNotFound {
		t.Errorf("error = %v, want %v", err, db.ErrRecordNotFound)
	}
}

func TestGetAllUsers(t *testing.T) {
	fs, cleanup := setupTestStore(t)
	defer cleanup()

	testKey := generateTestKey(t)

	// Initially might have admin user, so just record count
	users, _ := fs.GetAllUsers(context.Background(), nil)
	initialCount := len(users)

	// Create users
	fs.CreateUser(context.Background(), nil, "user1", false, []ssh.PublicKey{testKey})
	fs.CreateUser(context.Background(), nil, "user2", false, []ssh.PublicKey{testKey})

	users, err := fs.GetAllUsers(context.Background(), nil)
	if err != nil {
		t.Fatalf("GetAllUsers failed: %v", err)
	}

	if len(users) != initialCount+2 {
		t.Errorf("users count = %d, want %d", len(users), initialCount+2)
	}
}

func TestLoadUsersFromDirectory(t *testing.T) {
	// Create temp directory with pre-existing users
	tmpDir, err := os.MkdirTemp("", "filestore-load-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create user directories with keys
	testKey := generateTestKey(t)
	testKeyStr := string(ssh.MarshalAuthorizedKey(testKey))

	// Create alice
	aliceSSH := filepath.Join(tmpDir, "users", "alice", ".ssh")
	os.MkdirAll(aliceSSH, 0700)
	os.WriteFile(filepath.Join(aliceSSH, "id_ed25519.pub"), []byte(testKeyStr), 0600)

	// Create bob
	bobSSH := filepath.Join(tmpDir, "users", "bob", ".ssh")
	os.MkdirAll(bobSSH, 0700)
	os.WriteFile(filepath.Join(bobSSH, "id_ed25519.pub"), []byte(testKeyStr), 0600)

	// Create invalid user (no keys)
	os.MkdirAll(filepath.Join(tmpDir, "users", "invalid", ".ssh"), 0700)

	// Create store
	cfg := &config.Config{DataPath: tmpDir}
	ctx := context.Background()
	store, err := NewStore(ctx, cfg)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	fs := store.(*FileStore)

	// Check loaded users
	if len(fs.users) < 2 {
		t.Errorf("expected at least 2 users, got %d", len(fs.users))
	}

	// Check alice exists
	if _, ok := fs.users["alice"]; !ok {
		t.Error("alice should be loaded")
	}

	// Check bob exists
	if _, ok := fs.users["bob"]; !ok {
		t.Error("bob should be loaded")
	}

	// invalid user should not be loaded
	if _, ok := fs.users["invalid"]; ok {
		t.Error("invalid user (no keys) should not be loaded")
	}
}

func TestPathTraversalProtection(t *testing.T) {
	fs, cleanup := setupTestStore(t)
	defer cleanup()

	testKey := generateTestKey(t)

	// Try to create user with path traversal attempt
	traversalNames := []string{
		"../etc",
		"..\\windows",
		"user/../other",
		"user/../../etc",
	}

	for _, name := range traversalNames {
		// Username validation should catch these
		err := fs.CreateUser(context.Background(), nil, name, false, []ssh.PublicKey{testKey})
		if err == nil {
			t.Errorf("CreateUser(%q) should fail", name)
		}
	}
}

// Helper functions

func generateTestKey(t *testing.T) ssh.PublicKey {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate test key: %v", err)
	}
	pub, err := ssh.NewPublicKey(priv.Public())
	if err != nil {
		t.Fatalf("failed to create public key: %v", err)
	}
	return pub
}

func generateTestKeyWithComment(t *testing.T, comment string) ssh.PublicKey {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate test key: %v", err)
	}
	pub, err := ssh.NewPublicKey(priv.Public())
	if err != nil {
		t.Fatalf("failed to create public key: %v", err)
	}
	return pub
}
