package commands

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEveryJSONFlagHasAFormatFlagAndIsAHiddenAlias(t *testing.T) {
	// Arrange
	var checked int
	walkCommands(RootCmd, func(cmd *cobra.Command) {
		// Act
		jf := cmd.Flags().Lookup("json")
		if jf == nil {
			return
		}
		checked++

		// Assert
		ff := cmd.Flags().Lookup("format")
		if assert.NotNil(t, ff, "%s has --json but no --format", cmd.CommandPath()) {
			assert.Contains(t, formatsOf(ff), formatJSON, "%s --format must accept json", cmd.CommandPath())
			assert.Contains(t, formatsOf(ff), formatText, "%s --format must accept text", cmd.CommandPath())
		}
		assert.True(t, jf.Hidden, "%s --json must be hidden", cmd.CommandPath())
		assert.NotEmpty(t, jf.Deprecated, "%s --json must be deprecated", cmd.CommandPath())
	})
	assert.Positive(t, checked)
}

func TestEveryFormatFlagDeclaresItsValuesAndDefault(t *testing.T) {
	walkCommands(RootCmd, func(cmd *cobra.Command) {
		ff := cmd.Flags().Lookup("format")
		if ff == nil || cmd.Name() == "init" || cmd.Name() == "hook" { // config / template formats, not report formats
			return
		}
		assert.NotEmpty(t, formatsOf(ff), "%s --format must declare its values", cmd.CommandPath())
		assert.NotEmpty(t, ff.DefValue, "%s --format must show its default", cmd.CommandPath())
		assert.NotContains(t, ff.Usage, "(default", "%s --format usage must not repeat the default", cmd.CommandPath())
	})
}

func TestNormalizeFormatFlags(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantJSON bool
		wantErr  string
	}{
		{"neither", nil, false, ""},
		{"--format json sets the bool", []string{"--format", "json"}, true, ""},
		{"--json maps to json", []string{"--json"}, true, ""},
		{"--json with --format json", []string{"--json", "--format", "json"}, true, ""},
		{"--json=false is a no-op", []string{"--json=false"}, false, ""},
		{"--json conflicts with --format text", []string{"--json", "--format", "text"}, false, "--json conflicts with --format text"},
		{"unknown value", []string{"--format", "yaml"}, false, `unknown --format "yaml" (use text, json)`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			var asJSON bool
			cmd := &cobra.Command{Use: "x"}
			addJSONFormat(cmd.Flags(), &asJSON, "j")
			require.NoError(t, cmd.ParseFlags(tt.args))

			// Act
			err := normalizeFormatFlags(cmd)

			// Assert
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantJSON, asJSON)
		})
	}
}

func TestCheckFormatWording(t *testing.T) {
	err := checkFormat("yaml", []string{"text", "json", "sarif"})
	require.Error(t, err)
	assert.Equal(t, `unknown --format "yaml" (use text, json, sarif)`, err.Error())
	assert.NoError(t, checkFormat("", []string{"text"}))
	assert.NoError(t, checkFormat("sarif", []string{"text", "sarif"}))
}

func formatsOf(f *pflag.Flag) []string {
	return strings.Split(strings.Join(f.Annotations[formatValuesAnnotation], ","), ",")
}
