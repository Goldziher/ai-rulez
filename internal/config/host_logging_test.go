package config

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
	"github.com/Goldziher/ai-rulez/v5/internal/workspace"
)

// loggingProject is a project whose validation raises a warning: a malformed
// glob in a rule's targets.
func loggingProject(name string) *workspace.Mem {
	ws := workspace.NewMem("/virtual/" + name)
	ws.Set(".ai-rulez/config.toml", "version = \"5.0\"\nname = \""+name+"\"\npresets = [\"claude\"]\n", 0o644)
	ws.Set(".ai-rulez/rules/glob.md", "---\ntargets: [\"[unclosed\"]\n---\n# Glob\n", 0o644)
	return ws
}

func warnLines(rec *testutil.LogRecorder) []string { return rec.Level("WARN") }

func TestTwoProjectsInOneProcessKeepSeparateWarnings(t *testing.T) {
	tests := []struct {
		name string
		// projects are loaded one after the other, each with its own host logger.
		projects []string
	}{
		{name: "same project loaded twice", projects: []string{"same", "same"}},
		{name: "different projects", projects: []string{"alpha", "beta"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			recs := make([]*testutil.LogRecorder, len(tt.projects))
			cfgs := make([]*Config, len(tt.projects))

			// Act: each load, and each validation of it, runs with the host of its project.
			for i, name := range tt.projects {
				recs[i] = &testutil.LogRecorder{}
				cfg, err := LoadConfig(t.Context(), "/virtual/"+name, WithWorkspace(loggingProject(name)), WithoutRemote(),
					WithHost(ambient.Host{Log: recs[i]}))
				require.NoError(t, err)
				cfgs[i] = cfg
				require.NoError(t, cfg.Validate())
				require.NoError(t, cfg.Validate())
			}

			// Assert: every warning reached the logger of the project that raised it.
			for i, rec := range recs {
				got := warnLines(rec)
				assert.Len(t, got, 1, "%v", got)
				assert.Equal(t, 1, countContaining(got, "invalid glob in targets"), "validating twice warns once: %v", got)
				assert.Contains(t, got[0], "/virtual/"+tt.projects[i], "the warning names this project's files")
				for j, other := range recs {
					if j != i && tt.projects[i] != tt.projects[j] {
						assert.NotContains(t, other.String(), "/virtual/"+tt.projects[i], "project %s leaked into %s", tt.projects[i], tt.projects[j])
					}
				}
			}
		})
	}
}

func countContaining(lines []string, sub string) int {
	n := 0
	for _, l := range lines {
		if strings.Contains(l, sub) {
			n++
		}
	}
	return n
}

func TestMalformedFrontmatterIsReportedToTheHostLogger(t *testing.T) {
	// Arrange
	ws := workspace.NewMem("/virtual/fm")
	ws.Set(".ai-rulez/config.toml", memConfigTOML, 0o644)
	ws.Set(".ai-rulez/rules/broken.md", "---\ntitle: a: b: c\n---\n# Broken\n", 0o644)
	rec := &testutil.LogRecorder{}

	// Act
	cfg, err := LoadConfig(t.Context(), "/virtual/fm", WithWorkspace(ws), WithoutRemote(), WithHost(ambient.Host{Log: rec}))

	// Assert
	require.NoError(t, err)
	require.Len(t, cfg.Content.Rules, 1)
	assert.True(t, cfg.Content.Rules[0].MalformedFrontmatter)
	require.Len(t, rec.Level("WARN"), 1)
	assert.Contains(t, rec.String(), "/virtual/fm/.ai-rulez/rules/broken.md")
}
