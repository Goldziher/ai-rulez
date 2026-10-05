package vspec

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCompileGlob(t *testing.T) {
	tests := []struct {
		pattern string
		path    string
		want    bool
	}{
		{"src/**/*.go", "src/a/b.go", true},
		{"src/**/*.go", "src/b.go", true},
		{"*.go", "x/y/z.go", true},
		{"src/*.go", "src/a/b.go", false},
		{"{a,b}/*.md", "b/x.md", true},
		{"{a,b}/*.md", "c/x.md", false},
		{"docs/", "docs/a/b.md", true},
		{"a?.txt", "ab.txt", true},
	}
	for _, tt := range tests {
		t.Run(tt.pattern+" "+tt.path, func(t *testing.T) {
			g, err := CompileGlob(tt.pattern)

			require.NoError(t, err)
			assert.Equal(t, tt.want, g.Match(tt.path))
		})
	}
}

func TestCompileGlob_RejectsExponentialBraces(t *testing.T) {
	exploding := strings.Repeat("{a,b}", 30)

	_, err := CompileGlob(exploding)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "alternatives")
}

func TestCompileGlob_AcceptsBracesAtTheCap(t *testing.T) {
	_, err := CompileGlob("{a,b}{c,d}{e,f}{g,h}{i,j}{k,l}") // 64 alternatives

	assert.NoError(t, err)
}

func TestParseKey(t *testing.T) {
	tests := []struct {
		key     string
		want    []string
		wantErr bool
	}{
		{"a.b.c", []string{"a", "b", "c"}, false},
		{`dependencies.lodash\.merge`, []string{"dependencies", "lodash.merge"}, false},
		{`dependencies["lodash.merge"]`, []string{"dependencies", "lodash.merge"}, false},
		{`dependencies.["x"]`, []string{"dependencies", "x"}, false},
		{`a["b"].c`, []string{"a", "b", "c"}, false},
		{`list[0].id`, []string{"list", "0", "id"}, false},
		{`a\\b`, []string{`a\b`}, false},
		{`["a\"b"]`, []string{`a"b`}, false},
		{"a..b", nil, true},
		{".a", nil, true},
		{"a.", nil, true},
		{`a\`, nil, true},
		{`a["x`, nil, true},
		{`a[x]`, nil, true},
		{`a["x"]y`, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			got, err := ParseKey(tt.key)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
