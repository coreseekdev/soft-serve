//go:build filestore || !dbstore

package cmd

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/charmbracelet/soft-serve/pkg/backend"
	"github.com/charmbracelet/soft-serve/pkg/config"
	"github.com/charmbracelet/soft-serve/pkg/db"
	"github.com/charmbracelet/soft-serve/pkg/hooks"
	"github.com/charmbracelet/soft-serve/pkg/store"
	"github.com/charmbracelet/soft-serve/pkg/store/file"
	"github.com/spf13/cobra"
)

// InitBackendContext initializes the backend context for FileStore.
func InitBackendContext(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()
	cfg := config.FromContext(ctx)
	if _, err := os.Stat(cfg.DataPath); errors.Is(err, fs.ErrNotExist) {
		if err := os.MkdirAll(cfg.DataPath, os.ModePerm); err != nil {
			return fmt.Errorf("create data directory: %w", err)
		}
	}

	// Create necessary directories
	dirs := []string{
		cfg.DataPath,
		file.GetUsersPath(cfg),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("create directory %s: %w", dir, err)
		}
	}

	// Initialize FileStore
	filestore, err := file.NewStore(ctx, cfg)
	if err != nil {
		return fmt.Errorf("initialize file store: %w", err)
	}

	// Use NoopDB for filestore mode
	noopDB := db.NewNoopDB()
	ctx = db.WithContext(ctx, noopDB)
	ctx = store.WithContext(ctx, filestore)
	be := backend.New(ctx, cfg, noopDB, filestore)
	ctx = backend.WithContext(ctx, be)

	cmd.SetContext(ctx)

	return nil
}

// CloseDBContext closes the database context (no-op for FileStore).
func CloseDBContext(cmd *cobra.Command, _ []string) error {
	// No database to close for FileStore
	return nil
}

// InitializeHooks initializes the hooks.
func InitializeHooks(ctx context.Context, cfg *config.Config, be *backend.Backend) error {
	repos, err := be.Repositories(ctx)
	if err != nil {
		return err
	}

	for _, repo := range repos {
		if err := hooks.GenerateHooks(ctx, cfg, repo.Name()); err != nil {
			return err
		}
	}

	return nil
}
