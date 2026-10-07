package schema_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Goldziher/ai-rulez/v5/schema"
)

func TestMCPServerPackageIsAPackageURL(t *testing.T) {
	const head = "version = \"5.0\"\nname = \"p\"\npresets = [\"claude\"]\n[[mcp_servers]]\nname = \"srv\"\ncommand = \"x\"\n"
	tests := []struct {
		name    string
		pkg     string
		wantErr bool
	}{
		{"npm purl", "package = \"pkg:npm/%40scope/srv@1.2.3\"\n", false},
		{"purl with qualifiers", "package = \"pkg:oci/img@sha256:abc?repository_url=ghcr.io/o/img\"\n", false},
		{"not a purl", "package = \"left-pad\"\n", true},
		{"whitespace", "package = \"pkg:npm/a b\"\n", true},
		{"empty type", "package = \"pkg:/srv\"\n", true},
		{"not a string", "package = 3\n", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			path := writeTOML(t, head+tt.pkg)

			// Act
			err := schema.ValidateFile(path)

			// Assert
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
		})
	}
}
