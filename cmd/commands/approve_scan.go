package commands

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/Goldziher/ai-rulez/v5/internal/approval"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/includes"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/safefs"
)

// Bounds of what `approve` reads of one subject: a tree larger than this is not
// shown or scanned in full, and the reviewer is told so.
const (
	approveMaxFiles    = 2000
	approveMaxFileSize = lint.MaxServedScanBytes
)

// approvedFile is one file of the content being approved.
type approvedFile struct {
	Path       string
	Size       int64
	Executable bool
	Data       []byte // nil when the file was too large or unreadable to scan
}

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

// walkFiles lists the regular files below dir (only the named ones when only is
// given), skipping symlinks and VCS and cache bookkeeping.
func walkFiles(dir string, only ...string) (files []approvedFile, note string) {
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(dir, path)
		if relErr != nil || rel == "." {
			return nil //nolint:nilerr // the root itself
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() || rel == ".cache_meta.json" || (len(only) > 0 && rel != only[0]) {
			return nil
		}
		if len(files) >= approveMaxFiles {
			note = fmt.Sprintf("more than %d files: the rest are not listed or scanned", approveMaxFiles)
			return filepath.SkipAll
		}
		info, statErr := d.Info()
		if statErr != nil {
			return nil //nolint:nilerr // listed as unreadable below
		}
		f := approvedFile{Path: filepath.ToSlash(rel), Size: info.Size(), Executable: info.Mode().Perm()&0o100 != 0}
		if info.Size() <= approveMaxFileSize {
			if data, readErr := safefs.ReadRegular(path); readErr == nil {
				f.Data = data
			}
		}
		files = append(files, f)
		return nil
	})
	if err != nil {
		note = "the tree cannot be read completely: " + err.Error()
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, note
}

// scanApproved runs the security scan over the files and returns the findings,
// errors first. A file with no Data (over the size limit) is not scanned; the
// caller lists it. A binary file is reported by the scan itself.
func scanApproved(cfg *config.Config, name string, files []approvedFile) []lint.Finding {
	var served []lint.ServedFile
	for _, f := range files {
		if f.Data != nil {
			served = append(served, lint.ServedFile{Path: f.Path, Content: f.Data})
		}
	}
	found := lint.ScanServed(cfg, name, served, "")
	sort.SliceStable(found, func(i, j int) bool {
		if (found[i].Severity == lint.SeverityError) != (found[j].Severity == lint.SeverityError) {
			return found[i].Severity == lint.SeverityError
		}
		return found[i].Code < found[j].Code
	})
	return found
}

// safeText makes untrusted text safe to print: control characters (other than
// tab), bidirectional controls, zero-width and tag characters are replaced by
// their \u escape, so content cannot hide text from the reviewer.
func safeText(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '\t':
			b.WriteRune(r)
		case unicode.IsControl(r), r >= 0x200B && r <= 0x200F, r >= 0x202A && r <= 0x202E,
			r >= 0x2060 && r <= 0x2069, r == 0xFEFF, r >= 0xE0000 && r <= 0xE007F:
			fmt.Fprintf(&b, "\\u{%X}", r)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

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
