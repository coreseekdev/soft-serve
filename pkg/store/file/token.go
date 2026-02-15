//go:build filestore

package file

import (
	"context"
	"time"

	"github.com/charmbracelet/soft-serve/pkg/db"
	"github.com/charmbracelet/soft-serve/pkg/db/models"
)

// GetAccessToken returns an access token by ID.
func (s *FileStore) GetAccessToken(ctx context.Context, h db.Handler, id int64) (models.AccessToken, error) {
	return models.AccessToken{}, ErrNotSupported
}

// GetAccessTokenByToken returns an access token by token string.
func (s *FileStore) GetAccessTokenByToken(ctx context.Context, h db.Handler, token string) (models.AccessToken, error) {
	return models.AccessToken{}, ErrNotSupported
}

// GetAccessTokensByUserID returns all access tokens for a user.
func (s *FileStore) GetAccessTokensByUserID(ctx context.Context, h db.Handler, userID int64) ([]models.AccessToken, error) {
	return nil, ErrNotSupported
}

// CreateAccessToken creates a new access token.
func (s *FileStore) CreateAccessToken(ctx context.Context, h db.Handler, name string, userID int64, token string, expiresAt time.Time) (models.AccessToken, error) {
	return models.AccessToken{}, ErrNotSupported
}

// DeleteAccessToken deletes an access token by ID.
func (s *FileStore) DeleteAccessToken(ctx context.Context, h db.Handler, id int64) error {
	return ErrNotSupported
}

// DeleteAccessTokenForUser deletes an access token for a user by ID.
func (s *FileStore) DeleteAccessTokenForUser(ctx context.Context, h db.Handler, userID int64, id int64) error {
	return ErrNotSupported
}
