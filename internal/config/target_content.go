package config

import (
	"context"
	"path/filepath"

	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/Goldziher/ai-rulez/v5/internal/runner"
)

// ContentFileFromText builds the ContentFile of text that belongs to path: the
// name from the file name, the frontmatter split from the body. It reads
// nothing, so a caller can parse a file it already holds (or bytes a generate
// run would write) the way the loader parses the files of a content tree. A
// malformed frontmatter is recorded in the result and never logged.
func ContentFileFromText(path, text string) ContentFile {
	return parseContentFile(logger.Discard(), path, text)
}

// LoadSkillDir loads the skill whose SKILL.md lies in skillDir, with its
// resources, the way a content tree's skills/ directory is scanned. root is the
// project directory: a symlink inside it that stays inside it is followed, any
// other is refused. ok is false when skillDir holds no readable SKILL.md.
func LoadSkillDir(ctx context.Context, root, skillDir string) (skill ContentFile, ok bool) {
	s := newProjectScanner(ctx, osView(root))
	s.git = gitutil.New(runner.FromContext(ctx))
	s.log = logger.FromContext(ctx)
	s.shared = sharedBundleSnapshots(ctx)
	s.quietUnrecognized = true
	return s.skillAt(skillDir, filepath.Base(skillDir), nil)
}

type bundleSnapshotsKey struct{}

// WithBundleSnapshots returns ctx carrying one set of git answers about which
// files below anchor (the repository top) are part of the project. Every
// LoadSkillDir under that context reads them instead of asking git per skill,
// which on a large repository is two processes per skill. Use it for a
// read-only scan: the answers are not refreshed.
func WithBundleSnapshots(ctx context.Context, anchor string) context.Context {
	if sharedBundleSnapshots(ctx) != nil {
		return ctx
	}
	return context.WithValue(ctx, bundleSnapshotsKey{}, &bundleSnapshots{anchor: filepath.Clean(anchor), indexIgnores: true})
}

func sharedBundleSnapshots(ctx context.Context) *bundleSnapshots {
	if b, ok := ctx.Value(bundleSnapshotsKey{}).(*bundleSnapshots); ok {
		return b
	}
	return nil
}

// SeedBundleListing gives the snapshots WithBundleSnapshots put in ctx the
// listing the caller already holds: the slash paths, relative to the anchor, of
// the tracked files and the untracked files git does not ignore. It saves the
// second `git ls-files` over the whole repository.
func SeedBundleListing(ctx context.Context, files []string) {
	b := sharedBundleSnapshots(ctx)
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.wide == nil {
		b.wide = &wideListing{ok: true, paths: files}
	}
}
