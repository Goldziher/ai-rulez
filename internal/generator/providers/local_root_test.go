package providers_test

import (
	"testing"

	"github.com/Goldziher/ai-rulez/internal/generator/providers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRootSpec_LocalFile(t *testing.T) {
	tests := []struct {
		name      string
		localFile string
		want      string
		wantErr   bool
	}{
		{name: "default is the .local variant", want: "DEMO.local.md"},
		{name: "none disables the local root", localFile: `local_file = "none"`, want: ""},
		{name: "explicit path", localFile: `local_file = "rules/demo.local.md"`, want: "rules/demo.local.md"},
		{name: "absolute path is rejected", localFile: `local_file = "/etc/demo.md"`, wantErr: true},
		{name: "parent path is rejected", localFile: `local_file = "../demo.md"`, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			spec := "name = \"demo\"\n\n[root]\nfile = \"DEMO.md\"\nsections = [\"title\"]\n" + tt.localFile + "\n"

			// Act
			parsed, err := providers.LoadProviderSpec([]byte(spec), "demo.toml", providers.FormatTOML)

			// Assert
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, providers.New(parsed).LocalRootFile())
		})
	}
}
