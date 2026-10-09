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
func lockSubjectAt(path string) error {
	cfg, _, err := loadForLockCheck(path)
	if err != nil {
		return fail(err)
	}
	lock, err := lockfile.Load(cfg.ConfigDir)
	if err != nil {
		return fail(err)
	}
	if lock == nil || !lock.HasContentPins() {
		return fail(oops.Hint("Run `ai-rulez lock` first").Errorf("%s has no content pins to sign", lockfile.FileName))
	}
	subject := contentlock.SubjectOf(lock)
	if lock.Tree != subject.Tree {
		return failWithCode(exitFindings, oops.Hint("Run `ai-rulez lock` to rewrite it").Errorf("the tree digest of %s does not match its entries (edited by hand?); refusing to compute a subject", lockfile.FileName))
	}
	statement := subject.Statement()
	data, err := statement.JSON()
	if err != nil {
		return fail(err)
	}
	if lockSubjectOutput != "" {
		if err := os.WriteFile(lockSubjectOutput, data, 0o644); err != nil { //nolint:gosec // a public statement, signed next to the lock
			return fail(oops.With("path", lockSubjectOutput).Wrapf(err, "write lock subject"))
		}
		fmt.Fprintf(os.Stderr, "wrote %s\n", lockSubjectOutput)
		if lockFormat != formatJSON {
			fmt.Println(statement.Text())
		}
		return nil
	}
	if lockFormat == formatJSON {
		_, err = os.Stdout.Write(data)
	} else {
		_, err = fmt.Println(statement.Text())
	}
	if err != nil {
		return fail(oops.Wrapf(err, "write lock subject"))
	}
	return nil
}
