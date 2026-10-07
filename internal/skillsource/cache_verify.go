package skillsource

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/samber/oops"
)

// A cached tree lives at <cache>/<url key>/<commit>/<tree dir>. Next to it, never
// inside it (so the tree digest does not see it), sits <tree dir>.digest: the
// digest the tree had when it was stored, which is how an unlocked commit-SHA
// source is verified on later uses.

const (
	legacyTreeName = "tree"
	sidecarSuffix  = ".digest"
	sidecarVersion = 1
)

var (
	errNoSidecar        = errors.New("the cached tree has no digest record")
	errSidecarUntrusted = errors.New("the digest record of the cached tree is unreadable, for another commit or writable by others")
)

// sidecar is the content of <tree dir>.digest.
type sidecar struct {
	Version int    `json:"version"`
	Commit  string `json:"commit"`
	Digest  string `json:"digest"`
}

// treeDirFor is the cache directory of the tree of a commit for a source path.
// A source with a path caches only that subtree (a sparse checkout), so each
// path has a directory of its own. An existing shared "tree" directory, which
// older releases wrote for every path, is still used.
func treeDirFor(repoDir, commit, srcPath string) string {
	base := filepath.Join(repoDir, commit)
	p := cleanSrcPath(srcPath)
	if p == "" {
		return filepath.Join(base, legacyTreeName)
	}
	sum := sha256.Sum256([]byte(p))
	own := filepath.Join(base, legacyTreeName+"-"+hex.EncodeToString(sum[:])[:12])
	if _, err := os.Stat(own); err != nil {
		if _, legacyErr := os.Stat(filepath.Join(base, legacyTreeName)); legacyErr == nil {
			return filepath.Join(base, legacyTreeName)
		}
	}
	return own
}

// cleanSrcPath normalizes a source path to a slash-separated relative path;
// it is empty for the repository root.
func cleanSrcPath(p string) string {
	p = path.Clean(strings.Trim(filepath.ToSlash(p), "/"))
	if p == "." {
		return ""
	}
	return p
}

func sidecarPath(treeDir string) string { return treeDir + sidecarSuffix }

// storeDigest records the digest of a freshly stored tree. The write is atomic
// (a private temp file renamed into place), so a reader never sees half a record.
// A failure only costs a re-fetch on the next use, so it is logged, not returned.
func storeDigest(ctx context.Context, treeDir, commit, digest string) {
	data, err := json.Marshal(sidecar{Version: sidecarVersion, Commit: commit, Digest: digest})
	if err != nil {
		return
	}
	tmp, err := os.CreateTemp(filepath.Dir(treeDir), "digest-*.tmp")
	if err != nil {
		logger.FromContext(ctx).Warn("Could not record the digest of a cached skill source", "error", err.Error())
		return
	}
	name := tmp.Name()
	_, werr := tmp.Write(data)
	if cerr := tmp.Close(); werr == nil {
		werr = cerr
	}
	if werr == nil {
		werr = os.Chmod(name, 0o600)
	}
	if werr == nil {
		werr = os.Rename(name, sidecarPath(treeDir))
	}
	if werr != nil {
		_ = os.Remove(name) //nolint:errcheck // best-effort cleanup
		logger.FromContext(ctx).Warn("Could not record the digest of a cached skill source", "error", werr.Error())
	}
}

// checkDigest compares the digest of a cached tree with its record. It returns
// nil when they agree, errNoSidecar when there is no record, errSidecarUntrusted
// when the record cannot be relied on and an errDigest error on a mismatch.
func checkDigest(treeDir, commit, digest string) error {
	file := sidecarPath(treeDir)
	info, err := os.Lstat(file)
	if err != nil {
		if os.IsNotExist(err) {
			return errNoSidecar
		}
		return errors.Join(errSidecarUntrusted, err)
	}
	if !info.Mode().IsRegular() || (runtime.GOOS != "windows" && info.Mode().Perm()&0o022 != 0) {
		return errSidecarUntrusted
	}
	data, err := os.ReadFile(file) //nolint:gosec // a fixed name next to a tree of the private cache
	if err != nil {
		return errors.Join(errSidecarUntrusted, err)
	}
	var rec sidecar
	if json.Unmarshal(data, &rec) != nil || rec.Version != sidecarVersion || rec.Commit != commit {
		return errSidecarUntrusted
	}
	if rec.Digest != digest {
		return oops.Wrapf(errDigest, "the cached tree has digest %s but %s was recorded when it was stored; the files changed after they were fetched", digest, rec.Digest)
	}
	return nil
}

// removeTree deletes a cached tree and its record.
func removeTree(treeDir string) error {
	if err := os.RemoveAll(treeDir); err != nil {
		return oops.With("dir", treeDir).Wrapf(err, "remove a damaged skill source cache entry")
	}
	if err := os.Remove(sidecarPath(treeDir)); err != nil && !os.IsNotExist(err) {
		return oops.With("dir", treeDir).Wrapf(err, "remove the digest record of a skill source cache entry")
	}
	return nil
}
