//go:build dbstore

package backend

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/soft-serve/pkg/utils"
)

// repoPath returns the path to a repository.
// In dbstore mode, repos are stored in DataPath/repos/ with .git suffix.
func (d *Backend) repoPath(name string) string {
	name = utils.SanitizeRepo(name)
	rn := strings.ReplaceAll(name, "/", string(os.PathSeparator))

	// In dbstore mode, repos are in DataPath/repos/
	return filepath.Join(d.cfg.DataPath, d.reposPath, rn+".git")
}
