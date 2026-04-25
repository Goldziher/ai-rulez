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
		repos := []interface{}{
			map[string]interface{}{
				"repo": "local",
				"hooks": []interface{}{
					map[string]interface{}{"id": "ai-rulez", "name": "legacy"},
					map[string]interface{}{"id": "other-hook", "name": "keep"},
				},
			},
		}

		result := pruneLegacyLocalHooks(repos)
		localRepo := result[0].(map[string]interface{})
		hooks := localRepo["hooks"].([]interface{})

		assert.Len(t, hooks, 1)
		assert.Equal(t, "other-hook", hooks[0].(map[string]interface{})["id"])
	})

	t.Run("leaves non-local repos untouched", func(t *testing.T) {
		repos := []interface{}{
			map[string]interface{}{
				"repo": "https://example.com/repo",
				"hooks": []interface{}{
					map[string]interface{}{"id": "ai-rulez"},
				},
			},
		}

		result := pruneLegacyLocalHooks(repos)
		hooks := result[0].(map[string]interface{})["hooks"].([]interface{})
		assert.Len(t, hooks, 1)
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
