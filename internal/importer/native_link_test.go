package importer

import (
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
)

func TestNativePlan_RootLinkToAnImportedFileAddsThePresetThatGeneratesIt(t *testing.T) {
	tests := []struct {
		name        string
		fsys        fstest.MapFS
		wantPresets []string
	}{
		{
			"CLAUDE.md links to AGENTS.md",
			fstest.MapFS{
				"AGENTS.md": &fstest.MapFile{Data: []byte("# Rules\n\nBe kind.\n")},
				"CLAUDE.md": &fstest.MapFile{Mode: fs.ModeSymlink, Data: []byte("AGENTS.md")},
			},
			[]string{"claude", "codex"},
		},
		{
			"a link to a file that is not imported adds nothing",
			fstest.MapFS{
				"AGENTS.md": &fstest.MapFile{Data: []byte("# Rules\n\nBe kind.\n")},
				"CLAUDE.md": &fstest.MapFile{Mode: fs.ModeSymlink, Data: []byte("docs/missing.md")},
			},
			nil,
		},
		{
			"a link leaving the tree adds nothing",
			fstest.MapFS{
				"AGENTS.md": &fstest.MapFile{Data: []byte("# Rules\n\nBe kind.\n")},
				"CLAUDE.md": &fstest.MapFile{Mode: fs.ModeSymlink, Data: []byte("../AGENTS.md")},
			},
			nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			p := planOf(t, nativeImporter{}, tt.fsys, Options{})

			// Assert
			assert.ElementsMatch(t, tt.wantPresets, p.Presets)
		})
	}
}
