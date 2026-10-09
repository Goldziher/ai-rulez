package commands

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

func migrateOKFRun(t *testing.T, dry, check bool) (int, string) {
	t.Helper()
	oldDry, oldCheck, oldFormat := migrateDryRun, migrateCheck, migrateFormat
	migrateDryRun, migrateCheck, migrateFormat = dry, check, formatText
	t.Cleanup(func() { migrateDryRun, migrateCheck, migrateFormat = oldDry, oldCheck, oldFormat })
	var out bytes.Buffer
	code := runMigrateOKF(context.Background(), &out)
	return code, out.String()
}

func TestMigrateOKFRefusesAReservedRuleNameAndKeepsItsText(t *testing.T) {
	root := okfProject(t)
	rule := filepath.Join(root, ".ai-rulez", "rules", "index.md")
	writeFile(t, rule, "real rule\n")

	code, _ := migrateOKFRun(t, false, false)

	assert.Equal(t, 1, code)
	data, err := os.ReadFile(rule)
	require.NoError(t, err)
	assert.Equal(t, "real rule\n", string(data))
	assert.NoFileExists(t, filepath.Join(root, ".ai-rulez", "rules", "index_.md"))
	assert.NoFileExists(t, filepath.Join(root, ".ai-rulez", "index.md"), "nothing is written when the migration is refused")
}

func TestMigrateOKFBacksUpOriginalsAndFailsOnSkippedSymlink(t *testing.T) {
	root := okfProject(t)
	outside := filepath.Join(t.TempDir(), "secret.md")
	writeFile(t, outside, "# Secret\n")
	testutil.SymlinkOrSkip(t, outside, filepath.Join(root, ".ai-rulez", "rules", "link.md"))

	code, out := migrateOKFRun(t, false, false)

	assert.Equal(t, 1, code, "a skipped symlink must not exit 0: %s", out)
	assert.Contains(t, out, "[skipped] rules/link.md")
	assert.Contains(t, out, "skipped 1")
	backups, err := filepath.Glob(filepath.Join(root, ".ai-rulez.bak-okf-*", "rules", "style.md"))
	require.NoError(t, err)
	assert.Len(t, backups, 1, "the original of a rewritten rule is kept")

	// A dry run only reports.
	code, _ = migrateOKFRun(t, true, false)
	assert.Equal(t, 0, code)
}

func TestMigrateOKFSecondRunIsAQuietSuccess(t *testing.T) {
	okfProject(t)
	code, out := migrateOKFRun(t, false, false)
	require.Equal(t, 0, code, out)
	code, out = migrateOKFRun(t, false, true)
	assert.Equal(t, 0, code, out)
}
