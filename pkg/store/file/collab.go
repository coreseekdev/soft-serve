//go:build filestore

package file

import (
	"context"

	"github.com/charmbracelet/soft-serve/pkg/access"
	"github.com/charmbracelet/soft-serve/pkg/db"
	"github.com/charmbracelet/soft-serve/pkg/db/models"
)

// GetCollabByUsernameAndRepo returns a collaborator by username and repository.
func (s *FileStore) GetCollabByUsernameAndRepo(ctx context.Context, h db.Handler, username string, repo string) (models.Collab, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	r, ok := s.repos[repo]
	if !ok {
		return models.Collab{}, db.ErrRecordNotFound
	}

	accessLevel, ok := r.collabs[username]
	if !ok {
		return models.Collab{}, db.ErrRecordNotFound
	}

	return models.Collab{
		UserID:      hashUsername(username),
		RepoID:      hashUsername(repo),
		AccessLevel: accessLevelFromString(accessLevel),
	}, nil
}

// AddCollabByUsernameAndRepo adds a collaborator to a repository.
func (s *FileStore) AddCollabByUsernameAndRepo(ctx context.Context, h db.Handler, username string, repo string, level access.AccessLevel) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	r, ok := s.repos[repo]
	if !ok {
		return db.ErrRecordNotFound
	}

	// Check if user exists
	if _, ok := s.users[username]; !ok {
		return db.ErrRecordNotFound
	}

	r.collabs[username] = accessLevelToString(level)

	// Save metadata
	return s.saveRepoMeta(r.path, &repoMeta{
		Description:   r.description,
		Private:       r.private,
		ProjectName:   r.projectName,
		Hidden:        r.hidden,
		Mirror:        r.mirror,
		Collaborators: collabMapToMeta(r.collabs),
		Webhooks:      webhookInfoToMeta(r.webhooks),
	})
}

// RemoveCollabByUsernameAndRepo removes a collaborator from a repository.
func (s *FileStore) RemoveCollabByUsernameAndRepo(ctx context.Context, h db.Handler, username string, repo string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	r, ok := s.repos[repo]
	if !ok {
		return db.ErrRecordNotFound
	}

	delete(r.collabs, username)

	// Save metadata
	return s.saveRepoMeta(r.path, &repoMeta{
		Description:   r.description,
		Private:       r.private,
		ProjectName:   r.projectName,
		Hidden:        r.hidden,
		Mirror:        r.mirror,
		Collaborators: collabMapToMeta(r.collabs),
		Webhooks:      webhookInfoToMeta(r.webhooks),
	})
}

// ListCollabsByRepo lists all collaborators for a repository.
func (s *FileStore) ListCollabsByRepo(ctx context.Context, h db.Handler, repo string) ([]models.Collab, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	r, ok := s.repos[repo]
	if !ok {
		return nil, db.ErrRecordNotFound
	}

	collabs := make([]models.Collab, 0, len(r.collabs))
	for username, accessLevel := range r.collabs {
		collabs = append(collabs, models.Collab{
			UserID:      hashUsername(username),
			RepoID:      hashUsername(repo),
			AccessLevel: accessLevelFromString(accessLevel),
		})
	}

	return collabs, nil
}

// ListCollabsByRepoAsUsers lists all collaborators as users for a repository.
func (s *FileStore) ListCollabsByRepoAsUsers(ctx context.Context, h db.Handler, repo string) ([]models.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	r, ok := s.repos[repo]
	if !ok {
		return nil, db.ErrRecordNotFound
	}

	users := make([]models.User, 0, len(r.collabs))
	for username := range r.collabs {
		if u, ok := s.users[username]; ok {
			users = append(users, models.User{
				ID:       hashUsername(u.username),
				Username: u.username,
				Admin:    u.admin,
			})
		}
	}

	return users, nil
}

// accessLevelFromString converts a string to access level.
func accessLevelFromString(s string) access.AccessLevel {
	switch s {
	case "admin", "admin-access":
		return access.AdminAccess
	case "read-write", "read-write-access":
		return access.ReadWriteAccess
	case "read-only", "read-only-access":
		return access.ReadOnlyAccess
	default:
		return access.NoAccess
	}
}

// accessLevelToString converts access level to string.
func accessLevelToString(level access.AccessLevel) string {
	switch level {
	case access.AdminAccess:
		return "admin"
	case access.ReadWriteAccess:
		return "read-write"
	case access.ReadOnlyAccess:
		return "read-only"
	default:
		return "no-access"
	}
}
