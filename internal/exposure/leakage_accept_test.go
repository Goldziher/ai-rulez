package exposure

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/areas"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

func acceptMap(t *testing.T) *areas.Map {
	t.Helper()
	path := filepath.Join(t.TempDir(), "areas.toml")
	require.NoError(t, os.WriteFile(path, []byte(`
[[area]]
name = "search"
terms = ["ranker"]

[[accept]]
term = "ranker"
path = "AGENTS.md"
reason = "the product name"

[[accept]]
term = "ranker"
path = "gone.md"
reason = "stale"
`), 0o600))
	m, problems := areas.LoadFile(path)
	require.Empty(t, problems)
	return m
}

func TestCheckLeaks_RecordedAcceptancesAndStaleEntries(t *testing.T) {
	// Arrange
	root := writeRepo(t, map[string]string{
		"CLAUDE.md": "@AGENTS.md\nthe ranker\n",
		"AGENTS.md": "the ranker\n",
	})

	// Act
	leaks, stale := CheckLeaks(build(t, root).Start(), acceptMap(t))

	// Assert: the AGENTS.md occurrence is accepted with its reason, the CLAUDE.md one is not, and the
	// acceptance of gone.md accepted nothing.
	require.Len(t, leaks, 2)
	byPath := map[string]Leak{}
	for _, l := range leaks {
		byPath[l.Path] = l
	}
	assert.Equal(t, "the product name", byPath["AGENTS.md"].Reason)
	assert.Empty(t, byPath["CLAUDE.md"].Reason)
	require.Len(t, stale, 1)
	assert.Equal(t, "gone.md", stale[0].Path)
}

func TestLeaks_ExcludedCompoundIsNotALeak(t *testing.T) {
	// Arrange
	m, problems := areas.New([]config.ExposureArea{{Name: "xberg", Terms: []string{"xberg"}, Exclude: []string{"xberg-io"}}}, nil)
	require.Empty(t, problems)
	root := writeRepo(t, map[string]string{"CLAUDE.md": "@AGENTS.md\n", "AGENTS.md": "Owned by xberg-io.\n"})
	inv := build(t, root)

	// Act and assert: the org name is no leak; the bare term is.
	assert.Empty(t, Leaks(inv.Start(), m))
	assert.Len(t, Leaks([]Artifact{startArtifact(KindClaudeMD, "CLAUDE.md", "xberg-io and xberg")}, m), 1)
}
