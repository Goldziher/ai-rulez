package schema_test

import (
	"testing"

	"github.com/samber/oops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/schema"
)

// The hint on a schema failure names the current config: version "4.0" in
// TOML, not the V3 YAML spelling it used to suggest.
func TestValidateFile_SchemaHintDescribesTheCurrentConfig(t *testing.T) {
	err := schema.ValidateFile(writeTOML(t, "version = \"5.0\"\nname = \"x\"\nbogus_key = 1\n"))
	require.Error(t, err)

	o, ok := oops.AsOops(err)
	require.True(t, ok)
	hint := o.Hint()

	assert.Contains(t, hint, `version = "5.0"`)
	assert.NotContains(t, hint, `"3.0"`)
	assert.NotContains(t, hint, "YAML/JSON")
	assert.Contains(t, hint, "ai-rulez validate")
}
