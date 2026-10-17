package pluginimport_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/importer"
	"github.com/Goldziher/ai-rulez/v5/internal/pluginimport"
	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

func TestIndex_FindAndNames(t *testing.T) {
	// Arrange
	ix, err := pluginimport.ParseIndex([]byte(`{"name":"m","plugins":[{"name":"zeta","source":"./z"},{"name":"alpha","source":"./a"}]}`), pluginimport.IndexClaude)
	require.NoError(t, err)
	// Act
	hit, found := ix.Find("alpha")
	_, missing := ix.Find("nope")
	_, caseMiss := ix.Find("Alpha")
	// Assert
	require.True(t, found)
	assert.Equal(t, "alpha", hit.Name)
	assert.False(t, missing)
	assert.False(t, caseMiss, "names are case sensitive")
	assert.Equal(t, []string{"zeta", "alpha"}, ix.Names(), "index order")
	assert.Equal(t, []string{"alpha", "zeta"}, ix.SortedNames())
}

func TestReadIndexFile(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(t *testing.T, dir string)
		wantFormat string
		wantNames  []string
		wantErr    string
	}{
		{
			name: "a claude style index",
			setup: func(t *testing.T, dir string) {
				testutil.WriteTree(t, dir, map[string]string{"marketplace.json": `{"name":"m","plugins":[{"name":"p","source":"./p"}]}`})
			},
			wantFormat: pluginimport.IndexClaude, wantNames: []string{"p"},
		},
		{
			name: "a local object source is the codex format",
			setup: func(t *testing.T, dir string) {
				testutil.WriteTree(t, dir, map[string]string{"marketplace.json": `{"name":"m","plugins":[{"name":"p","source":{"source":"local","path":"./p"}}]}`})
			},
			wantFormat: pluginimport.IndexCodex, wantNames: []string{"p"},
		},
		{
			name:    "no file",
			setup:   func(*testing.T, string) {},
			wantErr: "marketplace.json",
		},
		{
			name: "a symlinked index is not followed",
			setup: func(t *testing.T, dir string) {
				outside := t.TempDir()
				testutil.WriteTree(t, outside, map[string]string{"m.json": `{"plugins":[]}`})
				testutil.SymlinkOrSkip(t, filepath.Join(outside, "m.json"), filepath.Join(dir, "marketplace.json"))
			},
			wantErr: "symlink",
		},
		{
			name: "an oversize index",
			setup: func(t *testing.T, dir string) {
				testutil.WriteTree(t, dir, map[string]string{"marketplace.json": strings.Repeat(" ", pluginimport.MaxIndexBytes+1)})
			},
			wantErr: "larger than",
		},
		{
			name: "not json",
			setup: func(t *testing.T, dir string) {
				testutil.WriteTree(t, dir, map[string]string{"marketplace.json": "<html>"})
			},
			wantErr: "marketplace",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir := t.TempDir()
			tt.setup(t, dir)
			// Act
			ix, err := pluginimport.ReadIndexFile(dir)
			// Assert
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantFormat, ix.Format)
			assert.Equal(t, tt.wantNames, ix.Names())
		})
	}
}

func TestSafeJoin_ReportsAnUnreadableComponent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows reports a path below a regular file as not existing, not as ENOTDIR")
	}
	// Arrange: a regular file used as a directory makes the stat fail with ENOTDIR, not ENOENT
	root := t.TempDir()
	testutil.WriteTree(t, root, map[string]string{"file.md": "x"})
	// Act
	_, err := pluginimport.SafeJoin(root, "file.md/child")
	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "file.md/child")
}

func TestSafeJoin_AcceptsTheRootAndMissingTails(t *testing.T) {
	root := t.TempDir()
	testutil.WriteTree(t, root, map[string]string{"a/b.md": "x"})
	tests := []struct {
		rel  string
		want string
	}{
		{".", root},
		{"./a/b.md", filepath.Join(root, "a", "b.md")},
		{"a/missing/deeper", filepath.Join(root, "a", "missing", "deeper")},
	}
	for _, tt := range tests {
		t.Run(tt.rel, func(t *testing.T) {
			got, err := pluginimport.SafeJoin(root, tt.rel)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
	_, err := pluginimport.SafeJoin(root, "../outside")
	require.Error(t, err)
}

func TestLoad_ServerPluginRootReferencesAreRefusedInEveryField(t *testing.T) {
	tests := []struct {
		name   string
		server string
	}{
		{"command", `{"command":"${CLAUDE_PLUGIN_ROOT}/bin/serve"}`},
		{"url", `{"url":"http://localhost/${CLAUDE_PLUGIN_ROOT}/mcp","type":"http"}`},
		{"argument", `{"command":"node","args":["--cfg","${CLAUDE_PLUGIN_ROOT}/c.json"]}`},
		{"environment", `{"command":"node","env":{"DIR":"${CLAUDE_PLUGIN_ROOT}"}}`},
		{"header", `{"url":"https://example.com/mcp","type":"http","headers":{"X-Dir":"${CLAUDE_PLUGIN_ROOT}"}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act: a fetched plugin has no project path for the root variable
			res, _ := loadDir(t, map[string]string{".mcp.json": `{"mcpServers":{"s":` + tt.server + `}}`}, pluginimport.Options{EnableMCP: true})
			// Assert
			assert.Empty(t, res.MCP)
			require.NotEmpty(t, res.Findings)
			var refused bool
			for _, f := range res.Findings {
				refused = refused || (f.Status == importer.StatusNeedsAction && strings.Contains(f.Reason, "root directory"))
			}
			assert.True(t, refused, "%v", res.Findings)
		})
	}
}

func TestLoad_SkillProblemsDropOnlyThatSkill(t *testing.T) {
	// Arrange
	files := map[string]string{
		".claude-plugin/plugin.json":     `{"name":"p"}`,
		"skills/good/SKILL.md":           "---\nname: good\ndescription: good\n---\n# Good\n",
		"skills/bad name/SKILL.md":       "# bad\n",
		"skills/-leading-dash/SKILL.md":  "# dash\n",
		"skills/no-marker/references.md": "# nothing\n",
	}
	// Act
	res, _ := loadDir(t, files, pluginimport.Options{})
	// Assert
	assert.Equal(t, []string{"good"}, names(res.Skills))
	var dropped []string
	for _, f := range res.Findings {
		if f.Status == importer.StatusDropped {
			dropped = append(dropped, f.Reason)
		}
	}
	assert.Len(t, dropped, 2, "%v", res.Findings)
	for _, reason := range dropped {
		assert.Contains(t, reason, "is not a valid skill name")
	}
}

func TestLoad_AtMostMaxSkillsAreDelivered(t *testing.T) {
	// Arrange
	files := map[string]string{".claude-plugin/plugin.json": `{"name":"p"}`}
	for i := range pluginimport.MaxSkills + 3 {
		files[fmt.Sprintf("skills/s%04d/SKILL.md", i)] = "# s\n"
	}
	// Act
	res, _ := loadDir(t, files, pluginimport.Options{})
	// Assert
	assert.Len(t, res.Skills, pluginimport.MaxSkills)
	var capped bool
	for _, f := range res.Findings {
		capped = capped || (f.Status == importer.StatusDropped && strings.Contains(f.Reason, fmt.Sprintf("at most %d skills", pluginimport.MaxSkills)))
	}
	assert.True(t, capped, "the cut-off is reported")
}

func TestLoad_ASkillTheScannerCannotReadIsDroppedWithItsReason(t *testing.T) {
	// Arrange
	files := map[string]string{".claude-plugin/plugin.json": `{"name":"p"}`, "skills/x/SKILL.md": "# x\n", "skills/y/SKILL.md": "# y\n"}
	dir := t.TempDir()
	testutil.WriteTree(t, dir, files)
	opt := &pluginimport.Options{Skills: func(_ context.Context, skillDir, name string) (config.ContentFile, error) {
		if filepath.Base(skillDir) == "x" {
			return config.ContentFile{}, fmt.Errorf("boom")
		}
		return config.ContentFile{Name: name}, nil
	}}
	// Act
	res, err := pluginimport.Load(context.Background(), dir, opt)
	// Assert
	require.NoError(t, err)
	assert.Equal(t, []string{"y"}, names(res.Skills))
	var reason string
	for _, f := range res.Findings {
		if f.Status == importer.StatusDropped {
			reason = f.Reason
		}
	}
	assert.Equal(t, "the skill cannot be read: boom", reason)
}

func TestResult_DescribeAndEmpty(t *testing.T) {
	tests := []struct {
		name     string
		files    map[string]string
		wantName string
		empty    bool
	}{
		{"manifest name", map[string]string{".claude-plugin/plugin.json": `{"name":"named"}`, "skills/s/SKILL.md": "# s\n"}, "named", false},
		{"directory name when the manifest has none", map[string]string{".claude-plugin/plugin.json": `{}`}, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			res, dir := loadDir(t, tt.files, pluginimport.Options{})
			want := tt.wantName
			if want == "" {
				want = filepath.Base(dir)
			}
			// Act and Assert
			assert.Equal(t, want, res.Describe(dir))
			assert.Equal(t, tt.empty, res.Empty())
		})
	}
}

func TestLoad_ARegularFileIsNotAPluginDirectory(t *testing.T) {
	// Arrange
	file := filepath.Join(t.TempDir(), "plugin")
	require.NoError(t, os.WriteFile(file, []byte("x"), 0o644))
	// Act
	_, err := pluginimport.Load(context.Background(), file, &pluginimport.Options{Name: "p"})
	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a real directory")
	assert.Contains(t, err.Error(), `plugin "p"`)
}
