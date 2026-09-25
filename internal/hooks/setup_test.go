package hooks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestSetupLefthook(t *testing.T) {
	t.Run("adds ai-rulez command to existing config", func(t *testing.T) {
		dir := chdirTemp(t)
		content := `pre-commit:
  commands:
    lint:
      run: npm run lint
`
		require.NoError(t, os.WriteFile(filepath.Join(dir, "lefthook.yaml"), []byte(content), 0o644))

		require.NoError(t, setupLefthook())

		data, err := os.ReadFile(filepath.Join(dir, "lefthook.yaml"))
		require.NoError(t, err)

		result := string(data)
		assert.Contains(t, result, "ai-rulez")
		assert.Contains(t, result, "ai-rulez validate")
		assert.Contains(t, result, ".ai-rulez/**")
	})

	t.Run("creates pre-commit section if missing", func(t *testing.T) {
		dir := chdirTemp(t)
		content := `post-commit:
  commands:
    notify:
      run: echo done
`
		require.NoError(t, os.WriteFile(filepath.Join(dir, "lefthook.yml"), []byte(content), 0o644))

		require.NoError(t, setupLefthook())

		data, err := os.ReadFile(filepath.Join(dir, "lefthook.yml"))
		require.NoError(t, err)

		result := string(data)
		assert.Contains(t, result, "ai-rulez validate")
	})

	t.Run("idempotent - does not duplicate on second run", func(t *testing.T) {
		dir := chdirTemp(t)
		content := `pre-commit:
  commands:
    lint:
      run: npm run lint
`
		require.NoError(t, os.WriteFile(filepath.Join(dir, "lefthook.yaml"), []byte(content), 0o644))

		require.NoError(t, setupLefthook())
		require.NoError(t, setupLefthook())

		data, err := os.ReadFile(filepath.Join(dir, "lefthook.yaml"))
		require.NoError(t, err)

		assert.Equal(t, 1, strings.Count(string(data), "ai-rulez validate"))
	})

	t.Run("errors when no config file found", func(t *testing.T) {
		_ = chdirTemp(t)
		err := setupLefthook()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}

func TestSetupPreCommit(t *testing.T) {
	t.Run("adds official repo to existing config", func(t *testing.T) {
		dir := chdirTemp(t)
		content := `repos:
  - repo: https://github.com/pre-commit/pre-commit-hooks
    rev: v4.0.0
    hooks:
      - id: trailing-whitespace
`
		require.NoError(t, os.WriteFile(filepath.Join(dir, ".pre-commit-config.yaml"), []byte(content), 0o644))

		require.NoError(t, setupPreCommit())

		data, err := os.ReadFile(filepath.Join(dir, ".pre-commit-config.yaml"))
		require.NoError(t, err)

		var config map[string]interface{}
		require.NoError(t, yaml.Unmarshal(data, &config))

		repos := config["repos"].([]interface{})
		assert.Len(t, repos, 2)

		lastRepo := repos[1].(map[string]interface{})
		assert.Equal(t, officialPreCommitRepo, lastRepo["repo"])
		assert.Equal(t, officialPreCommitRev, lastRepo["rev"])

		hooks := lastRepo["hooks"].([]interface{})
		hookIDs := make([]string, 0, len(hooks))
		for _, h := range hooks {
			hookIDs = append(hookIDs, h.(map[string]interface{})["id"].(string))
		}
		assert.Contains(t, hookIDs, "ai-rulez-validate")
		assert.Contains(t, hookIDs, "ai-rulez-generate")
	})

	t.Run("updates rev on existing official repo", func(t *testing.T) {
		dir := chdirTemp(t)
		content := `repos:
  - repo: https://github.com/Goldziher/ai-rulez
    rev: v1.0.0
    hooks:
      - id: ai-rulez-validate
`
		require.NoError(t, os.WriteFile(filepath.Join(dir, ".pre-commit-config.yaml"), []byte(content), 0o644))

		require.NoError(t, setupPreCommit())

		data, err := os.ReadFile(filepath.Join(dir, ".pre-commit-config.yaml"))
		require.NoError(t, err)

		var config map[string]interface{}
		require.NoError(t, yaml.Unmarshal(data, &config))

		repos := config["repos"].([]interface{})
		assert.Len(t, repos, 1)

		repo := repos[0].(map[string]interface{})
		assert.Equal(t, officialPreCommitRev, repo["rev"])
	})

	t.Run("adds missing hooks to existing official repo", func(t *testing.T) {
		dir := chdirTemp(t)
		content := `repos:
  - repo: https://github.com/Goldziher/ai-rulez
    rev: v2.0.0
    hooks:
      - id: ai-rulez-validate
`
		require.NoError(t, os.WriteFile(filepath.Join(dir, ".pre-commit-config.yaml"), []byte(content), 0o644))

		require.NoError(t, setupPreCommit())

		data, err := os.ReadFile(filepath.Join(dir, ".pre-commit-config.yaml"))
		require.NoError(t, err)

		var config map[string]interface{}
		require.NoError(t, yaml.Unmarshal(data, &config))

		repos := config["repos"].([]interface{})
		repo := repos[0].(map[string]interface{})
		hooks := repo["hooks"].([]interface{})

		hookIDs := make([]string, 0, len(hooks))
		for _, h := range hooks {
			hookIDs = append(hookIDs, h.(map[string]interface{})["id"].(string))
		}
		assert.Contains(t, hookIDs, "ai-rulez-validate")
		assert.Contains(t, hookIDs, "ai-rulez-generate")
	})

	t.Run("handles empty config file", func(t *testing.T) {
		dir := chdirTemp(t)
		require.NoError(t, os.WriteFile(filepath.Join(dir, ".pre-commit-config.yaml"), []byte(""), 0o644))

		require.NoError(t, setupPreCommit())

		data, err := os.ReadFile(filepath.Join(dir, ".pre-commit-config.yaml"))
		require.NoError(t, err)

		var config map[string]interface{}
		require.NoError(t, yaml.Unmarshal(data, &config))

		repos := config["repos"].([]interface{})
		assert.Len(t, repos, 1)
	})

	t.Run("idempotent - does not duplicate hooks", func(t *testing.T) {
		dir := chdirTemp(t)
		content := `repos: []
`
		require.NoError(t, os.WriteFile(filepath.Join(dir, ".pre-commit-config.yaml"), []byte(content), 0o644))

		require.NoError(t, setupPreCommit())
		require.NoError(t, setupPreCommit())

		data, err := os.ReadFile(filepath.Join(dir, ".pre-commit-config.yaml"))
		require.NoError(t, err)

		var config map[string]interface{}
		require.NoError(t, yaml.Unmarshal(data, &config))

		repos := config["repos"].([]interface{})
		assert.Len(t, repos, 1, "should not duplicate the official repo")

		repo := repos[0].(map[string]interface{})
		hooks := repo["hooks"].([]interface{})
		assert.Len(t, hooks, 2, "should not duplicate hooks")
	})

	t.Run("errors when no config file found", func(t *testing.T) {
		_ = chdirTemp(t)
		err := setupPreCommit()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}

func TestPruneLegacyLocalHooks(t *testing.T) {
	t.Run("removes legacy local ai-rulez hook", func(t *testing.T) {
		// Create a repos sequence node with a local repo
		reposNode := &yaml.Node{Kind: yaml.SequenceNode}

		localRepo := &yaml.Node{Kind: yaml.MappingNode}
		setMapValue(localRepo, "repo", "local")

		hooksKey := &yaml.Node{Kind: yaml.ScalarNode, Value: "hooks"}
		hooksValue := &yaml.Node{Kind: yaml.SequenceNode}

		// Add two hooks: one ai-rulez (to be removed) and one other-hook (to keep)
		aiRulezHook := &yaml.Node{Kind: yaml.MappingNode}
		setMapValue(aiRulezHook, "id", "ai-rulez")
		setMapValue(aiRulezHook, "name", "legacy")

		otherHook := &yaml.Node{Kind: yaml.MappingNode}
		setMapValue(otherHook, "id", "other-hook")
		setMapValue(otherHook, "name", "keep")

		hooksValue.Content = []*yaml.Node{aiRulezHook, otherHook}
		localRepo.Content = append(localRepo.Content, hooksKey, hooksValue)
		reposNode.Content = []*yaml.Node{localRepo}

		pruneLegacyLocalHooksNode(reposNode)

		// Verify only one hook remains
		resultHooks := localRepo.Content[3] // hooks value node
		assert.Len(t, resultHooks.Content, 1)
		assert.Equal(t, "other-hook", getMapValue(resultHooks.Content[0], "id"))
	})

	t.Run("leaves non-local repos untouched", func(t *testing.T) {
		reposNode := &yaml.Node{Kind: yaml.SequenceNode}

		repo := &yaml.Node{Kind: yaml.MappingNode}
		setMapValue(repo, "repo", "https://example.com/repo")

		hooksKey := &yaml.Node{Kind: yaml.ScalarNode, Value: "hooks"}
		hooksValue := &yaml.Node{Kind: yaml.SequenceNode}

		hook := &yaml.Node{Kind: yaml.MappingNode}
		setMapValue(hook, "id", "ai-rulez")

		hooksValue.Content = []*yaml.Node{hook}
		repo.Content = append(repo.Content, hooksKey, hooksValue)
		reposNode.Content = []*yaml.Node{repo}

		pruneLegacyLocalHooksNode(reposNode)

		// Verify hook still exists
		resultHooks := repo.Content[3] // hooks value node
		assert.Len(t, resultHooks.Content, 1)
	})
}

func TestSetupHusky(t *testing.T) {
	t.Run("creates pre-commit hook in existing .husky dir", func(t *testing.T) {
		dir := chdirTemp(t)
		require.NoError(t, os.MkdirAll(filepath.Join(dir, ".husky"), 0o755))

		require.NoError(t, setupHusky())

		data, err := os.ReadFile(filepath.Join(dir, ".husky", "pre-commit"))
		require.NoError(t, err)

		content := string(data)
		assert.Contains(t, content, "npx ai-rulez validate")
		assert.Contains(t, content, "#!/usr/bin/env sh")
	})

	t.Run("appends to existing pre-commit hook", func(t *testing.T) {
		dir := chdirTemp(t)
		huskyDir := filepath.Join(dir, ".husky")
		require.NoError(t, os.MkdirAll(huskyDir, 0o755))

		existing := "#!/usr/bin/env sh\nnpm run lint\n"
		require.NoError(t, os.WriteFile(filepath.Join(huskyDir, "pre-commit"), []byte(existing), 0o755))

		require.NoError(t, setupHusky())

		data, err := os.ReadFile(filepath.Join(huskyDir, "pre-commit"))
		require.NoError(t, err)

		content := string(data)
		assert.Contains(t, content, "npm run lint")
		assert.Contains(t, content, "npx ai-rulez validate")
	})

	t.Run("idempotent - does not duplicate on second run", func(t *testing.T) {
		dir := chdirTemp(t)
		require.NoError(t, os.MkdirAll(filepath.Join(dir, ".husky"), 0o755))

		require.NoError(t, setupHusky())
		require.NoError(t, setupHusky())

		data, err := os.ReadFile(filepath.Join(dir, ".husky", "pre-commit"))
		require.NoError(t, err)

		assert.Equal(t, 1, strings.Count(string(data), "ai-rulez validate"))
	})

	t.Run("errors when .husky directory missing", func(t *testing.T) {
		_ = chdirTemp(t)
		err := setupHusky()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}

func TestSetupHooks(t *testing.T) {
	t.Run("errors when no hook system detected", func(t *testing.T) {
		_ = chdirTemp(t)
		err := SetupHooks()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no git hook system detected")
	})

	t.Run("routes to lefthook when detected", func(t *testing.T) {
		dir := chdirTemp(t)
		content := `pre-commit:
  commands: {}
`
		require.NoError(t, os.WriteFile(filepath.Join(dir, "lefthook.yml"), []byte(content), 0o644))

		require.NoError(t, SetupHooks())

		data, err := os.ReadFile(filepath.Join(dir, "lefthook.yml"))
		require.NoError(t, err)
		assert.Contains(t, string(data), "ai-rulez validate")
	})
}

func TestSetupLefthookPreservesComments(t *testing.T) {
	t.Run("preserves comments and formatting", func(t *testing.T) {
		dir := chdirTemp(t)
		content := `# Lefthook config for acme-monorepo
# Owner: platform-team  (see docs/hooks.md)

pre-commit:
  parallel: true          # run all commands concurrently
  commands:
    # Go: format + vet the staged packages only
    gofmt:
      glob: "*.go"
      run: gofmt -l {staged_files}

    # Python
    ruff:
      glob: "*.py"
      run: uv run ruff check {staged_files}

commit-msg:
  commands:
    conventional:
      run: npx commitlint --edit {1}
`
		require.NoError(t, os.WriteFile(filepath.Join(dir, "lefthook.yaml"), []byte(content), 0o644))

		require.NoError(t, setupLefthook())

		data, err := os.ReadFile(filepath.Join(dir, "lefthook.yaml"))
		require.NoError(t, err)

		result := string(data)

		// Must contain the new ai-rulez command
		assert.Contains(t, result, "ai-rulez")
		assert.Contains(t, result, "ai-rulez validate")

		// Must preserve all comments
		assert.Contains(t, result, "# Lefthook config for acme-monorepo")
		assert.Contains(t, result, "# Owner: platform-team")
		assert.Contains(t, result, "# run all commands concurrently")
		assert.Contains(t, result, "# Go: format + vet")
		assert.Contains(t, result, "# Python")

		// Must preserve key order (pre-commit before commit-msg)
		preCommitIdx := strings.Index(result, "pre-commit:")
		commitMsgIdx := strings.Index(result, "commit-msg:")
		assert.Less(t, preCommitIdx, commitMsgIdx, "pre-commit should appear before commit-msg")

		// Must preserve parallel setting position (should be at same level as commands, not relocated)
		assert.Contains(t, result, "parallel:")

		// Must preserve the document's two-space indentation. yaml.Marshal
		// hardcodes four spaces, which re-indents every line of the file: the
		// comments survive but the diff still rewrites the whole document.
		assert.Contains(t, result, "\npre-commit:\n  parallel:",
			"top-level nesting must stay at two spaces, not be re-indented to four")
		assert.Contains(t, result, "\n    gofmt:\n      glob:",
			"nested mapping keys must keep the original indentation width")
		assert.NotContains(t, result, "\n    parallel:",
			"parallel must not be re-indented to four spaces")
	})

	t.Run("preserves a four-space document at four spaces", func(t *testing.T) {
		dir := chdirTemp(t)
		content := `# Four-space house style
pre-commit:
    parallel: true
    commands:
        gofmt:
            glob: "*.go"
            run: gofmt -l {staged_files}
`
		require.NoError(t, os.WriteFile(filepath.Join(dir, "lefthook.yaml"), []byte(content), 0o644))

		require.NoError(t, setupLefthook())

		data, err := os.ReadFile(filepath.Join(dir, "lefthook.yaml"))
		require.NoError(t, err)
		result := string(data)

		assert.Contains(t, result, "# Four-space house style")
		assert.Contains(t, result, "\npre-commit:\n    parallel:",
			"a four-space document must not be narrowed to two spaces")
		assert.Contains(t, result, "\n    commands:\n        gofmt:",
			"nested keys must keep the original four-space width")
		assert.Contains(t, result, "ai-rulez validate")
	})

	t.Run("idempotent with comments", func(t *testing.T) {
		dir := chdirTemp(t)
		content := `# Header comment
pre-commit:
  commands:
    lint:
      run: npm run lint
`
		require.NoError(t, os.WriteFile(filepath.Join(dir, "lefthook.yaml"), []byte(content), 0o644))

		require.NoError(t, setupLefthook())
		firstRun, err := os.ReadFile(filepath.Join(dir, "lefthook.yaml"))
		require.NoError(t, err)

		require.NoError(t, setupLefthook())
		secondRun, err := os.ReadFile(filepath.Join(dir, "lefthook.yaml"))
		require.NoError(t, err)

		assert.Equal(t, string(firstRun), string(secondRun), "second run should not modify the file")
		assert.Contains(t, string(secondRun), "# Header comment", "comment must be preserved")
	})
}

func TestSetupPreCommitPreservesComments(t *testing.T) {
	t.Run("preserves comments and formatting", func(t *testing.T) {
		dir := chdirTemp(t)
		content := `# Pre-commit hooks configuration
# Maintained by: platform-team

repos:
  # Standard pre-commit hooks
  - repo: https://github.com/pre-commit/pre-commit-hooks
    rev: v4.0.0
    hooks:
      - id: trailing-whitespace  # Remove trailing whitespace
      - id: end-of-file-fixer
`
		require.NoError(t, os.WriteFile(filepath.Join(dir, ".pre-commit-config.yaml"), []byte(content), 0o644))

		require.NoError(t, setupPreCommit())

		data, err := os.ReadFile(filepath.Join(dir, ".pre-commit-config.yaml"))
		require.NoError(t, err)

		result := string(data)

		// Must contain the new ai-rulez repo
		assert.Contains(t, result, officialPreCommitRepo)
		assert.Contains(t, result, "ai-rulez-validate")

		// Must preserve all comments
		assert.Contains(t, result, "# Pre-commit hooks configuration")
		assert.Contains(t, result, "# Maintained by: platform-team")
		assert.Contains(t, result, "# Standard pre-commit hooks")
		assert.Contains(t, result, "# Remove trailing whitespace")
	})

	t.Run("idempotent with comments", func(t *testing.T) {
		dir := chdirTemp(t)
		content := `# Header
repos:
  - repo: https://github.com/pre-commit/pre-commit-hooks
    rev: v4.0.0
    hooks:
      - id: trailing-whitespace
`
		require.NoError(t, os.WriteFile(filepath.Join(dir, ".pre-commit-config.yaml"), []byte(content), 0o644))

		require.NoError(t, setupPreCommit())
		firstRun, err := os.ReadFile(filepath.Join(dir, ".pre-commit-config.yaml"))
		require.NoError(t, err)

		require.NoError(t, setupPreCommit())
		secondRun, err := os.ReadFile(filepath.Join(dir, ".pre-commit-config.yaml"))
		require.NoError(t, err)

		assert.Equal(t, string(firstRun), string(secondRun), "second run should not modify the file")
		assert.Contains(t, string(secondRun), "# Header", "comment must be preserved")
	})
}

// expectedLefthookCommandBlock is the exact YAML the ai-rulez command must emit
// into a two-space document. Asserting the whole block rather than its pieces is
// the point: an unordered emission still contains every substring.
const expectedLefthookCommandBlock = `    ai-rulez:
      glob: .ai-rulez/**
      run: ai-rulez validate
      fail_text: AI rules validation failed
`

// TestSetupLefthookEmitsFixedFieldOrder guards the promise that the rewrite
// preserves key order: a map-backed emission reorders glob/run/fail_text on
// every invocation, so two developers setting up the same repo get different
// diffs and a regenerate-and-diff CI job flaps.
func TestSetupLefthookEmitsFixedFieldOrder(t *testing.T) {
	// Go randomizes map iteration per range, so a single emission can match by
	// luck. Repeating makes an unordered implementation fail deterministically.
	const emissions = 20

	content := `pre-commit:
  commands:
    lint:
      run: npm run lint
`

	for i := range emissions {
		dir := chdirTemp(t)
		require.NoError(t, os.WriteFile(filepath.Join(dir, "lefthook.yaml"), []byte(content), 0o644))

		require.NoError(t, setupLefthook())

		data, err := os.ReadFile(filepath.Join(dir, "lefthook.yaml"))
		require.NoError(t, err)

		assert.Equal(t, `pre-commit:
  commands:
    lint:
      run: npm run lint
`+expectedLefthookCommandBlock, string(data), "emission %d must match byte for byte", i)
	}
}

// TestSetupLefthookFillsEmptySections covers placeholder sections: a bare
// `pre-commit:` or `commands:` key is valid YAML and valid lefthook, so it must
// be filled in rather than refused.
func TestSetupLefthookFillsEmptySections(t *testing.T) {
	t.Run("fills an empty pre-commit section", func(t *testing.T) {
		dir := chdirTemp(t)
		require.NoError(t, os.WriteFile(filepath.Join(dir, "lefthook.yaml"), []byte("pre-commit:\n"), 0o644))

		require.NoError(t, setupLefthook())

		data, err := os.ReadFile(filepath.Join(dir, "lefthook.yaml"))
		require.NoError(t, err)

		assert.Equal(t, "pre-commit:\n  commands:\n"+expectedLefthookCommandBlock, string(data))
		assert.NotContains(t, string(data), "!!", "no explicit tag may leak into the output")
	})

	t.Run("fills an empty commands section", func(t *testing.T) {
		dir := chdirTemp(t)
		content := `pre-commit:
  parallel: true
  commands:
`
		require.NoError(t, os.WriteFile(filepath.Join(dir, "lefthook.yaml"), []byte(content), 0o644))

		require.NoError(t, setupLefthook())

		data, err := os.ReadFile(filepath.Join(dir, "lefthook.yaml"))
		require.NoError(t, err)

		assert.Equal(t, "pre-commit:\n  parallel: true\n  commands:\n"+expectedLefthookCommandBlock, string(data))
		assert.NotContains(t, string(data), "!!", "no explicit tag may leak into the output")
	})

	t.Run("still rejects a pre-commit section holding a real scalar", func(t *testing.T) {
		dir := chdirTemp(t)
		content := `pre-commit: "a string"
`
		require.NoError(t, os.WriteFile(filepath.Join(dir, "lefthook.yaml"), []byte(content), 0o644))

		err := setupLefthook()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "pre-commit must be a mapping")

		data, readErr := os.ReadFile(filepath.Join(dir, "lefthook.yaml"))
		require.NoError(t, readErr)
		assert.Equal(t, content, string(data), "user data must not be discarded")
	})

	t.Run("still rejects a commands section holding a real scalar", func(t *testing.T) {
		dir := chdirTemp(t)
		content := `pre-commit:
  commands: nope
`
		require.NoError(t, os.WriteFile(filepath.Join(dir, "lefthook.yaml"), []byte(content), 0o644))

		err := setupLefthook()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "commands must be a mapping")
	})
}

// TestSetupPreCommitFillsEmptySections covers bare `repos:` and bare `hooks:`
// keys. Their value is a `!!null` scalar, and flipping only Kind leaves that tag
// on a sequence node, which the encoder then has to write out explicitly.
func TestSetupPreCommitFillsEmptySections(t *testing.T) {
	t.Run("fills a bare repos key", func(t *testing.T) {
		dir := chdirTemp(t)
		require.NoError(t, os.WriteFile(filepath.Join(dir, ".pre-commit-config.yaml"), []byte("repos:\n"), 0o644))

		require.NoError(t, setupPreCommit())

		data, err := os.ReadFile(filepath.Join(dir, ".pre-commit-config.yaml"))
		require.NoError(t, err)

		assert.NotContains(t, string(data), "!!", "no explicit tag may leak into the output")

		var config map[string][]map[string]any
		require.NoError(t, yaml.Unmarshal(data, &config))
		require.Len(t, config["repos"], 1)
		assert.Equal(t, officialPreCommitRepo, config["repos"][0]["repo"])
		assert.Equal(t, officialPreCommitRev, config["repos"][0]["rev"])
		assert.Equal(t, []any{map[string]any{"id": "ai-rulez-validate"}, map[string]any{"id": "ai-rulez-generate"}},
			config["repos"][0]["hooks"])
	})

	t.Run("fills a bare hooks key on the official repo", func(t *testing.T) {
		dir := chdirTemp(t)
		content := `repos:
  - repo: https://github.com/Goldziher/ai-rulez
    rev: v1.0.0
    hooks:
`
		require.NoError(t, os.WriteFile(filepath.Join(dir, ".pre-commit-config.yaml"), []byte(content), 0o644))

		require.NoError(t, setupPreCommit())

		data, err := os.ReadFile(filepath.Join(dir, ".pre-commit-config.yaml"))
		require.NoError(t, err)

		assert.NotContains(t, string(data), "!!", "no explicit tag may leak into the output")

		var config map[string][]map[string]any
		require.NoError(t, yaml.Unmarshal(data, &config))
		require.Len(t, config["repos"], 1)
		assert.Equal(t, []any{map[string]any{"id": "ai-rulez-validate"}, map[string]any{"id": "ai-rulez-generate"}},
			config["repos"][0]["hooks"])
	})
}

// TestSetupPreCommitRewritesNonStringRev covers revs YAML resolves to a non-string
// type. Overwriting such a node's value in place keeps the tag yaml.v3 inferred
// while parsing, which it then writes out explicitly as `rev: !!int v4.11.5` — a
// document pre-commit rejects.
func TestSetupPreCommitRewritesNonStringRev(t *testing.T) {
	for name, rev := range map[string]string{
		"integer rev": "24",
		"float rev":   "1.0",
		"boolean rev": "true",
	} {
		t.Run(name, func(t *testing.T) {
			dir := chdirTemp(t)
			content := "repos:\n  - repo: " + officialPreCommitRepo + "\n    rev: " + rev + "\n    hooks:\n      - id: ai-rulez-validate\n      - id: ai-rulez-generate\n"
			require.NoError(t, os.WriteFile(filepath.Join(dir, ".pre-commit-config.yaml"), []byte(content), 0o644))

			require.NoError(t, setupPreCommit())

			data, err := os.ReadFile(filepath.Join(dir, ".pre-commit-config.yaml"))
			require.NoError(t, err)

			assert.NotContains(t, string(data), "!!", "no explicit tag may leak into the output")

			var config map[string][]map[string]any
			require.NoError(t, yaml.Unmarshal(data, &config))
			require.Len(t, config["repos"], 1)
			assert.Equal(t, officialPreCommitRev, config["repos"][0]["rev"])
		})
	}
}

// TestDetectYAMLIndent pins the width inference directly, including the cases
// the round-trip tests cannot reach: tab-indented and single-line documents.
func TestDetectYAMLIndent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		content  string
		expected int
	}{
		{"two-space document", "a:\n  b: 1\n", 2},
		{"four-space document", "a:\n    b: 1\n", 4},
		{"three-space document", "a:\n   b: 1\n", 3},
		{"flat document falls back to two", "a: 1\nb: 2\n", 2},
		{"empty document falls back to two", "", 2},
		{"comments do not set the width", "a:\n        # deep comment\n  b: 1\n", 2},
		{"tab indentation is ignored and falls back", "a:\n\tb: 1\n", 2},
		{"smallest nesting level wins", "a:\n  b:\n    c: 1\n", 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, detectYAMLIndent([]byte(tt.content)))
		})
	}
}
