// Package indexcache keeps what a repository index costs to compute between
// runs of `mcp --serve-skills --discover-skills`: the summary of each indexed
// file, keyed by path, size and modification time, and the verdict of the
// security scan, keyed by a hash of the scanned content. It never stores a file
// body.
//
// The cache lives in the user cache directory by default, or in a gitignored
// directory of the project. Its index file is written with mode 0600 through
// write-and-rename under a lock file, so several servers can share it, and it
// carries a keyed MAC: a file that was edited, truncated or written by another
// version is discarded and rebuilt. A cache entry is a hint for the work that
// follows it, never a decision: the allow and deny rules and the scan of the
// content read at load time run regardless.
package indexcache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
)

// The kinds of location a cache can be in.
const (
	KindUser = "user"
	KindRepo = "repo"
	KindOff  = "off"
)

const (
	// FormatVersion is the version of the index file; another one is discarded.
	FormatVersion = 5

	dirName    = "monorepo-index"
	indexFile  = "index.json"
	lockFile   = "lock"
	repoCache  = ".cache"
	rootCache  = ".ai-rulez-cache"
	repoIDSize = 16

	dirMode = 0o700
)

// Options say which cache to open.
type Options struct {
	// Host supplies the home directory, the clock and git's runner.
	Host ambient.Host
	// Root is the absolute project root.
	Root string
	// ConfigDir is the absolute configuration directory; empty or equal to Root
	// for the root layout, which has no directory of its own.
	ConfigDir string
	// CacheDir, when set, replaces the user cache root (<home>/.cache/ai-rulez).
	CacheDir string
	// Mode is [mcp] index_cache: user (the default), repo or off.
	Mode string
	// Key authenticates the index file (HMAC-SHA256). It is a per-user secret
	// kept outside the cache directory, so what is planted or edited there fails
	// the check. Without one there is no cache.
	Key []byte
	// Fingerprint identifies the scanner and its settings; verdicts of another
	// fingerprint are dropped.
	Fingerprint string
	// Tree identifies the git tree state (the HEAD tree); entries of another
	// tree are dropped. Empty outside a repository.
	Tree string
}

// Location says where a cache is kept.
type Location struct {
	// Kind is KindUser, KindRepo or KindOff.
	Kind string `json:"kind"`
	// Dir is the cache directory; empty when the cache is off.
	Dir string `json:"dir,omitempty"`
	// Fallback is true when the user cache was wanted but the project directory
	// is used because the user cache is not writable.
	Fallback bool `json:"fallback,omitempty"`
	// Warning says why the cache is not where it was asked to be, or off.
	Warning string `json:"warning,omitempty"`
}

// RepoID is the identifier of a project in the user cache: a hash of its
// canonical root.
func RepoID(root string) string {
	canon := filepath.Clean(root)
	if resolved, err := filepath.EvalSymlinks(canon); err == nil {
		canon = resolved
	}
	sum := sha256.Sum256([]byte(canon))
	return hex.EncodeToString(sum[:])[:repoIDSize]
}

// userDir is the directory a user cache of the project would use.
func (o Options) userDir() (string, error) {
	if o.CacheDir != "" {
		return filepath.Join(o.CacheDir, dirName, RepoID(o.Root)), nil
	}
	return config.CacheDirIn(o.Host.Env, dirName, RepoID(o.Root)) //nolint:wrapcheck // names what is missing
}

// repoDir is the directory a project cache would use.
func (o Options) repoDir() string {
	if o.rootLayout() {
		return filepath.Join(o.Root, rootCache, dirName)
	}
	return filepath.Join(o.ConfigDir, repoCache, dirName)
}

// rootLayout reports whether the project keeps no configuration directory.
func (o Options) rootLayout() bool {
	return o.ConfigDir == "" || filepath.Clean(o.ConfigDir) == filepath.Clean(o.Root)
}

// mode is the validated mode, user for an empty or unknown one.
func (o Options) mode() string {
	mode, err := config.ParseIndexCache(o.Mode)
	if err != nil {
		return config.IndexCacheUser
	}
	return mode
}

// Locate decides where the cache is and creates the directory. In user mode it
// falls back to the project when the user cache is not writable; when neither
// is, the cache is off and the Location says why.
func Locate(ctx context.Context, o Options) (Location, error) {
	if o.Root == "" {
		return Location{}, oops.Errorf("locate the index cache: no project root")
	}
	mode := o.mode()
	if mode == config.IndexCacheOff {
		return Location{Kind: KindOff}, nil
	}
	if len(o.Key) == 0 {
		return Location{Kind: KindOff, Warning: "there is no secret to authenticate the index cache; the index cache is off"}, nil
	}
	if mode == config.IndexCacheRepo {
		return o.locateRepo(ctx, false, "")
	}
	dir, err := o.userDir()
	if err == nil {
		if err = prepare(dir); err == nil {
			return Location{Kind: KindUser, Dir: dir}, nil
		}
	}
	return o.locateRepo(ctx, true, "the user cache is not writable ("+err.Error()+")")
}

// locateRepo prepares the project directory.
func (o Options) locateRepo(ctx context.Context, fallback bool, why string) (Location, error) {
	dir := o.repoDir()
	if err := prepare(dir); err != nil {
		reason := "the project cache directory is not writable (" + err.Error() + "); the index cache is off"
		if why != "" {
			reason = why + " and " + reason
		}
		return Location{Kind: KindOff, Warning: reason}, nil
	}
	loc := Location{Kind: KindRepo, Dir: dir, Fallback: fallback}
	if why != "" {
		loc.Warning = why + "; using " + dir
	}
	if err := o.exclude(ctx, dir); err != nil && loc.Warning == "" {
		loc.Warning = "could not exclude " + dir + " from git: " + err.Error()
	}
	return loc, nil
}

// prepare creates dir and checks that a file can be created in it.
func prepare(dir string) error {
	if err := os.MkdirAll(dir, dirMode); err != nil {
		return err //nolint:wrapcheck // reported in the location's warning
	}
	probe, err := os.CreateTemp(dir, ".probe-*")
	if err != nil {
		return err //nolint:wrapcheck // reported in the location's warning
	}
	name := probe.Name()
	closeErr := probe.Close()
	removeErr := os.Remove(name)
	if closeErr != nil {
		return closeErr //nolint:wrapcheck // reported in the location's warning
	}
	return removeErr //nolint:wrapcheck // reported in the location's warning
}

// exclude lists the project cache directory in .git/info/exclude so the working
// tree stays clean. Outside a repository there is nothing to do.
func (o Options) exclude(ctx context.Context, dir string) error {
	git := gitutil.New(o.Host.Runner).WithLog(o.Host.Log)
	exclude := git.InfoExcludePathContext(ctx, o.Root)
	if exclude == "" {
		return nil
	}
	top := git.TopLevelContext(ctx, o.Root)
	base := filepath.Dir(dir) // the .cache or .ai-rulez-cache directory
	rel := gitutil.RepoRelative(top, base)
	if rel == "" {
		return nil
	}
	line := "/" + strings.Trim(rel, "/") + "/"
	if data, err := os.ReadFile(exclude); err == nil && strings.Contains("\n"+strings.ReplaceAll(string(data), "\r\n", "\n"), "\n"+line+"\n") {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(exclude), 0o755); err != nil {
		return oops.With("path", exclude).Wrapf(err, "create git info directory")
	}
	f, err := os.OpenFile(exclude, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644) //nolint:gosec // git's own exclude file
	if err != nil {
		return oops.With("path", exclude).Wrapf(err, "open git exclude file")
	}
	_, werr := f.WriteString(line + "\n")
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	return oops.With("path", exclude).Wrapf(werr, "write git exclude file")
}

// TreeState is the identifier of the git tree state of the project: the HEAD
// tree. It is empty outside a repository or in one without a commit.
func TreeState(ctx context.Context, host ambient.Host, root string) string {
	out, err := gitutil.New(host.Runner).WithLog(host.Log).Output(ctx, root, "rev-parse", "--verify", "--quiet", "HEAD^{tree}")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// now is the host's time.
func (o Options) now() time.Time { return o.Host.Now() }
