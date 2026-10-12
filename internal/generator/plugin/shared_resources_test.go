package plugin

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeFile writes body at project-relative path rel under root, creating
// parents.
func writeFile(t *testing.T, root, rel, body string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
}

// sharedResourceFixture builds a project with one skill linking to a shared
// references/ and templates/ directory.
func sharedResourceFixture(t *testing.T, skillBody string) (project string, skill config.ContentFile, resources []config.SharedResource) {
	t.Helper()
	project = t.TempDir()
	writeFile(t, project, ".ai-rulez/skills/alpha/SKILL.md", skillBody)
	writeFile(t, project, ".ai-rulez/references/r.md", "ref\n")
	writeFile(t, project, ".ai-rulez/references/deep/n.md", "deep\n")
	writeFile(t, project, ".ai-rulez/templates/t.md", "template\n")
	skill = config.ContentFile{
		Name: "alpha",
		Path: filepath.Join(project, ".ai-rulez", "skills", "alpha", skillManifestName),
	}
	resources = []config.SharedResource{
		{Name: "references", Dir: filepath.Join(project, ".ai-rulez", "references")},
		{Name: "templates", Dir: filepath.Join(project, ".ai-rulez", "templates")},
	}
	return project, skill, resources
}

func TestBundleContent_SharedResources(t *testing.T) {
	t.Parallel()

	const skillBody = "---\nname: alpha\n---\n\n[r](../../references/r.md) [t](../../templates/t.md) [o](../other/x.md)\n"

	project, skill, resources := sharedResourceFixture(t, skillBody)
	out := filepath.Join(project, "out")
	m := &Manifest{Skills: []config.ContentFile{skill}, SharedResources: resources, SourceDir: project}

	outputs, err := bundleContent(m, out, contentLayout{Skills: true})
	require.NoError(t, err)

	paths := bundledPaths(t, outputs, out)
	for _, want := range []string{
		"skills/alpha/SKILL.md",
		"skills/alpha/references/r.md",
		"skills/alpha/references/deep/n.md",
		"skills/alpha/templates/t.md",
	} {
		assert.Contains(t, paths, want)
	}

	body := string(outputByPath(t, outputs, "skills/alpha/SKILL.md").RawContent)
	assert.Contains(t, body, "[r](references/r.md)", "a shared link is rewritten to the in-skill copy")
	assert.Contains(t, body, "[t](templates/t.md)")
	assert.Contains(t, body, "[o](../other/x.md)", "a link that does not point at a shared directory is left alone")
	assert.NotContains(t, body, "../../references/")
}

func TestBundleContent_SharedResourceNameClashIsAnError(t *testing.T) {
	t.Parallel()

	// The skill ships its own references/r.md, which the shared directory also provides.
	project, skill, resources := sharedResourceFixture(t, "---\nname: alpha\n---\nbody\n")
	writeFile(t, project, ".ai-rulez/skills/alpha/references/r.md", "own\n")
	m := &Manifest{Skills: []config.ContentFile{skill}, SharedResources: resources, SourceDir: project}

	_, err := bundleContent(m, filepath.Join(project, "out"), contentLayout{Skills: true})

	require.Error(t, err, "a shared file must not silently overwrite the skill's own file")
	assert.Contains(t, err.Error(), "references/r.md")
	assert.Contains(t, err.Error(), "shared_resources")
}

// TestBundleContent_SharedResourcesRecordedInProvenance guards that the shared
// copies are recorded in the provenance sidecar like the skill's own files, so
// verify --plugin covers them.
func TestBundleContent_SharedResourcesRecordedInProvenance(t *testing.T) {
	t.Parallel()

	project, skill, resources := sharedResourceFixture(t, "---\nname: alpha\n---\n[r](../../references/r.md)\n")
	out := filepath.Join(project, "out")
	m := &Manifest{Skills: []config.ContentFile{skill}, SharedResources: resources, SourceDir: project}

	outputs, err := bundleContent(m, out, contentLayout{Skills: true})
	require.NoError(t, err)
	decorated, err := AddProvenance(outputs, out)
	require.NoError(t, err)
	for _, output := range decorated {
		require.NoError(t, os.MkdirAll(filepath.Dir(output.Path), 0o755))
		require.NoError(t, os.WriteFile(output.Path, output.RawContent, 0o644))
	}
	require.NoError(t, VerifyProvenance(out))

	require.NoError(t, os.WriteFile(filepath.Join(out, "skills", "alpha", "references", "r.md"), []byte("tampered\n"), 0o644))
	assert.Error(t, VerifyProvenance(out), "a modified shared copy must be detected by verify --plugin")
}

func TestRewriteSharedRefs(t *testing.T) {
	resources := []config.SharedResource{{Name: "references"}}
	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "two levels up", body: "[r](../../references/x.md)", want: "[r](references/x.md)"},
		{name: "one level up", body: "[r](../references/x.md)", want: "[r](references/x.md)"},
		{name: "already local", body: "[r](references/x.md)", want: "[r](references/x.md)"},
		{name: "other directory untouched", body: "[o](../../other/x.md)", want: "[o](../../other/x.md)"},
		{name: "name prefix not matched", body: "[o](../../references-extra/x.md)", want: "[o](../../references-extra/x.md)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, rewriteSharedRefs(tt.body, resources))
		})
	}
}
