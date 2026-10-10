package schema_test

import (
	"testing"

	"github.com/Goldziher/ai-rulez/v5/schema"
	"github.com/stretchr/testify/require"
)

func TestSchemaAcceptsPiPluginAndPublishRuntime(t *testing.T) {
	require.NoError(t, schema.ValidateWithSchema([]byte(`{
		"version":"5.0", "name":"demo",
		"plugin":{"name":"demo","description":"Demo","version":"1.0.0","runtimes":["pi"]},
		"publish":{"runtimes":["pi"]},
		"marketplace":{"name":"demo","from_domains":{"runtimes":["pi"]}}
	}`)))
}
