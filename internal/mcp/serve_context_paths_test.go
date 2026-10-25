package mcp

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Goldziher/ai-rulez/v5/internal/repoindex"
)

func TestAppliesTo(t *testing.T) {
	paths := func(globs ...string) *repoindex.PathGlobs { return &repoindex.PathGlobs{Paths: globs} }
	cursor := func(globs ...string) *repoindex.PathGlobs { return &repoindex.PathGlobs{Globs: globs} }
	tests := []struct {
		name  string
		kind  repoindex.Kind
		scope string
		only  *repoindex.PathGlobs
		path  string
		asDir bool
		want  bool
	}{
		{name: "no restriction, file", kind: repoindex.KindSkill, scope: ".", path: "a/b.go", want: true},
		{name: "no restriction, directory", kind: repoindex.KindSkill, scope: ".", path: "a", asDir: true, want: true},
		{name: "skill, matching file", kind: repoindex.KindSkill, scope: ".", only: paths("backend/services/**"), path: "backend/services/x/k.py", want: true},
		{name: "skill, other file", kind: repoindex.KindSkill, scope: ".", only: paths("backend/services/**"), path: "products/web/k.py"},
		{name: "rule, matching file", kind: repoindex.KindRule, scope: ".", only: paths("**/*.go"), path: "cmd/main.go", want: true},
		{name: "rule, other file", kind: repoindex.KindRule, scope: ".", only: paths("**/*.go"), path: "README.md"},
		{name: "directory with a glob below it", kind: repoindex.KindSkill, scope: ".", only: paths("backend/services/**/kafka*.py"), path: "backend/services", asDir: true, want: true},
		{name: "directory with no glob below it", kind: repoindex.KindSkill, scope: ".", only: paths("backend/services/**/kafka*.py"), path: "products/web", asDir: true},
		{name: "any depth glob keeps any directory", kind: repoindex.KindSkill, scope: ".", only: paths("**/*.py"), path: "products/web", asDir: true, want: true},
		{name: "directory inside the literal prefix", kind: repoindex.KindSkill, scope: ".", only: paths("backend/services/**"), path: "backend/services/kafka", asDir: true, want: true},
		{name: "directory above the literal prefix", kind: repoindex.KindSkill, scope: ".", only: paths("backend/services/x.py"), path: "backend", asDir: true, want: true},
		{name: "rule, directory it does not reach", kind: repoindex.KindRule, scope: ".", only: paths("**/*.go", "docs/**"), path: "docs", asDir: true, want: true},
		{name: "any one glob is enough", kind: repoindex.KindSkill, scope: ".", only: paths("a/**", "b/**"), path: "b/x", asDir: true, want: true},
		{name: "the root directory", kind: repoindex.KindSkill, scope: ".", only: paths("a/**"), path: ".", asDir: true, want: true},
		{name: "the scope directory itself", kind: repoindex.KindSkill, scope: "pkg/api", only: paths("handlers/**"), path: "pkg/api", asDir: true, want: true},
		{name: "scope-relative globs, file", kind: repoindex.KindSkill, scope: "pkg/api", only: paths("handlers/**"), path: "pkg/api/handlers/u.go", want: true},
		{name: "scope-relative globs, anchored", kind: repoindex.KindSkill, scope: "pkg/api", only: paths("handlers/**"), path: "pkg/api/other/handlers/u.go"},
		{name: "scope-relative globs, directory", kind: repoindex.KindSkill, scope: "pkg/api", only: paths("handlers/**"), path: "pkg/api/other", asDir: true},
		{name: "file outside the scope", kind: repoindex.KindSkill, scope: "pkg/api", only: paths("**"), path: "pkg/web/x.go"},
		{name: "scope directory as a file", kind: repoindex.KindSkill, scope: "pkg/api", only: paths("**"), path: "pkg/api"},
		{name: "unreadable globs do not hide, file", kind: repoindex.KindSkill, scope: ".", only: paths("!x", ""), path: "a.go", want: true},
		{name: "unreadable globs do not hide, directory", kind: repoindex.KindSkill, scope: ".", only: paths("!x"), path: "a", asDir: true, want: true},
		{name: "cursor bare glob matches at depth", kind: repoindex.KindRule, scope: "pkg/web", only: cursor("*.tsx"), path: "pkg/web/src/App.tsx", want: true},
		{name: "cursor bare glob, other file", kind: repoindex.KindRule, scope: "pkg/web", only: cursor("*.tsx"), path: "pkg/web/src/App.ts"},
		{name: "cursor bare glob keeps a directory", kind: repoindex.KindRule, scope: "pkg/web", only: cursor("*.tsx"), path: "pkg/web/src", asDir: true, want: true},
		{name: "cursor glob with a slash, directory", kind: repoindex.KindRule, scope: ".", only: cursor("src/**/*.tsx"), path: "docs", asDir: true},
		{name: "cursor negation is unusable", kind: repoindex.KindRule, scope: ".", only: cursor("!*.tsx"), path: "a.go", want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			it := &repoindex.Item{Kind: tt.kind, Scope: tt.scope, Only: tt.only}

			// Act
			got := appliesTo(it, tt.path, tt.asDir)

			// Assert
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRepoView_IsDir(t *testing.T) {
	root := project(t, discoverConfig, map[string]string{"pkg/api/main.go": "package api\n"})
	v := &repoView{root: root}
	tests := []struct {
		name string
		path string
		want bool
	}{
		{name: "an existing directory", path: "pkg/api", want: true},
		{name: "an existing file", path: "pkg/api/main.go"},
		{name: "the root", path: ".", want: true},
		{name: "a missing path with an extension is a file", path: "pkg/api/new.go"},
		{name: "a missing path without one is a directory", path: "pkg/new", want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, v.isDir(tt.path))
		})
	}
}
