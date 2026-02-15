//go:build filestore

package backend

import (
	"context"

	"charm.land/log/v2"
	"github.com/charmbracelet/soft-serve/pkg/config"
	"github.com/charmbracelet/soft-serve/pkg/db"
	"github.com/charmbracelet/soft-serve/pkg/store/file"
	"github.com/charmbracelet/soft-serve/pkg/task"
	"golang.org/x/crypto/ssh"
)

// New returns a new Soft Serve backend for file store mode.
// Repositories are stored directly in DataPath (current directory).
// The db parameter should be a NoopDB in filestore mode.
func New(ctx context.Context, cfg *config.Config, noopDB *db.DB, st *file.FileStore) *Backend {
	logger := log.FromContext(ctx).WithPrefix("backend")
	b := &Backend{
		ctx:       ctx,
		cfg:       cfg,
		db:        noopDB, // Use NoopDB for TransactionContext calls
		store:     st,
		logger:    logger,
		manager:   task.NewManager(ctx),
		reposPath: "", // Repos are directly in DataPath
	}

	cache := newCache(b, 1000)
	b.cache = cache

	return b
}

// IsPublicKeyAuthorized checks if a public key is authorized to connect.
// In filestore mode, a key is authorized if it matches:
// - An admin key (from ~/.ssh/id_*.pub)
// - A user's key (from users_path/{username}/.ssh/*.pub)
func (d *Backend) IsPublicKeyAuthorized(_ context.Context, pk ssh.PublicKey) bool {
	if fs, ok := d.store.(*file.FileStore); ok {
		return fs.IsPublicKeyAuthorized(pk)
	}
	return false
}
