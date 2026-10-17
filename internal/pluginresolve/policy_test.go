package pluginresolve_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/pluginimport"
	"github.com/Goldziher/ai-rulez/v5/internal/pluginimport/plugintest"
)

// denySource is a policy that refuses one repository.
type denySource struct{ url string }

func (denySource) Enforce(context.Context, *config.Config) (*config.PolicyOutcome, error) {
	return nil, nil
}
func (denySource) Locks(string) bool { return false }
func (d denySource) CheckSource(_ *config.Config, kind, name, source string) error {
	if source == d.url {
		return config.ErrPolicyLoosens
	}
	return nil
}

func TestPolicy_HostRulesApplyToRepositoriesAMarketplaceNames(t *testing.T) {
	// Arrange
	isolate(t)
	plug := plugintest.NewRepo(t, plugintest.ClaudePlugin("ext"))
	market := plugintest.NewRepo(t, marketFiles(pluginimport.IndexClaude, `{"name":"ext","source":{"source":"url","url":"`+plug.URL()+`"}}`))
	project := plugintest.Project(t, "[[installed_marketplaces]]\nname = \"acme\"\nsource = \""+market.URL()+"\"\n\n[[installed_plugins]]\nname = \"ext\"\nmarketplace = \"acme\"\n")

	// Act
	_, denied := load(t, project, config.WithPolicy(denySource{url: plug.URL()}))
	cfg, allowed := load(t, project, config.WithPolicy(denySource{url: "https://elsewhere.example/x.git"}))

	// Assert
	require.ErrorIs(t, denied, config.ErrPolicyLoosens)
	require.NoError(t, allowed)
	assert.Equal(t, []string{"review-ext"}, skillNames(cfg))
}
