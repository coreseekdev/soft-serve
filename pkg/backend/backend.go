package backend

import (
	"context"

	"charm.land/log/v2"
	"github.com/charmbracelet/soft-serve/pkg/config"
	"github.com/charmbracelet/soft-serve/pkg/db"
	"github.com/charmbracelet/soft-serve/pkg/store"
	"github.com/charmbracelet/soft-serve/pkg/task"
)

// Backend is the Soft Serve backend that handles users, repositories, and
// server settings management and operations.
type Backend struct {
	ctx       context.Context
	cfg       *config.Config
	db        *db.DB
	store     store.Store
	logger    *log.Logger
	cache     *cache
	manager   *task.Manager
	reposPath string // Path to repos directory, relative to DataPath. Empty means DataPath directly.
}
