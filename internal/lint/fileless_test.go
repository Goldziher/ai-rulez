package lint

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A finding without a file keeps an empty path and a fingerprint that does not
// depend on the directory the run started in, and --tracked-only keeps it.
func TestFilelessFindingIsIndependentOfTheWorkingDirectory(t *testing.T) {
	p := fakeProject(t, isolationNone, fakeScannerCommand(t, "--only-global"))
	tests := []struct {
		name string
		cwd  string
	}{
		{"tree top", p.root},
		{"subdirectory", filepath.Join(p.root, ".ai-rulez", "rules")},
	}
	var fingerprints []string
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			cfg := loadNoRemote(t, p.root)
			tree, err := LoadTree(p.root)
			require.NoError(t, err)

			// Act
			rep, err := RunWith(cfg, tree, Options{External: true}, WithCwd(tt.cwd))
			require.NoError(t, err)
			got := scannerFindings(rep.Findings)
			require.Len(t, got, 1, dump(rep.Findings))
			dropped := rep.DropUntracked(tree, nil)

			// Assert
			assert.Empty(t, got[0].File)
			assert.Empty(t, got[0].RepoPath())
			assert.Zero(t, dropped)
			assert.Len(t, scannerFindings(rep.Findings), 1, "the finding survives --tracked-only")
			fingerprints = append(fingerprints, got[0].Fingerprint())
		})
	}
	require.Len(t, fingerprints, 2)
	assert.Equal(t, fingerprints[0], fingerprints[1])
}

func TestAssignIdentityGivesFilelessFindingsACwdFreePath(t *testing.T) {
	mk := func() []Finding { return []Finding{{Code: "AR401", Message: "project wide"}} }
	a, b := mk(), mk()

	assignIdentity(a, nil, "/one")
	assignIdentity(b, nil, "/two/deeper")

	assert.Empty(t, a[0].Meta.Path)
	assert.Equal(t, a[0].Fingerprint(), b[0].Fingerprint())
}

func TestDropUntrackedKeepsFindingsWithoutAFile(t *testing.T) {
	p := fakeProject(t, "", "")
	tree, err := LoadTree(p.root)
	require.NoError(t, err)
	require.True(t, tree.Git)
	for _, path := range []string{"", "."} {
		rep := &Report{Findings: []Finding{{Code: "AR401", Meta: &FindingMeta{Path: path}}}}

		assert.Zero(t, rep.DropUntracked(tree, nil), "path %q", path)
		assert.Len(t, rep.Findings, 1)
	}
}

func TestWritersEmitNoLocationForAFilelessFinding(t *testing.T) {
	c := Combine([]*Report{{Findings: []Finding{{Code: "AR401", Name: "path-missing", Severity: SeverityWarning, Message: "project wide"}}}})
	var gh, md, junit, sarif bytes.Buffer

	require.NoError(t, WriteGitHub(&gh, c))
	require.NoError(t, WriteMarkdown(&md, c))
	require.NoError(t, WriteJUnit(&junit, c, "error"))
	require.NoError(t, WriteSARIF(&sarif, c, "test"))

	assert.NotContains(t, gh.String(), "file=")
	assert.Contains(t, md.String(), "(no file)")
	assert.NotContains(t, junit.String(), ` file="`)
	assert.NotContains(t, sarif.String(), "physicalLocation")
	assert.NotContains(t, sarif.String(), `"locations"`)
}
