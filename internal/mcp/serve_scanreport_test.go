package mcp

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServeSetup_ServedScanReportsCoverAuthoredAndSourceSkills(t *testing.T) {
	// Arrange: authored skills the server refuses or serves with a warning, and a
	// source skill at trust=error with an unscannable supporting file.
	dir := unscannableSource(t)
	root := project(t, baseConfig+"\n[skills]\ndelivery = \"served\"\n", map[string]string{
		"skills/nulmd/SKILL.md":         skillFile("nulmd", "SKILL.md holds a NUL byte", "") + "\x00",
		"skills/bigref/SKILL.md":        skillFile("bigref", "Reference over 512 KiB", ""),
		"skills/bigref/references/b.md": strings.Repeat("x", 600*1024),
	})

	// Act
	reports, err := (&ServeSetup{WorkDir: root, NoWatch: true}).ServedScanReports(context.Background(),
		ServeSetup{WorkDir: root, NoWatch: true, Sources: []string{dir}})

	// Assert
	require.NoError(t, err)
	type key struct{ skill, sev string }
	got := map[key]ScanReport{}
	for _, r := range reports {
		for _, f := range r.Findings {
			require.Equal(t, "AR989", f.Code)
			got[key{r.Skill, string(f.Severity)}] = r
		}
	}
	assert.Contains(t, got, key{"nulmd", "error"}, "an unscannable authored SKILL.md is an error")
	assert.Contains(t, got, key{"bigref", "warning"}, "an unscannable authored reference is a warning at trust=warn")
	assert.Equal(t, "warn", got[key{"bigref", "warning"}].Level)
	assert.Empty(t, got[key{"bigref", "warning"}].Unserved, "trust=warn serves the file")
	assert.Contains(t, got, key{"b", "error"}, "an unscannable source SKILL.md is an error")
	a := got[key{"a", "warning"}]
	assert.Equal(t, "error", a.Level)
	assert.ElementsMatch(t, []string{"references/big.md", "references/nul.md"}, a.Unserved)
}
