package mcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/Goldziher/ai-rulez/v5/internal/mcp/handlers"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func textOfResult(t *testing.T, res *sdkmcp.CallToolResult) string {
	t.Helper()
	require.NotEmpty(t, res.Content)
	text, ok := res.Content[0].(*sdkmcp.TextContent)
	require.True(t, ok, "first content block is text")
	return text.Text
}

func structured(t *testing.T, res *sdkmcp.CallToolResult) map[string]any {
	t.Helper()
	doc, ok := res.StructuredContent.(map[string]any)
	require.True(t, ok, "structuredContent is an object, got %T", res.StructuredContent)
	return doc
}

func malformedProject(t *testing.T) string {
	t.Helper()
	dir := telemetryProject(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".ai-rulez", "config.toml"), []byte("this is [not toml\n"), 0o600))
	return dir
}

func TestResultsCarryStructuredContentAndTextFallback(t *testing.T) {
	t.Parallel()
	session := connect(t, NewServer("1.2.3", WithAnyDirectory()))
	dir := telemetryProject(t)

	for tool, args := range map[string]map[string]any{
		"get_version":  nil,
		"list_rules":   {"working_directory": dir},
		"read_rule":    {"working_directory": dir, "name": "atomic-commits"},
		"read_config":  {"working_directory": dir},
		"list_domains": {"working_directory": dir},
		"create_rule":  {"working_directory": dir, "name": "fresh", "content": "# Fresh\n"},
	} {
		t.Run(tool, func(t *testing.T) {
			res := call(t, session, tool, args)

			require.False(t, res.IsError, textOfResult(t, res))
			doc := structured(t, res)
			var fromText map[string]any
			require.NoError(t, json.Unmarshal([]byte(textOfResult(t, res)), &fromText), "text content is the JSON fallback")
			assert.Equal(t, fromText, doc, "the text fallback and structuredContent carry the same document")
		})
	}
	res := call(t, session, "get_version", nil)
	assert.Equal(t, "1.2.3", structured(t, res)["version"])
}

func TestUnknownAndMistypedArgumentsAreRejected(t *testing.T) {
	t.Parallel()
	session := connect(t, NewServer("test", WithAnyDirectory()))
	dir := telemetryProject(t)

	tests := map[string]map[string]any{
		"unknown argument":  {"working_directory": dir, "nope": true},
		"wrong type":        {"working_directory": dir, "local": "yes"},
		"enum violation":    {"working_directory": dir, "name": "x", "priority": "urgent"},
		"missing required":  {"working_directory": dir},
		"number for string": {"working_directory": dir, "name": 7},
	}
	for name, args := range tests {
		t.Run(name, func(t *testing.T) {
			res, err := session.CallTool(context.Background(), &sdkmcp.CallToolParams{Name: "create_rule", Arguments: args})

			require.NoError(t, err)
			assert.True(t, res.IsError, "invalid arguments are an error result, never success")
		})
	}
}

// Every reader tool must fail, as the CLI does, when config.toml is malformed:
// an empty list would read as "nothing is configured".
func TestReaderToolsFailOnMalformedConfig(t *testing.T) {
	t.Parallel()
	session := connect(t, NewServer("test", WithAnyDirectory()))
	dir := malformedProject(t)

	tools := map[string]map[string]any{
		"list_rules":            {},
		"list_context":          {},
		"list_skills":           {},
		"list_checks":           {},
		"list_domains":          {},
		"list_includes":         {},
		"list_installed_skills": {},
		"list_profiles":         {},
		"read_rule":             {"name": "atomic-commits"},
		"read_context":          {"name": "x"},
		"read_skill":            {"name": "x"},
		"read_check":            {"name": "x"},
		"read_config":           {},
		"list_roles":            {},
		"catalog":               {},
		"doctor":                {},
		"validate_config":       {},
		"lock_status":           {},
		"run_verifiers":         {},
		"generate_outputs":      {},
		"list_agents":           {},
		"list_commands":         {},
		"read_agent":            {"name": "x"},
		"read_command":          {"name": "x"},
		"token_report":          {},
		"cost_report":           {},
		"sbom":                  {},
		"approvals_status":      {},
		"list_verifiers":        {},
	}
	for tool, args := range tools {
		t.Run(tool, func(t *testing.T) {
			args["working_directory"] = dir

			res := call(t, session, tool, args)

			assert.True(t, res.IsError, "%s must report the malformed config, got: %s", tool, textOfResult(t, res))
		})
	}
}

func TestWorkingDirectoryIsConfinedToTheRoot(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	project := filepath.Join(root, "project")
	require.NoError(t, os.MkdirAll(filepath.Join(project, ".ai-rulez", "rules"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(project, ".ai-rulez", "config.toml"), []byte("version = \"5.0\"\nname = \"t\"\npresets = [\"claude\"]\n"), 0o600))
	outside := telemetryProject(t)
	require.NoError(t, os.Symlink(outside, filepath.Join(root, "escape")))
	session := connect(t, NewServer("test", WithRoot(root)))

	tests := []struct {
		name    string
		args    map[string]any
		wantErr string
	}{
		{"inside the root", map[string]any{"working_directory": project}, ""},
		{"relative to the root", map[string]any{"working_directory": "project"}, ""},
		{"the default is the root", map[string]any{}, "no .ai-rulez"},
		{"outside the root", map[string]any{"working_directory": outside}, "outside"},
		{"a parent of the root", map[string]any{"working_directory": filepath.Dir(root)}, "outside"},
		{"dot dot out of the root", map[string]any{"working_directory": "project/../../"}, "outside"},
		{"a symlink out of the root", map[string]any{"working_directory": filepath.Join(root, "escape")}, "outside"},
		{"an absolute config file elsewhere", map[string]any{"working_directory": project, "config_file": filepath.Join(outside, ".ai-rulez", "config.toml")}, "outside"},
		{"a config dir that climbs out", map[string]any{"working_directory": project, "config_dir": "../../x"}, "outside"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := call(t, session, "validate_config", tt.args)

			text := textOfResult(t, res)
			if tt.wantErr == "outside" {
				assert.True(t, res.IsError)
				assert.Contains(t, text, "outside the directory this server is allowed to use")
			} else {
				assert.NotContains(t, text, "outside the directory this server is allowed to use")
			}
		})
	}

	t.Run("writes outside the root are refused and nothing is written", func(t *testing.T) {
		res := call(t, session, "create_rule", map[string]any{"working_directory": outside, "name": "pwned", "content": "x"})

		assert.True(t, res.IsError)
		assert.NoFileExists(t, filepath.Join(outside, ".ai-rulez", "rules", "pwned.md"))
	})
	t.Run("--allow-any-dir lifts the confinement", func(t *testing.T) {
		open := connect(t, NewServer("test", WithRoot(root), WithAnyDirectory()))

		res := call(t, open, "list_rules", map[string]any{"working_directory": outside})

		assert.False(t, res.IsError, textOfResult(t, res))
	})
}

func stubValidator(outcome *handlers.ValidateOutcome, err error) handlers.Validator {
	return func(context.Context, *config.Config, handlers.ValidateParams) (*handlers.ValidateOutcome, error) {
		return outcome, err
	}
}

func TestValidateConfigReportsLintThroughTheValidator(t *testing.T) {
	t.Parallel()
	dir := telemetryProject(t)
	warning := lint.Finding{Code: "AR802", Severity: lint.SeverityWarning, File: "rules/a.md", Line: 3, Message: "glob matches nothing"}
	report := lint.Combined{Roots: []string{"."}, Findings: []lint.Finding{warning}}

	t.Run("warnings below fail_on are reported and the result succeeds", func(t *testing.T) {
		srv := NewServer("test", WithAnyDirectory(), WithValidator(stubValidator(&handlers.ValidateOutcome{Report: report, FailOn: "error"}, nil)))

		res := call(t, connect(t, srv), "validate_config", map[string]any{"working_directory": dir})

		require.False(t, res.IsError, textOfResult(t, res))
		doc := structured(t, res)
		assert.Equal(t, true, doc["valid"])
		assert.Equal(t, []any{"rules/a.md:3: AR802 glob matches nothing"}, doc["warnings"], "real warnings, not a hard-coded empty list")
	})
	t.Run("findings that reach fail_on are an error result with the full document", func(t *testing.T) {
		srv := NewServer("test", WithAnyDirectory(), WithValidator(stubValidator(&handlers.ValidateOutcome{Report: report, FailOn: "warning", Failed: true}, nil)))

		res := call(t, connect(t, srv), "validate_config", map[string]any{"working_directory": dir, "fail_on": "warning"})

		require.True(t, res.IsError)
		doc := structured(t, res)
		assert.Equal(t, false, doc["valid"])
		assert.Equal(t, "warning", doc["fail_on"])
		assert.Len(t, doc["findings"], 1)
	})
	t.Run("the lint selectors reach the validator", func(t *testing.T) {
		var got handlers.ValidateParams
		srv := NewServer("test", WithAnyDirectory(), WithValidator(func(_ context.Context, _ *config.Config, p handlers.ValidateParams) (*handlers.ValidateOutcome, error) {
			got = p
			return &handlers.ValidateOutcome{}, nil
		}))

		call(t, connect(t, srv), "validate_config", map[string]any{"working_directory": dir, "fail_on": "none", "strict": true, "config_only": true, "lint_profile": "strict", "analyzers": []string{"security"}})

		assert.Equal(t, handlers.ValidateParams{FailOn: "none", Strict: true, ConfigOnly: true, LintProfile: "strict", Analyzers: []string{"security"}}, got)
	})
	t.Run("a failure to lint is an error result", func(t *testing.T) {
		srv := NewServer("test", WithAnyDirectory(), WithValidator(stubValidator(nil, assert.AnError)))

		res := call(t, connect(t, srv), "validate_config", map[string]any{"working_directory": dir})

		assert.True(t, res.IsError)
	})
	t.Run("without a lint engine validate_config says so instead of passing", func(t *testing.T) {
		res := call(t, connect(t, NewServer("test", WithAnyDirectory())), "validate_config", map[string]any{"working_directory": dir})

		assert.True(t, res.IsError)
		assert.Contains(t, textOfResult(t, res), "lint")
	})
	t.Run("an invalid configuration is an error result with valid false", func(t *testing.T) {
		res := call(t, connect(t, NewServer("test", WithAnyDirectory(), WithValidator(stubValidator(&handlers.ValidateOutcome{}, nil)))), "validate_config", map[string]any{"working_directory": malformedProject(t)})

		require.True(t, res.IsError)
		doc := structured(t, res)
		assert.Equal(t, false, doc["valid"])
		assert.NotEmpty(t, doc["error"])
	})
}

func TestServerCapabilitiesAndMetadata(t *testing.T) {
	t.Parallel()
	session := connect(t, NewServer("test"))

	caps := session.InitializeResult().Capabilities

	require.NotNil(t, caps.Tools)
	assert.False(t, caps.Tools.ListChanged, "the tool set is fixed, so tools/list_changed is not promised")
	assert.Nil(t, caps.Resources, "the authoring server serves no resources")
}
