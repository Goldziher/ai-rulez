package generator

import (
	"path/filepath"
	"sort"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/safefs"
)

// PluginFile is one file of the plugin bundle a run renders: its slash path
// relative to the project root, its bytes and whether it is executable.
type PluginFile struct {
	Path       string
	Data       []byte
	Executable bool
}

// PluginFiles renders the plugin bundle and returns its files without writing
// anything, sorted by path. `publish` packages them after VerifyPlugin has
// shown that they equal what is on disk. A file that would land outside the
// project root is an error.
func (g *Generator) PluginFiles(profile string) ([]PluginFile, error) {
	g.diagnostics()
	outputs, err := g.collectPluginOutputs(profile)
	if err != nil {
		return nil, err
	}
	root := filepath.Clean(g.config.BaseDir)
	files := make([]PluginFile, 0, len(outputs))
	for _, output := range outputs {
		if output.IsDir {
			continue
		}
		rel, relErr := filepath.Rel(root, g.absOutputPath(output.Path))
		if relErr != nil || safefs.RelEscapes(rel) {
			return nil, oops.With("path", output.Path).Errorf("plugin output is outside the project root")
		}
		data := output.RawContent
		if data == nil {
			data = []byte(output.Content)
		}
		files = append(files, PluginFile{Path: filepath.ToSlash(rel), Data: data, Executable: output.Mode&0o111 != 0})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
}
