package generator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// A check run treats a skipped machine-local overlay as generate does: the files it
// wrote are kept, not orphans.
func TestCheckDriftKeepsFilesOfASkippedLocalOverlay(t *testing.T) {
	// Arrange: an overlay adds the devin preset; the files it wrote are kept by a --no-local run.
	p := newDriftProject(t, strings.Replace(driftShared, `presets = ["claude"]`, `presets = ["claude", "cursor"]`, 1))
	p.git(t, "init", "-q")
	p.overlay(t, "presets = [\"devin\"]\n")
	require.NoError(t, NewGenerator(p.load(t)).Generate(""))
	require.NoError(t, NewGenerator(p.load(t, config.WithoutLocal())).Generate(""))

	// Act
	drift, err := NewGenerator(p.load(t, config.WithoutLocal())).CheckDrift("")

	// Assert
	require.NoError(t, err)
	for _, d := range drift {
		assert.NotEqual(t, DriftOrphan, d.Kind, "a file kept for a skipped local input is not an orphan: %+v", d)
	}
}

const driftSharedIgnored = `version = "5.0"
agents_md = false
name = "shared-project"
presets = ["claude"]
gitignore = true

[header]
hashes = "full"
`

func driftPaths(drift []Drift) map[string]DriftKind {
	out := map[string]DriftKind{}
	for _, d := range drift {
		out[d.Path] = d.Kind
	}
	return out
}

func TestCheckDrift_IsCleanAfterGenerateWithALocalOverlay(t *testing.T) {
	// Arrange: the overlay renames the project (a shared output drifts, ignored
	// and untracked, so generate writes it) and adds a preset (local-only output).
	p := newDriftProject(t, driftSharedIgnored)
	p.overlay(t, "name = \"mine\"\npresets = [\"codex\"]\n")
	require.NoError(t, NewGenerator(p.load(t)).Generate(""))

	// Act
	drift, err := NewGenerator(p.load(t)).CheckDrift("")

	// Assert
	require.NoError(t, err)
	assert.Empty(t, drift, "generate and --check agree on a machine-local overlay")
}

func TestCheckDrift_StillReportsRealDriftWithALocalOverlay(t *testing.T) {
	// Arrange
	p := newDriftProject(t, driftSharedIgnored)
	p.overlay(t, "name = \"mine\"\npresets = [\"codex\"]\n")
	require.NoError(t, NewGenerator(p.load(t)).Generate(""))
	rule := filepath.Join(p.dir, "rules", "style.md")
	require.NoError(t, os.WriteFile(rule, []byte("---\npriority: high\n---\n\nUse spaces.\n"), 0o600))

	// Act
	drift, err := NewGenerator(p.load(t)).CheckDrift("")

	// Assert
	require.NoError(t, err)
	kinds := driftPaths(drift)
	assert.Equal(t, DriftStale, kinds["CLAUDE.md"])
	assert.Equal(t, DriftStale, kinds["AGENTS.md"])
}

func TestCheckDrift_ReportsTheLocalDriftRefusal(t *testing.T) {
	tests := []struct {
		name  string
		allow bool
		want  bool
	}{
		{"refused drift is reported as blocked", false, true},
		{"allowed drift is not", true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange: nothing ignores CLAUDE.md, so generate would refuse to write it.
			p := newDriftProject(t, driftShared)
			p.overlay(t, "name = \"mine\"\n")
			gen := NewGenerator(p.load(t))
			gen.SetAllowLocalDrift(tt.allow)

			// Act
			drift, err := gen.CheckDrift("")

			// Assert
			require.NoError(t, err)
			kinds := driftPaths(drift)
			if tt.want {
				assert.Equal(t, DriftBlocked, kinds["CLAUDE.md"])
			} else {
				assert.NotEqual(t, DriftBlocked, kinds["CLAUDE.md"])
			}
		})
	}
}

func TestCheckDrift_ResetsTheDiagnosticsPerRunWithoutFlushing(t *testing.T) {
	// Arrange
	p := newDriftProject(t, driftShared)
	gen := NewGenerator(p.load(t))
	d := gen.diagnostics()
	var issued []string
	d.SetSink(func(msg string, _ ...any) { issued = append(issued, msg) })
	require.True(t, d.Once("test", "key"))
	d.RecordDowngrade("rule", "left-over", "auto")

	// Act
	_, err := gen.CheckDrift("")
	require.NoError(t, err)
	d.RecordDowngrade("rule", "during", "auto")
	_, err = gen.CheckDrift("")
	require.NoError(t, err)

	// Assert
	assert.Empty(t, issued, "--check never prints the activation summary")
	assert.True(t, d.Once("test", "key"), "Once keys do not persist across check runs")
	d.Flush()
	assert.Empty(t, issued, "downgrades recorded before a check run are dropped with it")
}
