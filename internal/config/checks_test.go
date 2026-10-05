package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeCheckFile(t *testing.T, path, body string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
}

func TestScanContentTree_Checks(t *testing.T) {
	t.Parallel()

	// Arrange
	dir := t.TempDir()
	writeCheckFile(t, filepath.Join(dir, "checks", "security.md"),
		"---\ndescription: Security\nseverity: high\ntools: [Read, Grep]\ntargets: [cursor]\n---\n\nFlag injection.\n")
	writeCheckFile(t, filepath.Join(dir, "checks", "notes.txt"), "ignored")
	writeCheckFile(t, filepath.Join(dir, "domains", "backend", "checks", "perf.md"), "Look at hot loops.\n")

	// Act
	tree, err := ScanContentTree(dir)

	// Assert
	require.NoError(t, err)
	require.Len(t, tree.Checks, 1)
	check := tree.Checks[0]
	assert.Equal(t, "security", check.Name)
	assert.Equal(t, "Security", CheckDescription(&check))
	assert.Equal(t, "high", CheckSeverity(&check))
	assert.Equal(t, []string{"Read", "Grep"}, check.Metadata.Tools)
	assert.Equal(t, []string{"cursor"}, check.Metadata.Targets)
	require.Len(t, tree.Domains["backend"].Checks, 1)
	assert.Equal(t, "perf", tree.Domains["backend"].Checks[0].Name)
	assert.False(t, tree.IsEmpty())
	assert.Len(t, tree.GetChecksForDomains([]string{"backend"}), 2)
	assert.Len(t, tree.GetAllContentFiles(), 2)
}

func TestScanContentTree_OnlyChecksIsNotEmpty(t *testing.T) {
	t.Parallel()
	assert.False(t, (&ContentTree{Checks: []ContentFile{{Name: "x"}}}).IsEmpty())
	assert.False(t, (&ContentTree{Domains: map[string]*Domain{"d": {Checks: []ContentFile{{Name: "x"}}}}}).IsEmpty())
}

func TestSelectContentForProfile_Checks(t *testing.T) {
	t.Parallel()

	// Arrange
	tree := &ContentTree{
		Checks: []ContentFile{{Name: "root-check"}},
		Domains: map[string]*Domain{
			"backend":  {Name: "backend", Checks: []ContentFile{{Name: "be-check"}}},
			"frontend": {Name: "frontend", Checks: []ContentFile{{Name: "fe-check"}}},
		},
	}
	cfg := &Config{Profiles: map[string][]string{"api": {"backend"}}}

	// Act
	selected, err := cfg.SelectContentForProfile(tree, "api")

	// Assert
	require.NoError(t, err)
	require.Len(t, selected.Checks, 1)
	assert.Contains(t, selected.Domains, "backend")
	assert.NotContains(t, selected.Domains, "frontend")
}

func TestValidateChecks(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		check   ContentFile
		wantErr string
	}{
		{"valid", ContentFile{Name: "a.b_c-1", Metadata: &Metadata{Extra: map[string]string{"severity": "High"}}}, ""},
		{"no metadata", ContentFile{Name: "plain"}, ""},
		{"space in name", ContentFile{Name: "bad name"}, "invalid check name"},
		{"traversal", ContentFile{Name: ".."}, "invalid check name"},
		{"slash", ContentFile{Name: "../x"}, "invalid check name"},
		{"unknown severity", ContentFile{Name: "x", Metadata: &Metadata{Extra: map[string]string{"severity": "urgent"}}}, "invalid severity"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			rootCfg := &Config{Content: &ContentTree{Checks: []ContentFile{tt.check}}}
			domainCfg := &Config{Content: &ContentTree{Domains: map[string]*Domain{"d": {Checks: []ContentFile{tt.check}}}}}

			// Act
			rootErr, domainErr := rootCfg.validateChecks(), domainCfg.validateChecks()

			// Assert
			if tt.wantErr == "" {
				assert.NoError(t, rootErr)
				assert.NoError(t, domainErr)
				return
			}
			require.Error(t, rootErr)
			assert.Contains(t, rootErr.Error(), tt.wantErr)
			require.Error(t, domainErr)
			assert.Contains(t, domainErr.Error(), tt.wantErr)
		})
	}
}
