package skillsource

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolve_LocalSourceOfAProjectConfigMustStayInsideTheProject(t *testing.T) {
	// Arrange
	base := t.TempDir()
	project := filepath.Join(base, "project")
	outside := filepath.Join(base, "victim", "skills")
	write(t, project, "vendor/skills/pdf/SKILL.md", skillMD("pdf", "Work with PDF files"))
	write(t, outside, "stolen/SKILL.md", skillMD("stolen", "Outside the project"))
	require.NoError(t, os.Symlink(outside, filepath.Join(project, "vendor", "link")))

	tests := []struct {
		name    string
		spec    Spec
		wantErr bool
	}{
		{"relative path inside", Spec{Name: "in", URL: "vendor/skills"}, false},
		{"relative path with dot prefix", Spec{Name: "in", URL: "./vendor/skills"}, false},
		{"absolute path inside", Spec{Name: "in", URL: filepath.Join(project, "vendor", "skills")}, false},
		{"project root itself", Spec{Name: "in", URL: ".", Path: "vendor/skills"}, false},
		{"absolute path outside", Spec{Name: "out", URL: outside}, true},
		{"parent traversal in url", Spec{Name: "out", URL: "../victim/skills"}, true},
		{"parent traversal in path", Spec{Name: "out", URL: "vendor", Path: "../../victim/skills"}, true},
		{"symlink inside pointing outside", Spec{Name: "out", URL: "vendor/link"}, true},
		{"outside path allowed for --source", Spec{Name: "cli", URL: outside, AllowOutside: true}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			res, err := Resolve(context.Background(), tt.spec, Options{ProjectRoot: project})

			// Assert
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "outside the project")
				assert.Contains(t, err.Error(), "--source")
				assert.Nil(t, res)
				return
			}
			require.NoError(t, err)
			assert.NotEmpty(t, res.Skills)
		})
	}
}

func TestResolve_LocalSourceWithoutAProjectRootFailsClosed(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	write(t, dir, "pdf/SKILL.md", skillMD("pdf", "Work with PDF files"))

	// Act
	_, err := Resolve(context.Background(), Spec{Name: "x", URL: dir}, Options{})

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "project root")
}

func TestParseArg_LocalDirectoryMayLeaveTheProjectButAGitURLIsNotLocal(t *testing.T) {
	local, err := ParseArg("/somewhere/else")
	require.NoError(t, err)
	assert.True(t, local.AllowOutside)

	remote, err := ParseArg("https://github.com/org/repo")
	require.NoError(t, err)
	assert.False(t, remote.AllowOutside)
}

func TestDiscover_NeverServesTheFileTheDigestLeavesOut(t *testing.T) {
	// Arrange: a source that is itself one skill, with the bookkeeping file at its root.
	root := t.TempDir()
	write(t, root, "SKILL.md", skillMD("solo", "A single skill"))
	write(t, root, ".cache_meta.json", `{"x":1}`)
	write(t, root, ".cache_meta.json.tmp", "tmp")
	write(t, root, ".cache_meta.json.bak", "authored content")
	write(t, root, "refs/.cache_meta.json", "nested is content")

	// Act
	skills, err := Discover(Spec{Name: "solo"}, root)

	// Assert
	require.NoError(t, err)
	require.Len(t, skills, 1)
	var got []string
	for _, f := range skills[0].Files {
		got = append(got, f.Path)
	}
	assert.Equal(t, []string{"SKILL.md", ".cache_meta.json.bak", "refs/.cache_meta.json"}, got)
}
