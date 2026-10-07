package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/kaptinlin/jsonschema"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/Goldziher/ai-rulez/v5/tests/e2e/testutil"
)

// Every `--format json` document carries schema_version, and the documents that
// have a schema under schema/ validate against it.
type JSONSchemaSuite struct {
	suite.Suite
	dir string
}

func TestJSONSchemaSuite(t *testing.T) { suite.Run(t, new(JSONSchemaSuite)) }

func (s *JSONSchemaSuite) SetupTest()     { s.dir = testutil.CreateTempDir(s.T()) }
func (s *JSONSchemaSuite) TearDownSuite() { testutil.CleanupTestBinary() }

func (s *JSONSchemaSuite) write(rel, body string) {
	path := filepath.Join(s.dir, filepath.FromSlash(rel))
	s.Require().NoError(os.MkdirAll(filepath.Dir(path), 0o755))
	s.Require().NoError(os.WriteFile(path, []byte(body), 0o644))
}

func (s *JSONSchemaSuite) project() {
	s.write(".ai-rulez/config.toml", "version = \"5.0\"\nname = \"json-schema\"\npresets = [\"claude\"]\n")
	s.write(".ai-rulez/rules/style.md", "---\ndescription: style\n---\n# Style\n\nUse tabs.\n")
}

func (s *JSONSchemaSuite) output(args ...string) []byte {
	result := testutil.RunCLI(s.T(), s.dir, args...)
	s.Require().Equal(0, result.ExitCode, "%v: %s", args, result.Stderr)
	return []byte(result.Stdout)
}

func (s *JSONSchemaSuite) validate(schemaFile string, doc []byte) {
	schemaBytes, err := os.ReadFile(filepath.Join("..", "..", "..", "schema", schemaFile))
	s.Require().NoError(err)
	compiled, err := jsonschema.NewCompiler().Compile(schemaBytes)
	s.Require().NoError(err)
	var v any
	s.Require().NoError(json.NewDecoder(bytes.NewReader(doc)).Decode(&v))
	res := compiled.Validate(v)
	s.True(res.IsValid(), "%s: %v\n%s", schemaFile, res.Errors, doc)
}

func (s *JSONSchemaSuite) TestDocumentsMatchTheirSchemas() {
	s.project()
	s.write("bundle/decisions/use-go.md", "---\ntype: Decision\ndescription: Use Go\n---\nWe write Go.\n")
	cases := []struct {
		schema string
		args   []string
	}{
		{"validate-report.schema.json", []string{"validate", "--format", "json"}},
		{"validate-report.schema.json", []string{"scan", "--format", "json"}},
		{"cost-report.schema.json", []string{"cost", "--format", "json"}},
		{"telemetry-doctor.schema.json", []string{"telemetry", "doctor", "--format", "json"}},
		{"eval-report.schema.json", []string{"eval", "run", "--dry-run", "--format", "json"}},
		{"okf-validate.schema.json", []string{"okf", "validate", "bundle", "--format", "json"}},
		{"catalog.schema.json", []string{"catalog", "--format", "json"}},
	}
	for _, tc := range cases {
		s.validate(tc.schema, s.output(tc.args...))
	}
}

func (s *JSONSchemaSuite) TestEveryJSONDocumentIsVersioned() {
	s.project()
	for _, args := range [][]string{
		{"validate", "--format", "json"},
		{"cost", "--format", "json"},
		{"catalog", "--format", "json"},
		{"doctor", "--format", "json"},
		{"tokens", "--format", "json"},
		{"verifiers", "run", "--format", "json"},
		{"list", "rules", "--format", "json"},
		{"domain", "list", "--format", "json"},
		{"profile", "list", "--format", "json"},
		{"include", "list", "--format", "json"},
		{"skill", "list", "--format", "json"},
		{"builtins", "list", "--format", "json"},
		{"telemetry", "doctor", "--format", "json"},
		{"migrate", "v5", "--check", "--format", "json"},
	} {
		result := testutil.RunCLI(s.T(), s.dir, args...)
		var doc map[string]any
		s.Require().NoError(json.Unmarshal([]byte(result.Stdout), &doc), "%v: exit %d\nstdout: %s\nstderr: %s", args, result.ExitCode, result.Stdout, result.Stderr)
		s.EqualValues(1, doc["schema_version"], "%v", args)
	}
}

func TestSchemaFilesAreValidJSONSchemas(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("..", "..", "..", "schema", "*.schema.json"))
	require.NoError(t, err)
	require.NotEmpty(t, files)
	for _, f := range files {
		data, err := os.ReadFile(f)
		require.NoError(t, err)
		_, err = jsonschema.NewCompiler().Compile(data)
		require.NoError(t, err, f)
	}
}
