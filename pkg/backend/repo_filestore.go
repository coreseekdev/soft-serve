//go:build filestore

package backend

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/soft-serve/pkg/utils"
)

// repoPath returns the path to a repository.
// In filestore mode, repos are stored directly in DataPath.
// It checks both with and without .git suffix to support bare and normal repos.
func (d *Backend) repoPath(name string) string {
	name = utils.SanitizeRepo(name)
	rn := strings.ReplaceAll(name, "/", string(os.PathSeparator))

	// In filestore mode, repos are directly in DataPath
	basePath := d.cfg.DataPath

	// Check if repo exists without .git suffix first (normal repos)
	withoutGit := filepath.Join(basePath, rn)
	if _, err := os.Stat(withoutGit); err == nil {
		return withoutGit
	}

	// Default to .git suffix (bare repos)
	return filepath.Join(basePath, rn+".git")
}
