package agentssource_test

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

func TestNestedScopeEscapesGlobMetacharactersInTheDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory names with glob metacharacters are not valid on Windows")
	}
	tests := []struct {
		dir  string
		want string
	}{
		{"app[1]", `app\[1\]/**`},
		{"{x}", `\{x\}/**`},
		{"a*b", `a\*b/**`},
		{"what?", `what\?/**`},
		{"!bang", `\!bang/**`},
		{"a,b", `a\,b/**`},
		{"plain/sub", `plain/sub/**`},
	}
	for _, tt := range tests {
		t.Run(tt.dir, func(t *testing.T) {
			// Arrange
			root := nestedProject(t, map[string]string{tt.dir + "/AGENTS.md": "# Scoped\n\nTEXT\n"})

			// Act
			cfg := load(t, root)

			// Assert
			var scoped *config.ContentFile
			for i := range cfg.Content.Context {
				if cfg.Content.Context[i].Name != "agents" {
					scoped = &cfg.Content.Context[i]
				}
			}
			require.NotNil(t, scoped)
			assert.Equal(t, []string{tt.want}, scoped.Metadata.PathScope())
		})
	}
}
