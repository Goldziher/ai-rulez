package commands

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPluginSelection(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		plugin    bool
		recursive bool
		want      pluginSelection
		wantErr   string
	}{
		{name: "no selector", plugin: false},
		{name: "plugin names", args: []string{"--plugin-name", "a", "--plugin-name", "b"}, plugin: true,
			want: pluginSelection{plugins: []string{"a", "b"}}},
		{name: "marketplace name", args: []string{"--marketplace-name", "m"}, plugin: true,
			want: pluginSelection{marketplaces: []string{"m"}}},
		{name: "without --plugin", args: []string{"--plugin-name", "a"}, wantErr: "only apply to --plugin",
			want: pluginSelection{plugins: []string{"a"}}},
		{name: "with --recursive", args: []string{"--plugin-name", "a"}, plugin: true, recursive: true,
			wantErr: "--recursive", want: pluginSelection{plugins: []string{"a"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			cmd := &cobra.Command{Use: "x"}
			addPluginSelectorFlags(cmd.Flags())
			require.NoError(t, cmd.ParseFlags(tt.args))

			// Act
			got, err := pluginSelectionOf(cmd)
			require.NoError(t, err)
			checkErr := got.check(tt.plugin, tt.recursive)

			// Assert
			assert.ElementsMatch(t, tt.want.plugins, got.plugins)
			assert.ElementsMatch(t, tt.want.marketplaces, got.marketplaces)
			if tt.wantErr == "" {
				assert.NoError(t, checkErr)
				return
			}
			require.Error(t, checkErr)
			assert.Contains(t, checkErr.Error(), tt.wantErr)
		})
	}
}
