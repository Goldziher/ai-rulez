package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/areas"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/exposure"
)

// An areas file excludes a compound name and records acceptances with a reason; an acceptance that accepts
// nothing is a stale acceptance, which fails the report (#537).
func TestRunExposureLeakageAreasFileExcludeAndAccept(t *testing.T) {
	areasBody := `[[area]]
name = "xberg"
terms = ["xberg"]
exclude = ["xberg-io"]

[[accept]]
term = "xberg"
path = "AGENTS.md"
reason = "the product is named in the overview"

[[accept]]
term = "xberg"
path = "gone.md"
reason = "left over"
`
	f := leakageRepo(t, areasBody, map[string]string{
		"CLAUDE.md": "Owned by xberg-io.\n@AGENTS.md\n",
		"AGENTS.md": "xberg overview\n",
	})

	got, err := runExposureLeakage(t.Context(), &f)

	require.NoError(t, err)
	require.Len(t, got.Findings, 1, "xberg-io is excluded; the AGENTS.md occurrence is accepted")
	assert.Equal(t, exposure.LeakAccepted, got.Findings[0].Severity)
	assert.Equal(t, "AGENTS.md", got.Findings[0].Path)
	assert.Equal(t, "the product is named in the overview", got.Findings[0].Reason)
	require.Len(t, got.StaleAcceptances, 1)
	assert.Contains(t, got.StaleAcceptances[0], `"xberg" in gone.md`)
	assert.False(t, got.OK())
}

func TestRunExposureLeakageRejectsAnAcceptWithoutAReason(t *testing.T) {
	f := leakageRepo(t, "[[area]]\nname = \"a\"\nterms = [\"x\"]\n[[accept]]\nterm = \"x\"\npath = \"CLAUDE.md\"\n", map[string]string{"CLAUDE.md": "x\n"})

	_, err := runExposureLeakage(t.Context(), &f)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "reason is required")
}

// An import that is not read is listed as unchecked instead of passing as clean.
func TestRunExposureLeakageListsUnreadImportsAsUnchecked(t *testing.T) {
	f := leakageRepo(t, "[[area]]\nname = \"billing\"\nterms = [\"invoice\"]\n", map[string]string{
		"CLAUDE.md": "@../x.md\n@~/y.md\n",
	})

	got, err := runExposureLeakage(t.Context(), &f)

	require.NoError(t, err)
	assert.Empty(t, got.Findings)
	require.Len(t, got.Unchecked, 2)
	assert.Equal(t, "../x.md", got.Unchecked[0].Path)
	assert.Contains(t, got.Unchecked[0].Reason, "outside the repository root")
	assert.Equal(t, "~/y.md", got.Unchecked[1].Path)
	assert.True(t, got.OK(), "an unchecked file does not change the status, as an unchecked artifact does not")
}

// Acceptance paths are relative to the project root, also for the leaks of an inventory of --root below it.
func TestExposureLintSourceAcceptsByProjectRelativePathBelowTheRoot(t *testing.T) {
	root := t.TempDir()
	for name, body := range map[string]string{
		"services/billing/CLAUDE.md": "Every ranker.\n",
	} {
		p := filepath.Join(root, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o750))
		require.NoError(t, os.WriteFile(p, []byte(body), 0o600))
	}
	areasFile := filepath.Join(t.TempDir(), "areas.toml")
	body := `[[area]]
name = "search"
terms = ["ranker"]

[[accept]]
term = "ranker"
path = "services/billing/CLAUDE.md"
reason = "named on purpose"

[[accept]]
term = "ranker"
path = "gone.md"
reason = "left over"
`
	require.NoError(t, os.WriteFile(areasFile, []byte(body), 0o600))
	m, file := loadAreasFile(t, areasFile)
	cfg := &config.Config{BaseDir: root, Exposure: &config.ExposureConfig{}}
	below := exposureLintSource{ctx: t.Context(), cfg: cfg, dir: filepath.Join(root, "services", "billing"), areas: m, areasFile: file, cache: &exposureCache{}}
	atRoot := exposureLintSource{ctx: t.Context(), cfg: cfg, areas: m, areasFile: file, cache: &exposureCache{}}

	gotBelow, ok := below.Leakage()
	require.True(t, ok)
	gotRoot, ok := atRoot.Leakage()
	require.True(t, ok)

	require.Len(t, gotBelow.Leaks, 1)
	assert.Equal(t, "named on purpose", gotBelow.Leaks[0].AcceptReason, "matched against services/billing/CLAUDE.md")
	assert.Empty(t, gotBelow.StaleAccepts, "stale entries are not judged under a subdirectory root")
	assert.Empty(t, gotRoot.Leaks, "the root CLAUDE.md is absent; no start-of-session text there")
}

func loadAreasFile(t *testing.T, path string) (*areas.Map, string) {
	t.Helper()
	m, problems := areas.LoadFile(path)
	require.Empty(t, problems)
	return m, path
}
