package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPlainHTTPRemotesAreRejected(t *testing.T) {
	err := ValidateInstalledSkills([]InstalledSkillConfig{{Name: "s", Source: "http://example.com/o/r.git"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "plain http://")
	assert.NoError(t, ValidateInstalledSkills([]InstalledSkillConfig{{Name: "s", Source: "https://example.com/o/r.git"}}))

	src := SkillSourceConfig{Name: "s", URL: "git+http://example.com/o/r.git"}
	err = src.Validate(0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "plain http://")
	src.URL = "git+https://example.com/o/r.git"
	assert.NoError(t, src.Validate(0))
}
