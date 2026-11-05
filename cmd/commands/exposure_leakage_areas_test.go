package commands

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
