package policy

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/pluginimport/plugintest"
)

func TestMarketplaceNamedRepositoryWithCredentialsIsRefusedBeforeThePolicyIsAsked(t *testing.T) {
	// Arrange: user@ in front of a denied host is the classic way to make the URL look trusted
	market := plugintest.NewRepo(t, plugintest.ClaudeMarketplace("acme", `{"name":"ext","source":{"source":"url","url":"https://github.com@evil.example/o/p.git"}}`))
	project := plugintest.Project(t, "[[installed_marketplaces]]\nname = \"acme\"\nsource = \""+market.URL()+"\"\n\n[[installed_plugins]]\nname = \"ext\"\nmarketplace = \"acme\"\n")

	// Act
	_, err := loadWithPolicy(t, project, "policy_version = 1\n[sources]\ndeny_hosts = [\"evil.example\"]\n")

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must not embed credentials")
}
