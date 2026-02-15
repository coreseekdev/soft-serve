//go:build filestore

package file

import (
	"context"

	"github.com/charmbracelet/soft-serve/pkg/access"
	"github.com/charmbracelet/soft-serve/pkg/db"
)

// Default anonymous access level for file store
const defaultAnonAccess = access.ReadOnlyAccess

// GetAnonAccess returns the anonymous access level.
func (s *FileStore) GetAnonAccess(ctx context.Context, h db.Handler) (access.AccessLevel, error) {
	// In file store, we use a fixed default value
	// Could be read from config.yaml in the future
	return defaultAnonAccess, nil
}

// SetAnonAccess sets the anonymous access level.
func (s *FileStore) SetAnonAccess(ctx context.Context, h db.Handler, level access.AccessLevel) error {
	// Not supported in file store
	return ErrNotSupported
}

// GetAllowKeylessAccess returns whether keyless access is allowed.
func (s *FileStore) GetAllowKeylessAccess(ctx context.Context, h db.Handler) (bool, error) {
	// In file store, we allow keyless access by default
	return true, nil
}

// SetAllowKeylessAccess sets whether keyless access is allowed.
func (s *FileStore) SetAllowKeylessAccess(ctx context.Context, h db.Handler, allow bool) error {
	// Not supported in file store
	return ErrNotSupported
}
