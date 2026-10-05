package lint

import (
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProfileTablesUseRegisteredNonSecurityCodes(t *testing.T) {
	for name, p := range profiles {
		for code, sev := range p.Severity {
			_, ok := lookupRule(code)
			assert.True(t, ok, "%s preset names unregistered code %s", name, code)
			assert.False(t, isSecurityCode(code), "%s preset must never change security rule %s", name, code)
			assert.NotEmpty(t, sev)
		}
		assert.NotEmpty(t, p.Summary, name)
	}
	assert.Equal(t, []string{ProfileDefault, ProfilePermissive, ProfileStrict}, ProfileNames())
}

func TestProfileSeverities(t *testing.T) {
	sevOf := func(lc config.LintConfig, code string) Severity {
		r := &runner{lc: lc}
		r.resolveSettings()
		return r.sev[code]
	}
	assert.Equal(t, SeverityWarning, sevOf(config.LintConfig{}, CodePathMissing))
	assert.Equal(t, SeverityError, sevOf(config.LintConfig{Profile: "strict"}, CodePathMissing))
	assert.Equal(t, SeverityWarning, sevOf(config.LintConfig{Profile: "strict"}, CodeDescriptionStyle), "strict turns on AR803")
	assert.Equal(t, SeverityWarning, sevOf(config.LintConfig{Profile: "permissive"}, CodeLinkUnresolved))
	assert.Equal(t, SeverityError, sevOf(config.LintConfig{Profile: "permissive"}, CodeSecretDetected), "security is never relaxed")
	assert.Equal(t, SeverityOff, sevOf(config.LintConfig{Profile: "strict", Severity: map[string]string{"AR401": "off"}}, CodePathMissing), "explicit severity beats the preset")
	assert.Equal(t, SeverityError, sevOf(config.LintConfig{Profile: "STRICT"}, CodePathMissing), "names are case-insensitive")
}

func TestProfileFailOnAndValidation(t *testing.T) {
	assert.Equal(t, "warning", ProfileFailOn("strict"))
	assert.Equal(t, "", ProfileFailOn(""))
	assert.Equal(t, "", ProfileFailOn("nope"))
	problems := ValidateSettings(&config.LintConfig{Profile: "paranoid"})
	require.Len(t, problems, 1)
	assert.Contains(t, problems[0], "unknown profile")
	assert.Empty(t, ValidateSettings(&config.LintConfig{Profile: "strict"}))
}

func TestDescribeProfileListsDeltas(t *testing.T) {
	got := DescribeProfile("strict")
	assert.Contains(t, got, "fail_on = warning")
	assert.Contains(t, got, "AR401 path-missing: warning -> error")
	assert.Nil(t, DescribeProfile("nope"))
	assert.Empty(t, DescribeProfile("default"))
}
