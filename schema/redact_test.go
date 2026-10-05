package schema_test

import (
	"fmt"
	"testing"

	"github.com/samber/oops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/schema"
)

// Overlay validation errors name the key path and the allowed values but never
// the supplied value: the file may hold secrets.
func TestValidateLocalFile_ErrorsNeverEchoTheSuppliedValue(t *testing.T) {
	tests := []struct {
		name   string
		body   string
		secret string
		want   string
	}{
		{"enum on a list entry", "[[mcp_servers]]\nname = \"a\"\ntransport = \"SECRETTRANS\"\n", "SECRETTRANS", "stdio, http, sse"},
		{"enum on a table key", "[header]\nstyle = \"SECRETSTYLE\"\n", "SECRETSTYLE", "detailed, compact, minimal"},
		{"enum containing a dash separator", "[header]\nstyle = \"SEC - RET\"\n", "SEC", "detailed, compact, minimal"},
		{"enum on an include", "[[includes]]\nname = \"a\"\nsource = \"x\"\nmerge_strategy = \"SECRETM\"\n", "SECRETM", "local-override"},
		{"wrong type", "[[mcp_servers]]\nname = \"a\"\nargs = \"SECRETARGS\"\n", "SECRETARGS", "should be array"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			err := schema.ValidateLocalFile(writeTOML(t, tt.body))

			// Assert
			require.Error(t, err)
			o, ok := oops.AsOops(err)
			require.True(t, ok)
			rendered := fmt.Sprintf("%v %v", err, o.Context()["errors"])
			assert.NotContains(t, rendered, tt.secret)
			assert.Contains(t, rendered, tt.want)
		})
	}
}

func TestValidateFile_MainConfigErrorsStayDetailed(t *testing.T) {
	err := schema.ValidateFile(writeTOML(t, "version = \"4.0\"\nname = \"x\"\n[header]\nstyle = \"BADSTYLE\"\n"))

	require.Error(t, err)
	o, ok := oops.AsOops(err)
	require.True(t, ok)
	assert.Contains(t, fmt.Sprint(o.Context()["errors"]), "BADSTYLE", "the shared config is not secret, so its errors keep the value")
}
