package importer

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const mergeCursorRule = "---\nglobs: '**/*.ts'\n---\nUse strict mode.\n"

func mergeProject() map[string]string {
	return map[string]string{
		".cursor/rules/ts.mdc":         mergeCursorRule,
		".claude/skills/lint/SKILL.md": "---\nname: lint\ndescription: Run the linter\n---\nRun lint.\n",
		"CLAUDE.md":                    "# Project\n",
	}
}

func TestConvert_MergeKeepsExistingFilesAndImportsBesideThem(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	writeTree(t, dir, mergeProject())
	writeTree(t, dir, map[string]string{
		".ai-rulez/rules/ts.md":          "Mine, hand written.\n",
		".ai-rulez/skills/lint/SKILL.md": "---\nname: lint\ndescription: Mine\n---\nMy lint.\n",
	})

	// Act
	report, err := Convert(context.Background(), ConvertOptions{Source: dir, Write: true, Merge: true})

	// Assert
	require.NoError(t, err)
	require.True(t, report.Written)
	got := snapshot(t, filepath.Join(dir, ".ai-rulez"))
	assert.Equal(t, "Mine, hand written.\n", got["rules/ts.md"], "an existing file is never touched")
	assert.Contains(t, got["rules/ts-imported.md"], "Use strict mode.")
	assert.Contains(t, got["skills/lint/SKILL.md"], "My lint.")
	assert.Contains(t, got["skills/lint-imported/SKILL.md"], "name: lint-imported", "a renamed skill carries its new name")
	assert.Contains(t, got["skills/lint-imported/SKILL.md"], "Run lint.")
	assert.Contains(t, got["context/claude.md"], "# Project")
	f := findingFor(&Plan{Findings: report.Findings}, StatusApproximated, ".cursor/rules/ts.mdc", "name")
	require.NotNil(t, f)
	assert.Equal(t, "rules/ts-imported.md", f.Target)
}

func TestConvert_MergeIsIdempotent(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	writeTree(t, dir, mergeProject())
	writeTree(t, dir, map[string]string{".ai-rulez/rules/ts.md": "Mine.\n"})
	_, err := Convert(context.Background(), ConvertOptions{Source: dir, Write: true, Merge: true})
	require.NoError(t, err)
	first := snapshot(t, filepath.Join(dir, ".ai-rulez"))

	// Act
	report, err := Convert(context.Background(), ConvertOptions{Source: dir, Write: true, Merge: true})

	// Assert
	require.NoError(t, err)
	assert.Equal(t, first, snapshot(t, filepath.Join(dir, ".ai-rulez")), "a second run adds nothing, not even ts-imported-2")
	for _, a := range report.Files {
		assert.Equal(t, ActionUnchanged, a.Action, a.Path)
	}
}

func TestConvert_WithoutMergeTheSameTreeConflicts(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	writeTree(t, dir, mergeProject())
	writeTree(t, dir, map[string]string{".ai-rulez/rules/ts.md": "Mine.\n"})

	// Act
	_, err := Convert(context.Background(), ConvertOptions{Source: dir, Write: true})

	// Assert
	require.ErrorIs(t, err, ErrConflicts)
}

func TestConvert_MergeAndForceExcludeEachOther(t *testing.T) {
	dir := t.TempDir()
	writeTree(t, dir, mergeProject())

	_, err := Convert(context.Background(), ConvertOptions{Source: dir, Write: true, Merge: true, Force: true})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "--merge and --force")
}

func TestConvert_MergeWithKeepNamesLeavesTheConflict(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	writeTree(t, dir, mergeProject())
	writeTree(t, dir, map[string]string{".ai-rulez/rules/ts.md": "Mine.\n"})

	// Act
	report, err := Convert(context.Background(), ConvertOptions{Source: dir, Write: true, Merge: true, KeepNames: true})

	// Assert
	require.ErrorIs(t, err, ErrConflicts)
	assert.Equal(t, 1, report.Conflicts())
	assert.NoFileExists(t, filepath.Join(dir, ".ai-rulez", "rules", "ts-imported.md"))
}

func TestConvert_KeepNamesRefusesACollisionBetweenImportedItems(t *testing.T) {
	// Two sources that map to the same name with different content.
	files := map[string]string{
		".cursor/rules/style.mdc":  "---\nglobs: '*.go'\n---\nGo style.\n",
		".windsurf/rules/style.md": "---\ntrigger: always_on\n---\nWindsurf style.\n",
	}
	tests := []struct {
		name      string
		keepNames bool
		wantErr   string
	}{
		{name: "renamed with a stable suffix by default"},
		{name: "keep-names reports the collision", keepNames: true, wantErr: "name collisions with --keep-names"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir := t.TempDir()
			writeTree(t, dir, files)

			// Act
			_, err := Convert(context.Background(), ConvertOptions{Source: dir, Write: true, KeepNames: tt.keepNames})

			// Assert
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				assert.NoDirExists(t, filepath.Join(dir, ".ai-rulez"))
				return
			}
			require.NoError(t, err)
			assert.Len(t, snapshot(t, filepath.Join(dir, ".ai-rulez", "rules")), 2)
		})
	}
}

func TestConvert_Delivery(t *testing.T) {
	tests := []struct {
		name       string
		opts       ConvertOptions
		existing   string
		wantGlobal string
		wantDomain string
		wantErr    string
	}{
		{name: "global default", opts: ConvertOptions{Delivery: "served"}, wantGlobal: "served"},
		{name: "domain default", opts: ConvertOptions{Delivery: "both", Domain: "imported"}, wantDomain: "both"},
		{name: "unknown value", opts: ConvertOptions{Delivery: "sometimes"}, wantErr: "unknown --delivery"},
		{name: "none leaves the config alone", opts: ConvertOptions{}},
		{
			name:       "an existing value wins",
			opts:       ConvertOptions{Delivery: "served"},
			existing:   "version = \"5.0\"\nname = \"mine\"\npresets = [\"claude\"]\n\n[skills]\ndelivery = \"static\"\n",
			wantGlobal: "static",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir := t.TempDir()
			writeTree(t, dir, mergeProject())
			if tt.existing != "" {
				writeTree(t, dir, map[string]string{".ai-rulez/config.toml": tt.existing})
			}
			tt.opts.Source, tt.opts.Write = dir, true

			// Act
			report, err := Convert(context.Background(), tt.opts)

			// Assert
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			data, readErr := os.ReadFile(filepath.Join(dir, ".ai-rulez", "config.toml"))
			require.NoError(t, readErr)
			cfg, decodeErr := config.DecodeTOMLConfig(data, "config.toml")
			require.NoError(t, decodeErr)
			global := ""
			if cfg.Skills != nil {
				global = cfg.Skills.Delivery
			}
			assert.Equal(t, tt.wantGlobal, global)
			assert.Equal(t, tt.wantDomain, cfg.DomainSettings[tt.opts.Domain].Delivery)
			if tt.existing != "" {
				assert.NotNil(t, findingFor(&Plan{Findings: report.Findings}, StatusNeedsAction, "config.toml", "skills.delivery"))
			}
		})
	}
}

func TestConvert_DeliveryWithoutSkillsIsReportedNotWritten(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{"CLAUDE.md": "x\n"})

	// Act
	report, err := Convert(context.Background(), ConvertOptions{Source: dir, Write: true, Delivery: "served"})

	// Assert
	require.NoError(t, err)
	data, _ := os.ReadFile(filepath.Join(dir, ".ai-rulez", "config.toml"))
	assert.NotContains(t, string(data), "delivery")
	assert.NotNil(t, findingFor(&Plan{Findings: report.Findings}, StatusApproximated, "(project)", "delivery"))
}
