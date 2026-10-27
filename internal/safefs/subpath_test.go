package safefs

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSubPath(t *testing.T) {
	root := filepath.Join("a", "src")
	tests := []struct {
		name    string
		sub     string
		want    string
		wantErr bool
	}{
		{name: "empty is the root", sub: "", want: root},
		{name: "slash is the root", sub: "/", want: root},
		{name: "dot is the root", sub: ".", want: root},
		{name: "nested", sub: "modules/x", want: filepath.Join(root, "modules", "x")},
		{name: "leading slash is trimmed", sub: "/modules/x/", want: filepath.Join(root, "modules", "x")},
		{name: "inner dotdot stays inside", sub: "modules/../lib", want: filepath.Join(root, "lib")},
		{name: "parent", sub: "..", wantErr: true},
		{name: "parent prefix", sub: "../other", wantErr: true},
		{name: "climbs out after descending", sub: "a/../../b", wantErr: true},
		{name: "dotdot-named child is ordinary", sub: "..foo", want: filepath.Join(root, "..foo")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			got, err := SubPath(root, tt.sub)

			// Assert
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
