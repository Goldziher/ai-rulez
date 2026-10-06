package schema_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Every schema names itself with one host and path style, the one `init` writes
// into config.toml, so editors resolve $schema and $id to the same place.
func TestSchemaIDsUseOneHost(t *testing.T) {
	// Arrange
	files, err := filepath.Glob("*.schema.json")
	require.NoError(t, err)
	require.NotEmpty(t, files)
	prefix := "https://raw.githubusercontent.com/Goldziher/ai-rulez/main/schema/"

	for _, name := range files {
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile(name)
			require.NoError(t, err)

			// Act
			var doc struct {
				ID string `json:"$id"`
			}
			require.NoError(t, json.Unmarshal(data, &doc))

			// Assert
			assert.Equal(t, prefix+name, doc.ID)
		})
	}
}

func TestInitSchemaURLMatchesTheIDHost(t *testing.T) {
	assert.True(t, strings.HasPrefix(schema.SchemaURL(schema.ConfigSchemaFile), "https://raw.githubusercontent.com/Goldziher/ai-rulez/"))
	assert.True(t, strings.HasSuffix(schema.SchemaURL(schema.ConfigSchemaFile), "/schema/"+schema.ConfigSchemaFile))
}
