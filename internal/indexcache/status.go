package indexcache

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// Report describes a cache without changing it.
type Report struct {
	// Mode is the configured [mcp] index_cache mode.
	Mode string `json:"mode"`
	// Kind is where the cache is: KindUser, KindRepo or KindOff.
	Kind string `json:"kind"`
	// Dir is the cache directory, or where it would be; empty when off.
	Dir string `json:"dir,omitempty"`
	// Exists is true when the directory holds a cache.
	Exists bool `json:"exists"`
	// Fallback is true when the project directory holds the cache of a project
	// that asked for the user cache.
	Fallback bool `json:"fallback,omitempty"`
	// Bytes is the size of the cache directory.
	Bytes int64 `json:"bytes"`
	// Entries and Verdicts count the records of the index file.
	Entries  int `json:"entries"`
	Verdicts int `json:"verdicts"`
	// Warnings lists what is wrong: an unwritable location, a corrupt file.
	Warnings []string `json:"warnings,omitempty"`
}

// Inspect reports where the cache is and what it holds. It creates nothing.
func Inspect(_ context.Context, o Options) (Report, error) {
	if o.Root == "" {
		return Report{}, oops.Errorf("inspect the index cache: no project root")
	}
	rep := Report{Mode: o.mode(), Kind: KindOff}
	if rep.Mode == config.IndexCacheOff {
		return rep, nil
	}
	userDir, userErr := o.userDir()
	repoDir := o.repoDir()
	switch {
	case rep.Mode == config.IndexCacheRepo:
		rep.Kind, rep.Dir = KindRepo, repoDir
	case userErr == nil && exists(userDir):
		rep.Kind, rep.Dir = KindUser, userDir
	case exists(repoDir):
		rep.Kind, rep.Dir, rep.Fallback = KindRepo, repoDir, true
	case userErr == nil:
		rep.Kind, rep.Dir = KindUser, userDir
	default:
		rep.Kind, rep.Dir, rep.Fallback = KindRepo, repoDir, true
		rep.Warnings = append(rep.Warnings, userErr.Error())
	}
	if !writable(rep.Dir) {
		rep.Warnings = append(rep.Warnings, "the cache location "+rep.Dir+" is not writable")
	}
	if !exists(rep.Dir) {
		return rep, nil
	}
	rep.Exists = true
	rep.Bytes = dirSize(rep.Dir)
	p, reason := readPayload(rep.Dir, o.Key)
	if reason != "" {
		rep.Warnings = append(rep.Warnings, "the cache index is "+reasonText(reason)+" and is rebuilt on the next start")
	}
	rep.Entries, rep.Verdicts = len(p.Entries), len(p.Verdicts)
	return rep, nil
}

func reasonText(reason string) string {
	switch reason {
	case ReasonVersion:
		return "of another version"
	case ReasonIntegrity:
		return "not authentic (corrupt or edited)"
	}
	return "corrupt"
}

func exists(dir string) bool {
	info, err := os.Stat(dir)
	return err == nil && info.IsDir()
}

// writable reports whether a file could be created in dir, or in the nearest
// existing directory above it.
func writable(dir string) bool {
	for cur := dir; ; cur = filepath.Dir(cur) {
		info, err := os.Stat(cur)
		if err == nil {
			if !info.IsDir() {
				return false
			}
			probe, err := os.CreateTemp(cur, ".probe-*")
			if err != nil {
				return false
			}
			name := probe.Name()
			_ = probe.Close()   //nolint:errcheck // a probe file
			_ = os.Remove(name) //nolint:errcheck // a probe file
			return true
		}
		if parent := filepath.Dir(cur); parent == cur {
			return false
		}
	}
}

func dirSize(dir string) int64 {
	var total int64
	//nolint:errcheck // an unreadable entry is not counted
	_ = filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil //nolint:nilerr // an unreadable entry is not counted
		}
		if info, infoErr := d.Info(); infoErr == nil {
			total += info.Size()
		}
		return nil
	})
	return total
}

// Cleared says what Clear removed.
type Cleared struct {
	// Removed lists the directories that were removed.
	Removed []string `json:"removed"`
	// Bytes is their total size.
	Bytes int64 `json:"bytes"`
}

// Clear removes the cache of the project, from the user cache and from the
// project directory, whichever exist.
func Clear(_ context.Context, o Options) (Cleared, error) {
	if o.Root == "" {
		return Cleared{}, oops.Errorf("clear the index cache: no project root")
	}
	dirs := []string{}
	if userDir, err := o.userDir(); err == nil {
		dirs = append(dirs, userDir)
	}
	dirs = append(dirs, o.repoDir())
	return remove(dirs)
}

// ClearAll removes the user cache of every project.
func ClearAll(_ context.Context, o Options) (Cleared, error) {
	userDir, err := o.userDir()
	if err != nil {
		return Cleared{}, err
	}
	return remove([]string{filepath.Dir(userDir)})
}

func remove(dirs []string) (Cleared, error) {
	out := Cleared{Removed: []string{}}
	for _, dir := range dirs {
		if !exists(dir) {
			continue
		}
		size := dirSize(dir)
		if err := os.RemoveAll(dir); err != nil {
			return out, oops.With("dir", dir).Wrapf(err, "remove the index cache")
		}
		out.Removed = append(out.Removed, dir)
		out.Bytes += size
	}
	return out, nil
}
