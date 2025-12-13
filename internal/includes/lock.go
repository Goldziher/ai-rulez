package includes

import (
	"os"
	"path/filepath"
	"time"

	"github.com/samber/oops"
	"gopkg.in/yaml.v3"
)

const lockFileName = "includes.lock"

// LockEntry represents a locked include source
type LockEntry struct {
	Source      string    `yaml:"source" json:"source"`
	Type        string    `yaml:"type" json:"type"`                                     // "git" or "local"
	ResolvedRef string    `yaml:"resolved_ref,omitempty" json:"resolved_ref,omitempty"` // git only
	ResolvedAt  time.Time `yaml:"resolved_at" json:"resolved_at"`
}

// Lock tracks resolved include sources
type Lock struct {
	Includes map[string]LockEntry `yaml:"includes" json:"includes"`
}

// LoadLockFile loads .ai-rulez/includes.lock
func LoadLockFile(dir string) (*Lock, error) {
	lockPath := filepath.Join(dir, ".ai-rulez", lockFileName)

	// If doesn't exist, return empty lock
	if _, err := os.Stat(lockPath); os.IsNotExist(err) {
		return &Lock{
			Includes: make(map[string]LockEntry),
		}, nil
	}

	data, err := os.ReadFile(lockPath)
	if err != nil {
		return nil, oops.Wrapf(err, "failed to read lock file")
	}

	var lock Lock
	if err := yaml.Unmarshal(data, &lock); err != nil {
		return nil, oops.Wrapf(err, "failed to parse lock file")
	}

	return &lock, nil
}

// SaveLockFile saves resolved refs/timestamps to lock file
func SaveLockFile(dir string, lock *Lock) error {
	lockPath := filepath.Join(dir, ".ai-rulez", lockFileName)

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		return oops.Wrapf(err, "failed to create lock file directory")
	}

	data, err := yaml.Marshal(lock)
	if err != nil {
		return oops.Wrapf(err, "failed to marshal lock file")
	}

	if err := os.WriteFile(lockPath, data, 0o644); err != nil {
		return oops.Wrapf(err, "failed to write lock file")
	}

	return nil
}

// UpdateLock updates lock with newly resolved include
func UpdateLock(lock *Lock, name string, entry LockEntry) {
	if lock.Includes == nil {
		lock.Includes = make(map[string]LockEntry)
	}
	lock.Includes[name] = entry
}

// CreateLockEntry creates a lock entry for a resolved include
func CreateLockEntry(sourceType, source, resolvedRef string) LockEntry {
	return LockEntry{
		Source:      source,
		Type:        sourceType,
		ResolvedRef: resolvedRef,
		ResolvedAt:  time.Now(),
	}
}
