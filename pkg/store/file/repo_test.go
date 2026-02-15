//go:build filestore

package file

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/charmbracelet/soft-serve/pkg/config"
	"github.com/charmbracelet/soft-serve/pkg/db"
)

func TestRepoDiscovery(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "filestore-repo-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create bare repo directly in DataPath
	bareRepo := filepath.Join(tmpDir, "bare.git")
	os.MkdirAll(filepath.Join(bareRepo, "objects"), 0755)
	os.MkdirAll(filepath.Join(bareRepo, "refs", "heads"), 0755)
	os.WriteFile(filepath.Join(bareRepo, "HEAD"), []byte("ref: refs/heads/main\n"), 0644)
	os.WriteFile(filepath.Join(bareRepo, "config"), []byte("[core]\n\tbare = true\n"), 0644)

	// Create normal repo
	normalRepo := filepath.Join(tmpDir, "normal")
	os.MkdirAll(filepath.Join(normalRepo, ".git", "objects"), 0755)
	os.MkdirAll(filepath.Join(normalRepo, ".git", "refs", "heads"), 0755)
	os.WriteFile(filepath.Join(normalRepo, ".git", "HEAD"), []byte("ref: refs/heads/main\n"), 0644)

	// Create non-git directory (should be ignored)
	os.MkdirAll(filepath.Join(tmpDir, "not-a-repo"), 0755)

	// Create hidden directory (should be ignored)
	os.MkdirAll(filepath.Join(tmpDir, ".hidden"), 0755)

	// Create store
	cfg := &config.Config{DataPath: tmpDir}
	ctx := context.Background()
	store, err := NewStore(ctx, cfg)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	fs := store.(*FileStore)

	// Check discovered repos
	if len(fs.repos) != 2 {
		t.Errorf("expected 2 repos, got %d", len(fs.repos))
	}

	// Check bare repo
	if _, ok := fs.repos["bare"]; !ok {
		t.Error("bare repo should be discovered")
	}

	// Check normal repo
	if _, ok := fs.repos["normal"]; !ok {
		t.Error("normal repo should be discovered")
	}

	// Check non-git ignored
	if _, ok := fs.repos["not-a-repo"]; ok {
		t.Error("not-a-repo should be ignored")
	}

	// Check hidden ignored
	if _, ok := fs.repos[".hidden"]; ok {
		t.Error(".hidden should be ignored")
	}
}

func TestRepoCreate(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "filestore-repo-create-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &config.Config{DataPath: tmpDir}
	ctx := context.Background()
	store, err := NewStore(ctx, cfg)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	fs := store.(*FileStore)

	// Create repo
	err = fs.CreateRepo(context.Background(), nil, "new-repo", 0, "Test Repo", "A test repository", false, false, false)
	if err != nil {
		t.Fatalf("CreateRepo failed: %v", err)
	}

	// Check repo exists
	if _, ok := fs.repos["new-repo"]; !ok {
		t.Error("new-repo should exist")
	}

	// Check directory created
	repoPath := filepath.Join(tmpDir, "new-repo.git")
	if _, err := os.Stat(repoPath); os.IsNotExist(err) {
		t.Error("repo directory should be created")
	}

	// Check HEAD file
	headPath := filepath.Join(repoPath, "HEAD")
	if _, err := os.Stat(headPath); os.IsNotExist(err) {
		t.Error("HEAD file should be created")
	}
}

func TestRepoDelete(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "filestore-repo-delete-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &config.Config{DataPath: tmpDir}
	ctx := context.Background()
	store, err := NewStore(ctx, cfg)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	fs := store.(*FileStore)

	// Create repo
	fs.CreateRepo(context.Background(), nil, "to-delete", 0, "", "", false, false, false)

	// Delete repo
	err = fs.DeleteRepoByName(context.Background(), nil, "to-delete")
	if err != nil {
		t.Fatalf("DeleteRepoByName failed: %v", err)
	}

	// Check repo removed from memory
	if _, ok := fs.repos["to-delete"]; ok {
		t.Error("to-delete should be removed from memory")
	}

	// Check directory removed
	repoPath := filepath.Join(tmpDir, "to-delete.git")
	if _, err := os.Stat(repoPath); !os.IsNotExist(err) {
		t.Error("repo directory should be removed")
	}
}

func TestRepoDeleteNonExistent(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "filestore-repo-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &config.Config{DataPath: tmpDir}
	ctx := context.Background()
	store, err := NewStore(ctx, cfg)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	fs := store.(*FileStore)

	err = fs.DeleteRepoByName(context.Background(), nil, "nonexistent")
	if err != db.ErrRecordNotFound {
		t.Errorf("error = %v, want %v", err, db.ErrRecordNotFound)
	}
}

func TestRepoRename(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "filestore-repo-rename-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &config.Config{DataPath: tmpDir}
	ctx := context.Background()
	store, err := NewStore(ctx, cfg)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	fs := store.(*FileStore)

	// Create repo
	fs.CreateRepo(context.Background(), nil, "old-name", 0, "", "", false, false, false)

	// Rename repo
	err = fs.SetRepoNameByName(context.Background(), nil, "old-name", "new-name")
	if err != nil {
		t.Fatalf("SetRepoNameByName failed: %v", err)
	}

	// Check old name removed
	if _, ok := fs.repos["old-name"]; ok {
		t.Error("old-name should not exist")
	}

	// Check new name exists
	if _, ok := fs.repos["new-name"]; !ok {
		t.Error("new-name should exist")
	}

	// Check directory renamed
	if _, err := os.Stat(filepath.Join(tmpDir, "new-name.git")); os.IsNotExist(err) {
		t.Error("new-name.git should exist")
	}

	if _, err := os.Stat(filepath.Join(tmpDir, "old-name.git")); !os.IsNotExist(err) {
		t.Error("old-name.git should not exist")
	}
}

func TestRepoMetadata(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "filestore-repo-meta-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create repo with metadata directly in DataPath
	repoPath := filepath.Join(tmpDir, "meta-test.git")
	os.MkdirAll(filepath.Join(repoPath, "objects"), 0755)
	os.WriteFile(filepath.Join(repoPath, "HEAD"), []byte("ref: refs/heads/main\n"), 0644)

	// Write metadata
	meta := &repoMeta{
		Description: "Test repository",
		Private:     true,
		ProjectName: "Meta Test",
	}
	s := &FileStore{reposPath: tmpDir, repos: make(map[string]*repoInfo)}
	s.saveRepoMeta(repoPath, meta)

	// Create store to load metadata
	cfg := &config.Config{DataPath: tmpDir}
	ctx := context.Background()
	store, err := NewStore(ctx, cfg)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	fs := store.(*FileStore)

	// Check loaded metadata
	repo, ok := fs.repos["meta-test"]
	if !ok {
		t.Fatal("meta-test repo should be discovered")
	}

	if repo.description != "Test repository" {
		t.Errorf("description = %q, want %q", repo.description, "Test repository")
	}

	if !repo.private {
		t.Error("repo should be private")
	}

	if repo.projectName != "Meta Test" {
		t.Errorf("projectName = %q, want %q", repo.projectName, "Meta Test")
	}
}

func TestRepoSetDescription(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "filestore-repo-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &config.Config{DataPath: tmpDir}
	ctx := context.Background()
	store, err := NewStore(ctx, cfg)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	fs := store.(*FileStore)

	// Create repo
	fs.CreateRepo(context.Background(), nil, "test", 0, "", "Original description", false, false, false)

	// Update description
	err = fs.SetRepoDescriptionByName(context.Background(), nil, "test", "New description")
	if err != nil {
		t.Fatalf("SetRepoDescriptionByName failed: %v", err)
	}

	// Check description updated
	desc, err := fs.GetRepoDescriptionByName(context.Background(), nil, "test")
	if err != nil {
		t.Fatalf("GetRepoDescriptionByName failed: %v", err)
	}

	if desc != "New description" {
		t.Errorf("description = %q, want %q", desc, "New description")
	}
}

func TestRepoSetPrivate(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "filestore-repo-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &config.Config{DataPath: tmpDir}
	ctx := context.Background()
	store, err := NewStore(ctx, cfg)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	fs := store.(*FileStore)

	// Create repo (public)
	fs.CreateRepo(context.Background(), nil, "test", 0, "", "", false, false, false)

	// Set private
	err = fs.SetRepoIsPrivateByName(context.Background(), nil, "test", true)
	if err != nil {
		t.Fatalf("SetRepoIsPrivateByName failed: %v", err)
	}

	// Check private
	isPrivate, err := fs.GetRepoIsPrivateByName(context.Background(), nil, "test")
	if err != nil {
		t.Fatalf("GetRepoIsPrivateByName failed: %v", err)
	}

	if !isPrivate {
		t.Error("repo should be private")
	}
}

func TestCheckRepo(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(string) // Setup function for test directory
		isBare    bool
		isRepo    bool
	}{
		{
			name: "bare repo",
			setup: func(dir string) {
				os.WriteFile(filepath.Join(dir, "HEAD"), []byte("ref: refs/heads/main\n"), 0644)
				os.WriteFile(filepath.Join(dir, "config"), []byte("[core]\n\tbare = true\n"), 0644)
			},
			isBare: true,
			isRepo: true,
		},
		{
			name: "normal repo",
			setup: func(dir string) {
				gitDir := filepath.Join(dir, ".git")
				os.MkdirAll(gitDir, 0755)
				os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/main\n"), 0644)
			},
			isBare: false,
			isRepo: true,
		},
		{
			name: "not a repo",
			setup: func(dir string) {
				// Just an empty directory
			},
			isBare: false,
			isRepo: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir, err := os.MkdirTemp("", "check-repo-test-*")
			if err != nil {
				t.Fatalf("failed to create temp dir: %v", err)
			}
			defer os.RemoveAll(tmpDir)

			repoDir := filepath.Join(tmpDir, "test")
			os.MkdirAll(repoDir, 0755)
			tt.setup(repoDir)

			fs := &FileStore{reposPath: tmpDir}
			isBare, isRepo := fs.checkRepo(repoDir)

			if isBare != tt.isBare {
				t.Errorf("isBare = %v, want %v", isBare, tt.isBare)
			}

			if isRepo != tt.isRepo {
				t.Errorf("isRepo = %v, want %v", isRepo, tt.isRepo)
			}
		})
	}
}
