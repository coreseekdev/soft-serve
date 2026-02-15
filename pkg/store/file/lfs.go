//go:build filestore

package file

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/charmbracelet/soft-serve/pkg/db"
	"github.com/charmbracelet/soft-serve/pkg/db/models"
)

// LFS object storage structure:
// {lfs_path}/
// └── objects/
//     └── {oid[:2]}/
//         └── {oid}/
//             └── data

// LFS Lock storage structure:
// {lfs_path}/
// └── locks/
//     └── {repo_name}.json

// lockMeta represents an LFS lock stored in JSON
type lockMeta struct {
	ID        int64     `json:"id"`
	Path      string    `json:"path"`
	UserID    int64     `json:"user_id"`
	RepoID    int64     `json:"repo_id"`
	Refname   string    `json:"refname"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// lockFile represents the lock file structure
type lockFile struct {
	Locks []lockMeta `json:"locks"`
}

// lockMutex protects concurrent access to lock files
var lockMutex sync.Mutex

// CreateLFSObject creates an LFS object.
func (s *FileStore) CreateLFSObject(ctx context.Context, h db.Handler, repoID int64, oid string, size int64) error {
	// LFS objects are stored on disk, no database record needed
	objPath := s.lfsObjectPath(oid)
	if err := os.MkdirAll(filepath.Dir(objPath), 0755); err != nil {
		return err
	}
	return nil
}

// GetLFSObjectByOid returns an LFS object by OID.
func (s *FileStore) GetLFSObjectByOid(ctx context.Context, h db.Handler, repoID int64, oid string) (models.LFSObject, error) {
	objPath := s.lfsObjectPath(oid)
	if !fileExists(objPath) {
		return models.LFSObject{}, db.ErrRecordNotFound
	}

	info, err := os.Stat(objPath)
	if err != nil {
		return models.LFSObject{}, err
	}

	return models.LFSObject{
		Oid:    oid,
		Size:   info.Size(),
		RepoID: repoID,
	}, nil
}

// GetLFSObjects returns all LFS objects for a repository.
func (s *FileStore) GetLFSObjects(ctx context.Context, h db.Handler, repoID int64) ([]models.LFSObject, error) {
	// Scan LFS directory for objects
	objects := make([]models.LFSObject, 0)
	objectsDir := filepath.Join(s.lfsPath, "objects")

	entries, err := os.ReadDir(objectsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return objects, nil
		}
		return nil, err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		oidDir := filepath.Join(objectsDir, entry.Name())
		oidEntries, err := os.ReadDir(oidDir)
		if err != nil {
			continue
		}

		for _, oidEntry := range oidEntries {
			if !oidEntry.IsDir() {
				continue
			}

			oid := oidEntry.Name()
			dataPath := filepath.Join(oidDir, oid, "data")
			if info, err := os.Stat(dataPath); err == nil {
				objects = append(objects, models.LFSObject{
					Oid:    oid,
					Size:   info.Size(),
					RepoID: repoID,
				})
			}
		}
	}

	return objects, nil
}

// GetLFSObjectsByName returns all LFS objects for a repository by name.
func (s *FileStore) GetLFSObjectsByName(ctx context.Context, h db.Handler, name string) ([]models.LFSObject, error) {
	s.mu.RLock()
	_, ok := s.repos[name]
	s.mu.RUnlock()

	if !ok {
		return nil, db.ErrRecordNotFound
	}

	repoID := hashUsername(name)
	return s.GetLFSObjects(ctx, h, repoID)
}

// DeleteLFSObjectByOid deletes an LFS object by OID.
func (s *FileStore) DeleteLFSObjectByOid(ctx context.Context, h db.Handler, repoID int64, oid string) error {
	objPath := s.lfsObjectPath(oid)
	return os.RemoveAll(filepath.Dir(objPath))
}

// LFS Lock storage structure:
// {lfs_path}/
// └── locks/
//     └── {repo_name}.json

// lockFilePath returns the path to the lock file for a repo
func (s *FileStore) lockFilePath(repoName string) string {
	return filepath.Join(s.lfsPath, "locks", repoName+".json")
}

// loadLocks loads locks from a repo's lock file
func (s *FileStore) loadLocks(repoName string) (*lockFile, error) {
	lockPath := s.lockFilePath(repoName)
	data, err := os.ReadFile(lockPath)
	if err != nil {
		if os.IsNotExist(err) {
			return &lockFile{Locks: []lockMeta{}}, nil
		}
		return nil, err
	}

	var lf lockFile
	if err := json.Unmarshal(data, &lf); err != nil {
		return nil, err
	}

	return &lf, nil
}

// saveLocks saves locks to a repo's lock file
func (s *FileStore) saveLocks(repoName string, lf *lockFile) error {
	lockPath := s.lockFilePath(repoName)
	if err := os.MkdirAll(filepath.Dir(lockPath), 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(lf, "", "  ")
	if err != nil {
		return err
	}

	// Atomic write
	tmpPath := lockPath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return err
	}

	return os.Rename(tmpPath, lockPath)
}

// getRepoNameByID returns repo name by ID (using hash)
func (s *FileStore) getRepoNameByID(repoID int64) string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for name, info := range s.repos {
		if hashUsername(name) == repoID {
			return info.name
		}
	}
	return ""
}

// getRepoNameByIDWithHandler returns repo name by ID
func (s *FileStore) getRepoNameByIDWithHandler(repoID int64, h db.Handler) string {
	return s.getRepoNameByID(repoID)
}

// getUserNameByID returns username by ID
func (s *FileStore) getUserNameByID(userID int64) string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for name, info := range s.users {
		if hashUsername(name) == userID {
			return info.username
		}
	}
	return ""
}

// lockMetaToModel converts lockMeta to models.LFSLock
func lockMetaToModel(lm lockMeta) models.LFSLock {
	return models.LFSLock{
		ID:        lm.ID,
		Path:      lm.Path,
		UserID:    lm.UserID,
		RepoID:    lm.RepoID,
		Refname:   lm.Refname,
		CreatedAt: lm.CreatedAt,
		UpdatedAt: lm.UpdatedAt,
	}
}

// CreateLFSLockForUser creates an LFS lock for a user.
func (s *FileStore) CreateLFSLockForUser(ctx context.Context, h db.Handler, repoID int64, userID int64, path string, refname string) error {
	lockMutex.Lock()
	defer lockMutex.Unlock()

	repoName := s.getRepoNameByIDWithHandler(repoID, h)
	if repoName == "" {
		return db.ErrRecordNotFound
	}

	lf, err := s.loadLocks(repoName)
	if err != nil {
		return err
	}

	// Check if path is already locked
	for _, l := range lf.Locks {
		if l.Path == path {
			return errors.New("path is already locked")
		}
	}

	// Create new lock
	now := time.Now()
	newLock := lockMeta{
		ID:        now.UnixNano(), // Use nanosecond timestamp as ID
		Path:      path,
		UserID:    userID,
		RepoID:    repoID,
		Refname:   refname,
		CreatedAt: now,
		UpdatedAt: now,
	}

	lf.Locks = append(lf.Locks, newLock)
	return s.saveLocks(repoName, lf)
}

// GetLFSLocks returns LFS locks for a repository.
func (s *FileStore) GetLFSLocks(ctx context.Context, h db.Handler, repoID int64, page int, limit int) ([]models.LFSLock, error) {
	repoName := s.getRepoNameByIDWithHandler(repoID, h)
	if repoName == "" {
		return nil, db.ErrRecordNotFound
	}

	lf, err := s.loadLocks(repoName)
	if err != nil {
		return nil, err
	}

	// Paginate
	start := 0
	if page > 0 {
		start = (page - 1) * limit
	}
	end := start + limit
	if end > len(lf.Locks) {
		end = len(lf.Locks)
	}
	if start > len(lf.Locks) {
		start = len(lf.Locks)
	}

	locks := make([]models.LFSLock, 0, end-start)
	for i := start; i < end; i++ {
		locks = append(locks, lockMetaToModel(lf.Locks[i]))
	}

	return locks, nil
}

// GetLFSLocksWithCount returns LFS locks for a repository with count.
func (s *FileStore) GetLFSLocksWithCount(ctx context.Context, h db.Handler, repoID int64, page int, limit int) ([]models.LFSLock, int64, error) {
	repoName := s.getRepoNameByIDWithHandler(repoID, h)
	if repoName == "" {
		return nil, 0, db.ErrRecordNotFound
	}

	lf, err := s.loadLocks(repoName)
	if err != nil {
		return nil, 0, err
	}

	// Paginate
	start := 0
	if page > 0 {
		start = (page - 1) * limit
	}
	end := start + limit
	if end > len(lf.Locks) {
		end = len(lf.Locks)
	}
	if start > len(lf.Locks) {
		start = len(lf.Locks)
	}

	locks := make([]models.LFSLock, 0, end-start)
	for i := start; i < end; i++ {
		locks = append(locks, lockMetaToModel(lf.Locks[i]))
	}

	return locks, int64(len(lf.Locks)), nil
}

// GetLFSLocksForUser returns LFS locks for a user.
func (s *FileStore) GetLFSLocksForUser(ctx context.Context, h db.Handler, repoID int64, userID int64) ([]models.LFSLock, error) {
	repoName := s.getRepoNameByIDWithHandler(repoID, h)
	if repoName == "" {
		return nil, db.ErrRecordNotFound
	}

	lf, err := s.loadLocks(repoName)
	if err != nil {
		return nil, err
	}

	locks := make([]models.LFSLock, 0)
	for _, l := range lf.Locks {
		if l.UserID == userID {
			locks = append(locks, lockMetaToModel(l))
		}
	}

	return locks, nil
}

// GetLFSLockForPath returns an LFS lock for a path.
func (s *FileStore) GetLFSLockForPath(ctx context.Context, h db.Handler, repoID int64, path string) (models.LFSLock, error) {
	repoName := s.getRepoNameByIDWithHandler(repoID, h)
	if repoName == "" {
		return models.LFSLock{}, db.ErrRecordNotFound
	}

	lf, err := s.loadLocks(repoName)
	if err != nil {
		return models.LFSLock{}, err
	}

	for _, l := range lf.Locks {
		if l.Path == path {
			return lockMetaToModel(l), nil
		}
	}

	return models.LFSLock{}, db.ErrRecordNotFound
}

// GetLFSLockForUserPath returns an LFS lock for a user path.
func (s *FileStore) GetLFSLockForUserPath(ctx context.Context, h db.Handler, repoID int64, userID int64, path string) (models.LFSLock, error) {
	repoName := s.getRepoNameByIDWithHandler(repoID, h)
	if repoName == "" {
		return models.LFSLock{}, db.ErrRecordNotFound
	}

	lf, err := s.loadLocks(repoName)
	if err != nil {
		return models.LFSLock{}, err
	}

	for _, l := range lf.Locks {
		if l.Path == path && l.UserID == userID {
			return lockMetaToModel(l), nil
		}
	}

	return models.LFSLock{}, db.ErrRecordNotFound
}

// GetLFSLockByID returns an LFS lock by ID.
func (s *FileStore) GetLFSLockByID(ctx context.Context, h db.Handler, id int64) (models.LFSLock, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, repo := range s.repos {
		lf, err := s.loadLocks(repo.name)
		if err != nil {
			continue
		}

		for _, l := range lf.Locks {
			if l.ID == id {
				return lockMetaToModel(l), nil
			}
		}
	}

	return models.LFSLock{}, db.ErrRecordNotFound
}

// GetLFSLockForUserByID returns an LFS lock for a user by ID.
func (s *FileStore) GetLFSLockForUserByID(ctx context.Context, h db.Handler, repoID int64, userID int64, id int64) (models.LFSLock, error) {
	repoName := s.getRepoNameByIDWithHandler(repoID, h)
	if repoName == "" {
		return models.LFSLock{}, db.ErrRecordNotFound
	}

	lf, err := s.loadLocks(repoName)
	if err != nil {
		return models.LFSLock{}, err
	}

	for _, l := range lf.Locks {
		if l.ID == id && l.UserID == userID {
			return lockMetaToModel(l), nil
		}
	}

	return models.LFSLock{}, db.ErrRecordNotFound
}

// DeleteLFSLock deletes an LFS lock.
func (s *FileStore) DeleteLFSLock(ctx context.Context, h db.Handler, repoID int64, id int64) error {
	lockMutex.Lock()
	defer lockMutex.Unlock()

	repoName := s.getRepoNameByIDWithHandler(repoID, h)
	if repoName == "" {
		return db.ErrRecordNotFound
	}

	lf, err := s.loadLocks(repoName)
	if err != nil {
		return err
	}

	for i, l := range lf.Locks {
		if l.ID == id {
			lf.Locks = append(lf.Locks[:i], lf.Locks[i+1:]...)
			return s.saveLocks(repoName, lf)
		}
	}

	return db.ErrRecordNotFound
}

// DeleteLFSLockForUserByID deletes an LFS lock for a user by ID.
func (s *FileStore) DeleteLFSLockForUserByID(ctx context.Context, h db.Handler, repoID int64, userID int64, id int64) error {
	lockMutex.Lock()
	defer lockMutex.Unlock()

	repoName := s.getRepoNameByIDWithHandler(repoID, h)
	if repoName == "" {
		return db.ErrRecordNotFound
	}

	lf, err := s.loadLocks(repoName)
	if err != nil {
		return err
	}

	for i, l := range lf.Locks {
		if l.ID == id && l.UserID == userID {
			lf.Locks = append(lf.Locks[:i], lf.Locks[i+1:]...)
			return s.saveLocks(repoName, lf)
		}
	}

	return db.ErrRecordNotFound
}

// lfsObjectPath returns the path to an LFS object.
func (s *FileStore) lfsObjectPath(oid string) string {
	if len(oid) < 2 {
		oid = "00" + oid
	}
	return filepath.Join(s.lfsPath, "objects", oid[:2], oid, "data")
}
