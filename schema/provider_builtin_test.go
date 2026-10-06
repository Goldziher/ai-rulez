package schema_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/kaptinlin/jsonschema"
	"github.com/pelletier/go-toml/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Every builtin provider spec must satisfy schema/provider.schema.json, the
// schema user-defined providers are checked against.
func TestBuiltinProvidersMatchTheProviderSchema(t *testing.T) {
	// Arrange
	schemaBytes, err := os.ReadFile("provider.schema.json")
	require.NoError(t, err)
	compiled, err := jsonschema.NewCompiler().Compile(schemaBytes)
	require.NoError(t, err)
	files, err := filepath.Glob(filepath.Join("..", "internal", "generator", "providers", "builtin", "*.toml"))
	require.NoError(t, err)
	require.NotEmpty(t, files)

	for _, file := range files {
		t.Run(filepath.Base(file), func(t *testing.T) {
			data, err := os.ReadFile(file)
			require.NoError(t, err)
			var doc map[string]any
			require.NoError(t, toml.Unmarshal(data, &doc))
			asJSON, err := json.Marshal(doc)
			require.NoError(t, err)
			var v any
			require.NoError(t, json.Unmarshal(asJSON, &v))

			// Act
			res := compiled.Validate(v)

			// Assert
			assert.True(t, res.IsValid(), "%s violates provider.schema.json: %v", filepath.Base(file), res.Errors)
		})
	}
}
