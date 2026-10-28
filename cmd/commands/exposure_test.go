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
	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

func TestExposureMap(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "areas.toml")
	require.NoError(t, os.WriteFile(file, []byte("[[area]]\nname = \"search\"\n"), 0o600))
	bad := filepath.Join(dir, "bad.toml")
	require.NoError(t, os.WriteFile(bad, []byte("[[area]]\nname = \"a\"\npaths = [\"x/\"]\n"), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "sub"), 0o750))
	outside := filepath.Join(t.TempDir(), "areas.toml")
	require.NoError(t, os.WriteFile(outside, []byte("[[area]]\nname = \"far\"\n"), 0o600))
	inline := &config.ExposureConfig{Areas: []config.ExposureArea{{Name: "billing"}}}
	cfgFile := filepath.Join(dir, "config.toml")
	require.NoError(t, os.WriteFile(cfgFile, []byte("[exposure]\n[[exposure.areas]]\nname = \"a\"\n[[exposure.areas]]\nname = \"a\"\n"), 0o600))
	duplicate := &config.ExposureConfig{Areas: []config.ExposureArea{{Name: "a"}, {Name: "a"}}}
	tests := []struct {
		name      string
		exp       *config.ExposureConfig
		flag      string
		config    string
		wantAreas []string
		root      string
		wantFile  string
		wantErr   string
	}{
		{name: "no table and no flag is no map"},
		{name: "a table without areas is no map", exp: &config.ExposureConfig{ContextWindow: 1000}},
		{name: "the inline map", exp: inline, wantAreas: []string{"billing"}},
		{name: "the flag replaces the table", exp: inline, flag: file, wantAreas: []string{"search"}, wantFile: "areas.toml"},
		{name: "a file outside the root is absolute", flag: outside, wantAreas: []string{"far"}, wantFile: filepath.ToSlash(outside)},
		{name: "the configured file", exp: &config.ExposureConfig{AreasFile: "areas.toml"}, wantAreas: []string{"search"}, wantFile: "areas.toml"},
		{name: "a configured file outside the root is absolute", exp: &config.ExposureConfig{AreasFile: "areas.toml"}, root: filepath.Join(dir, "sub"), wantAreas: []string{"search"}, wantFile: filepath.ToSlash(file)},
		{name: "a file problem names its line", flag: bad, wantErr: bad + `:3: area "a" path "x/"`},
		{name: "a table problem without a configuration file names the table", exp: duplicate, wantErr: `[exposure]: duplicate area name "a"`},
		{name: "a table problem names its configuration line", exp: duplicate, config: cfgFile, wantErr: `config.toml:5: duplicate area name "a"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := tt.root
			if root == "" {
				root = dir
			}
			m, from, err := exposureMap(tt.exp, dir, root, tt.flag, tt.config)
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
		{"with-text without json", exposureFlags{withText: true, root: t.TempDir()}, "--with-text applies to --format json"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := runExposureInventory(t.Context(), &bytes.Buffer{}, &tt.flags)
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
			{Kind: exposure.KindRule, QualifiedName: "big", Path: ".claude/rules/big.md", Phase: exposure.PhaseUnknown, Tokens: 7},
		},
		Problems: []exposure.Problem{{Path: "big.md", Error: "over the read limit"}},
	}
	tests := []struct {
		name      string
		withAreas bool
		want      []string
		absent    []string
	}{
		{"without areas", false, []string{"plugins enabled, loaded: none", "3 artifacts: 2 start (17 tokens, unknown included), 1 on_demand, 0 manual, 1 unknown", "problem: big.md: over the read limit"}, []string{"AREA"}},
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

// With --root, -C still names the configuration: an invalid one fails, a valid
// one supplies the area map; without -C the map is discovered from the root.
func TestLoadExposureUsesTheConfigFlagsWithRoot(t *testing.T) {
	project := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(project, ".ai-rulez"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(project, ".ai-rulez", "config.toml"), []byte("version = \"5.0\"\nname = \"p\"\n\n[[exposure.areas]]\nname = \"billing\"\n"), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(project, "sub"), 0o750))
	bad := filepath.Join(t.TempDir(), "config.toml")
	require.NoError(t, os.WriteFile(bad, []byte("version = \"5.0\"\nname = \"bad\"\n\n[exposure]\nplugins = \"some\"\n"), 0o600))
	empty := t.TempDir()
	tests := []struct {
		name, config, root, wantErr string
		wantMap                     bool
	}{
		{name: "an invalid -C fails", config: bad, root: empty, wantErr: "AR9W5"},
		{name: "a missing -C fails", config: filepath.Join(empty, "nope.toml"), root: empty, wantErr: "does not exist"},
		{name: "a valid -C supplies the map", config: filepath.Join(project, ".ai-rulez"), root: empty, wantMap: true},
		{name: "a subdirectory finds its project", root: filepath.Join(project, "sub"), wantMap: true},
		{name: "a directory without a configuration has no map", root: empty},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prev := cfgFile
			cfgFile = tt.config
			t.Cleanup(func() { cfgFile = prev })

			in, err := loadExposure(t.Context(), &exposureFlags{root: tt.root})

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantMap, in.areas != nil)
		})
	}
}

func TestRunExposureReportRejectsBadFlags(t *testing.T) {
	tests := []struct {
		name  string
		flags exposureFlags
		want  string
	}{
		{"unknown harness", exposureFlags{harness: "codex", root: t.TempDir()}, `unknown --harness "codex" (only claude)`},
		{"negative window", exposureFlags{harness: "claude", contextWindow: -1, root: t.TempDir()}, "--context-window must not be negative"},
		{"unknown plugin mode", exposureFlags{harness: "claude", plugins: "some", root: t.TempDir()}, `unknown --plugins "some"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := runExposureReport(t.Context(), &bytes.Buffer{}, &tt.flags)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestWriteReportText(t *testing.T) {
	inv := &exposure.Inventory{
		Root: "/r", Harness: exposure.HarnessClaude,
		Artifacts: []exposure.Artifact{
			{Kind: exposure.KindClaudeMD, QualifiedName: "CLAUDE.md", Path: "CLAUDE.md", Phase: exposure.PhaseStart, Tokens: 50},
			{Kind: exposure.KindSkill, QualifiedName: "a", Path: ".claude/skills/a/SKILL.md", Phase: exposure.PhaseStart, Tokens: 10, ListingChars: 60, Truncated: true},
			{Kind: exposure.KindSkill, QualifiedName: "b", Path: ".claude/skills/b/SKILL.md", Phase: exposure.PhaseUnknown, Tokens: 4, ListingChars: 10},
			{Kind: exposure.KindRule, QualifiedName: "r", Path: "svc/.claude/rules/r.md", Phase: exposure.PhaseOnDemand, Tokens: 300},
		},
	}
	tests := []struct {
		name   string
		opts   exposure.ReportOptions
		want   []string
		absent []string
	}{
		{
			name: "within budget",
			opts: exposure.ReportOptions{ContextWindow: 200000},
			want: []string{
				"start of session: 3 artifacts, ~64 tokens",
				"skill listing: 2 skills, 70 chars of 8000 budget (200000 token window), 1 description cut at the per-skill cap",
				"start tokens by kind:\n  claude_md: 50\n  skill: 14\n",
				"  start: claude_md 1, skill 1\n  on_demand: rule 1\n  manual: none\n  unknown: skill 1\n",
				"tokens if loaded, by phase:\n  start: 60\n  on_demand: 300\n  unknown: 4\n",
				"heaviest at start:\n  ~50 claude_md CLAUDE.md\n  ~10 skill .claude/skills/a/SKILL.md\n  ~4 skill .claude/skills/b/SKILL.md\n",
			},
			absent: []string{"OVER", "start budget", "svc/.claude/rules/r.md"},
		},
		{
			name: "over the listing budget and the start budget",
			opts: exposure.ReportOptions{ContextWindow: 1000, StartBudget: 60},
			want: []string{"70 chars of 40 budget (1000 token window), OVER by 30: descriptions will be dropped", "start budget: OVER by 4: [exposure] start_budget 60"},
		},
		{
			name: "within the start budget",
			opts: exposure.ReportOptions{ContextWindow: 200000, StartBudget: 64},
			want: []string{"start budget: within [exposure] start_budget 64"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer

			require.NoError(t, writeReportText(&out, exposure.NewReport(inv, tt.opts)))

			for _, w := range tt.want {
				assert.Contains(t, out.String(), w)
			}
			for _, w := range tt.absent {
				assert.NotContains(t, out.String(), w)
			}
		})
	}
}

// A file named through a symlink to the root is still relative to it, as on
// macOS, where the temporary directory /var/... is /private/var/....
func TestRootRelativeThroughASymlink(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real")
	require.NoError(t, os.MkdirAll(real, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(real, "areas.toml"), nil, 0o600))
	link := filepath.Join(dir, "link")
	testutil.SymlinkOrSkip(t, real, link)

	assert.Equal(t, "areas.toml", rootRelative(real, filepath.Join(link, "areas.toml")))
	assert.Equal(t, "areas.toml", rootRelative(link, filepath.Join(real, "areas.toml")))
	outside := filepath.Join(t.TempDir(), "areas.toml")
	assert.Equal(t, filepath.ToSlash(outside), rootRelative(real, outside))
}
