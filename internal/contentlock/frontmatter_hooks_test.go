package contentlock

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

const fmHookAgent = "---\nname: reviewer\ndescription: reviews\nhooks:\n  PreToolUse:\n    - matcher: Bash\n      hooks:\n        - type: command\n          command: ./scripts/guard.sh --strict\n---\nBody.\n"

func (f *fixture) agent(name, body string) {
	p := f.write("agents/"+name+".md", []byte(body), 0o644)
	f.cfg.Content.Agents = append(f.cfg.Content.Agents, config.ContentFile{Name: name, Path: p, Content: "stripped"})
}

func (f *fixture) projectFile(rel, body string) {
	abs := filepath.Join(f.root, filepath.FromSlash(rel))
	require.NoError(f.t, os.MkdirAll(filepath.Dir(abs), 0o755))
	require.NoError(f.t, os.WriteFile(abs, []byte(body), 0o755))
}

func TestComputePinsFrontmatterHookScripts(t *testing.T) {
	tests := []struct {
		name        string
		agent       string
		changeAfter func(f *fixture)
		wantChanged bool
	}{
		{"editing the script changes the agent digest", fmHookAgent, func(f *fixture) {
			f.projectFile("scripts/guard.sh", "#!/bin/sh\nexit 1\n")
		}, true},
		{"an unrelated project file does not", fmHookAgent, func(f *fixture) {
			f.projectFile("scripts/other.sh", "#!/bin/sh\nexit 1\n")
		}, false},
		{"a script appearing where it was missing changes it", fmHookAgent, func(f *fixture) {
			f.projectFile("scripts/guard.sh", "#!/bin/sh\nexit 0\n")
		}, true},
		{"project-dir variable form", "---\nname: reviewer\ndescription: r\nhooks:\n  Stop:\n    - hooks:\n        - type: command\n          command: \"$CLAUDE_PROJECT_DIR/scripts/guard.sh\"\n---\nBody.\n",
			func(f *fixture) { f.projectFile("scripts/guard.sh", "changed") }, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			f := newFixture(t)
			f.projectFile("scripts/guard.sh", "#!/bin/sh\nexit 0\n")
			f.agent("reviewer", tt.agent)
			if tt.name == "a script appearing where it was missing changes it" {
				require.NoError(t, os.Remove(filepath.Join(f.root, "scripts", "guard.sh")))
			}
			before := f.items()["agent:/reviewer"]

			// Act
			tt.changeAfter(f)
			after := f.items()["agent:/reviewer"]

			// Assert
			require.NotEmpty(t, before)
			assert.Equal(t, tt.wantChanged, before != after)
		})
	}
}

func TestComputeFrontmatterHookScriptOutsideProjectIsAProblem(t *testing.T) {
	// Arrange
	f := newFixture(t)
	f.agent("reviewer", "---\nname: reviewer\ndescription: r\nhooks:\n  Stop:\n    - hooks:\n        - type: command\n          command: ../outside/run.sh\n---\nBody.\n")
	f.agent("other", "---\nname: other\ndescription: r\nhooks:\n  Stop:\n    - hooks:\n        - type: command\n          command: ./../outside/run.sh\n---\nBody.\n")

	// Act
	snap, err := Compute(f.cfg, Options{})

	// Assert: nothing outside the project is read; the declaration is a lock problem.
	require.NoError(t, err)
	assert.NotEmpty(t, snap.Problems)
}
