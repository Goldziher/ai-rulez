package generator

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// firstSymlink returns the first symlink on abs, walking from the project
// directory down through every parent directory to the file itself. It reports
// "" when the path crosses none. A path outside the project is not checked.
func (g *Generator) firstSymlink(abs string) string {
	base := filepath.Clean(g.config.BaseDir)
	rel, err := filepath.Rel(base, filepath.Clean(abs))
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return ""
	}
	current := base
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		current = filepath.Join(current, part)
		if info, statErr := os.Lstat(current); statErr == nil && info.Mode()&os.ModeSymlink != 0 {
			return current
		}
	}
	return ""
}

// symlinkedOutputs lists, in output order, each distinct symlink that a planned
// plugin file would be written through, as an absolute path.
func (g *Generator) symlinkedOutputs(outputs []config.OutputFile) []string {
	var links []string
	seen := make(map[string]bool)
	for _, output := range outputs {
		if output.IsDir {
			continue
		}
		if link := g.firstSymlink(g.absOutputPath(output.Path)); link != "" && !seen[link] {
			seen[link] = true
			links = append(links, link)
		}
	}
	return links
}

// refuseSymlinkedOutputs fails when a planned plugin file would be written
// through a symlink, the file itself or any parent directory. Generation never
// follows or replaces such a link: the target of a link stays untouched.
func (g *Generator) refuseSymlinkedOutputs(outputs []config.OutputFile) error {
	links := g.symlinkedOutputs(outputs)
	if len(links) == 0 {
		return nil
	}
	rels := make([]string, 0, len(links))
	for _, link := range links {
		rels = append(rels, filepath.ToSlash(g.convertToRelativePath(link)))
	}
	joined := strings.Join(rels, ", ")
	return oops.With("links", joined).
		Hint("Replace the symlink with a regular file or directory (or remove it); generate --plugin never writes through a link").
		Errorf("refusing to write the plugin bundle through a symlink: %s", joined)
}
