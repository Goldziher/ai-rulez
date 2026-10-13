package crud_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/crud"
)

const agentsCRUDConfig = "version = \"5.0\"\nname = \"demo\"\npresets = [\"claude\"]\n"

// agentsProjects are the two places an "agents" source can be configured.
var agentsProjects = []struct {
	name       string
	configPath string
	configBody string
	stateDir   string
}{
	{"config in .agents", ".agents/ai-rulez.toml", agentsCRUDConfig, ".agents/.ai-rulez"},
	{"source key in .ai-rulez", ".ai-rulez/config.toml", agentsCRUDConfig + "source = \"agents\"\n", ".ai-rulez"},
}

func agentsProject(t *testing.T, configPath, configBody string) string {
	t.Helper()
	root := t.TempDir()
	full := filepath.Join(root, filepath.FromSlash(configPath))
	require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
	require.NoError(t, os.WriteFile(full, []byte(configBody), 0o644))
	return root
}

func TestOperatorWritesContentToTheAgentsTree(t *testing.T) {
	for _, p := range agentsProjects {
		t.Run(p.name, func(t *testing.T) {
			// Arrange
			root := agentsProject(t, p.configPath, p.configBody)
			op, err := crud.NewOperator(root)
			require.NoError(t, err)
			ctx := context.Background()

			// Act
			rule, ruleErr := op.AddRule(ctx, &crud.AddFileRequest{Name: "style", Content: "# Style\n"})
			skill, skillErr := op.AddSkill(ctx, &crud.AddFileRequest{Name: "review", Content: "Read the diff.\n"})
			_, domainErr := op.AddDomain(ctx, &crud.AddDomainRequest{Name: "backend", Description: "Backend"})
			domainRule, domainRuleErr := op.AddRule(ctx, &crud.AddFileRequest{Name: "api", Domain: "backend", Content: "# API\n"})
			listed, listErr := op.ListFiles(ctx, "", "rules")

			// Assert
			require.NoError(t, ruleErr)
			require.NoError(t, skillErr)
			require.NoError(t, domainErr)
			require.NoError(t, domainRuleErr)
			require.NoError(t, listErr)
			assert.Equal(t, filepath.Join(root, ".agents", "rules", "style.md"), rule.FullPath)
			assert.Equal(t, filepath.Join(root, ".agents", "skills", "review", "SKILL.md"), skill.FullPath)
			assert.Equal(t, filepath.Join(root, ".agents", "domains", "backend", "rules", "api.md"), domainRule.FullPath)
			assert.FileExists(t, rule.FullPath)
			assert.Len(t, listed, 1)
			assert.NoDirExists(t, filepath.Join(root, ".ai-rulez", "rules"), "nothing lands in the unused content tree")
			domains, domainsErr := op.ListDomains(ctx)
			require.NoError(t, domainsErr)
			require.Len(t, domains, 1)
			assert.Equal(t, "backend", domains[0].Name)
		})
	}
}

func TestOperatorWritesConfigChangesToTheRightFile(t *testing.T) {
	for _, p := range agentsProjects {
		t.Run(p.name, func(t *testing.T) {
			// Arrange
			root := agentsProject(t, p.configPath, p.configBody)
			op, err := crud.NewOperator(root)
			require.NoError(t, err)
			ctx := context.Background()
			require.NoError(t, os.MkdirAll(filepath.Join(root, ".agents", "domains", "backend"), 0o755))

			// Act
			err = op.AddProfile(ctx, "team", []string{"backend"})

			// Assert
			require.NoError(t, err)
			data, readErr := os.ReadFile(filepath.Join(root, filepath.FromSlash(p.configPath)))
			require.NoError(t, readErr)
			assert.Contains(t, string(data), "team", "the profile is saved to the config that was loaded")
			assert.Equal(t, filepath.Join(root, filepath.FromSlash(p.configPath)), op.ConfigFile())
			assert.NoFileExists(t, filepath.Join(root, ".agents", "config.toml"))
			assert.NoFileExists(t, filepath.Join(root, ".agents", ".ai-rulez", "config.toml"))
		})
	}
}

func TestLocalOperatorKeepsMachineLocalContentInTheStateDirectory(t *testing.T) {
	for _, p := range agentsProjects {
		t.Run(p.name, func(t *testing.T) {
			// Arrange
			root := agentsProject(t, p.configPath, p.configBody)
			op, err := crud.NewOperator(root)
			require.NoError(t, err)

			// Act
			result, addErr := op.Local().AddRule(context.Background(), &crud.AddFileRequest{Name: "mine", Content: "# Mine\n"})

			// Assert
			require.NoError(t, addErr)
			assert.Equal(t, filepath.Join(root, filepath.FromSlash(p.stateDir), "local", "rules", "mine.md"), result.FullPath)
			assert.NoFileExists(t, filepath.Join(root, ".agents", "rules", "mine.md"), "personal content never enters the shared source")
			assert.Equal(t, filepath.Join(root, filepath.FromSlash(p.stateDir), "config.local.toml"), op.Local().ConfigFile())
		})
	}
}

func TestOperatorAtAcceptsEitherSpellingOfTheAgentsDirectory(t *testing.T) {
	for _, dir := range []string{".agents", ".agents/.ai-rulez"} {
		t.Run(dir, func(t *testing.T) {
			// Arrange
			root := agentsProject(t, ".agents/ai-rulez.toml", agentsCRUDConfig)
			require.NoError(t, os.MkdirAll(filepath.Join(root, ".agents", ".ai-rulez"), 0o755))

			// Act
			op, err := crud.NewOperatorAt(filepath.Join(root, filepath.FromSlash(dir)))
			require.NoError(t, err)
			result, addErr := op.AddRule(context.Background(), &crud.AddFileRequest{Name: "style", Content: "# Style\n"})

			// Assert
			require.NoError(t, addErr)
			assert.Equal(t, filepath.Join(root, ".agents", "rules", "style.md"), result.FullPath)
			assert.Equal(t, filepath.Join(root, ".agents"), op.ConfigDir())
		})
	}
}
