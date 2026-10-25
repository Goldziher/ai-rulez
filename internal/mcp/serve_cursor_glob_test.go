package mcp

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCursorGlob(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		file    string
		want    bool
	}{
		{name: "a bare pattern matches at any depth", pattern: "*.tsx", file: "a/b/c.tsx", want: true},
		{name: "a bare pattern matches at the top", pattern: "*.tsx", file: "c.tsx", want: true},
		{name: "a pattern with a slash is anchored", pattern: "src/*.ts", file: "a/src/x.ts"},
		{name: "a pattern with a slash matches where it is anchored", pattern: "src/*.ts", file: "src/x.ts", want: true},
		{name: "every brace alternative without a slash matches at any depth", pattern: "{*.ts,src/*.tsx}", file: "a/b/c.ts", want: true},
		{name: "an alternative with a slash stays anchored", pattern: "{*.ts,src/*.tsx}", file: "a/src/c.tsx"},
		{name: "an alternative with a slash matches where anchored", pattern: "{*.ts,src/*.tsx}", file: "src/c.tsx", want: true},
		{name: "brackets are literal", pattern: "[id].tsx", file: "pages/[id].tsx", want: true},
		{name: "brackets are not a class", pattern: "[id].tsx", file: "pages/i.tsx"},
		{name: "a trailing slash means everything below", pattern: "src/", file: "src/a/b.go", want: true},
		{name: "a globstar crosses directories", pattern: "src/**/*.go", file: "src/a/b/c.go", want: true},
		{name: "a leading dot slash is dropped", pattern: "./src/*.go", file: "src/c.go", want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			g, ok := cursorGlob(tt.pattern)
			assert.True(t, ok)

			// Act
			got := g.match(tt.file)

			// Assert
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCursorGlob_Unusable(t *testing.T) {
	for _, pattern := range []string{"!*.tsx", "{a,b}{c,d}{e,f}{g,h}{i,j}{k,l}{m,n}"} {
		_, ok := cursorGlob(pattern)
		assert.False(t, ok, pattern)
	}
}

func TestCursorGlob_MayMatchBelow(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		dir     string
		want    bool
	}{
		{name: "a bare pattern reaches any directory", pattern: "*.tsx", dir: "pages/admin", want: true},
		{name: "an anchored pattern below the directory", pattern: "src/*.ts", dir: "src", want: true},
		{name: "an anchored pattern elsewhere", pattern: "src/*.ts", dir: "docs"},
		{name: "a slash-less alternative reaches any directory", pattern: "{src/*.tsx,*.ts}", dir: "docs", want: true},
		{name: "a bracket never hides a directory", pattern: "[id].tsx", dir: "pages", want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			g, ok := cursorGlob(tt.pattern)
			assert.True(t, ok)

			// Act
			got := g.below(tt.dir)

			// Assert
			assert.Equal(t, tt.want, got)
		})
	}
}
