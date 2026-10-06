package verifiers

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kaptinlin/jsonschema"
	"github.com/pelletier/go-toml/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSpecSchemaAgreesWithTheLoader(t *testing.T) {
	schemaBytes, err := os.ReadFile(filepath.Join("..", "..", "schema", "verifiers-spec.schema.json"))
	require.NoError(t, err)
	compiled, err := jsonschema.NewCompiler().Compile(schemaBytes)
	require.NoError(t, err)
	const head = "[[verifiers]]\nid = \"v\"\nrule = \"database\"\n"
	tests := []struct {
		name  string
		toml  string
		valid bool
	}{
		{"regex", head + "[verifiers.require.regex]\nregex = \"x\"\nin = \"any-file\"\nfiles = \"*.go\"\n", true},
		{"paired with examples", head + "when_changed = [\"a/*.py\"]\n[verifiers.require.paired]\nfor_each = \"a/{rel}.py\"\nrequires_changed = \"t/{stem}.py\"\n[[verifiers.examples]]\nname = \"e\"\nexpect = \"pass\"\nfiles = { \"a.py\" = \"x\" }\n", true},
		{"nested combinators", head + "[[verifiers.require.all]]\n[verifiers.require.all.not.file_exists]\npath = \"x\"\n", true},
		{"two predicates", head + "[verifiers.require.regex]\nregex = \"x\"\n[verifiers.require.forbid]\nregex = \"y\"\n", false},
		{"two targets", head + "skill = \"s\"\n[verifiers.require.regex]\nregex = \"x\"\n", false},
		{"unknown key", head + "bogus = 1\n[verifiers.require.regex]\nregex = \"x\"\n", false},
		{"bad in", head + "[verifiers.require.regex]\nregex = \"x\"\nin = \"nowhere\"\n", false},
		{"paired without requirement", head + "[verifiers.require.paired]\nfor_each = \"a\"\n", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var doc any
			require.NoError(t, toml.Unmarshal([]byte(tt.toml), &doc))

			res := compiled.Validate(doc)

			assert.Equal(t, tt.valid, res.IsValid(), "%v", res.Errors)
		})
	}
}
