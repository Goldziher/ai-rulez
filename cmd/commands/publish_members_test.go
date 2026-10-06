package commands

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/publish"
)

const publishMembersRootConfig = `version = "4.0"
name = "acme-plugins"
description = "Acme plugin collection."
presets = ["claude"]

[marketplace]
name = "acme"
description = "Curated Acme plugins."
members = ["plugins/alpha", "plugins/beta"]

[marketplace.owner]
name = "Acme Inc"
email = "dev@acme.example"
`

func memberConfig(name string, runtimes string) string {
	return `version = "4.0"
name = "` + name + `"
description = "` + name + ` plugin."

[plugin]
name = "` + name + `"
description = "` + name + ` does things."
version = "1.0.0"
repository = "https://github.com/acme/` + name + `"
runtimes = ` + runtimes + `

[plugin.author]
name = "Jane"
`
}

// membersProject is a marketplace of two member projects: alpha ships claude and cursor, beta only claude.
func membersProject(t *testing.T) string {
	t.Helper()
	root := publishProject(t)
	for name, runtimes := range map[string]string{"alpha": `["claude", "cursor"]`, "beta": `["claude"]`} {
		dir := filepath.Join(root, "plugins", name)
		writeFile(t, filepath.Join(dir, ".ai-rulez", "config.toml"), memberConfig(name, runtimes))
		writeFile(t, filepath.Join(dir, ".ai-rulez", "skills", name+"-skill", "SKILL.md"),
			"---\nname: "+name+"-skill\ndescription: Use when working on "+name+" things; not otherwise.\n---\n\n# S\n\nDo "+name+".\n")
	}
	reconfigure(t, root, publishMembersRootConfig)
	return root
}

func memberRuntimes(t *testing.T, dist map[string]string, plugin string) []string {
	t.Helper()
	var m publish.Manifest
	require.NoError(t, json.Unmarshal([]byte(dist["plugins/"+plugin+"/"+plugin+"-1.0.0.manifest.json"]), &m))
	return m.Runtimes
}

func TestPublish_RuntimeFiltersMarketplaceMembers(t *testing.T) {
	// Arrange
	root := membersProject(t)
	_, err := runPublishCapture(t)
	require.NoError(t, err)
	all := readDist(t, filepath.Join(root, "dist"))
	require.Equal(t, []string{"claude", "cursor"}, memberRuntimes(t, all, "alpha"), "without --runtime a member ships its own runtimes")

	// Act
	publishRuntimes = []string{"claude"}
	_, err = runPublishCapture(t)

	// Assert: each member's bundle is limited to the requested runtime, the other runtimes' files are gone
	require.NoError(t, err)
	filtered := readDist(t, filepath.Join(root, "dist"))
	assert.Equal(t, []string{"claude"}, memberRuntimes(t, filtered, "alpha"))
	assert.Equal(t, []string{"claude"}, memberRuntimes(t, filtered, "beta"))
	assert.NotEqual(t, all["plugins/alpha/alpha-1.0.0.tar.gz"], filtered["plugins/alpha/alpha-1.0.0.tar.gz"], "the archive no longer holds the cursor files")
}

func TestPublish_RuntimeRefusesAMemberThatDoesNotShipIt(t *testing.T) {
	// Arrange
	membersProject(t)
	publishRuntimes = []string{"cursor"}

	// Act
	_, err := runPublishCapture(t)

	// Assert: beta cannot ship cursor, and publishing less than the marketplace index lists is never silent
	requirePublishError(t, err, publish.CodeConfig, publish.ExitFailed)
	assert.Contains(t, err.Error(), "plugins/beta")
	assert.Contains(t, err.Error(), "cursor")
}
