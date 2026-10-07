package commands

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/approval"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/includes"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/lockrun"
	"github.com/Goldziher/ai-rulez/v5/internal/safefs"
)

// approveMaxFileSize bounds what `approve` reads of one file (lockrun.MaxFileSize).
const approveMaxFileSize = lockrun.MaxFileSize

// approvedFile is one file of the content being approved.
type approvedFile = lockrun.File

// subjectFiles lists the files behind a subject, for display and scanning. note
// explains what could not be listed (content declared in config.toml, a tree
// that is not cached, a limit that was hit). It never follows a symlink.
func subjectFiles(cfg *config.Config, s approval.Subject) (files []approvedFile, note string) {
	switch s.Kind {
	case approval.KindInclude, approval.KindInstalledSkill:
		want := lockfile.KindInclude
		if s.Kind == approval.KindInstalledSkill {
			want = lockfile.KindSkill
		}
		wants := includes.Lockable(cfg)
		for i := range wants {
			w := &wants[i]
			if w.Kind == want && w.Name == s.ID {
				if dir := includes.CachedTreeDir(cfg, *w); dir != "" {
					return walkFiles(dir)
				}
			}
		}
		return nil, "the fetched tree is not in the local cache: only the digest is shown (run `ai-rulez generate` to fetch it)"
	case approval.KindSource, approval.KindServed:
		return nil, "only the digest is shown for this kind"
	case approval.KindRoleOutput:
		return roleOutputFiles(cfg, s.ID)
	}
	if s.Path == "" {
		return nil, "declared in config.toml: no files to list"
	}
	abs := filepath.Join(cfg.ConfigDir, filepath.FromSlash(s.Path))
	info, err := os.Lstat(abs)
	switch {
	case err != nil:
		return nil, "the source path cannot be read: " + err.Error()
	case info.IsDir():
		return walkFiles(abs)
	case info.Mode().IsRegular():
		return walkFiles(filepath.Dir(abs), filepath.Base(abs))
	}
	return nil, "the source is not a regular file"
}

// walkFiles lists the regular files below dir (see lockrun.WalkFiles).
func walkFiles(dir string, only ...string) (files []approvedFile, note string) {
	return lockrun.WalkFiles(dir, only...)
}

// scanApproved runs the security scan over the files (see lockrun.ScanFiles).
func scanApproved(cfg *config.Config, name string, files []approvedFile) []lint.Finding {
	return lockrun.ScanFiles(cfg, name, files)
}

// safeText makes untrusted text safe to print (see lockrun.SafeText).
func safeText(s string) string { return lockrun.SafeText(s) }

// gitUserEmail reads user.email from the git configuration files that apply to
// dir: the repository's, then the global ones. It reads them directly instead of
// running git, and returns "" when none sets it.
func gitUserEmail(dir string) string {
	var candidates []string
	for d := dir; d != "" && d != filepath.Dir(d); d = filepath.Dir(d) {
		if info, err := os.Stat(filepath.Join(d, ".git")); err == nil && info.IsDir() {
			candidates = append(candidates, filepath.Join(d, ".git", "config"))
			break
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates, filepath.Join(home, ".gitconfig"), filepath.Join(home, ".config", "git", "config"))
	}
	for _, path := range candidates {
		if email := emailFromGitConfig(path); email != "" {
			return email
		}
	}
	return ""
}

func emailFromGitConfig(path string) string {
	data, err := safefs.OpenRegular(path)
	if err != nil {
		return ""
	}
	defer data.Close() //nolint:errcheck // read-only
	inUser := false
	sc := bufio.NewScanner(data)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		switch {
		case strings.HasPrefix(line, "["):
			inUser = strings.EqualFold(strings.TrimSpace(strings.Trim(line, "[]")), "user")
		case inUser:
			if key, value, ok := strings.Cut(line, "="); ok && strings.EqualFold(strings.TrimSpace(key), "email") {
				return strings.Trim(strings.TrimSpace(value), `"`)
			}
		}
	}
	return ""
}
