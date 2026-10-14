package generator

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

func gitCmd(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.email=t@example.com", "-c", "user.name=t"}, args...)...)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
}

func TestPluginVersionDrift(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	tests := []struct {
		name      string
		version   string
		bump      string // version after the content change; "" keeps it
		wantDrift bool
	}{
		{"content changed, version kept", "1.0.0", "", true},
		{"content changed, version bumped", "1.0.0", "1.1.0", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := newDomainsProject(t, driftTail(tt.version))
			require.NoError(t, loadDomainsProject(t, dir).GeneratePlugin(""))
			gitCmd(t, dir, "init", "-q")
			gitCmd(t, dir, "add", "-A")
			gitCmd(t, dir, "commit", "-q", "-m", "baseline")

			drift, err := loadDomainsProject(t, dir).PluginVersionDrift("")
			require.NoError(t, err)
			assert.Empty(t, drift, "nothing changed since the baseline")

			writeDomainsFile(t, filepath.Join(dir, ".ai-rulez", "domains", "teamA", "skills", "a-s", "SKILL.md"),
				"---\ndescription: a\n---\nchanged body\n")
			if tt.bump != "" {
				cfgPath := filepath.Join(dir, ".ai-rulez", "config.toml")
				data, err := os.ReadFile(cfgPath)
				require.NoError(t, err)
				require.NoError(t, os.WriteFile(cfgPath, []byte(replaceAll(string(data), `version = "`+tt.version+`"`+"\n", `version = "`+tt.bump+`"`+"\n")), 0o600))
			}

			drift, err = loadDomainsProject(t, dir).PluginVersionDrift("")
			require.NoError(t, err)
			if !tt.wantDrift {
				assert.Empty(t, drift)
				return
			}
			require.Len(t, drift, 1)
			assert.Equal(t, "demo-teama", drift[0].Plugin)
			assert.Equal(t, tt.version, drift[0].Version)
			assert.Contains(t, drift[0].Changed, "skills/a-s/SKILL.md")
		})
	}
}

// generate --plugin never reads machine-local inputs, so the drift check must not
// either: a personal skill would otherwise make a clean bundle look changed.
func TestPluginVersionDrift_IgnoresMachineLocalInputs(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	// Arrange
	dir := t.TempDir()
	writeDomainsFile(t, filepath.Join(dir, ".ai-rulez", "config.toml"),
		"version = \"5.0\"\nname = \"demo\"\npresets = [\"claude\"]\ngitignore = false\n\n"+
			"[plugin]\nname = \"demo\"\nversion = \"1.0.0\"\ndescription = \"demo plugin\"\nruntimes = [\"claude\", \"opencode\"]\n")
	writeDomainsFile(t, filepath.Join(dir, ".ai-rulez", "skills", "core-s", "SKILL.md"), "---\ndescription: core\n---\nbody\n")
	shared, err := config.LoadConfig(context.Background(), dir, config.WithoutLocal())
	require.NoError(t, err)
	require.NoError(t, NewGenerator(shared).GeneratePlugin(""))
	gitCmd(t, dir, "init", "-q")
	gitCmd(t, dir, "add", "-A")
	gitCmd(t, dir, "commit", "-q", "-m", "baseline")
	writeDomainsFile(t, filepath.Join(dir, ".ai-rulez", "local", "skills", "mine", "SKILL.md"),
		"---\ndescription: personal\n---\nmy own skill\n")
	writeDomainsFile(t, filepath.Join(dir, ".ai-rulez", "config.local.toml"),
		"[[mcp_servers]]\nname = \"loc\"\ncommand = \"echo\"\n")
	withLocal, err := config.LoadConfig(context.Background(), dir)
	require.NoError(t, err)
	require.True(t, withLocal.HasLocalInputs(), "the personal skill is loaded")

	// Act
	drift, err := NewGenerator(withLocal).PluginVersionDrift("")

	// Assert
	require.NoError(t, err)
	assert.Empty(t, drift, "the committed bundle is what generate --plugin writes")
}

func replaceAll(s, old, replacement string) string {
	out := ""
	for {
		i := indexOf(s, old)
		if i < 0 {
			return out + s
		}
		out += s[:i] + replacement
		s = s[i+len(old):]
	}
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func driftTail(version string) string {
	v := ""
	if version != "" {
		v = "version = \"" + version + "\"\n"
	}
	return `
[marketplace]
name = "mk"
output_dir = "mkt"
[marketplace.from_domains]
name_prefix = "demo-"
` + v
}

// #351: a plugin whose content root overlaps its output tree (content_root =
// ".") reads its own decorated output as the source. generate --plugin must
// rewrite the header, not stack another, so a second run leaves the tree
// unchanged and verify converges right after generate.
func TestGeneratePlugin_ContentRootOverlappingTheOutputIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	writeDomainsFile(t, filepath.Join(dir, ".ai-rulez", "config.toml"),
		"version = \"5.0\"\nname = \"overlap\"\ngitignore = false\n\n"+
			"[plugin]\nname = \"overlap\"\nversion = \"1.0.0\"\ndescription = \"d\"\n"+
			"runtimes = [\"agent-plugins\"]\ncontent_root = \".\"\n")
	writeDomainsFile(t, filepath.Join(dir, "skills", "alpha", "SKILL.md"),
		"---\nname: alpha\ndescription: a\n---\n\n# alpha\n\nBody.\n")
	load := func() *Generator {
		cfg, err := config.LoadConfig(context.Background(), dir, config.WithoutLocal())
		require.NoError(t, err)
		return NewGenerator(cfg)
	}

	require.NoError(t, load().GeneratePlugin(""))
	skill := filepath.Join(dir, "skills", "alpha", "SKILL.md")
	afterFirst := readFileString(t, skill)
	require.NoError(t, load().GeneratePlugin(""))

	assert.Equal(t, 1, strings.Count(readFileString(t, skill), "AI-RULEZ :: GENERATED FILE — DO NOT EDIT"),
		"a re-run must rewrite the header, not stack another")
	assert.Equal(t, afterFirst, readFileString(t, skill), "the second run leaves the file unchanged")
	require.NoError(t, load().VerifyPlugin(""), "verify --plugin converges right after generate --plugin")
}

func TestPluginVersionDriftUsesRegisteredMemberLayout(t *testing.T) {
	for _, member := range []bool{false, true} {
		t.Run(map[bool]string{false: "standalone", true: "registered member"}[member], func(t *testing.T) {
			root := t.TempDir()
			child := filepath.Join(root, "plugin")
			writeDomainsFile(t, filepath.Join(root, ".ai-rulez/config.toml"), `version = "5.0"
name = "producer"
gitignore = false
[marketplace]
name = "catalog"
members = ["plugin"]
[marketplace.owner]
name = "Owner"
`)
			childConfig := filepath.Join(child, ".ai-rulez/config.toml")
			writeDomainsFile(t, childConfig, `version = "5.0"
name = "child"
gitignore = false
[plugin]
name = "child"
version = "1.0.0"
runtimes = ["claude", "copilot"]
`)
			skill := filepath.Join(child, ".ai-rulez/skills/shared/SKILL.md")
			writeDomainsFile(t, skill, "---\nname: shared\ndescription: shared\n---\nBody.\n")
			load := func(dir string) *Generator {
				cfg, err := config.LoadConfig(context.Background(), dir, config.WithoutLocal())
				require.NoError(t, err)
				return NewGenerator(cfg)
			}
			producer := child
			if member {
				producer = root
			}
			require.NoError(t, load(producer).GeneratePlugin(""))
			if !member {
				writeDomainsFile(t, filepath.Join(root, ".ai-rulez/config.toml"), "version=\"5.0\"\nname=\"unrelated\"\ngitignore=false\n")
			}
			gitCmd(t, root, "init", "-q")
			gitCmd(t, root, "add", "-A")
			gitCmd(t, root, "commit", "-qm", "baseline")
			drift, err := load(child).PluginVersionDrift("")
			require.NoError(t, err)
			require.Empty(t, drift, "fresh child matches its producer's committed layout")
			writeDomainsFile(t, skill, "---\nname: shared\ndescription: shared\n---\nChanged.\n")
			for _, dir := range []string{child, producer} {
				drift, err = load(dir).PluginVersionDrift("")
				require.NoError(t, err)
				require.Len(t, drift, 1)
				assert.Contains(t, drift[0].Changed, "skills/shared/SKILL.md")
				assert.NotContains(t, drift[0].Changed, ".claude-plugin/marketplace.json")
				assert.NotContains(t, drift[0].Changed, ".github/plugin/marketplace.json")
			}
			data, err := os.ReadFile(childConfig)
			require.NoError(t, err)
			writeDomainsFile(t, childConfig, strings.ReplaceAll(string(data), `version = "1.0.0"`, `version = "1.1.0"`))
			drift, err = load(child).PluginVersionDrift("")
			require.NoError(t, err)
			require.Empty(t, drift, "bumping the changed plugin's version clears the warning")
		})
	}
}
