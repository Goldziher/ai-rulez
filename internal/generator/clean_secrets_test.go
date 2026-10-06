package generator

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const cleanMCPShared = `version = "4.0"
name = "clean-secrets"
presets = ["claude", "poolside"]

[[mcp_servers]]
name = "svc"
command = "svc-bin"
[mcp_servers.env]
SVC_TOKEN = "${AR_CLEAN_TEST_TOKEN}"
`

func unsetEnvForTest(t *testing.T, key string) {
	t.Helper()
	old, had := os.LookupEnv(key)
	require.NoError(t, os.Unsetenv(key))
	t.Cleanup(func() {
		if had {
			_ = os.Setenv(key, old)
		}
	})
}

func TestClean_NeedsNoMCPSecrets(t *testing.T) {
	for _, dryRun := range []bool{true, false} {
		name := "removes"
		if dryRun {
			name = "dry run plans"
		}
		t.Run(name, func(t *testing.T) {
			// Arrange
			p := newDriftProject(t, cleanMCPShared)
			t.Setenv("AR_CLEAN_TEST_TOKEN", "tok-123")
			require.NoError(t, NewGenerator(p.load(t)).Generate(""))
			require.True(t, p.exists(".mcp.json"))
			unsetEnvForTest(t, "AR_CLEAN_TEST_TOKEN")

			// Act
			plan, err := NewGenerator(p.load(t)).Clean("", CleanOptions{DryRun: dryRun})

			// Assert
			require.NoError(t, err)
			assert.NotEmpty(t, plan.Files)
			assert.Equal(t, dryRun, p.exists(".mcp.json"))
			assert.Equal(t, dryRun, p.exists("CLAUDE.md"))
			if !dryRun {
				assert.False(t, p.exists(".ai-rulez/"+generatedManifestName))
			}
		})
	}
}

func TestClean_KeepsTrackedLocalManifest(t *testing.T) {
	// Arrange
	p := newDriftProject(t, driftSharedIgnoring)
	p.git(t, "init", "-q")
	require.NoError(t, NewGenerator(p.load(t)).Generate(""))
	rel := ".ai-rulez/.generated-manifest.local.json"
	p.writeFile(t, rel, `{"files":[]}`)
	p.git(t, "add", "-f", rel)

	// Act
	plan, err := NewGenerator(p.load(t)).Clean("", CleanOptions{RemoveEdited: true})

	// Assert
	require.NoError(t, err)
	assert.Empty(t, plan.LocalManifestPath)
	assert.True(t, p.exists(rel), "a git-tracked local manifest must survive clean")
	assert.False(t, strings.Contains(plan.ManifestPath, "local"))
}
