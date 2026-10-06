package govview

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
)

func TestUniqueRef(t *testing.T) {
	// Arrange
	seen := map[string]int{}

	// Act
	got := []string{
		uniqueRef(seen, "skill", "ops", "deploy"),
		uniqueRef(seen, "rule", "", "style"),
		uniqueRef(seen, "skill", "ops", "deploy"),
		uniqueRef(seen, "skill", "ops", "deploy"),
	}

	// Assert
	assert.Equal(t, []string{"skill/ops/deploy", "rule/-/style", "skill/ops/deploy#2", "skill/ops/deploy#3"}, got)
}

func TestExcerptOf(t *testing.T) {
	long := strings.Repeat("é", ExcerptLimit) // two bytes each: the cut must land on a rune start
	tests := []struct {
		name          string
		body          string
		wantText      string
		wantTruncated bool
	}{
		{"short body kept", "hello\n", "hello\n", false},
		{"CR and CRLF normalised", "a\r\nb\rc", "a\nb\nc", false},
		{"frontmatter dropped", "---\nname: x\n---\n\nbody", "body", false},
		{"invalid UTF-8 replaced", "a\xffb", "a�b", false},
		{"frontmatter closed at the end of the text", "---\nname: x\n---", "", false},
		{"empty frontmatter", "---\n---\nbody", "body", false},
		{"a longer dash line is not the closing fence", "---\nname: x\n----\nbody", "---\nname: x\n----\nbody", false},
		{"an unclosed fence is not frontmatter", "---\nintro text\nmore", "---\nintro text\nmore", false},
		{"a horizontal rule later in the body is kept", "---\nname: x\n---\nbody\n---\nmore", "body\n---\nmore", false},
		{"no frontmatter keeps the leading rule text", "---not a fence\nbody", "---not a fence\nbody", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			got := excerptOf(tt.body)

			// Assert
			assert.Equal(t, tt.wantText, got.Text)
			assert.Equal(t, tt.wantTruncated, got.Truncated)
		})
	}
	t.Run("long body cut on a rune boundary", func(t *testing.T) {
		// Act
		got := excerptOf(long)

		// Assert
		assert.True(t, got.Truncated)
		assert.True(t, utf8.ValidString(got.Text))
		assert.LessOrEqual(t, len(got.Text), ExcerptLimit)
		assert.Greater(t, len(got.Text), ExcerptLimit-4)
	})
}

func TestLintAttribution(t *testing.T) {
	// Arrange
	base := t.TempDir()
	skillDir := filepath.Join(base, ".ai-rulez", "skills", "deploy")
	require.NoError(t, os.MkdirAll(skillDir, 0o755))
	skill := &config.ContentFile{Name: "deploy", Path: filepath.Join(skillDir, "SKILL.md"),
		Resources: []config.SkillResource{{Kind: "scripts", RelPath: "scripts/run.sh"}}}
	rule := &config.ContentFile{Name: "style", Path: filepath.Join(base, ".ai-rulez", "rules", "style.md")}
	cfg := &config.Config{BaseDir: base, ConfigDir: filepath.Join(base, ".ai-rulez")}
	report := &lint.Report{Findings: []lint.Finding{
		{Code: "AR802", Severity: lint.SeverityWarning, File: skill.Path, Line: 9, Message: "late in " + base + "/x"},
		{Code: "AR010", Severity: lint.SeverityError, File: filepath.Join(skillDir, "scripts", "run.sh"), Line: 1, Message: "script"},
		{Code: "AR802", Severity: lint.SeverityInfo, File: skill.Path, Line: 2, Message: "early"},
		{Code: "AR900", Severity: lint.SeverityError, File: filepath.Join(base, ".ai-rulez", "config.toml"), Line: 3, Message: "config"},
		{Code: "AR901", Severity: lint.SeverityWarning, File: "/elsewhere/file.md", Line: 1, Message: "outside"},
	}}
	attr := newLintAttribution(cfg, report)

	// Act
	skillLint := attr.forItem(skill)
	ruleLint := attr.forItem(rule)
	overview := attr.overview("")

	// Assert
	assert.Equal(t, LintError, skillLint.Status)
	assert.Equal(t, LintCounts{Error: 1, Warning: 1, Info: 1}, skillLint.Counts)
	require.Len(t, skillLint.Findings, 3)
	assert.Equal(t, []int{1, 2, 9}, []int{skillLint.Findings[0].Line, skillLint.Findings[1].Line, skillLint.Findings[2].Line}, "sorted by line")
	assert.NotContains(t, skillLint.Findings[2].Message, base, "no absolute path of this machine")
	assert.Equal(t, LintOK, ruleLint.Status)
	assert.True(t, overview.Available)
	assert.Equal(t, LintSummary{Errors: 2, Warnings: 2, Infos: 1}, overview.Summary)
	assert.Equal(t, map[string]int{"AR802": 2, "AR010": 1, "AR900": 1, "AR901": 1}, overview.ByCode)
	require.Len(t, overview.Unattributed, 2)
	assert.Equal(t, ".ai-rulez/config.toml", overview.Unattributed[0].File)
	assert.Empty(t, overview.Unattributed[1].File, "a file outside the project is not named")
}

func TestLintAttributionWithoutReport(t *testing.T) {
	// Arrange
	attr := newLintAttribution(&config.Config{BaseDir: t.TempDir()}, nil)

	// Act
	overview := attr.overview("could not run")

	// Assert
	assert.False(t, overview.Available)
	assert.Equal(t, "could not run", overview.Reason)
	assert.NotNil(t, overview.Unattributed)
}

func TestViewCatalogV2(t *testing.T) {
	// Arrange
	doc := &CatalogDocV2{
		Items: []CatalogItemV2{{ID: "a", Roles: []string{"dev"}}, {ID: "b", Roles: []string{"ops"}}, {ID: "c", Roles: []string{"dev", "ops"}}},
		Roles: []CatalogRole{{Name: "dev"}, {Name: "ops"}},
	}
	tests := []struct {
		name    string
		role    string
		want    []string
		wantErr bool
	}{
		{"no role keeps everything", "", []string{"a", "b", "c"}, false},
		{"role filters items and roles", "dev", []string{"a", "c"}, false},
		{"unknown role", "nope", nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			got, err := ViewCatalogV2(doc, tt.role)

			// Assert
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			var ids []string
			for _, it := range got.Items {
				ids = append(ids, it.ID)
			}
			assert.Equal(t, tt.want, ids)
			if tt.role != "" {
				require.Len(t, got.Roles, 1)
				assert.Len(t, doc.Roles, 2, "the source document is not modified")
			}
		})
	}
}

func TestLintAttributionScrubsBothSeparatorsAndKeepsDotDotNames(t *testing.T) {
	// Arrange
	base := t.TempDir()
	cfg := &config.Config{BaseDir: base, ConfigDir: filepath.Join(base, ".ai-rulez")}
	report := &lint.Report{Findings: []lint.Finding{
		{Code: "AR900", Severity: lint.SeverityWarning, File: filepath.Join(base, "..hidden", "notes.md"), Line: 1, Message: "see " + base + `\sub\f.md and ` + base + "/sub/g.md"},
		{Code: "AR901", Severity: lint.SeverityWarning, File: filepath.Join(filepath.Dir(base), "sibling.md"), Line: 1, Message: "outside"},
	}}
	attr := newLintAttribution(cfg, report)

	// Act
	overview := attr.overview("")

	// Assert
	require.Len(t, overview.Unattributed, 2)
	assert.Equal(t, "..hidden/notes.md", overview.Unattributed[0].File, "a name that merely starts with two dots is inside the project")
	assert.Equal(t, "see sub\\f.md and sub/g.md", overview.Unattributed[0].Message)
	assert.Empty(t, overview.Unattributed[1].File, "a sibling of the project is outside it")
}
