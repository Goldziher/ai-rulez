package crud

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestChecks_AddListReadRemove(t *testing.T) {
	// Arrange
	dir := setupTestProject(t)
	op, err := NewOperator(dir)
	require.NoError(t, err)
	ctx := context.Background()

	// Act
	root, err := op.AddCheck(ctx, &AddFileRequest{Name: "security", Description: "Security issues"})
	require.NoError(t, err)
	_, err = op.AddCheck(ctx, &AddFileRequest{Name: "security"})

	// Assert
	assert.Equal(t, filepath.Join(dir, ".ai-rulez", "checks", "security.md"), root.FullPath)
	data, readErr := os.ReadFile(root.FullPath)
	require.NoError(t, readErr)
	fm, _ := frontmatterOf(t, string(data))
	assert.Equal(t, "Security issues", fm["description"])
	assert.Equal(t, "medium", fm["severity"])
	var existsErr *FileExistsError
	assert.ErrorAs(t, err, &existsErr)

	files, err := op.ListFiles(ctx, "", ContentTypeChecks)
	require.NoError(t, err)
	require.Len(t, files, 1)
	assert.Equal(t, "security", files[0].Name)

	require.NoError(t, op.RemoveFile(ctx, "", ContentTypeChecks, "security"))
	assert.NoFileExists(t, root.FullPath)
}

func TestChecks_Domain(t *testing.T) {
	// Arrange
	dir := setupTestProject(t)
	op, err := NewOperator(dir)
	require.NoError(t, err)
	ctx := context.Background()
	_, err = op.AddDomain(ctx, &AddDomainRequest{Name: "backend"})
	require.NoError(t, err)

	// Act
	res, err := op.AddCheck(ctx, &AddFileRequest{Name: "perf", Domain: "backend", Content: "Look at hot loops."})

	// Assert
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, ".ai-rulez", "domains", "backend", "checks", "perf.md"), res.FullPath)
	files, err := op.ListFiles(ctx, "backend", ContentTypeChecks)
	require.NoError(t, err)
	assert.Len(t, files, 1)
}

func TestChecks_RejectsUnsafeNames(t *testing.T) {
	dir := setupTestProject(t)
	op, err := NewOperator(dir)
	require.NoError(t, err)

	for _, name := range []string{"", "..", "../escape", "a/b", "has space", "x-->y", "semi;colon"} {
		t.Run(name, func(t *testing.T) {
			_, err := op.AddCheck(context.Background(), &AddFileRequest{Name: name})
			assert.Error(t, err)
		})
	}
	assert.NoDirExists(t, filepath.Join(dir, ".ai-rulez", "checks"))
}

// frontmatterOf splits a check file into its parsed frontmatter and body.
func frontmatterOf(t *testing.T, content string) (map[string]any, string) {
	t.Helper()
	content = config.NativeContent(content) // a stored check is an OKF concept
	require.True(t, strings.HasPrefix(content, "---\n"), content)
	rest := strings.TrimPrefix(content, "---\n")
	fmText, body, found := strings.Cut(rest, "\n---\n")
	if !found {
		fmText, found = strings.CutSuffix(rest, "\n---")
		require.True(t, found, content)
	}
	var fm map[string]any
	require.NoError(t, yaml.Unmarshal([]byte(fmText), &fm), content)
	return fm, body
}

func TestBuildCheckContent_MarshalsFrontmatterWithYAML(t *testing.T) {
	tests := []struct {
		name        string
		description string
	}{
		{"plain", "Flags injection"},
		{"colon", "a: b"},
		{"quotes and hash", `say "hi" # not a comment`},
		{"leading dash", "- item"},
		{"multi line", "line one\nline two"},
		{"document marker", "---"},
		{"brackets", "[x] {y}"},
		{"unicode", "Überprüfung ✓"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			got, err := BuildCheckContent("Body", tt.description, "HIGH", []string{"Read", "Grep: all"}, []string{"cursor", "src/**"})

			// Assert
			require.NoError(t, err)
			fm, body := frontmatterOf(t, got)
			assert.Equal(t, tt.description, fm["description"])
			assert.Equal(t, "high", fm["severity"])
			assert.Equal(t, []any{"Read", "Grep: all"}, fm["tools"])
			assert.Equal(t, []any{"cursor", "src/**"}, fm["targets"])
			assert.Equal(t, "Body", strings.TrimSpace(body))
		})
	}
}

func TestBuildCheckContent_NoFieldsLeavesTheBodyAlone(t *testing.T) {
	got, err := BuildCheckContent("Just text.", "", "", nil, nil)

	require.NoError(t, err)
	assert.Equal(t, "Just text.", got)
}

func TestBuildCheckContent_FullFileWithFrontmatter(t *testing.T) {
	// Without structured fields the file is taken as written.
	got, err := BuildCheckContent("---\nseverity: low\n---\nx", "", "", nil, nil)
	require.NoError(t, err)
	assert.Equal(t, "---\nseverity: low\n---\nx", got)

	// With fields they are applied over it, and its other keys survive.
	got, err = BuildCheckContent("---\nseverity: low\ncustom: 1\n---\nx", "Desc", "high", nil, nil)
	require.NoError(t, err)
	fm, body := frontmatterOf(t, got)
	assert.Equal(t, "high", fm["severity"])
	assert.Equal(t, "Desc", fm["description"])
	assert.Equal(t, 1, fm["custom"])
	assert.Equal(t, "x", strings.TrimSpace(body))
}

func TestBuildCheckContent_RejectsBadSeverityAndTargets(t *testing.T) {
	tests := []struct {
		name     string
		severity string
		targets  []string
		wantErr  string
	}{
		{"severity", "urgent", nil, "severity"},
		{"misspelled preset", "", []string{"curser"}, "curser"},
		{"bad glob", "", []string{"src/[x"}, "src/[x"},
		{"unknown bare word", "", []string{"cursor", "nonsense"}, "nonsense"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := BuildCheckContent("x", "", tt.severity, nil, tt.targets)

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestValidateCheckTargets_AcceptsPresetsAndGlobs(t *testing.T) {
	for _, target := range []string{"cursor", "Cursor", "claude", "*", "**", "src/**", "**/*.go", "REVIEW.md", ".cursor/BUGBOT.md", "docs/"} {
		assert.NoError(t, ValidateCheckTargets([]string{target}), target)
	}
}

func TestMergeCheckContent(t *testing.T) {
	existing := "---\n# the team's note\ndescription: Old\nseverity: low\ncustom: keep\n---\n\nOld body.\n"

	t.Run("new body keeps the frontmatter", func(t *testing.T) {
		got, err := MergeCheckContent(existing, "New body.", true, CheckFields{})

		require.NoError(t, err)
		assert.Contains(t, got, "# the team's note")
		assert.Contains(t, got, "custom: keep")
		fm, body := frontmatterOf(t, got)
		assert.Equal(t, "Old", fm["description"])
		assert.Equal(t, "low", fm["severity"])
		assert.Equal(t, "New body.", strings.TrimSpace(body))
	})

	t.Run("fields alone keep the body", func(t *testing.T) {
		got, err := MergeCheckContent(existing, "", false, CheckFields{Severity: "critical", Tools: []string{"Read"}})

		require.NoError(t, err)
		fm, body := frontmatterOf(t, got)
		assert.Equal(t, "critical", fm["severity"])
		assert.Equal(t, []any{"Read"}, fm["tools"])
		assert.Equal(t, "keep", fm["custom"])
		assert.Equal(t, "Old body.", strings.TrimSpace(body))
	})

	t.Run("content with its own frontmatter replaces the old one", func(t *testing.T) {
		got, err := MergeCheckContent(existing, "---\nseverity: high\n---\nFresh.", true, CheckFields{})

		require.NoError(t, err)
		fm, body := frontmatterOf(t, got)
		assert.Equal(t, "high", fm["severity"])
		assert.NotContains(t, fm, "custom")
		assert.Equal(t, "Fresh.", strings.TrimSpace(body))
	})

	t.Run("no existing frontmatter and no fields stays plain", func(t *testing.T) {
		got, err := MergeCheckContent("Plain body.\n", "Other body.", true, CheckFields{})

		require.NoError(t, err)
		assert.Equal(t, "Other body.", got)
	})

	t.Run("invalid existing frontmatter is an error", func(t *testing.T) {
		_, err := MergeCheckContent("---\n: : [\n---\nx", "", false, CheckFields{Severity: "low"})

		require.Error(t, err)
	})
}

func TestUpdateCheck(t *testing.T) {
	// Arrange
	dir := setupTestProject(t)
	op, err := NewOperator(dir)
	require.NoError(t, err)
	ctx := context.Background()
	created, err := op.AddCheck(ctx, &AddFileRequest{Name: "sec", Content: "---\ndescription: D\nseverity: low\n---\n\nBody.\n"})
	require.NoError(t, err)

	t.Run("nothing to update is rejected", func(t *testing.T) {
		_, err := op.UpdateCheck(ctx, "", "sec", "", false, CheckFields{})

		require.Error(t, err)
		data, readErr := os.ReadFile(created.FullPath)
		require.NoError(t, readErr)
		assert.Contains(t, string(data), "Body.")
	})

	t.Run("body only keeps frontmatter", func(t *testing.T) {
		_, err := op.UpdateCheck(ctx, "", "sec", "Changed.", true, CheckFields{})

		require.NoError(t, err)
		data, readErr := os.ReadFile(created.FullPath)
		require.NoError(t, readErr)
		fm, body := frontmatterOf(t, string(data))
		assert.Equal(t, "D", fm["description"])
		assert.Equal(t, "low", fm["severity"])
		assert.NotContains(t, fm, "priority", "a check never gets a rule's priority key")
		assert.Equal(t, "Changed.", strings.TrimSpace(body))
	})

	t.Run("missing check", func(t *testing.T) {
		_, err := op.UpdateCheck(ctx, "", "absent", "x", true, CheckFields{})

		assert.ErrorIs(t, err, ErrFileNotFound)
	})
}

func TestReservedDomainNameChecks(t *testing.T) {
	assert.Error(t, ValidateDomainName("checks"))
}
