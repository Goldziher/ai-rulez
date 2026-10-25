package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"strings"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Goldziher/ai-rulez/v5/internal/mcp/handlers"
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

// UserInvokedRef is UserInvoked for a reference that is a skill name, a skill://
// URI or the source path of the skill (its directory, or its SKILL.md), as an
// authoring tool or resource may be given any of them.
func (c *Catalog) UserInvokedRef(ref string) bool {
	if c == nil || len(c.userOnly) == 0 {
		return false
	}
	ref = strings.TrimSpace(ref)
	if rest, ok := strings.CutPrefix(ref, SkillURIScheme); ok {
		ref, _, _ = strings.Cut(rest, "/")
	}
	if c.userOnly[ref] {
		return true
	}
	ref = strings.TrimSuffix(path.Clean(strings.ReplaceAll(ref, "\\", "/")), "/SKILL.md")
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

// hidesUserInvoked reports whether this server withholds user-only skills from
// the model: a serving server (skills-only or merged) that was not told to serve
// them. The plain authoring server (`ai-rulez mcp`) has no serving state; it
// edits the configuration, so it reads and lists every skill.
func (s *Server) hidesUserInvoked() bool {
	return s.serve != nil && !s.serve.opts.ServeUserInvoked
}

// refuseUserInvoked is the error a read of the skill ref gets on a server that
// hides user-only skills, or nil.
func (s *Server) refuseUserInvoked(ref string) error {
	if s.hidesUserInvoked() && s.cat().UserInvokedRef(ref) {
		return userInvokedError(strings.TrimSpace(ref))
	}
	return nil
}

// guardUserInvokedRead wraps the authoring read_skill so it refuses a user-only skill.
func (s *Server) guardUserInvokedRead(next handlerFunc) handlerFunc {
	return func(ctx context.Context, req *handlers.ToolRequest) (*sdkmcp.CallToolResult, error) {
		if err := s.refuseUserInvoked(req.GetString(keyName, "")); err != nil {
			return handlers.ToolError(err)
		}
		return next(ctx, req)
	}
}

// hideUserInvokedItems wraps a handler whose JSON result lists entries under
// field (list_skills: "skills", catalog: "items"), dropping the ones held says
// name a user-only skill.
func (s *Server) hideUserInvokedItems(next handlerFunc, field string, held func(*Catalog, map[string]any) bool) handlerFunc {
	return func(ctx context.Context, req *handlers.ToolRequest) (*sdkmcp.CallToolResult, error) {
		res, err := next(ctx, req)
		if err != nil || res == nil || res.IsError || !s.hidesUserInvoked() {
			return res, err
		}
		for i, c := range res.Content {
			if t, ok := c.(*sdkmcp.TextContent); ok {
				res.Content[i] = &sdkmcp.TextContent{Text: s.withoutHeldEntries(t.Text, field, held)}
				break
			}
		}
		return res, nil
	}
}

// withoutHeldEntries removes from the JSON object text the entries of field that
// held reports, keeping a count next to the list in step. Text that is not a
// JSON object, or has none to remove, is returned unchanged.
func (s *Server) withoutHeldEntries(text, field string, held func(*Catalog, map[string]any) bool) string {
	if !s.hidesUserInvoked() {
		return text
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(text), &doc); err != nil || doc == nil {
		return text
	}
	list, ok := doc[field].([]any)
	if !ok {
		return text
	}
	cat := s.cat()
	kept := make([]any, 0, len(list))
	for _, e := range list {
		if m, ok := e.(map[string]any); ok && held(cat, m) {
			continue
		}
		kept = append(kept, e)
	}
	if len(kept) == len(list) {
		return text
	}
	doc[field] = kept
	if _, ok := doc[keyCount]; ok {
		doc[keyCount] = len(kept)
	}
	out, err := json.Marshal(doc)
	if err != nil {
		return text
	}
	return string(out)
}

// heldFileInfo reports whether a list_skills entry is a held skill.
func heldFileInfo(cat *Catalog, m map[string]any) bool {
	name, ok := m[keyName].(string)
	return ok && cat.UserInvokedRef(name)
}

// scrubUserInvoked wraps a read-only report tool (roles, token and cost reports,
// sbom, lock status, lint findings, ...) of a serving server so its JSON result
// does not name a user-only skill: see scrubHeld.
func (s *Server) scrubUserInvoked(next handlerFunc) handlerFunc {
	return func(ctx context.Context, req *handlers.ToolRequest) (*sdkmcp.CallToolResult, error) {
		res, err := next(ctx, req)
		if err != nil || res == nil || !s.hidesUserInvoked() {
			return res, err
		}
		for i, c := range res.Content {
			if t, ok := c.(*sdkmcp.TextContent); ok {
				res.Content[i] = &sdkmcp.TextContent{Text: s.scrubText(t.Text)}
			}
		}
		return res, nil
	}
}

// scrubText is scrubHeld on a JSON text; any other text is returned unchanged.
func (s *Server) scrubText(text string) string {
	cat := s.cat()
	if !s.hidesUserInvoked() || len(cat.userOnly) == 0 {
		return text
	}
	var doc any
	if err := json.Unmarshal([]byte(text), &doc); err != nil {
		return text
	}
	h := newHeldScrubber(cat)
	doc = h.scrub(doc)
	if !h.changed {
		return text
	}
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return text
	}
	return string(out)
}

// heldScrubber removes the user-only skills from a decoded JSON report.
type heldScrubber struct {
	names   []string
	words   *regexp.Regexp
	changed bool
}

func newHeldScrubber(cat *Catalog) *heldScrubber {
	h := &heldScrubber{}
	var quoted []string
	for name := range cat.userOnly {
		h.names = append(h.names, name)
		quoted = append(quoted, regexp.QuoteMeta(name))
	}
	h.words = regexp.MustCompile(`(^|[^\w-])(` + strings.Join(quoted, "|") + `)([^\w-]|$)`)
	return h
}

// scrub drops, from every array of the report, the objects that describe a
// held skill and the free-text notes that name one, and takes a dropped skill
// out of the totals that sit next to its list (a role's items and totals).
func (h *heldScrubber) scrub(v any) any {
	switch t := v.(type) {
	case []any:
		kept := make([]any, 0, len(t))
		for _, e := range t {
			if h.namesHeld(e) {
				h.changed = true
				continue
			}
			kept = append(kept, h.scrub(e))
		}
		return kept
	case map[string]any:
		h.adjustTotals(t)
		for k, e := range t {
			t[k] = h.scrub(e)
		}
		return t
	}
	return v
}

// namesHeld reports whether an array element is a held skill's object, or a note
// that names one.
func (h *heldScrubber) namesHeld(e any) bool {
	switch t := e.(type) {
	case string:
		return h.words.MatchString(t)
	case map[string]any:
		return h.isHeldObject(t)
	}
	return false
}

// isHeldObject: a skill-typed object whose id or name is held, or any object with
// a value that is the skill's path or its sbom reference.
func (h *heldScrubber) isHeldObject(m map[string]any) bool {
	skill := false
	for _, k := range []string{"kind", "group", "type"} {
		if v, ok := m[k].(string); ok && (v == "skill" || v == "skills") {
			skill = true
		}
	}
	for _, v := range m {
		s, ok := v.(string)
		if !ok {
			continue
		}
		for _, name := range h.names {
			if skill && s == name || strings.Contains(s, "item:skill::"+name) ||
				strings.Contains(s, "skills/"+name+"/") || strings.HasSuffix(s, "skills/"+name) {
				return true
			}
		}
	}
	return false
}

// adjustTotals takes the held skills out of an object that has both a list of
// items and the totals of that list (a role).
func (h *heldScrubber) adjustTotals(m map[string]any) {
	items, ok := m["items"].([]any)
	totals, tok := m["totals"].(map[string]any)
	if !ok || !tok {
		return
	}
	for _, e := range items {
		it, ok := e.(map[string]any)
		if !ok || !h.isHeldObject(it) {
			continue
		}
		bump := func(obj map[string]any, key string, by float64) {
			if n, ok := obj[key].(float64); ok {
				obj[key] = n - by
			}
		}
		bump(totals, "items", 1)
		if n, ok := it["bytes"].(float64); ok {
			bump(totals, "bytes", n)
		}
		tokens, _ := it["tokens"].(float64) //nolint:errcheck // absent counts as zero
		bump(totals, "tokens", tokens)
		if by, ok := totals["by_kind"].(map[string]any); ok {
			bump(by, "skill", 1)
		}
		if it["delivery"] == "served" {
			bump(totals, "served_skills", 1)
			bump(totals, "served_tokens", tokens)
		}
	}
}
