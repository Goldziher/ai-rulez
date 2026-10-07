package commands

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
)

const pluginMemberConfig = `version = "5.0"
name = "crawl"

[plugin]
name = "crawl"
version = "1.0.0"
description = "Crawl sites"

[plugin.author]
name = "Acme"

[plugin.interface]
display_name = "Crawl"
short_description = "Crawl sites"
long_description = "Crawl sites to markdown."
developer_name = "Acme"
category = "web-scraping"
capabilities = ["Read"]
default_prompt = ["Crawl this site"]
website_url = "https://example.com"
privacy_policy_url = "https://example.com/privacy"
terms_of_service_url = "https://example.com/terms"
`

// TestLock_RecursiveLocksAMarketplacePluginMember is the C3 corpus finding: a
// marketplace monorepo whose plugin member configures no preset (its skills ship
// in the bundles `generate --plugin` writes). `lock -r` must pin the member's
// content without trying to render its skills for the MCP server.
func TestLock_RecursiveLocksAMarketplacePluginMember(t *testing.T) {
	// Arrange
	root := lockProject(t, "\n[marketplace]\nname = \"mk\"\nmembers = [\"plugin\"]\n\n[marketplace.owner]\nname = \"Acme\"\n")
	member := filepath.Join(root, "plugin", ".ai-rulez")
	writeFile(t, filepath.Join(member, "config.toml"), pluginMemberConfig)
	writeFile(t, filepath.Join(member, "skills", "crawl", "SKILL.md"), "---\nname: crawl\ndescription: Crawl a site. Use when asked to crawl.\n---\nCrawl.\n")
	lockRecursive = true

	// Act
	var code int
	_, stderr := capture(t, func() { code = runLockFor("", nil) })

	// Assert
	require.Equal(t, 0, code, stderr)
	lock, err := lockfile.Load(member)
	require.NoError(t, err)
	require.NotNil(t, lock, "the member is locked")
	pinned := false
	for _, it := range lock.Item {
		pinned = pinned || (it.Kind == "skill" && it.ID == "crawl")
	}
	assert.True(t, pinned, "the member's skill is pinned")
	assert.Empty(t, lock.Served, "nothing is served from a plugin member")

	lockCheck = true
	_, stderr = capture(t, func() { code = runLockFor("", nil) })
	assert.Equal(t, 0, code, stderr)
}
