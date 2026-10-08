package commands

import (
	"fmt"
	"os"

	"github.com/Goldziher/ai-rulez/v5/internal/contentlock"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/samber/oops"
)

// lockSubjectAt prints or writes the lock-subject statement of the lock at path
// (see docs/lockfile.md, "Signing the lock"). It reads the lock only: no
// network, no sources. The tree is recomputed from the entries and must match
// the stored one, so a hand-edited lock is refused instead of signed.
func lockSubjectAt(path string) int {
	cfg, _, err := loadForLockCheck(path)
	if err != nil {
		renderError(os.Stderr, err)
		return 1
	}
	lock, err := lockfile.Load(cfg.ConfigDir)
	if err != nil {
		renderError(os.Stderr, err)
		return 1
	}
	if lock == nil || !lock.HasContentPins() {
		renderError(os.Stderr, oops.Hint("Run `ai-rulez lock` first").Errorf("%s has no content pins to sign", lockfile.FileName))
		return 1
	}
	subject := contentlock.SubjectOf(lock)
	if lock.Tree != subject.Tree {
		renderError(os.Stderr, oops.Hint("Run `ai-rulez lock` to rewrite it").Errorf("the tree digest of %s does not match its entries (edited by hand?); refusing to compute a subject", lockfile.FileName))
		return exitDrift
	}
	statement := subject.Statement()
	data, err := statement.JSON()
	if err != nil {
		renderError(os.Stderr, err)
		return 1
	}
	if lockSubjectOutput != "" {
		if err := os.WriteFile(lockSubjectOutput, data, 0o644); err != nil { //nolint:gosec // a public statement, signed next to the lock
			renderError(os.Stderr, oops.With("path", lockSubjectOutput).Wrapf(err, "write lock subject"))
			return 1
		}
		fmt.Fprintf(os.Stderr, "wrote %s\n", lockSubjectOutput)
		if lockFormat != formatJSON {
			fmt.Println(statement.Text())
		}
		return 0
	}
	if lockFormat == formatJSON {
		_, err = os.Stdout.Write(data)
	} else {
		_, err = fmt.Println(statement.Text())
	}
	if err != nil {
		renderError(os.Stderr, oops.Wrapf(err, "write lock subject"))
		return 1
	}
	return 0
}
