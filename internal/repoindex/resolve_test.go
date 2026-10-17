package repoindex

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func resolvedKeys(items []Resolved) []string {
	out := make([]string, 0, len(items))
	for _, r := range items {
		key := string(r.Kind) + ":" + r.Scope + ":" + r.Name
		if r.ShadowedBy != "" {
			key += " (shadowed by " + r.ShadowedBy + ")"
		}
		out = append(out, key)
	}
	return out
}

func TestResolve(t *testing.T) {
	files := monorepo()
	files["pkg/api/v2/AGENTS.md"] = "# API v2\n"
	files["pkg/api/v2/.claude/agents/reviewer.md"] = ruleMD("Reviews v2")
	ix := build(t, layout(t, files), "claude", "cursor")

	tests := []struct {
		name       string
		path       string
		wantPath   string
		wantScopes []string
		wantItems  []string
	}{
		{
			name:       "the root resolves to the root scope",
			path:       ".",
			wantPath:   ".",
			wantScopes: []string{"."},
			wantItems: []string{
				"instruction:.:AGENTS.md", "instruction:.:CLAUDE.md", "rule:.:style", "skill:.:lint",
			},
		},
		{
			name:       "a directory outside every nested scope falls back to the root",
			path:       "docs",
			wantPath:   "docs",
			wantScopes: []string{"."},
			wantItems:  []string{"instruction:.:AGENTS.md", "instruction:.:CLAUDE.md", "rule:.:style", "skill:.:lint"},
		},
		{
			name:       "a file inside a scope resolves nearest first and the nearest skill shadows the farther one",
			path:       "pkg/api/handler.go",
			wantPath:   "pkg/api/handler.go",
			wantScopes: []string{"pkg/api", "."},
			wantItems: []string{
				"agent:pkg/api:reviewer", "instruction:pkg/api:AGENTS.md", "skill:pkg/api:lint",
				"instruction:.:AGENTS.md", "instruction:.:CLAUDE.md", "rule:.:style", "skill:.:lint (shadowed by pkg/api)",
			},
		},
		{
			name:       "a path that does not exist still resolves",
			path:       "pkg/api/new/dir/file.go",
			wantPath:   "pkg/api/new/dir/file.go",
			wantScopes: []string{"pkg/api", "."},
			wantItems: []string{
				"agent:pkg/api:reviewer", "instruction:pkg/api:AGENTS.md", "skill:pkg/api:lint",
				"instruction:.:AGENTS.md", "instruction:.:CLAUDE.md", "rule:.:style", "skill:.:lint (shadowed by pkg/api)",
			},
		},
		{
			name:       "three scopes deep: a nearer agent shadows the same-named one above, instructions add up",
			path:       "pkg/api/v2/x.go",
			wantPath:   "pkg/api/v2/x.go",
			wantScopes: []string{"pkg/api/v2", "pkg/api", "."},
			wantItems: []string{
				"agent:pkg/api/v2:reviewer", "instruction:pkg/api/v2:AGENTS.md",
				"agent:pkg/api:reviewer (shadowed by pkg/api/v2)", "instruction:pkg/api:AGENTS.md", "skill:pkg/api:lint",
				"instruction:.:AGENTS.md", "instruction:.:CLAUDE.md", "rule:.:style", "skill:.:lint (shadowed by pkg/api)",
			},
		},
		{
			name:       "a sibling scope does not apply",
			path:       "pkg/web/index.ts",
			wantPath:   "pkg/web/index.ts",
			wantScopes: []string{"pkg/web", "."},
			wantItems: []string{
				"rule:pkg/web:ui", "skill:pkg/web:ui",
				"instruction:.:AGENTS.md", "instruction:.:CLAUDE.md", "rule:.:style", "skill:.:lint",
			},
		},
		{
			name:       "the path is cleaned",
			path:       "./pkg//api/../web/",
			wantPath:   "pkg/web",
			wantScopes: []string{"pkg/web", "."},
			wantItems: []string{
				"rule:pkg/web:ui", "skill:pkg/web:ui",
				"instruction:.:AGENTS.md", "instruction:.:CLAUDE.md", "rule:.:style", "skill:.:lint",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			got, err := ix.Resolve(tt.path)

			// Assert
			require.NoError(t, err)
			assert.Equal(t, tt.wantPath, got.Path)
			assert.Equal(t, tt.wantScopes, got.Scopes)
			assert.Equal(t, tt.wantItems, resolvedKeys(got.Items))
		})
	}
}

func TestResolve_RejectsPathsThatEscape(t *testing.T) {
	ix := build(t, layout(t, monorepo()), "claude")

	for _, bad := range []string{"../etc", "pkg/../../x", "/abs/path", "a\\b", "a\x00b", "C:/x", ""} {
		t.Run(bad, func(t *testing.T) {
			_, err := ix.Resolve(bad)

			require.Error(t, err)
		})
	}
}

func TestWithin(t *testing.T) {
	files := monorepo()
	files["pkg/api/v2/AGENTS.md"] = "# API v2\n"
	ix := build(t, layout(t, files), "claude", "cursor")

	tests := []struct {
		name      string
		scope     string
		recursive bool
		want      []string
		wantErr   bool
	}{
		{name: "exact scope", scope: "pkg/api", want: []string{"agent:pkg/api:reviewer", "instruction:pkg/api:AGENTS.md", "skill:pkg/api:lint"}},
		{
			name: "recursive adds descendants", scope: "pkg/api", recursive: true,
			want: []string{"agent:pkg/api:reviewer", "instruction:pkg/api/v2:AGENTS.md", "instruction:pkg/api:AGENTS.md", "skill:pkg/api:lint"},
		},
		{name: "root exact has only root items", scope: ".", want: []string{"instruction:.:AGENTS.md", "instruction:.:CLAUDE.md", "rule:.:style", "skill:.:lint"}},
		{name: "root recursive is everything", scope: ".", recursive: true, want: keys(ix.Items())},
		{name: "a sibling prefix is not a descendant", scope: "pkg/ap", recursive: true, wantErr: true},
		{name: "unknown scope", scope: "nope", wantErr: true},
		{name: "escape", scope: "../x", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ix.Within(tt.scope, tt.recursive)

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, keys(got))
		})
	}
}

func TestCleanPath(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{in: "pkg/api", want: "pkg/api"},
		{in: "pkg/api/", want: "pkg/api"},
		{in: "./pkg//api", want: "pkg/api"},
		{in: ".", want: "."},
		{in: "a/../b", want: "b"},
		{in: "a/..", want: "."},
		{in: "..", wantErr: true},
		{in: "../a", wantErr: true},
		{in: "a/../../b", wantErr: true},
		{in: "/a", wantErr: true},
		{in: "//a", wantErr: true},
		{in: "a\\b", wantErr: true},
		{in: "C:\\x", wantErr: true},
		{in: "c:/x", wantErr: true},
		{in: "a\x00", wantErr: true},
		{in: "a\nb", wantErr: true},
		{in: "", wantErr: true},
		{in: "x/" + string(make([]byte, 5000)), wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := CleanPath(tt.in)

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestNamespacedName(t *testing.T) {
	tests := []struct{ in, want string }{
		{"pkg/.claude/skills/baz", "pkg.claude.skills.baz"},
		{".claude/skills/bar", "claude.skills.bar"},
		{"skills/impostor", "skills.impostor"},
		{"a", "a"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			assert.Equal(t, tt.want, NamespacedName(tt.in))
		})
	}
}
