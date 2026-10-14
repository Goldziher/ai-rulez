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
	return s.skillAt(skillDir, filepath.Base(skillDir), nil)
}
