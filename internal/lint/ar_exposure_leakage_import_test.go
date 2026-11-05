package lint

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// importedLeak is a leak in AGENTS.md, which CLAUDE.md imports.
func importedLeak() ExposureLeak {
	return ExposureLeak{
		Area: "billing", Term: "invoice", Kind: "claude_md", Name: "CLAUDE.md", Path: "AGENTS.md", Scope: ".", Line: 2,
		Excerpt: "The invoice flow.", Message: leakMessage("claude_md", "CLAUDE.md", "invoice", "billing"), ImportedBy: "CLAUDE.md",
	}
}

func importedProject(t *testing.T, table string) string {
	t.Helper()
	root := leakProject(t, table)
	writeFiles(t, root, map[string]string{"AGENTS.md": "# Agents\nThe invoice flow.\n"})
	gitAdd(t, root)
	return root
}

func TestExposureLeakageImportedFile(t *testing.T) {
	// Arrange
	root := importedProject(t, "")

	// Act
	rep := leakRun(t, root, WithExposure(leakSource(root, importedLeak())))

	// Assert: AR9W0 is reported on the imported file and its line.
	fs := codeFindings(rep.Findings, CodeExposureAreaTerm)
	require.Len(t, fs, 1, dump(rep.Findings))
	assert.Equal(t, "AGENTS.md", filepath.ToSlash(fs[0].File))
	assert.Equal(t, 2, fs[0].Line)
}

// The finding depends on the importing CLAUDE.md as well: dropping the import removes it.
func TestExposureLeakageImportedFileUnderChanged(t *testing.T) {
	tests := []struct {
		name    string
		changed []string
		want    int
	}{
		{name: "the imported file changed", changed: []string{"AGENTS.md"}, want: 1},
		{name: "the importing file changed", changed: []string{"CLAUDE.md"}, want: 1},
		{name: "an unrelated file changed", changed: []string{"README.md"}, want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := importedProject(t, "")
			rep := leakRun(t, root, WithExposure(leakSource(root, importedLeak())))

			NarrowToChanged(rep, tt.changed, "HEAD")

			assert.Len(t, codeFindings(rep.Findings, CodeExposureAreaTerm), tt.want, dump(rep.Findings))
		})
	}
}

func TestExposureLeakageAcceptedMeta(t *testing.T) {
	// Arrange
	root := leakProject(t, "")
	leak := claudeLeak
	leak.AcceptReason = "recorded in the areas file"

	// Act
	rep := leakRun(t, root, WithExposure(leakSource(root, leak, serverLeak)))

	// Assert: the accepted finding carries the reason, the other none.
	fs := codeFindings(rep.Findings, CodeExposureAreaTerm)
	require.Len(t, fs, 2, dump(rep.Findings))
	var accepted, open *Finding
	for i := range fs {
		if fs[i].Meta != nil && fs[i].Meta.Accepted {
			accepted = &fs[i]
		} else {
			open = &fs[i]
		}
	}
	require.NotNil(t, accepted, dump(rep.Findings))
	require.NotNil(t, open)
	assert.Equal(t, "CLAUDE.md", accepted.File[len(accepted.File)-len("CLAUDE.md"):])
	assert.True(t, accepted.Meta.AcceptedByConfig)
	assert.Equal(t, "recorded in the areas file", accepted.Meta.AcceptReason)
	view := rep.Leakage()
	require.NotNil(t, view)
	var views int
	for _, l := range view.Leaks {
		if l.Accepted {
			views++
			assert.Equal(t, "recorded in the areas file", l.Reason)
		}
	}
	assert.Equal(t, 1, views)
}

func staleSource(root string, extra ...ExposureStaleAccept) fakeLeakage {
	src := leakSource(root, claudeLeak)
	src.leakage.AreasFile = filepath.Join(root, "tools", "areas.toml")
	src.leakage.StaleAccepts = extra
	src.inputs = append(src.inputs, src.leakage.AreasFile)
	return src
}

func TestExposureLeakageStaleAreasFileAccept(t *testing.T) {
	stale := ExposureStaleAccept{Index: 1, Term: "ledger", Path: "gone.md", Line: 7}
	wantMessage := `accept[1] ("ledger" in gone.md) of the areas file accepted no leak; remove it or fix its term or path`
	tests := []struct {
		name         string
		table        string
		wantFindings int
	}{
		{name: "AR9I0 is emitted on the areas file line", wantFindings: 1},
		{name: "AR9I0 off emits nothing", table: "[lint.severity]\nAR9I0 = \"off\"\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := leakProject(t, tt.table)

			// Act
			rep := leakRun(t, root, WithExposure(staleSource(root, stale)))

			// Assert
			fs := codeFindings(rep.Findings, CodeAcceptUnused)
			require.Len(t, fs, tt.wantFindings, dump(rep.Findings))
			if tt.wantFindings > 0 {
				assert.Equal(t, "tools/areas.toml", filepath.ToSlash(fs[0].File))
				assert.Equal(t, 7, fs[0].Line)
				assert.Equal(t, wantMessage, fs[0].Message)
			}
			view := rep.Leakage()
			require.NotNil(t, view)
			assert.Equal(t, []string{wantMessage}, view.Stale, "the view keeps the stale entry whether or not AR9I0 is reported")
		})
	}
}
