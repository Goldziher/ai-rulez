package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/exposure"
)

func TestExposureMap(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "areas.toml")
	require.NoError(t, os.WriteFile(file, []byte("[[area]]\nname = \"search\"\n"), 0o600))
	bad := filepath.Join(dir, "bad.toml")
	require.NoError(t, os.WriteFile(bad, []byte("[[area]]\nname = \"a\"\npaths = [\"x/\"]\n"), 0o600))
	inline := &config.ExposureConfig{Areas: []config.ExposureArea{{Name: "billing"}}}
	tests := []struct {
		name      string
		exp       *config.ExposureConfig
		flag      string
		wantAreas []string
		wantFile  string
		wantErr   string
	}{
		{name: "no table and no flag is no map"},
		{name: "a table without areas is no map", exp: &config.ExposureConfig{ContextWindow: 1000}},
		{name: "the inline map", exp: inline, wantAreas: []string{"billing"}},
		{name: "the flag replaces the table", exp: inline, flag: file, wantAreas: []string{"search"}, wantFile: file},
		{name: "the configured file", exp: &config.ExposureConfig{AreasFile: "areas.toml"}, wantAreas: []string{"search"}, wantFile: "areas.toml"},
		{name: "a file problem names its line", flag: bad, wantErr: bad + `:3: area "a" path "x/"`},
		{name: "a table problem names the table", exp: &config.ExposureConfig{Areas: []config.ExposureArea{{Name: "a"}, {Name: "a"}}}, wantErr: `[exposure]: duplicate area name "a"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, from, err := exposureMap(tt.exp, dir, tt.flag)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "AR9W5")
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			var names []string
			for _, a := range m.Areas() {
				names = append(names, a.Name)
			}
			assert.Equal(t, tt.wantAreas, names)
			assert.Equal(t, tt.wantFile, from)
		})
	}
}

func TestLoadExposureRejectsBadFlags(t *testing.T) {
	tests := []struct {
		name  string
		flags exposureFlags
		want  string
	}{
		{"unknown plugin mode", exposureFlags{plugins: "some", root: t.TempDir()}, `unknown --plugins "some" (use none, enabled or all)`},
		{"missing root", exposureFlags{root: filepath.Join(t.TempDir(), "nope")}, "is not a directory"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := loadExposure(t.Context(), &tt.flags)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestWriteInventoryText(t *testing.T) {
	inv := &exposure.Inventory{
		Root: "/r", Harness: exposure.HarnessClaude,
		Plugins: exposure.PluginSummary{Mode: exposure.PluginsEnabled, Loaded: []string{}},
		Artifacts: []exposure.Artifact{
			{Kind: exposure.KindClaudeMD, QualifiedName: "CLAUDE.md", Path: "CLAUDE.md", Phase: exposure.PhaseStart, Tokens: 10, Area: "billing", AreaSource: "terms"},
			{Kind: exposure.KindSkill, QualifiedName: "s", Path: "a/.claude/skills/s/SKILL.md", Phase: exposure.PhaseOnDemand, Tokens: 4},
		},
		Problems: []exposure.Problem{{Path: "big.md", Error: "over the read limit"}},
	}
	tests := []struct {
		name      string
		withAreas bool
		want      []string
		absent    []string
	}{
		{"without areas", false, []string{"plugins enabled, loaded: none", "2 artifacts: 1 start (10 tokens), 1 on_demand, 0 manual, 0 unknown", "problem: big.md: over the read limit"}, []string{"AREA"}},
		{"with areas", true, []string{"AREA", "billing (terms)"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer

			require.NoError(t, writeInventoryText(&out, inv, tt.withAreas))

			for _, w := range tt.want {
				assert.Contains(t, out.String(), w)
			}
			for _, w := range tt.absent {
				assert.NotContains(t, out.String(), w)
			}
		})
	}
}
