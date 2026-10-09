package mcp

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The agent and command tools are the MCP side of `add|remove|list agent|command`.
func TestAgentAndCommandToolsRunTheWholeLifecycle(t *testing.T) {
	t.Parallel()
	session := connect(t, NewServer("test", WithAnyDirectory()))
	dir := telemetryProject(t)

	for _, kind := range []struct{ singular, plural string }{{"agent", "agents"}, {"command", "commands"}} {
		t.Run(kind.singular, func(t *testing.T) {
			// Create from a description: the template names the item.
			res := call(t, session, "create_"+kind.singular, map[string]any{"working_directory": dir, "name": "helper", "description": "Helps with reviews"})
			require.False(t, res.IsError, textOfResult(t, res))
			path, ok := structured(t, res)["path"].(string)
			require.True(t, ok)
			assert.FileExists(t, path)

			// Read it back.
			res = call(t, session, "read_"+kind.singular, map[string]any{"working_directory": dir, "name": "helper"})
			require.False(t, res.IsError, textOfResult(t, res))
			assert.Contains(t, structured(t, res)["content"], "helper")

			// Update replaces the content as given, with no priority frontmatter added.
			res = call(t, session, "update_"+kind.singular, map[string]any{"working_directory": dir, "name": "helper", "content": "---\nname: helper\ndescription: Replaced\n---\nBody\n"})
			require.False(t, res.IsError, textOfResult(t, res))
			written, err := os.ReadFile(path)
			require.NoError(t, err)
			assert.Contains(t, string(written), "description: Replaced")
			assert.NotContains(t, string(written), "priority:")

			// List shows it.
			res = call(t, session, "list_"+kind.plural, map[string]any{"working_directory": dir})
			require.False(t, res.IsError, textOfResult(t, res))
			assert.EqualValues(t, 1, structured(t, res)["count"])

			// A second create of the same name is refused.
			res = call(t, session, "create_"+kind.singular, map[string]any{"working_directory": dir, "name": "helper"})
			assert.True(t, res.IsError, "create must not overwrite")

			// Delete removes it, and a second delete reports that nothing is there.
			res = call(t, session, "delete_"+kind.singular, map[string]any{"working_directory": dir, "name": "helper"})
			require.False(t, res.IsError, textOfResult(t, res))
			assert.NoFileExists(t, path)
			res = call(t, session, "delete_"+kind.singular, map[string]any{"working_directory": dir, "name": "helper"})
			assert.True(t, res.IsError)
		})
	}
}

func TestUpdateOfAnItemThatDoesNotExistIsAnError(t *testing.T) {
	t.Parallel()
	session := connect(t, NewServer("test", WithAnyDirectory()))
	dir := telemetryProject(t)

	res := call(t, session, "update_agent", map[string]any{"working_directory": dir, "name": "ghost", "content": "x"})

	assert.True(t, res.IsError)
}

func TestCreateSkillTakesADescription(t *testing.T) {
	t.Parallel()
	session := connect(t, NewServer("test", WithAnyDirectory()))
	dir := telemetryProject(t)

	res := call(t, session, "create_skill", map[string]any{"working_directory": dir, "name": "deploy-app", "description": "Deploy the app to staging"})

	require.False(t, res.IsError, textOfResult(t, res))
	data, err := os.ReadFile(filepath.Join(dir, ".ai-rulez", "skills", "deploy-app", "SKILL.md"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "Deploy the app to staging")
}

// generate_outputs takes the selectors of `generate`.
func TestGenerateOutputsSelectorsAndCheck(t *testing.T) {
	t.Parallel()
	session := connect(t, NewServer("test", WithAnyDirectory()))
	dir := telemetryProject(t)
	// No shared AGENTS.md: with it the command's own `generate --check` reports drift right after `generate`.
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".ai-rulez", "config.toml"), []byte("version = \"5.0\"\nname = \"t\"\npresets = [\"claude\"]\nagents_md = false\n"), 0o600))

	t.Run("check before generating reports drift as an error result", func(t *testing.T) {
		res := call(t, session, "generate_outputs", map[string]any{"working_directory": dir, "check": true})

		require.True(t, res.IsError, "drift is the CLI's exit 2")
		doc := structured(t, res)
		assert.Equal(t, "drift", doc["status"])
		assert.NotEmpty(t, doc["differing"])
	})
	t.Run("generate then check is clean and writes nothing more", func(t *testing.T) {
		res := call(t, session, "generate_outputs", map[string]any{"working_directory": dir})
		require.False(t, res.IsError, textOfResult(t, res))

		res = call(t, session, "generate_outputs", map[string]any{"working_directory": dir, "check": true})

		require.False(t, res.IsError, textOfResult(t, res))
		assert.Equal(t, "ok", structured(t, res)["status"])
	})
	t.Run("a role and a profile exclude each other", func(t *testing.T) {
		res := call(t, session, "generate_outputs", map[string]any{"working_directory": dir, "role": "r", "profile": "p"})

		assert.True(t, res.IsError)
		assert.Contains(t, textOfResult(t, res), "mutually exclusive")
	})
	t.Run("check and dry_run exclude each other", func(t *testing.T) {
		res := call(t, session, "generate_outputs", map[string]any{"working_directory": dir, "check": true, "dry_run": true})

		assert.True(t, res.IsError)
	})
	t.Run("an unknown role is an error", func(t *testing.T) {
		res := call(t, session, "generate_outputs", map[string]any{"working_directory": dir, "role": "nobody"})

		assert.True(t, res.IsError)
	})
	t.Run("an unknown profile is an error", func(t *testing.T) {
		res := call(t, session, "generate_outputs", map[string]any{"working_directory": dir, "profile": "nobody", "dry_run": true})

		assert.True(t, res.IsError)
	})
	t.Run("offline generates from a project with no remote sources", func(t *testing.T) {
		res := call(t, session, "generate_outputs", map[string]any{"working_directory": dir, "offline": true, "dry_run": true})

		require.False(t, res.IsError, textOfResult(t, res))
	})
}

func TestBuiltinsAndVerifiersAreListed(t *testing.T) {
	t.Parallel()
	session := connect(t, NewServer("test", WithAnyDirectory()))
	dir := telemetryProject(t)

	res := call(t, session, "list_builtins", nil)
	require.False(t, res.IsError, textOfResult(t, res))
	assert.Positive(t, structured(t, res)["count"])

	res = call(t, session, "list_verifiers", map[string]any{"working_directory": dir})
	require.False(t, res.IsError, textOfResult(t, res))
	assert.EqualValues(t, 0, structured(t, res)["count"])
}

func TestTokenAndCostReportsAreOffline(t *testing.T) {
	t.Parallel()
	session := connect(t, NewServer("test", WithAnyDirectory()))
	dir := telemetryProject(t)

	res := call(t, session, "token_report", map[string]any{"working_directory": dir})
	require.False(t, res.IsError, textOfResult(t, res))
	assert.Equal(t, "claude", structured(t, res)["headline_preset"])

	res = call(t, session, "token_report", map[string]any{"working_directory": dir, "budget": 1})
	assert.True(t, res.IsError, "an exceeded budget is the CLI's exit 2")
	assert.Equal(t, true, structured(t, res)["budget"].(map[string]any)["exceeded"])

	res = call(t, session, "token_report", map[string]any{"working_directory": dir, "compare_profiles": []string{"default", "default"}})
	require.False(t, res.IsError, textOfResult(t, res))
	assert.Len(t, structured(t, res)["items"], 2, "several targets are the items of one document, as in the command")

	res = call(t, session, "token_report", map[string]any{"working_directory": dir, "role": "r", "profile": "p"})
	assert.True(t, res.IsError, "role excludes profile")

	res = call(t, session, "cost_report", map[string]any{"working_directory": dir})
	require.False(t, res.IsError, textOfResult(t, res))
	assert.NotEmpty(t, structured(t, res)["items"])

	res = call(t, session, "cost_report", map[string]any{"working_directory": dir, "budget": 1})
	assert.True(t, res.IsError)
}

func TestSBOMIsReproducibleAndReadOnly(t *testing.T) {
	t.Parallel()
	session := connect(t, NewServer("test", WithAnyDirectory()))
	dir := telemetryProject(t)

	first := call(t, session, "sbom", map[string]any{"working_directory": dir})
	require.False(t, first.IsError, textOfResult(t, first))
	second := call(t, session, "sbom", map[string]any{"working_directory": dir})

	assert.Equal(t, textOfResult(t, first), textOfResult(t, second), "byte-identical across runs")
	assert.Equal(t, "CycloneDX", structured(t, first)["bomFormat"])
	spdx := call(t, session, "sbom", map[string]any{"working_directory": dir, "format": "spdx-json"})
	require.False(t, spdx.IsError, textOfResult(t, spdx))
	assert.NotEmpty(t, structured(t, spdx)["spdxVersion"])
	assert.NoFileExists(t, filepath.Join(dir, ".ai-rulez", "ai-rulez.lock"), "sbom must not write the lock")
}

func TestOKFValidateReadsLocalDirectoriesOnly(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	srv := NewServer("test", WithRoot(root))
	session := connect(t, srv)
	require.NoError(t, os.MkdirAll(filepath.Join(root, "bundle"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(root, "bundle", "index.md"), []byte("---\nokf_version: \"0.2\"\n---\n# Bundle\n"), 0o600))

	res := call(t, session, "okf_validate", map[string]any{"bundle": "bundle"})
	assert.NotNil(t, res.StructuredContent, textOfResult(t, res))
	assert.Contains(t, structured(t, res), "findings")

	for name, bundle := range map[string]string{
		"a git url":        "https://example.com/bundle.git",
		"an scp url":       "git@example.com:org/bundle.git",
		"outside the root": "../outside",
		"a missing path":   "nope",
	} {
		t.Run(name, func(t *testing.T) {
			res := call(t, session, "okf_validate", map[string]any{"bundle": bundle})
			assert.True(t, res.IsError)
		})
	}
}

func TestApprovalsStatusNeedsALock(t *testing.T) {
	t.Parallel()
	session := connect(t, NewServer("test", WithAnyDirectory()))
	dir := telemetryProject(t)

	res := call(t, session, "approvals_status", map[string]any{"working_directory": dir})

	require.True(t, res.IsError)
	assert.Contains(t, textOfResult(t, res), "ai-rulez lock")
}

func TestPolicyShowWithoutADiscoveryEngineSaysSo(t *testing.T) {
	t.Parallel()
	session := connect(t, NewServer("test", WithAnyDirectory()))

	res := call(t, session, "policy_show", map[string]any{"working_directory": telemetryProject(t)})

	assert.True(t, res.IsError)
}
