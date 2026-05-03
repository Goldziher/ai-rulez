package includes

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Goldziher/ai-rulez/internal/logger"
	"github.com/samber/oops"
	"github.com/zeebo/blake3"
)

const refHead = "HEAD"

var (
	gitCheckOnce sync.Once
	gitCheckErr  error
)

// requireGit checks that git is in PATH and version >= 2.25.
// Result is cached for the process lifetime.
func requireGit(ctx context.Context) error {
	gitCheckOnce.Do(func() {
		cmd := exec.CommandContext(ctx, "git", "version")
		out, err := cmd.Output()
		if err != nil {
			gitCheckErr = oops.Wrapf(err, "git not found in PATH")
			return
		}
		gitCheckErr = checkVersion225(strings.TrimSpace(string(out)))
	})
	return gitCheckErr
}

var gitVersionRe = regexp.MustCompile(`git version (\d+)\.(\d+)`)

// checkVersion225 parses a "git version X.Y.Z ..." line.
// Returns nil if version >= 2.25.0, error otherwise.
func checkVersion225(versionLine string) error {
	m := gitVersionRe.FindStringSubmatch(versionLine)
	if m == nil {
		return oops.
			With("line", versionLine).
			Errorf("could not parse git version string")
	}
	major, _ := strconv.Atoi(m[1]) //nolint:errcheck // regex \d+ guarantees numeric input
	minor, _ := strconv.Atoi(m[2]) //nolint:errcheck // regex \d+ guarantees numeric input
	if major > 2 || (major == 2 && minor >= 25) {
		return nil
	}
	return oops.
		With("version", fmt.Sprintf("%d.%d", major, minor)).
		Errorf("git >= 2.25 required for sparse checkout; found %d.%d", major, minor)
}

// injectToken rewrites an HTTPS URL to embed credentials.
// https://host/path → https://token:x-oauth-basic@host/path
// No-op for SSH URLs (git@, ssh://) and file:// URLs and empty token.
func injectToken(rawURL, token string) string {
	if token == "" {
		return rawURL
	}
	if strings.HasPrefix(rawURL, "git@") || strings.HasPrefix(rawURL, "ssh://") || strings.HasPrefix(rawURL, "file://") {
		return rawURL
	}
	for _, scheme := range []string{"https://", "http://"} {
		if strings.HasPrefix(rawURL, scheme) {
			host := strings.TrimPrefix(rawURL, scheme)
			return scheme + token + ":x-oauth-basic@" + host
		}
	}
	return rawURL
}

// remoteHEADSHA returns the commit SHA that <ref> currently points to on the remote.
// ref may be "main", "HEAD", a tag, etc.
// For branches, queries refs/heads/<ref>; for HEAD, queries HEAD directly.
// If the branch lookup returns nothing, falls back to refs/tags/<ref>.
// Token is injected into HTTPS URLs before running git ls-remote.
func remoteHEADSHA(ctx context.Context, repoURL, ref, token string) (string, error) {
	url := injectToken(repoURL, token)

	var refspecs []string
	if ref == "" || ref == refHead {
		refspecs = []string{refHead}
	} else {
		refspecs = []string{"refs/heads/" + ref, "refs/tags/" + ref}
	}

	env := append(os.Environ(), "GIT_TERMINAL_PROMPT=0") //nolint:gocritic

	for i, refspec := range refspecs {
		// nolint: gosec
		cmd := exec.CommandContext(ctx, "git", "ls-remote", url, refspec)
		cmd.Env = env
		out, err := cmd.Output()
		if err != nil {
			return "", oops.
				With("url", repoURL).
				With("ref", ref).
				With("refspec", refspec).
				Wrapf(err, "git ls-remote failed")
		}
		line := strings.TrimSpace(string(out))
		if line == "" {
			// branch lookup returned nothing; try tag fallback on second refspec
			if i == 0 && len(refspecs) > 1 {
				continue
			}
			return "", oops.
				With("url", repoURL).
				With("ref", ref).
				Errorf("ref %q not found on remote", ref)
		}
		sha := strings.SplitN(line, "\t", 2)[0]
		logger.Debug("resolved remote HEAD SHA", "url", repoURL, "ref", ref, "sha", sha)
		return sha, nil
	}

	return "", oops.
		With("url", repoURL).
		With("ref", ref).
		Errorf("ref %q not found on remote", ref)
}

// sparseClone clones only pathSpec from the remote into destDir.
//
// Runs:
//
//	git clone --depth 1 --filter=blob:none --sparse [--branch <ref>] <url> <destDir>
//	git -C <destDir> sparse-checkout set <pathSpec>   (omitted when pathSpec == "")
//
// destDir must not exist when this is called (caller does RemoveAll+MkdirAll first
// so the dir exists but is empty — that is fine; git clone into an empty dir works).
// Token is injected into HTTPS URLs.
// On any error, destDir is cleaned up before returning.
func sparseClone(ctx context.Context, repoURL, ref, pathSpec, destDir, token string) error {
	url := injectToken(repoURL, token)
	env := append(os.Environ(), "GIT_TERMINAL_PROMPT=0") //nolint:gocritic

	cloneArgs := []string{"clone", "--depth", "1", "--filter=blob:none", "--sparse"}
	if ref != "" && ref != refHead {
		cloneArgs = append(cloneArgs, "--branch", ref)
	}
	cloneArgs = append(cloneArgs, url, destDir)

	// nolint: gosec
	cloneCmd := exec.CommandContext(ctx, "git", cloneArgs...)
	cloneCmd.Env = env
	if out, err := cloneCmd.CombinedOutput(); err != nil {
		_ = os.RemoveAll(destDir) //nolint:errcheck // best-effort cleanup on clone failure
		return oops.
			With("url", repoURL).
			With("ref", ref).
			With("output", string(out)).
			Wrapf(err, "git sparse clone failed")
	}

	if pathSpec == "" {
		return nil
	}

	// nolint: gosec
	checkoutCmd := exec.CommandContext(ctx, "git", "-C", destDir, "sparse-checkout", "set", pathSpec)
	checkoutCmd.Env = env
	if out, err := checkoutCmd.CombinedOutput(); err != nil {
		_ = os.RemoveAll(destDir) //nolint:errcheck // best-effort cleanup on sparse-checkout failure
		return oops.
			With("url", repoURL).
			With("path_spec", pathSpec).
			With("output", string(out)).
			Wrapf(err, "git sparse-checkout set failed")
	}

	return nil
}

const cacheMetaFile = ".cache_meta.json"

// CacheMeta is persisted alongside cached content and drives cache invalidation.
type CacheMeta struct {
	RemoteHEADSHA string            `json:"remote_head_sha"`
	FetchedAt     time.Time         `json:"fetched_at"`
	FileHashes    map[string]string `json:"file_hashes"` // relPath → "blake3:<hex>"
}

// readCacheMeta reads .cache_meta.json from cacheDir.
// Returns nil, nil when the file does not exist (treated as cache miss by callers).
func readCacheMeta(cacheDir string) (*CacheMeta, error) {
	data, err := os.ReadFile(filepath.Join(cacheDir, cacheMetaFile))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, oops.
			With("cache_dir", cacheDir).
			Wrapf(err, "read cache meta")
	}
	var meta CacheMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, oops.
			With("cache_dir", cacheDir).
			Wrapf(err, "unmarshal cache meta")
	}
	return &meta, nil
}

// writeCacheMeta writes meta atomically (temp file + rename) to cacheDir.
func writeCacheMeta(cacheDir string, meta *CacheMeta) error {
	data, err := json.Marshal(meta)
	if err != nil {
		return oops.
			With("cache_dir", cacheDir).
			Wrapf(err, "marshal cache meta")
	}
	tmp := filepath.Join(cacheDir, cacheMetaFile+".tmp")
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return oops.
			With("cache_dir", cacheDir).
			Wrapf(err, "write cache meta temp file")
	}
	if err := os.Rename(tmp, filepath.Join(cacheDir, cacheMetaFile)); err != nil {
		return oops.
			With("cache_dir", cacheDir).
			Wrapf(err, "rename cache meta temp file")
	}
	return nil
}

// isCacheHit returns true when meta exists and its RemoteHEADSHA matches currentSHA.
func isCacheHit(cacheDir, currentSHA string) bool {
	meta, err := readCacheMeta(cacheDir)
	if err != nil || meta == nil {
		return false
	}
	return meta.RemoteHEADSHA == currentSHA
}

// computeFileHashes walks dir and returns relPath → "blake3:<hex>" for every
// regular non-hidden file, excluding cacheMetaFile itself.
func computeFileHashes(dir string) (map[string]string, error) {
	hashes := make(map[string]string)
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		name := d.Name()
		if strings.HasPrefix(name, ".") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if name == cacheMetaFile {
			return nil
		}
		content, err := os.ReadFile(path) //nolint:gosec // path comes from WalkDir; root is a trusted cache directory
		if err != nil {
			return oops.
				With("path", path).
				Wrapf(err, "read file for hashing")
		}
		sum := blake3.Sum256(content)
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return oops.
				With("path", path).
				Wrapf(err, "compute relative path")
		}
		hashes[rel] = "blake3:" + hex.EncodeToString(sum[:])
		return nil
	})
	if err != nil {
		return nil, oops.
			With("dir", dir).
			Wrapf(err, "walk directory for file hashes")
	}
	return hashes, nil
}
