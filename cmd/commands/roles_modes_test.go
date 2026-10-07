package commands

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRolesResolveReportsDegradedSkillModes(t *testing.T) {
	// Arrange: the dev role sets name-only on migrate; add a harness without a setting.
	root := rolesCmdProject(t)
	writeFile(t, filepath.Join(root, ".ai-rulez", "config.toml"),
		strings.Replace(rolesTestConfig, `presets = ["claude"]`, `presets = ["claude", "cursor"]`, 1))

	// Act
	var text bytes.Buffer
	require.NoError(t, runRolesResolve(&text, "dev"))
	rolesFormat = formatJSON
	var doc bytes.Buffer
	require.NoError(t, runRolesResolve(&doc, "dev"))

	// Assert
	assert.Contains(t, text.String(), "skill_mode not honored")
	assert.Contains(t, text.String(), "migrate = name-only")
	var parsed struct {
		SkillModes []struct {
			ID       string   `json:"id"`
			Mode     string   `json:"mode"`
			Degraded []string `json:"degraded"`
		} `json:"skill_modes"`
	}
	require.NoError(t, json.Unmarshal(doc.Bytes(), &parsed))
	require.Len(t, parsed.SkillModes, 1)
	assert.Equal(t, "migrate", parsed.SkillModes[0].ID)
	assert.NotEmpty(t, parsed.SkillModes[0].Degraded)
}
