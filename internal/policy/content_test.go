package policy

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// contentFile builds a content file from frontmatter, like the loader does.
func contentFile(t *testing.T, name, path, frontmatter string) config.ContentFile {
	t.Helper()
	meta, _ := config.ParseFrontmatterPublic("---\nname: " + name + "\n" + frontmatter + "---\nbody\n")
	require.NotNil(t, meta)
	return config.ContentFile{Name: name, Path: path, Metadata: meta}
}

const hookFrontmatter = "hooks:\n  PreToolUse:\n    - matcher: Bash\n      hooks:\n        - type: command\n          command: ./check.sh\n"

func contentConfig(t *testing.T, agents ...config.ContentFile) *config.Config {
	t.Helper()
	root := t.TempDir()
	return &config.Config{
		BaseDir: root, ConfigDir: filepath.Join(root, ".ai-rulez"),
		Content: &config.ContentTree{Agents: agents},
	}
}

func TestApplyContent_HooksForbidden(t *testing.T) {
	// Arrange: an agent from an include and an authored one both declare hooks
	root := t.TempDir()
	own := contentFile(t, "own", filepath.Join(root, ".ai-rulez", "agents", "own.md"), hookFrontmatter)
	imported := contentFile(t, "shared", filepath.Join(t.TempDir(), "agents", "shared.md"), hookFrontmatter)
	cfg := &config.Config{BaseDir: root, ConfigDir: filepath.Join(root, ".ai-rulez"), Content: &config.ContentTree{Agents: []config.ContentFile{own, imported}}}
	res := Resolve([]Layer{layer("managed", Policy{Hooks: Hooks{Forbidden: true}})})

	// Act
	got := res.ApplyContent(cfg)

	// Assert
	require.Len(t, got, 1)
	assert.Equal(t, "AR748", got[0].Code)
	assert.Equal(t, "hooks.allow", got[0].Key)
	assert.Equal(t, imported.Path, got[0].File)
	assert.Contains(t, got[0].Message, "shared")
	_, ownHas := cfg.Content.Agents[0].Metadata.TypedExtra("hooks")
	_, importedHas := cfg.Content.Agents[1].Metadata.TypedExtra("hooks")
	assert.True(t, ownHas, "the repository's own content is bounded by its config.toml, not by this check")
	assert.False(t, importedHas, "the imported hooks are not loaded")
}

func TestApplyContent_MCPServers(t *testing.T) {
	remote := "mcpServers:\n  docs:\n    url: https://docs.example.org/mcp\n"
	sse := "mcpServers:\n  feed:\n    type: sse\n    url: https://feed.example.org/sse\n"
	local := "mcpServers:\n  helper:\n    command: npx\n    args: [\"-y\", \"helper@1.0.0\"]\n"
	rogue := "mcpServers:\n  helper:\n    command: bash\n"
	byName := "mcpServers:\n  - docs\n"
	tests := []struct {
		name        string
		frontmatter string
		policy      MCP
		wantKey     string
		wantDropped bool
	}{
		{"a denied transport", remote, MCP{DenyTransports: []string{"http"}}, "mcp.deny_transports", true},
		{"a denied sse transport", sse, MCP{DenyTransports: []string{"sse"}}, "mcp.deny_transports", true},
		{"an allowed transport", remote, MCP{DenyTransports: []string{"sse"}}, "", false},
		{"a command outside the list", rogue, MCP{AllowedCommands: List{Set: true, Items: []string{"npx"}}}, "mcp.allowed_commands", true},
		{"a command inside the list", local, MCP{AllowedCommands: List{Set: true, Items: []string{"npx"}}}, "", false},
		{"a server named elsewhere is skipped", byName, MCP{DenyTransports: []string{"http"}}, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			skill := contentFile(t, "vendor", filepath.Join(t.TempDir(), "skills", "vendor", "SKILL.md"), tt.frontmatter)
			cfg := contentConfig(t)
			cfg.Content.Skills = []config.ContentFile{skill}
			res := Resolve([]Layer{layer("managed", Policy{MCP: tt.policy})})

			// Act
			got := res.ApplyContent(cfg)

			// Assert
			_, still := cfg.Content.Skills[0].Metadata.TypedExtra("mcpServers")
			assert.Equal(t, !tt.wantDropped, still)
			if tt.wantKey == "" {
				assert.Empty(t, got)
				return
			}
			require.Len(t, got, 1)
			assert.Equal(t, "AR748", got[0].Code)
			assert.Equal(t, tt.wantKey, got[0].Key)
		})
	}
}

func TestApplyContent_LocalIncludeInsideTheProjectIsStillImported(t *testing.T) {
	// Arrange: ./shared is an include, so its content is foreign although it lives in the project
	root := t.TempDir()
	imported := contentFile(t, "shared", filepath.Join(root, "shared", ".ai-rulez", "agents", "shared.md"), hookFrontmatter)
	cfg := &config.Config{
		BaseDir: root, ConfigDir: filepath.Join(root, ".ai-rulez"),
		Includes: []config.IncludeConfig{{Name: "shared", Source: "./shared"}},
		Content:  &config.ContentTree{Agents: []config.ContentFile{imported}},
	}
	res := Resolve([]Layer{layer("managed", Policy{Hooks: Hooks{Forbidden: true}})})

	// Act
	got := res.ApplyContent(cfg)

	// Assert
	require.Len(t, got, 1)
	assert.Equal(t, "hooks.allow", got[0].Key)
}

func TestApplyContent_NoPolicyNoChange(t *testing.T) {
	// Arrange
	agent := contentFile(t, "shared", filepath.Join(t.TempDir(), "agents", "shared.md"), hookFrontmatter)
	cfg := contentConfig(t, agent)
	res := Resolve([]Layer{layer("managed", Policy{})})

	// Act
	got := res.ApplyContent(cfg)

	// Assert
	assert.Empty(t, got)
	_, has := cfg.Content.Agents[0].Metadata.TypedExtra("hooks")
	assert.True(t, has)
}
