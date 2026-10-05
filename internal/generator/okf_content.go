package generator

import "github.com/Goldziher/ai-rulez/v5/internal/config"

// ContentForProfile returns the shared content tree the named profile selects
// (the default profile when empty). Machine-local content is not part of it.
// `export okf` uses it so the bundle contains exactly what `generate` renders.
func (g *Generator) ContentForProfile(profile string) (*config.ContentTree, error) {
	return g.getContentForProfile(g.resolveProfile(profile))
}

// ContentForRole returns the shared content tree the named role selects, the
// same slice `generate --role` renders (domains, per-kind include and exclude
// selectors, one level of extends; checks included). Unlike SetRole it does not
// touch the delivery or skill-mode settings of the configuration: a knowledge
// export is not written for a harness, so served skills are exported like static
// ones.
func (g *Generator) ContentForRole(name string) (*config.ContentTree, error) {
	resolved, err := g.config.FlattenRole(name)
	if err != nil {
		return nil, err //nolint:wrapcheck // already contextual
	}
	if g.config.Content == nil {
		return nil, config.ErrNoContent
	}
	return g.config.FilterTreeForRole(g.config.Content, &resolved.RoleConfig) //nolint:wrapcheck // already contextual
}
