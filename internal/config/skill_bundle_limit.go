package config

import (
	"context"
	"fmt"
	"sync"

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

// One installed_skills entry may resolve many skills; all of them together stay
// under these caps (the same 64 MiB skillsource allows one source).
const (
	maxEntryBundleFiles = 20000
	maxEntryBundleBytes = 64 << 20
)

// SkillBundleLimitError is returned when an installed skill bundles more than
// the per-skill file or byte cap, or an entry's skills more than the entry cap.
type SkillBundleLimitError struct {
	Dir   string
	Limit string
	// Entry is set when Limit is the cap on all the skills of one entry.
	Entry bool
}

func (e *SkillBundleLimitError) Error() string {
	if e.Entry {
		return fmt.Sprintf("the skills of one installed_skills entry bundle more than %s in total (reached at %s); "+
			"select fewer skills with the entry's `include`/`exclude`, or narrow what each bundles with `resources`", e.Limit, e.Dir)
	}
	return fmt.Sprintf("installed skill %s bundles more than %s; narrow it with the entry's `resources` patterns "+
		"(for example [\"*\", \"!docs\", \"!evals\", \"!agents\", \"!.*\"])", e.Dir, e.Limit)
}

type skillBundleBudgetKey struct{}

// skillBundleBudget counts what the skills of one installed_skills entry have
// bundled so far.
type skillBundleBudget struct {
	mu    sync.Mutex
	files int
	bytes int
}

// WithSkillBundleBudget starts the running total of one installed_skills
// entry: every skill scanned with the returned context counts against the same
// aggregate file and byte cap.
func WithSkillBundleBudget(ctx context.Context) context.Context {
	return context.WithValue(ctx, skillBundleBudgetKey{}, &skillBundleBudget{})
}

func skillBundleBudgetFrom(ctx context.Context) *skillBundleBudget {
	if b, ok := ctx.Value(skillBundleBudgetKey{}).(*skillBundleBudget); ok {
		return b
	}
	return nil
}

// charge adds one file and fails once the entry passes a cap.
func (b *skillBundleBudget) charge(skillDir string, size int) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.files++
	b.bytes += size
	switch {
	case b.files > maxEntryBundleFiles:
		return oops.With("path", skillDir).Wrap(&SkillBundleLimitError{Dir: skillDir, Limit: fmt.Sprintf("%d files", maxEntryBundleFiles), Entry: true})
	case b.bytes > maxEntryBundleBytes:
		return oops.With("path", skillDir).Wrap(&SkillBundleLimitError{Dir: skillDir, Limit: fmt.Sprintf("%d MiB", maxEntryBundleBytes>>20), Entry: true})
	}
	return nil
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
	if s.budget != nil {
		if err := s.budget.charge(skillDir, size); err != nil {
			return err
		}
	}
	switch {
	case s.bundledFiles > maxSkillBundleFiles:
		return oops.With("path", skillDir).Wrap(&SkillBundleLimitError{Dir: skillDir, Limit: fmt.Sprintf("%d files", maxSkillBundleFiles)})
	case s.bundledBytes > maxSkillBundleBytes:
		return oops.With("path", skillDir).Wrap(&SkillBundleLimitError{Dir: skillDir, Limit: fmt.Sprintf("%d MiB", maxSkillBundleBytes>>20)})
	}
	return nil
}
