package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/kaptinlin/jsonschema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/schema"
)

// validateAgainstSchema checks a command's stdout against the schema published
// under schema/ for it.
func validateAgainstSchema(t *testing.T, schemaFile, stdout string) {
	t.Helper()
	schemaBytes, err := os.ReadFile(filepath.Join("..", "..", "..", "schema", schemaFile))
	require.NoError(t, err)
	compiled, err := jsonschema.NewCompiler().Compile(schemaBytes)
	require.NoError(t, err, schemaFile)
	var v any
	require.NoError(t, json.NewDecoder(bytes.NewReader([]byte(stdout))).Decode(&v), "stdout is not JSON: %s", stdout)
	res := compiled.Validate(v)
	assert.True(t, res.IsValid(), "%s: %v\n%s", schemaFile, res.Errors, stdout)
	doc, ok := v.(map[string]any)
	require.True(t, ok, "a report is a JSON object: %s", stdout)
	assert.EqualValues(t, 1, doc["schema_version"], "every report carries schema_version: %s", stdout)
}

const reportProjectConfig = `
[[roles]]
name = "dev"
description = "developer"
`

const reportVerifierSpec = `[[verifiers]]
id = "has-down"
rule = "local"
severity = "warning"
fix = "add a down section"
when_changed = ["db/*.sql"]
[verifiers.require.regex]
regex = "-- down"
[[verifiers.examples]]
name = "good"
files = { "db/1.sql" = "-- down\n" }
changed = ["db/1.sql"]
expect = "pass"
`

// reportProject is a generated and locked project with a role and a verifier.
func reportProject(t *testing.T, env *isoEnv, extraConfig string) string {
	t.Helper()
	root := minimalProject(t, reportProjectConfig+extraConfig)
	writeTree(t, root, map[string]string{
		".ai-rulez/verifiers/db.toml": reportVerifierSpec,
		"db/1.sql":                    "create\n",
	})
	for _, args := range [][]string{{"generate", "--yes"}, {"lock"}} {
		res := env.run(root, args...)
		require.Equal(t, 0, res.ExitCode, "%v: %s", args, res.Stderr)
	}
	return root
}

type reportCase struct {
	contract string
	args     []string
	exit     int
}

// TestEveryJSONContractValidatesAgainstItsSchema runs every report command with
// --format json and validates the document against the schema published for it,
// then checks that no entry of schema.JSONContracts went untested.
func TestEveryJSONContractValidatesAgainstItsSchema(t *testing.T) {
	env := newIsoEnv(t)
	contracts := map[string]string{}
	for _, c := range schema.JSONContracts {
		contracts[c.Command] = c.Schema
		_, err := os.Stat(filepath.Join("..", "..", "..", "schema", c.Schema))
		require.NoError(t, err, "%s names a schema that is not published", c.Command)
	}
	covered := map[string]bool{}
	runAll := func(root string, cases []reportCase) {
		for _, tc := range cases {
			want, ok := contracts[tc.contract]
			require.True(t, ok, "%s is not in schema.JSONContracts", tc.contract)
			res := env.run(root, append(append([]string{}, tc.args...), "--format", "json")...)
			require.Equal(t, tc.exit, res.ExitCode, "%v\nstdout: %s\nstderr: %s", tc.args, res.Stdout, res.Stderr)
			validateAgainstSchema(t, want, res.Stdout)
			covered[tc.contract] = true
		}
	}

	root := reportProject(t, env, "")
	catalog := filepath.Join(t.TempDir(), "catalog.json")
	snapshot := env.run(root, "catalog", "--format", "json", "--schema-version", "2")
	require.Equal(t, 0, snapshot.ExitCode, snapshot.Stderr)
	require.NoError(t, os.WriteFile(catalog, []byte(snapshot.Stdout), 0o600))
	writeTree(t, root, map[string]string{"bundle/index.md": "---\nokf_version: \"0.2\"\n---\n\n# Concepts\n"})
	runAll(root, []reportCase{
		{"validate", []string{"validate"}, 0},
		{"scan", []string{"scan"}, 0},
		{"doctor", []string{"doctor"}, 0},
		{"tokens", []string{"tokens"}, 0},
		{"cost", []string{"cost"}, 0},
		{"catalog", []string{"catalog"}, 0},
		{"catalog diff", []string{"catalog", "diff", catalog, catalog}, 0},
		{"lock --check", []string{"lock", "--check"}, 0},
		{"lock --diff", []string{"lock", "--diff"}, 0},
		{"lock --outdated", []string{"lock", "--outdated"}, 0},
		{"lock --subject", []string{"lock", "--subject"}, 0},
		{"update", []string{"update"}, 0},
		{"scanners list", []string{"scanners", "list"}, 0},
		{"scanners doctor", []string{"scanners", "doctor", "--all"}, 0},
		{"roles list", []string{"roles", "list"}, 0},
		{"roles show", []string{"roles", "show", "dev"}, 0},
		{"roles resolve", []string{"roles", "resolve", "dev"}, 0},
		{"verifiers run", []string{"verifiers", "run"}, 0},
		{"verifiers list", []string{"verifiers", "list"}, 0},
		{"verifiers test", []string{"verifiers", "test"}, 0},
		{"verifiers explain", []string{"verifiers", "explain", "has-down"}, 0},
		{"okf validate", []string{"okf", "validate", "bundle"}, 0},
		{"export okf", []string{"export", "okf"}, 0},
		{"eval run", []string{"eval", "run", "--dry-run"}, 0},
		{"verify --approvals", []string{"verify", "--approvals"}, 0},
		{"sbom", []string{"sbom", "--output", filepath.Join(t.TempDir(), "sbom.json")}, 0},
	})

	// sign and verify need a trusted key pair.
	signed, releaseKey, _ := signedProject(t, env)
	runAll(signed, []reportCase{
		{"sign", []string{"sign", "--lock", "--key", releaseKey}, 0},
		{"verify --attestation", []string{"verify", "--attestation"}, 0},
	})

	// A plugin project verifies its bundles and publishes.
	plugin := publishableProject(t, env)
	runAll(plugin, []reportCase{
		{"verify --plugin", []string{"verify", "--plugin"}, 0},
		{"publish", []string{"publish", "--dry-run"}, 0},
	})
	require.Equal(t, 0, env.run(plugin, "publish").ExitCode)
	runAll(plugin, []reportCase{
		{"publish verify", []string{"publish", "verify", "dist"}, 0},
		{"publish emit", []string{"publish", "emit", "kiro-steering", "--experimental", "--out", filepath.Join(t.TempDir(), "emit")}, 0},
	})

	for _, c := range schema.JSONContracts {
		assert.True(t, covered[c.Command], "%s has a schema but no test runs it", c.Command)
	}
}

// TestFailuresPrintTheErrorDocument: a command asked for --format json that fails
// before it has a report prints the error document on stdout, and the same text
// on stderr.
func TestFailuresPrintTheErrorDocument(t *testing.T) {
	env := newIsoEnv(t)
	root := reportProject(t, env, "")
	for _, args := range [][]string{
		{"verify", "--format", "json"},
		{"roles", "show", "nope", "--format", "json"},
		{"verifiers", "explain", "nope", "--format", "json"},
		{"lock", "--outdated", "--check", "--format", "json"},
		{"sbom", "--check", "--format", "json"},
	} {
		res := env.run(root, args...)
		require.Equal(t, 1, res.ExitCode, "%v\nstdout: %s\nstderr: %s", args, res.Stdout, res.Stderr)
		validateAgainstSchema(t, schema.ErrorDocumentSchema, res.Stdout)
		assert.Contains(t, res.Stderr, "Error:", "%v", args)
	}
}

// TestReportSchemasRejectAWrongDocument: the schemas check something.
func TestReportSchemasRejectAWrongDocument(t *testing.T) {
	for file, doc := range map[string]string{
		"doctor-report.schema.json":  `{"schema_version":1,"summary":{"error":0,"warning":0},"findings":[]}`,
		"sbom-report.schema.json":    `{"schema_version":1,"status":"bogus","type":"cyclonedx","findings":[],"differences":[]}`,
		"verifiers-test.schema.json": `{"schema_version":1,"ok":true,"passed":0,"total":0,"results":null,"untested":[],"problems":[]}`,
		schema.ErrorDocumentSchema:   `{"schema_version":1,"status":"error","error":"x","exit_code":0}`,
	} {
		schemaBytes, err := os.ReadFile(filepath.Join("..", "..", "..", "schema", file))
		require.NoError(t, err)
		compiled, err := jsonschema.NewCompiler().Compile(schemaBytes)
		require.NoError(t, err)
		var v any
		require.NoError(t, json.Unmarshal([]byte(doc), &v))
		assert.False(t, compiled.Validate(v).IsValid(), "%s accepted %s", file, doc)
	}
}
