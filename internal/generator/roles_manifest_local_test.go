package generator

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/roles"
)

const manifestSharedConfig = `version = "4.0"
name = "manifest"
presets = ["claude"]
gitignore = false

[role_manifest]
enabled = true

[[roles]]
name = "shared-role"
`

const manifestLocalOverlay = `
[[roles]]
name = "local-only-role"
`

// rolesFileNames reads roles.json and returns the role names it lists.
func rolesFileNames(t *testing.T, p *driftProject) []string {
	t.Helper()
	var doc struct {
		Roles []struct {
			Name string `json:"name"`
		} `json:"roles"`
	}
	require.NoError(t, json.Unmarshal([]byte(p.read(t, ".ai-rulez/"+roles.FileName)), &doc))
	names := make([]string, 0, len(doc.Roles))
	for _, r := range doc.Roles {
		names = append(names, r.Name)
	}
	return names
}

func TestRolesManifestExcludesLocalRoles(t *testing.T) {
	tests := []struct {
		name      string
		overlay   string
		wantRoles []string
	}{
		{name: "no overlay lists the shared roles", wantRoles: []string{"shared-role"}},
		{name: "a local-only role stays out of roles.json", overlay: manifestLocalOverlay, wantRoles: []string{"shared-role"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			p := newDriftProject(t, manifestSharedConfig)
			if tt.overlay != "" {
				p.overlay(t, tt.overlay)
			}
			g := NewGenerator(p.load(t))
			g.SetAllowLocalDrift(true)

			// Act
			require.NoError(t, g.Generate(""))

			// Assert
			assert.Equal(t, tt.wantRoles, rolesFileNames(t, p))
		})
	}
}

func TestRolesManifestCheckIgnoresLocalRoles(t *testing.T) {
	// Arrange: the committed roles.json was generated from the shared sources on
	// a machine without an overlay.
	p := newDriftProject(t, manifestSharedConfig)
	require.NoError(t, NewGenerator(p.load(t)).Generate(""))
	committed := p.read(t, ".ai-rulez/"+roles.FileName)
	p.overlay(t, manifestLocalOverlay)

	// Act
	drift, err := NewGenerator(p.load(t)).CheckDrift("")

	// Assert
	require.NoError(t, err)
	for _, d := range drift {
		assert.NotContains(t, d.Path, roles.FileName, "roles.json must not drift because of a local role: %v", drift)
	}
	assert.Equal(t, committed, p.read(t, ".ai-rulez/"+roles.FileName))
}

func TestRolesManifestKeepsLocalRolesUsable(t *testing.T) {
	// Arrange
	p := newDriftProject(t, manifestSharedConfig)
	p.overlay(t, manifestLocalOverlay)
	g := NewGenerator(p.load(t))

	// Act
	err := g.SetRole("local-only-role")

	// Assert: a local role still renders; it is only absent from the manifest.
	require.NoError(t, err)
	_, statErr := os.Stat(filepath.Join(p.dir, roles.FileName))
	assert.True(t, os.IsNotExist(statErr))
}
