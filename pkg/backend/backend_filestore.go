//go:build filestore

package backend

import (
	"context"

	"charm.land/log/v2"
	"github.com/charmbracelet/soft-serve/pkg/config"
	"github.com/charmbracelet/soft-serve/pkg/db"
	"github.com/charmbracelet/soft-serve/pkg/store"
	"github.com/charmbracelet/soft-serve/pkg/task"
)

// New returns a new Soft Serve backend for file store mode.
// Repositories are stored directly in DataPath (current directory).
// The db parameter should be a NoopDB in filestore mode.
func New(ctx context.Context, cfg *config.Config, noopDB *db.DB, st store.Store) *Backend {
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
