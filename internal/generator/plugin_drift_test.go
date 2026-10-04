package generator

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
