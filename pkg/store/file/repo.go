//go:build filestore

package file

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/soft-serve/pkg/db"
	"github.com/charmbracelet/soft-serve/pkg/db/models"
)

// repoMeta represents repository metadata stored in .soft-serve.json
type repoMeta struct {
	Description   string                   `json:"description"`
	Private       bool                     `json:"private"`
	ProjectName   string                   `json:"project_name"`
	Hidden        bool                     `json:"hidden"`
	Mirror        bool                     `json:"mirror"`
	Collaborators []collaboratorMeta       `json:"collaborators"`
	Webhooks      []webhookMeta            `json:"webhooks"`
}

type collaboratorMeta struct {
	Username string `json:"username"`
	Access   string `json:"access"`
}

type webhookMeta struct {
	URL    string `json:"url"`
	Secret string `json:"secret"`
	Active bool   `json:"active"`
	Events []int  `json:"events"`
}

// discoverRepos discovers repositories in the data directory.
// Supports both bare repos (xxx.git/) and normal repos (xxx/).
func (s *FileStore) discoverRepos() error {
	entries, err := os.ReadDir(s.reposPath)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		name := entry.Name()
		fullPath := filepath.Join(s.reposPath, name)

		// Skip hidden directories
		if strings.HasPrefix(name, ".") {
			continue
		}

		// Check if it's a git repository
		isBare, isRepo := s.checkRepo(fullPath)
		if !isRepo {
			continue
		}

		// Normalize name (remove .git suffix)
		repoName := strings.TrimSuffix(name, ".git")

		// Get directory modification time
		info, err := entry.Info()
		var modTime time.Time
		if err != nil {
			modTime = time.Now()
		} else {
			modTime = info.ModTime()
		}

		// Load metadata if exists
		meta := s.loadRepoMeta(fullPath)

		s.repos[repoName] = &repoInfo{
			name:        repoName,
			path:        fullPath,
			description: meta.Description,
			private:     meta.Private,
			hidden:      meta.Hidden,
			mirror:      meta.Mirror,
			projectName: meta.ProjectName,
			collabs:     make(map[string]string),
			webhooks:    make([]webhookInfo, 0),
			modTime:     modTime,
		}

		// Load collaborators from metadata
		for _, c := range meta.Collaborators {
			s.repos[repoName].collabs[c.Username] = c.Access
		}

		// Load webhooks from metadata
		for _, w := range meta.Webhooks {
			s.repos[repoName].webhooks = append(s.repos[repoName].webhooks, webhookInfo{
				url:    w.URL,
				secret: w.Secret,
				events: w.Events,
				active: w.Active,
			})
		}

		_ = isBare // Could be used for logging
	}

	return nil
}

// checkRepo checks if a directory is a git repository.
// Returns (isBare, isRepo).
func (s *FileStore) checkRepo(path string) (bool, bool) {
	// Check for bare repo: has HEAD file and no .git directory
	headPath := filepath.Join(path, "HEAD")
	gitPath := filepath.Join(path, ".git")

	hasHead := fileExists(headPath)
	hasGitDir := dirExists(gitPath)

	if hasHead && !hasGitDir {
		// Bare repository
		return true, true
	}

	if hasGitDir {
		// Normal repository
		return false, true
	}

	return false, false
}

// loadRepoMeta loads repository metadata from .soft-serve.json.
func (s *FileStore) loadRepoMeta(repoPath string) *repoMeta {
	metaPath := filepath.Join(repoPath, ".soft-serve.json")
	data, err := os.ReadFile(metaPath)
	if err != nil {
		return &repoMeta{}
	}

	var meta repoMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return &repoMeta{}
	}

	return &meta
}

// saveRepoMeta saves repository metadata to .soft-serve.json.
func (s *FileStore) saveRepoMeta(repoPath string, meta *repoMeta) error {
	metaPath := filepath.Join(repoPath, ".soft-serve.json")
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}

	// Use atomic write: write to temp file first, then rename
	// This prevents corruption from concurrent writes
	tmpPath := metaPath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return err
	}

	// Rename is atomic on most filesystems
	return os.Rename(tmpPath, metaPath)
}

// fileExists checks if a file exists.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// dirExists checks if a directory exists.
func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// GetRepoByName returns a repository by name.
func (s *FileStore) GetRepoByName(ctx context.Context, h db.Handler, name string) (models.Repo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	r, ok := s.repos[name]
	if !ok {
		return models.Repo{}, db.ErrRecordNotFound
	}

	return repoInfoToModel(r), nil
}

// GetAllRepos returns all repositories.
func (s *FileStore) GetAllRepos(ctx context.Context, h db.Handler) ([]models.Repo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	repos := make([]models.Repo, 0, len(s.repos))
	for _, r := range s.repos {
		repos = append(repos, repoInfoToModel(r))
	}

	return repos, nil
}

// GetUserRepos returns repositories owned by a user.
func (s *FileStore) GetUserRepos(ctx context.Context, h db.Handler, userID int64) ([]models.Repo, error) {
	// In file store, all repos are accessible
	return s.GetAllRepos(ctx, h)
}

// CreateRepo creates a new repository.
func (s *FileStore) CreateRepo(ctx context.Context, h db.Handler, name string, userID int64, projectName string, description string, isPrivate bool, isHidden bool, isMirror bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.repos[name]; exists {
		return errors.New("repository already exists")
	}

	// Create bare repository
	repoPath := filepath.Join(s.reposPath, name+".git")
	if err := os.MkdirAll(repoPath, 0755); err != nil {
		return err
	}

	// Initialize bare repo
	for _, dir := range []string{"objects", "refs/heads", "refs/tags"} {
		if err := os.MkdirAll(filepath.Join(repoPath, dir), 0755); err != nil {
			return err
		}
	}

	// Create HEAD file
	if err := os.WriteFile(filepath.Join(repoPath, "HEAD"), []byte("ref: refs/heads/main\n"), 0644); err != nil {
		return err
	}

	// Create config file
	configContent := `[core]
	repositoryformatversion = 0
	filemode = true
	bare = true
`
	if err := os.WriteFile(filepath.Join(repoPath, "config"), []byte(configContent), 0644); err != nil {
		return err
	}

	// Save metadata
	meta := &repoMeta{
		Description: description,
		Private:     isPrivate,
		ProjectName: projectName,
		Hidden:      isHidden,
		Mirror:      isMirror,
	}
	if err := s.saveRepoMeta(repoPath, meta); err != nil {
		return err
	}

	s.repos[name] = &repoInfo{
		name:        name,
		path:        repoPath,
		description: description,
		private:     isPrivate,
		hidden:      isHidden,
		mirror:      isMirror,
		projectName: projectName,
		collabs:     make(map[string]string),
		webhooks:    make([]webhookInfo, 0),
	}

	return nil
}

// DeleteRepoByName deletes a repository by name.
func (s *FileStore) DeleteRepoByName(ctx context.Context, h db.Handler, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	r, ok := s.repos[name]
	if !ok {
		return db.ErrRecordNotFound
	}

	// Remove directory
	if err := os.RemoveAll(r.path); err != nil {
		return err
	}

	delete(s.repos, name)
	return nil
}

// SetRepoNameByName renames a repository.
func (s *FileStore) SetRepoNameByName(ctx context.Context, h db.Handler, name string, newName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	r, ok := s.repos[name]
	if !ok {
		return db.ErrRecordNotFound
	}

	if _, exists := s.repos[newName]; exists {
		return errors.New("repository name already exists")
	}

	// Rename directory
	newPath := filepath.Join(s.reposPath, newName+".git")
	if err := os.Rename(r.path, newPath); err != nil {
		return err
	}

	delete(s.repos, name)
	r.name = newName
	r.path = newPath
	s.repos[newName] = r

	return nil
}

// GetRepoProjectNameByName returns the project name of a repository.
func (s *FileStore) GetRepoProjectNameByName(ctx context.Context, h db.Handler, name string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	r, ok := s.repos[name]
	if !ok {
		return "", db.ErrRecordNotFound
	}

	return r.projectName, nil
}

// SetRepoProjectNameByName sets the project name of a repository.
func (s *FileStore) SetRepoProjectNameByName(ctx context.Context, h db.Handler, name string, projectName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	r, ok := s.repos[name]
	if !ok {
		return db.ErrRecordNotFound
	}

	r.projectName = projectName
	return s.saveRepoMeta(r.path, &repoMeta{
		Description:   r.description,
		Private:       r.private,
		ProjectName:   projectName,
		Hidden:        r.hidden,
		Mirror:        r.mirror,
		Collaborators: collabMapToMeta(r.collabs),
		Webhooks:      webhookInfoToMeta(r.webhooks),
	})
}

// GetRepoDescriptionByName returns the description of a repository.
func (s *FileStore) GetRepoDescriptionByName(ctx context.Context, h db.Handler, name string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	r, ok := s.repos[name]
	if !ok {
		return "", db.ErrRecordNotFound
	}

	return r.description, nil
}

// SetRepoDescriptionByName sets the description of a repository.
func (s *FileStore) SetRepoDescriptionByName(ctx context.Context, h db.Handler, name string, description string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	r, ok := s.repos[name]
	if !ok {
		return db.ErrRecordNotFound
	}

	r.description = description
	return s.saveRepoMeta(r.path, &repoMeta{
		Description:   description,
		Private:       r.private,
		ProjectName:   r.projectName,
		Hidden:        r.hidden,
		Mirror:        r.mirror,
		Collaborators: collabMapToMeta(r.collabs),
		Webhooks:      webhookInfoToMeta(r.webhooks),
	})
}

// GetRepoIsPrivateByName returns whether a repository is private.
func (s *FileStore) GetRepoIsPrivateByName(ctx context.Context, h db.Handler, name string) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	r, ok := s.repos[name]
	if !ok {
		return false, db.ErrRecordNotFound
	}

	return r.private, nil
}

// SetRepoIsPrivateByName sets whether a repository is private.
func (s *FileStore) SetRepoIsPrivateByName(ctx context.Context, h db.Handler, name string, isPrivate bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	r, ok := s.repos[name]
	if !ok {
		return db.ErrRecordNotFound
	}

	r.private = isPrivate
	return s.saveRepoMeta(r.path, &repoMeta{
		Description:   r.description,
		Private:       isPrivate,
		ProjectName:   r.projectName,
		Hidden:        r.hidden,
		Mirror:        r.mirror,
		Collaborators: collabMapToMeta(r.collabs),
		Webhooks:      webhookInfoToMeta(r.webhooks),
	})
}

// GetRepoIsHiddenByName returns whether a repository is hidden.
func (s *FileStore) GetRepoIsHiddenByName(ctx context.Context, h db.Handler, name string) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	r, ok := s.repos[name]
	if !ok {
		return false, db.ErrRecordNotFound
	}

	return r.hidden, nil
}

// SetRepoIsHiddenByName sets whether a repository is hidden.
func (s *FileStore) SetRepoIsHiddenByName(ctx context.Context, h db.Handler, name string, isHidden bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	r, ok := s.repos[name]
	if !ok {
		return db.ErrRecordNotFound
	}

	r.hidden = isHidden
	return s.saveRepoMeta(r.path, &repoMeta{
		Description:   r.description,
		Private:       r.private,
		ProjectName:   r.projectName,
		Hidden:        isHidden,
		Mirror:        r.mirror,
		Collaborators: collabMapToMeta(r.collabs),
		Webhooks:      webhookInfoToMeta(r.webhooks),
	})
}

// GetRepoIsMirrorByName returns whether a repository is a mirror.
func (s *FileStore) GetRepoIsMirrorByName(ctx context.Context, h db.Handler, name string) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	r, ok := s.repos[name]
	if !ok {
		return false, db.ErrRecordNotFound
	}

	return r.mirror, nil
}

// repoInfoToModel converts repoInfo to models.Repo.
func repoInfoToModel(r *repoInfo) models.Repo {
	return models.Repo{
		ID:          hashUsername(r.name),
		Name:        r.name,
		ProjectName: r.projectName,
		Description: r.description,
		Private:     r.private,
		Mirror:      r.mirror,
		Hidden:      r.hidden,
		UserID:      sql.NullInt64{},
		CreatedAt:   r.modTime,
		UpdatedAt:   r.modTime,
	}
}

// collabMapToMeta converts collaborator map to metadata slice.
func collabMapToMeta(m map[string]string) []collaboratorMeta {
	result := make([]collaboratorMeta, 0, len(m))
	for username, access := range m {
		result = append(result, collaboratorMeta{
			Username: username,
			Access:   access,
		})
	}
	return result
}

// webhookInfoToMeta converts webhook info to metadata slice.
func webhookInfoToMeta(w []webhookInfo) []webhookMeta {
	result := make([]webhookMeta, 0, len(w))
	for _, wh := range w {
		result = append(result, webhookMeta{
			URL:    wh.url,
			Secret: wh.secret,
			Active: wh.active,
			Events: wh.events,
		})
	}
	return result
}
