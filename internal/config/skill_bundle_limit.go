package config

import (
	"fmt"

	"github.com/samber/oops"
)

// Total bundle caps for an installed skill, which is untrusted input bundled
// whole by default. The file cap matches skillsource (2000 files per skill) and
// the byte cap sits well under its 64 MiB per-source default. A skill over
// either must be narrowed with the entry's `resources` patterns.
const (
	maxSkillBundleFiles = 2000
	maxSkillBundleBytes = 16 << 20
)

// SkillBundleLimitError is returned when an installed skill bundles more than
// the per-skill file or byte cap.
type SkillBundleLimitError struct {
	Dir   string
	Limit string
}

func (e *SkillBundleLimitError) Error() string {
	return fmt.Sprintf("installed skill %s bundles more than %s; narrow it with the entry's `resources` patterns "+
		"(for example [\"*\", \"!docs\", \"!evals\", \"!agents\", \"!.*\"])", e.Dir, e.Limit)
}

// bundleLimited reports whether the scan is of an installed skill, which gets
// the credential, size and total-cap checks.
func (s *contentScanner) bundleLimited() bool {
	return len(s.extraResources) > 0
}

// countBundled adds one bundled file of size bytes to the running total and
// fails once the skill passes a cap.
func (s *contentScanner) countBundled(skillDir string, size int) error {
	if !s.bundleLimited() {
		return nil
	}
	s.bundledFiles++
	s.bundledBytes += size
	switch {
	case s.bundledFiles > maxSkillBundleFiles:
		return oops.With("path", skillDir).Wrap(&SkillBundleLimitError{Dir: skillDir, Limit: fmt.Sprintf("%d files", maxSkillBundleFiles)})
	case s.bundledBytes > maxSkillBundleBytes:
		return oops.With("path", skillDir).Wrap(&SkillBundleLimitError{Dir: skillDir, Limit: fmt.Sprintf("%d MiB", maxSkillBundleBytes>>20)})
	}
	return nil
}
