package policy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/kaptinlin/jsonschema"
	toml "github.com/pelletier/go-toml/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func compileSchema(t *testing.T, name string) *jsonschema.Schema {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "schema", name))
	require.NoError(t, err)
	s, err := jsonschema.NewCompiler().Compile(data)
	require.NoError(t, err)
	return s
}

func tomlToJSON(t *testing.T, body string) []byte {
	t.Helper()
	var doc map[string]any
	require.NoError(t, toml.Unmarshal([]byte(body), &doc))
	out, err := json.Marshal(doc)
	require.NoError(t, err)
	return out
}

// The policy documented in docs/policy.md is the one the parser and the schema accept.
func TestDocumentedPolicyMatchesParserAndSchema(t *testing.T) {
	// Arrange
	doc, err := os.ReadFile(filepath.Join("..", "..", "docs", "policy.md"))
	require.NoError(t, err)
	fence := regexp.MustCompile("(?s)```toml\n(# ai-rulez-policy\\.toml.*?)```")
	blocks := fence.FindAllStringSubmatch(string(doc), -1)
	require.NotEmpty(t, blocks, "docs/policy.md must show a policy file")
	schema := compileSchema(t, "policy.schema.json")
	for _, b := range blocks {
		// Act
		_, _, perr := Parse("docs/policy.md", []byte(b[1]))
		result := schema.Validate(tomlToJSON(t, b[1]))
		// Assert
		require.NoError(t, perr)
		assert.True(t, result.IsValid(), "%v", result.Errors)
	}
}

func TestPolicySchemaRejectsWhatTheParserRejects(t *testing.T) {
	schema := compileSchema(t, "policy.schema.json")
	tests := []struct{ name, body string }{
		{"unknown key", "policy_version = 1\n[lint]\nrequired_code = []\n"},
		{"wrong version", "policy_version = 2\n"},
		{"missing version", "name = \"x\"\n"},
		{"scheme in a host", "policy_version = 1\n[sources]\nallowed_hosts = [\"https://github.com\"]\n"},
		{"path in a security host", "policy_version = 1\n[lint.security]\nallowed_hosts = [\"github.com/org\"]\n"},
		{"off floor", "policy_version = 1\n[lint.severity_floor]\nAR001 = \"off\"\n"},
		{"scan_imports off", "policy_version = 1\n[lint.security]\nscan_imports = \"off\"\n"},
		{"tlog off", "policy_version = 1\n[signing]\ntlog = \"off\"\n"},
		{"unknown signing subject", "policy_version = 1\n[signing]\nrequire_verified = [\"sbom\"]\n"},
		{"trust key file", "policy_version = 1\n[[signing.trust]]\nkey_file = \"k.pem\"\n"},
		{"trust without issuer", "policy_version = 1\n[[signing.trust]]\nidentity = \"a@b.c\"\n"},
		{"trust with both identity forms", "policy_version = 1\n[[signing.trust]]\nidentity = \"a\"\nidentity_regexp = \"^a$\"\nissuer = \"i\"\n"},
		{"unknown transport", "policy_version = 1\n[mcp]\ndeny_transports = [\"ws\"]\n"},
		{"too many extends", "policy_version = 1\nextends = [\"a\",\"b\",\"c\",\"d\",\"e\",\"f\",\"g\",\"h\",\"i\"]\n"},
		{"empty extends entry", "policy_version = 1\nextends = [\"\"]\n"},
		{"negative ceiling", "policy_version = 1\n[lint.max_findings]\nAR001 = -1\n"},
		{"bad release age", "policy_version = 1\n[sources]\nmin_release_age = \"soon\"\n"},
		{"unknown budget kind", "policy_version = 1\n[lint.budgets.poem]\nmax_lines = 3\n"},
		{"zero budget", "policy_version = 1\n[lint.budgets.rule]\nmax_lines = 0\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.False(t, schema.Validate(tomlToJSON(t, tt.body)).IsValid(), "schema accepted it")
			_, _, err := Parse("p.toml", []byte(tt.body))
			assert.Error(t, err, "parser accepted it")
		})
	}
}

func TestShowPolicyJSONMatchesItsSchema(t *testing.T) {
	// Arrange
	schema := compileSchema(t, "policy-effective.schema.json")
	golden, err := os.ReadFile(filepath.Join("testdata", "show_json.golden"))
	require.NoError(t, err)
	// Act and Assert
	result := schema.Validate(golden)
	assert.True(t, result.IsValid(), "%v", result.Errors)
	empty, err := json.Marshal(BuildReport(nil, nil))
	require.NoError(t, err)
	assert.True(t, schema.Validate(empty).IsValid())
	assert.False(t, schema.Validate([]byte(`{"schema_version":1}`)).IsValid())
}
