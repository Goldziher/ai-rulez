package crud

import (
	"os"
	"path/filepath"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/okf"
	"github.com/Goldziher/ai-rulez/v5/internal/okfbridge"
)

// concept turns content in the native layout into the OKF concept the operator
// stores. previous is the file's former content on a rewrite, "" on a create.
func (op *OperatorImpl) concept(ftype, domain, name, content, previous string) (string, error) {
	data, err := okfbridge.RenderConceptKeeping(okfbridge.Kind(ftype), domain, name, []byte(content), []byte(previous))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// refreshIndexes rewrites the index.md files after a change to the content tree.
// A tree that is not an OKF bundle yet (no root index.md, see "migrate okf") and
// the machine-local tree, which has no indexes, are left alone.
func (op *OperatorImpl) refreshIndexes() error {
	if op.local {
		return nil
	}
	if info, err := os.Stat(filepath.Join(op.aiRulezDir, okf.IndexFile)); err != nil || info.IsDir() {
		return nil
	}
	if err := okfbridge.RefreshIndexes(op.aiRulezDir); err != nil {
		return oops.Hint("The file was written; run 'ai-rulez migrate okf' to rebuild the index.md files.").Wrapf(err, "refresh index.md files")
	}
	return nil
}
