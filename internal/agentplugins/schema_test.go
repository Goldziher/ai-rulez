package agentplugins

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	"github.com/kaptinlin/jsonschema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVendoredSchemasMatchThePinnedUpstreamBytes(t *testing.T) {
	tests := []struct {
		file   string
		digest string
	}{
		{"schemas/1.0.0/plugin.schema.json", "0a4aad95ce337878ad38802ebf0daa3fde76abe3f65400c86bcbb1ec0b3ab883"},
		{"schemas/1.0.0/mcp.schema.json", "6539175bfcdf43085855183e86da40ea94b166547a72b47ae9a0a390516d3acb"},
		{"schemas/1.1.0/plugin.schema.json", "fdc7bb3962c48c9d2d561641d2bc96225c94ca69c4087010241b9423a290370f"},
		{"schemas/1.1.0/mcp.schema.json", "f227ec2c0e40cd23051bd7a6ba1f64789eff7773d4e481d80002ab9fd3c45137"},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			// Arrange
			data, err := schemaFS.ReadFile(tt.file)
			require.NoError(t, err)

			// Act
			sum := sha256.Sum256(data)

			// Assert
			assert.Equal(t, tt.digest, hex.EncodeToString(sum[:]),
				"vendored schema differs from upstream commit %s; re-vendor and update the digest together", SchemaSourceCommit)
		})
	}
}

func TestVendoredSchemaIDsAreTheCanonicalIdentifiers(t *testing.T) {
	for _, spec := range Specs {
		for _, kind := range []string{"plugin", "mcp"} {
			t.Run(spec+"/"+kind, func(t *testing.T) {
				// Arrange
				data, err := schemaFS.ReadFile("schemas/" + spec + "/" + kind + ".schema.json")
				require.NoError(t, err)
				var doc map[string]any
				require.NoError(t, json.Unmarshal(data, &doc))
				want := PluginSchemaID(spec)
				if kind == "mcp" {
					want = MCPSchemaID(spec)
				}

				// Act
				props := doc["properties"].(map[string]any)
				constID := props["$schema"].(map[string]any)["const"]

				// Assert
				assert.Equal(t, want, doc["$id"])
				assert.Equal(t, want, constID)
			})
		}
	}
}

func TestSchemasCompileAndCheckManifests(t *testing.T) {
	tests := []struct {
		name  string
		doc   func(spec string) map[string]any
		valid bool
	}{
		{"minimal", func(s string) map[string]any { return map[string]any{"$schema": PluginSchemaID(s), "name": "a"} }, true},
		{"dotted name", func(s string) map[string]any {
			return map[string]any{"$schema": PluginSchemaID(s), "name": "acme.tools"}
		}, true},
		{"double hyphen", func(s string) map[string]any {
			return map[string]any{"$schema": PluginSchemaID(s), "name": "has--double"}
		}, false},
		{"double period", func(s string) map[string]any {
			return map[string]any{"$schema": PluginSchemaID(s), "name": "too.many..dots"}
		}, false},
		{"uppercase", func(s string) map[string]any {
			return map[string]any{"$schema": PluginSchemaID(s), "name": "My-Plugin"}
		}, false},
		{"wrong schema id", func(string) map[string]any {
			return map[string]any{"$schema": "https://agent-plugins.org/schemas/9.9.9/plugin.schema.json", "name": "a"}
		}, false},
		{"unknown field", func(s string) map[string]any {
			return map[string]any{"$schema": PluginSchemaID(s), "name": "a", "skills": "./x"}
		}, false},
		{"author extra field", func(s string) map[string]any {
			return map[string]any{"$schema": PluginSchemaID(s), "name": "a", "author": map[string]any{"name": "x", "x": "y"}}
		}, false},
	}
	for _, spec := range Specs {
		set, err := schemasFor(spec)
		require.NoError(t, err)
		for _, tt := range tests {
			t.Run(spec+"/"+tt.name, func(t *testing.T) {
				// Act
				errs := schemaErrors(set.plugin, tt.doc(spec))

				// Assert
				assert.Equal(t, tt.valid, len(errs) == 0, "%v", errs)
			})
		}
	}
}

func TestSchemasForRejectsAnUnknownSpec(t *testing.T) {
	_, err := schemasFor("2.0.0")

	assert.Error(t, err)
}

// The official name pattern uses a lookahead, which Go's RE2 engine (and so the
// schema validator) cannot compile. This test documents why the pattern is
// substituted at load time.
func TestVendoredPluginSchemaDoesNotCompileUnmodified(t *testing.T) {
	// Arrange
	data, err := schemaFS.ReadFile("schemas/1.0.0/plugin.schema.json")
	require.NoError(t, err)

	// Act
	_, err = jsonschema.NewCompiler().Compile(data)

	// Assert
	assert.Error(t, err)
}

// re2NamePattern must accept exactly the names the specification's four rules
// accept (§5.5). The check is exhaustive over every string up to length 6 drawn
// from an alphabet that exercises each rule.
func TestRE2NamePatternAgreesWithTheSpecRules(t *testing.T) {
	// Arrange
	re := regexp.MustCompile(re2NamePattern)
	alphabet := []string{"a", "0", "-", ".", "A", "_"}
	words := []string{""}
	frontier := []string{""}
	for range 6 {
		var next []string
		for _, w := range frontier {
			for _, c := range alphabet {
				next = append(next, w+c)
			}
		}
		words = append(words, next...)
		frontier = next
	}

	for _, w := range words {
		// Act
		got := re.MatchString(w)

		// Assert
		if got != ValidPluginName(w) {
			t.Fatalf("pattern and rules disagree on %q: pattern=%v rules=%v", w, got, ValidPluginName(w))
		}
	}
	assert.Greater(t, len(words), 50000)
}

func TestValidPluginName(t *testing.T) {
	tests := []struct {
		name  string
		valid bool
	}{
		{"my-plugin", true},
		{"acme.tools", true},
		{"lint3r", true},
		{"a", true},
		{"a.-b", true},
		{strings.Repeat("a", 64), true},
		{strings.Repeat("a", 65), false},
		{"My-Plugin", false},
		{"-start", false},
		{"end.", false},
		{"has--double", false},
		{"too.many..dots", false},
		{"", false},
		{"under_score", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.valid, ValidPluginName(tt.name))
		})
	}
}
