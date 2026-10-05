package generator

import "github.com/Goldziher/ai-rulez/internal/config"

// ContentForProfile returns the shared content tree the named profile selects
// (the default profile when empty). Machine-local content is not part of it.
// `export okf` uses it so the bundle contains exactly what `generate` renders.
func (g *Generator) ContentForProfile(profile string) (*config.ContentTree, error) {
	return g.getContentForProfile(g.resolveProfile(profile))
}
