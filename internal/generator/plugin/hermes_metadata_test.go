package plugin

import (
	"testing"

	"github.com/pelletier/go-toml/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHermesPyprojectShouldIncludePublishingMetadata(t *testing.T) {
	m := &Manifest{
		Name: "example", Version: "1.2.3", Description: "Example plugin.",
		License: "MIT", Homepage: "https://example.com", Repository: "https://github.com/example/plugin",
	}
	data, err := hermesPyproject(m, "example_hermes_plugin")
	require.NoError(t, err)
	var document struct {
		Project map[string]any `toml:"project"`
	}
	require.NoError(t, toml.Unmarshal(data, &document))
	assert.Equal(t, "MIT", document.Project["license"])
	assert.Equal(t, map[string]any{
		"text": "# example Hermes plugin\n\nExample plugin.\n\n" +
			"Install this package in the same Python environment as Hermes Agent:\n\n" +
			"```sh\npython -m pip install example-hermes-plugin\n```\n\n" +
			"Enable the plugin through Hermes Agent's plugin configuration. " +
			"Available registrations depend on the plugin's authored Hermes adapter.\n",
		"content-type": "text/markdown",
	}, document.Project["readme"])
	assert.Equal(t, map[string]any{
		"Homepage": "https://example.com", "Repository": "https://github.com/example/plugin",
	}, document.Project["urls"])
}

func TestHermesPyprojectShouldOmitUnconfiguredLicenseAndURLs(t *testing.T) {
	for _, tc := range []struct {
		name       string
		homepage   string
		repository string
		urls       map[string]any
	}{
		{name: "both absent"},
		{
			name: "homepage only", homepage: "https://example.com",
			urls: map[string]any{"Homepage": "https://example.com"},
		},
		{
			name: "repository only", repository: "https://github.com/example/plugin",
			urls: map[string]any{"Repository": "https://github.com/example/plugin"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, err := hermesPyproject(&Manifest{
				Name: "example", Version: "1.2.3", Homepage: tc.homepage, Repository: tc.repository,
			}, "example_hermes_plugin")
			require.NoError(t, err)
			var document struct {
				Project map[string]any `toml:"project"`
			}
			require.NoError(t, toml.Unmarshal(data, &document))
			assert.NotContains(t, document.Project, "license")
			if tc.urls == nil {
				assert.NotContains(t, document.Project, "urls")
			} else {
				assert.Equal(t, tc.urls, document.Project["urls"])
			}
		})
	}
}
