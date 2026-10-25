package mcp

import (
	"fmt"
	"strings"
)

// keyDisableModelInvocation is the Claude Code frontmatter key that makes a
// skill user-only: the model never sees or invokes it, only the user does, with
// a slash command.
const keyDisableModelInvocation = "disable-model-invocation"

// userInvokedHint tells a caller how an operator serves such skills anyway.
const userInvokedHint = "the user runs it with a slash command; [mcp] serve_user_invoked_skills = true serves it to the model"

// userInvokedOnly reports whether a skill sets disable-model-invocation to true.
func userInvokedOnly(s *CatalogSkill) bool {
	switch v := s.Frontmatter[keyDisableModelInvocation].(type) {
	case bool:
		return v
	case string:
		return strings.EqualFold(strings.TrimSpace(v), "true")
	}
	return false
}

// UserInvoked reports whether name is a skill the server holds back from the
// model because it sets disable-model-invocation.
func (c *Catalog) UserInvoked(name string) bool { return c.userOnly[name] }

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
	out.userOnly = map[string]bool{}
	for _, name := range held {
		out.userOnly[name] = true
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
