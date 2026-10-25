package mcp

import (
	"fmt"
	"path"
	"strings"
)

// keyDisableModelInvocation is the Claude Code frontmatter key that makes a
// skill user-only: the model never sees or invokes it, only the user does, with
// a slash command.
const keyDisableModelInvocation = "disable-model-invocation"

// userInvokedHint tells a caller how an operator serves such skills anyway.
const userInvokedHint = "the user runs it with a slash command; [mcp] serve_user_invoked_skills = true serves it to the model"

// userInvokedOnly reports whether a skill sets disable-model-invocation to true.
func userInvokedOnly(s *CatalogSkill) bool { return frontmatterUserInvoked(s.Frontmatter) }

// frontmatterUserInvoked reports whether parsed SKILL.md frontmatter sets
// disable-model-invocation to true. Only the hyphenated key counts: it is the one
// Claude Code honors, so the underscore spelling does not hide a skill.
func frontmatterUserInvoked(front map[string]any) bool {
	switch v := front[keyDisableModelInvocation].(type) {
	case bool:
		return v
	case string:
		return strings.EqualFold(strings.TrimSpace(v), "true")
	}
	return false
}

// contentUserInvoked is frontmatterUserInvoked for the text of a SKILL.md.
func contentUserInvoked(content string) bool {
	front, err := parseFrontmatter([]byte(content))
	return err == nil && frontmatterUserInvoked(front)
}

// UserInvoked reports whether name is a skill the server holds back from the
// model because it sets disable-model-invocation.
func (c *Catalog) UserInvoked(name string) bool { return c.userOnly[name] }

// UserInvokedPath reports whether ref is the source path (the directory, or its
// SKILL.md) of a discovered skill the server holds back from the model.
func (c *Catalog) UserInvokedPath(ref string) bool {
	if c == nil || len(c.userOnlyPaths) == 0 {
		return false
	}
	ref = strings.TrimSuffix(path.Clean(strings.ReplaceAll(strings.TrimSpace(ref), "\\", "/")), "/SKILL.md")
	return c.userOnlyPaths[ref]
}

// userInvokedError is what load_skill and get_skill answer for such a skill.
func userInvokedError(name string) error {
	return fmt.Errorf("%s is user-invoked only (disable-model-invocation): %s", name, userInvokedHint)
}

// withoutUserInvoked is the catalog the model is served: the skills that set
// disable-model-invocation are left out of every listing, ranking, resource and
// load, and remembered by name so a load can say why. The receiver, which the
// lock and the cache still describe in full, is not modified. A catalog with
// none of them is returned as it is.
func (c *Catalog) withoutUserInvoked() *Catalog {
	if c == nil {
		return nil
	}
	var held []string
	for _, s := range c.skills {
		if userInvokedOnly(s) {
			held = append(held, s.Name)
		}
	}
	if len(held) == 0 {
		return c
	}
	out := *c
	out.skills = nil
	out.byName, out.byURI, out.byFile = map[string]*CatalogSkill{}, map[string]*CatalogSkill{}, map[string]*CatalogFile{}
	out.userOnly, out.userOnlyPaths = map[string]bool{}, map[string]bool{}
	for _, name := range held {
		out.userOnly[name] = true
	}
	out.discovery = c.discovery.withoutHeld(out.userOnly, out.userOnlyPaths)
	if c.repo != nil {
		repo := *c.repo
		repo.held = out.userOnly
		out.repo = &repo
	}
	for _, s := range c.skills {
		if out.userOnly[s.Name] {
			continue
		}
		out.skills = append(out.skills, s)
		out.byName[s.Name] = s
		out.byURI[s.URI] = s
		for j := range s.Files {
			out.byFile[s.Files[j].URI] = &s.Files[j]
		}
	}
	return &out
}

// served is the catalog this server shows the model.
func (s *Server) served(c *Catalog) *Catalog {
	if s.serve != nil && s.serve.opts.ServeUserInvoked {
		return c
	}
	return c.withoutUserInvoked()
}

// withoutHeld is the report without the skills held back from the model: their
// names and paths would otherwise be listed under "served". The paths of the
// held skills are added to paths. The receiver is not modified.
func (r *DiscoveryReport) withoutHeld(held, paths map[string]bool) *DiscoveryReport {
	if r == nil {
		return nil
	}
	out := *r
	out.Served = make([]DiscoveredSkill, 0, len(r.Served))
	for _, d := range r.Served {
		if held[d.Name] {
			paths[d.Path] = true
			continue
		}
		out.Served = append(out.Served, d)
	}
	out.Renamed = nil
	for _, d := range r.Renamed {
		if held[d.ServedAs] {
			paths[d.Path] = true
			continue
		}
		out.Renamed = append(out.Renamed, d)
	}
	return &out
}
