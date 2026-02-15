//go:build dbstore

package backend

import (
	"context"

	"charm.land/log/v2"
	"github.com/charmbracelet/soft-serve/pkg/config"
	"github.com/charmbracelet/soft-serve/pkg/db"
	"github.com/charmbracelet/soft-serve/pkg/store"
	"github.com/charmbracelet/soft-serve/pkg/task"
)

// New returns a new Soft Serve backend for database store mode.
// Repositories are stored in DataPath/repos/ directory.
func New(ctx context.Context, cfg *config.Config, database *db.DB, st store.Store) *Backend {
	logger := log.FromContext(ctx).WithPrefix("backend")
	b := &Backend{
		ctx:       ctx,
		cfg:       cfg,
		db:        database,
		store:     st,
		logger:    logger,
		manager:   task.NewManager(ctx),
		reposPath: "repos", // Repos are in DataPath/repos/
	}

	cache := newCache(b, 1000)
	b.cache = cache

	return b
}
