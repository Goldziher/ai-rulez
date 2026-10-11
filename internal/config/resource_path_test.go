package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// An installed skill that selects several skills with include/exclude and names
// no path is a directory of skill directories: it defaults to "skills", so
// `name` is only the entry's label. A single-skill entry keeps "skills/<name>".
func TestInstalledSkillConfig_GetPath(t *testing.T) {
	tests := []struct {
		name  string
		entry InstalledSkillConfig
		want  string
	}{
		{"single skill defaults to skills/<name>", InstalledSkillConfig{Name: "kreuzberg"}, "skills/kreuzberg"},
		{"an explicit path wins", InstalledSkillConfig{Name: "lib", Path: "libs/my-lib"}, "libs/my-lib"},
		{"include selects several under skills", InstalledSkillConfig{Name: "vendor", Include: []string{"db-*"}}, "skills"},
		{"exclude alone selects several under skills", InstalledSkillConfig{Name: "vendor", Exclude: []string{"*-wip"}}, "skills"},
		{"an explicit path with include is kept", InstalledSkillConfig{Name: "vendor", Path: "pkg", Include: []string{"db-*"}}, "pkg"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.entry.GetPath())
		})
	}
}

// A skill source that selects several skills with include/exclude and names no
// path is a directory of skill directories and defaults to "skills"; without
// include/exclude an empty path means the source root.
func TestSkillSourceConfig_GetPath(t *testing.T) {
	tests := []struct {
		name string
		src  SkillSourceConfig
		want string
	}{
		{"no path and no globs means the source root", SkillSourceConfig{Name: "s"}, ""},
		{"an explicit path wins", SkillSourceConfig{Name: "s", Path: "team"}, "team"},
		{"include defaults to skills", SkillSourceConfig{Name: "s", Include: []string{"pdf-*"}}, "skills"},
		{"exclude alone defaults to skills", SkillSourceConfig{Name: "s", Exclude: []string{"*-wip"}}, "skills"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.src.GetPath())
		})
	}
}

// The loader fills the effective path of an include/exclude skill source, so the
// lock, the server and search all read the same path.
func TestDecodeConfigTOML_DefaultsSkillSourcePath(t *testing.T) {
	cfg, err := decodeConfigTOML([]byte(`
version = "5.0"
name = "p"

[[skill_sources]]
name = "basemind-test"
url = "https://github.com/Goldziher/basemind"
include = ["basemind*", "multi-agent-room"]

[[skill_sources]]
name = "single"
url = "https://example.com/one"
`), "config.toml")
	require.NoError(t, err)
	require.Len(t, cfg.SkillSources, 2)
	assert.Equal(t, "skills", cfg.SkillSources[0].Path, "an include/exclude source defaults to skills")
	assert.Equal(t, "", cfg.SkillSources[1].Path, "a source without include/exclude keeps the source root")
}
