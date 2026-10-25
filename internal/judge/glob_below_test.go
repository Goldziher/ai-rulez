package judge

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGlob_MayMatchBelow(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		dir     string
		want    bool
	}{
		{name: "a glob under the directory", pattern: "armis/services/**/kafka*.py", dir: "armis/services", want: true},
		{name: "an unrelated directory", pattern: "armis/services/**/kafka*.py", dir: "products/va", want: false},
		{name: "any depth reaches any directory", pattern: "**/*.py", dir: "products/va", want: true},
		{name: "the directory is inside the literal prefix", pattern: "armis/services/**", dir: "armis/services/kafka/lib", want: true},
		{name: "the literal prefix is inside the directory", pattern: "armis/services/x.py", dir: "armis", want: true},
		{name: "the pattern ends above the directory", pattern: "armis/x.py", dir: "armis/services", want: false},
		{name: "the pattern names the directory itself", pattern: "armis/services", dir: "armis/services", want: true},
		{name: "a wildcard segment", pattern: "armis/*/lib/*.py", dir: "armis/services", want: true},
		{name: "a wildcard segment that cannot match", pattern: "armis/s*ice/lib/*.py", dir: "armis/tools", want: false},
		{name: "the base directory itself", pattern: "src/**/*.go", dir: "", want: true},
		{name: "braces", pattern: "{src,lib}/**/*.go", dir: "lib/api", want: true},
		{name: "braces without a match", pattern: "{src,lib}/**/*.go", dir: "docs", want: false},
		{name: "a trailing slash means everything below", pattern: "src/", dir: "src/a/b", want: true},
		{name: "a leading dot slash is dropped", pattern: "./src/**", dir: "src", want: true},
		{name: "a character class", pattern: "v[0-9]/**", dir: "v2", want: true},
		{name: "a character class that cannot match", pattern: "v[0-9]/**", dir: "vx", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			g, ok := CompileGlob(tt.pattern)
			require.True(t, ok)

			// Act
			got := g.MayMatchBelow(tt.dir)

			// Assert
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestGlob_MayMatchBelowIsFalseForTheZeroGlob(t *testing.T) {
	assert.False(t, Glob{}.MayMatchBelow("src"))
}
