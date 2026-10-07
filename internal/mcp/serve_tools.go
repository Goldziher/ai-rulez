package mcp

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/Goldziher/ai-rulez/v5/internal/mcp/handlers"
	"github.com/Goldziher/ai-rulez/v5/internal/skillsearch"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// The dynamic-loading tools. All three are read-only: this server mode registers
// no tool that writes, creates, updates or deletes anything.

const (
	toolFindSkill          = "find_skill"
	toolLoadSkill          = "load_skill"
	toolListSkillResources = "list_skill_resources"
)

func (s *Server) registerServeTools() {
	s.addTool(
		newAnnotatedTool(toolFindSkill,
			"Find skills for a task. Returns ranked matches (name, description, score) over skill names, triggers, keywords and descriptions; load one with load_skill. Call this before starting work in an unfamiliar area.",
			newSchemaBuilder().
				String("task", "What you are about to do, in a sentence", true).
				Number("limit", fmt.Sprintf("Maximum results (default %d, max %d)", defaultFindLimit, maxFindLimit), false).
				String("role", "Rank the skills of this role first (default: the server's role)", false),
			readOnlyAnnotations(),
		),
		s.findSkillHandler,
	)
	s.addTool(
		newAnnotatedTool(toolLoadSkill,
			"Load a skill: its SKILL.md (or one supporting file via path), provenance with digests, and an index of the skill's other files. Each session has a byte budget.",
			newSchemaBuilder().
				String("name", "Skill name from find_skill", true).
				String("path", "File inside the skill, for example references/FORMS.md (default SKILL.md)", false).
				Number("budget_bytes", "Return at most this many bytes of the file (default: the whole file)", false),
			readOnlyAnnotations(),
		),
		s.loadSkillHandler,
	)
	s.addTool(
		newAnnotatedTool(toolListSkillResources,
			"List the files of a skill (references, scripts, assets) with size and digest, without loading them",
			newSchemaBuilder().String("name", "Skill name from find_skill", true),
			readOnlyAnnotations(),
		),
		s.listSkillResourcesHandler,
	)
}

func (s *Server) findSkillHandler(ctx context.Context, req *handlers.ToolRequest) (*sdkmcp.CallToolResult, error) {
	task := strings.TrimSpace(req.GetString("task", ""))
	if task == "" {
		return handlers.ToolError(fmt.Errorf("task is required: describe what you are about to do"))
	}
	limit := int(req.GetNumber("limit", defaultFindLimit))
	if limit <= 0 {
		limit = defaultFindLimit
	}
	limit = min(limit, maxFindLimit)

	role := req.GetString("role", s.serve.opts.Role)
	var scope RoleScope
	if role != "" {
		var ok bool
		if scope, ok = s.serve.scope(role); !ok {
			return handlers.ToolError(fmt.Errorf("unknown role %q", role))
		}
	}

	rk := s.serve.opts.Search.rank(ctx, s.cat(), task)
	results := rankForRole(rk.hits, role, scope, limit)
	out := map[string]any{"task": task, "count": len(results), "results": results}
	if rk.ranking != skillsearch.ModeLexical || rk.degraded != "" {
		out["ranking"] = rk.ranking
	}
	if rk.degraded != "" {
		out["degraded"] = rk.degraded
	}
	if rk.log != nil {
		session, _ := s.sessionInfo(req)
		ids := make([]string, 0, len(results))
		for _, r := range results {
			name, _ := r[keyName].(string) //nolint:errcheck // every result carries its name
			ids = append(ids, name)
		}
		rk.log.Query(session, task, rk.ranking, ids)
	}
	if role != "" {
		out["role"] = role
	}
	if len(results) == 0 {
		out["hint"] = "no served skill matches; try different words, or proceed without a skill"
	}
	return handlers.ToolSuccess(out)
}

func (s *Server) lookupServed(name string) (*CatalogSkill, error) {
	cat := s.cat()
	key := strings.TrimSpace(name)
	if key == "" {
		return nil, fmt.Errorf("name is required: use a name from find_skill")
	}
	if strings.HasPrefix(key, SkillURIScheme) {
		key = strings.TrimPrefix(key, SkillURIScheme)
		key, _, _ = strings.Cut(key, "/")
	}
	if !validSkillSegment(key) {
		return nil, fmt.Errorf("%q is not a valid skill name", name)
	}
	if skill, ok := cat.Lookup(key); ok {
		return skill, nil
	}
	if r, refused := cat.Refusal(key); refused {
		return nil, fmt.Errorf("skill %q is refused (%s): %s", key, r.Code, r.Reason)
	}
	return nil, fmt.Errorf("no served skill %q; call find_skill to see what is available", key)
}

func (s *Server) loadSkillHandler(ctx context.Context, req *handlers.ToolRequest) (*sdkmcp.CallToolResult, error) {
	skill, err := s.lookupServed(req.GetString(keyName, ""))
	if err != nil {
		return handlers.ToolError(err)
	}
	rel, ok := cleanRelPath(req.GetString("path", ""))
	if !ok {
		return handlers.ToolError(fmt.Errorf("path %q must be a relative path inside the skill", req.GetString("path", "")))
	}
	file := skill.file(rel)
	if file == nil {
		return handlers.ToolError(fmt.Errorf("skill %q has no file %q; list_skill_resources shows its files", skill.Name, rel))
	}
	if !utf8.Valid(file.Content) {
		return handlers.ToolError(fmt.Errorf("%s is binary; read it with resources/read on %s", rel, file.URI))
	}
	content := string(file.Content)
	truncated := false
	if limit := int(req.GetNumber("budget_bytes", 0)); limit > 0 && len(content) > limit {
		content, truncated = truncateUTF8(content, limit), true
	}

	session, client := s.sessionInfo(req)
	remaining, ok := s.serve.charge(session, len(content))
	if !ok {
		return handlers.ToolError(fmt.Errorf("session budget exhausted: loading %d bytes would exceed the %d-byte cap (%d bytes left); pass a smaller budget_bytes or restart the session",
			len(content), s.serve.opts.budget(), remaining))
	}
	s.serve.opts.Search.Log().Loaded(session, skill.Name)
	if t := s.serve.opts.Telemetry; t != nil {
		t(SessionTelemetry{Skill: skill.Name, Digest: skill.Digest, Session: session, Client: client, Resource: rel != skillMarkdown, Role: s.serve.opts.Role})
	}

	resources := make([]map[string]any, 0, len(skill.Files))
	for i := range skill.Files {
		if skill.Files[i].RelPath == rel {
			continue
		}
		f := &skill.Files[i]
		resources = append(resources, map[string]any{"path": f.RelPath, keyURI: f.URI, keySize: f.Size, keyDigest: f.Digest})
	}
	out := skillSummary(skill, 0)
	delete(out, "files")
	out["path"] = rel
	out[keyURI] = file.URI
	out["content"] = content
	out["bytes"] = len(content)
	out["truncated"] = truncated
	out["file_digest"] = file.Digest
	out[keyResources] = resources
	if truncated {
		out["total_bytes"] = file.Size
	}
	if remaining >= 0 {
		out["budget_remaining_bytes"] = remaining
	}
	s.addProvenance(out, skill)
	return handlers.ToolSuccess(out)
}

func (s *Server) listSkillResourcesHandler(_ context.Context, req *handlers.ToolRequest) (*sdkmcp.CallToolResult, error) {
	skill, err := s.lookupServed(req.GetString(keyName, ""))
	if err != nil {
		return handlers.ToolError(err)
	}
	files := make([]map[string]any, 0, len(skill.Files))
	for i := range skill.Files {
		f := &skill.Files[i]
		files = append(files, map[string]any{"path": f.RelPath, keyURI: f.URI, keySize: f.Size, "mime": f.MIME, keyDigest: f.Digest})
	}
	out := map[string]any{keyName: skill.Name, keyDigest: skill.Digest, keyResources: files}
	s.addProvenance(out, skill)
	return handlers.ToolSuccess(out)
}

// addProvenance records where a served skill came from and whether the lock
// vouches for it, so an audit log of a session can pin exactly what was loaded.
func (s *Server) addProvenance(out map[string]any, skill *CatalogSkill) {
	prov := map[string]any{keyDigest: skill.Digest}
	if skill.Source != "" {
		prov["source"] = skill.Source
	}
	if skill.Ref != "" {
		prov["ref"] = skill.Ref
		prov["pinned"] = skill.Pinned
	}
	if skill.Commit != "" {
		prov["commit"] = skill.Commit
	}
	if skill.Delivery != "" {
		prov["delivery"] = skill.Delivery
	}
	prov["lock_digest"] = skill.LockDigest
	prov["locked"] = skill.Locked
	if skill.Approved {
		prov["approved"] = true
		prov["approvers"] = skill.Approvers
	}
	if skill.ScanFindings > 0 {
		prov["scan_warnings"] = skill.ScanFindings
	}
	if len(skill.Unscanned) > 0 {
		prov["unserved_unscannable_files"] = skill.Unscanned
	}
	out["provenance"] = prov
}

func (s *Server) sessionInfo(req *handlers.ToolRequest) (session, client string) {
	raw := req.Raw()
	if raw == nil || raw.Session == nil {
		return "", ""
	}
	if p := raw.Session.InitializeParams(); p != nil && p.ClientInfo != nil {
		client = p.ClientInfo.Name
	}
	return s.serve.sessionID(raw.Session), client
}

// truncateUTF8 cuts s to at most n bytes without splitting a rune.
func truncateUTF8(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

func (c *CatalogSkill) file(rel string) *CatalogFile {
	for i := range c.Files {
		if c.Files[i].RelPath == rel {
			return &c.Files[i]
		}
	}
	return nil
}

// rankForRole lists the hits as find_skill returns them: with a role, the skills
// in its scope first (keeping BM25 order) and then the rest, each capped by limit.
func rankForRole(ranked []FindHit, role string, scope RoleScope, limit int) []map[string]any {
	results := make([]map[string]any, 0, limit)
	add := func(hit FindHit, inRole bool) {
		entry := map[string]any{
			keyName:       hit.Skill.Name,
			"description": hit.Skill.Description,
			"score":       hit.Score,
			"domain":      domainLabel(hit.Skill.Domain),
			keyDigest:     hit.Skill.Digest,
		}
		if role != "" {
			entry["in_role"] = inRole
		}
		if hit.LexRank > 0 || hit.VecRank > 0 {
			entry["lexical_rank"], entry["vector_rank"] = hit.LexRank, hit.VecRank
		}
		if hit.StaleVector {
			entry["stale_vector"] = true
		}
		results = append(results, entry)
	}
	for _, h := range ranked {
		if len(results) < limit && (role == "" || scope.Includes(h.Skill)) {
			add(h, true)
		}
	}
	if role != "" {
		for _, h := range ranked {
			if len(results) < limit && !scope.Includes(h.Skill) {
				add(h, false)
			}
		}
	}
	return results
}
