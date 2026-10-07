package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"
	"unicode/utf8"

	"github.com/Goldziher/ai-rulez/v5/internal/mcp/handlers"
	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Skills extension JSON-RPC methods (SEP-2640).
const (
	methodSkillsList = "skills/list"
	methodSkillsGet  = "skills/get"

	// methodDirectoryRead is the extension's optional directory listing, gated
	// by the directoryRead capability setting.
	methodDirectoryRead = "resources/directory/read"
	mimeDirectory       = "inode/directory"

	methodResourcesRead = "resources/read"

	resultTypeComplete = "complete"
	defaultSearchLimit = 10
	maxSearchLimit     = 50
)

// keyDigest is the JSON key digests are reported under.
const (
	keyDigest     = "digest"
	keyURI        = "uri"
	keyName       = "name"
	keySize       = "size"
	keyResources  = "resources"
	keyResultType = "resultType"
	// domainRoot names the skills that belong to no domain.
	domainRoot = "root"
)

const skillServerInstructions = "ai-rulez serves the skills of one profile read-only. " +
	"Skills are MCP resources under skill://<name>/SKILL.md (supporting files sit beside it); " +
	"skills/list and skills/get return each skill's frontmatter plus the SHA-256 digest and size of every file. " +
	"Call find_skill with a task to get ranked matches and load_skill to read one (list_skill_resources shows its files); " +
	"search_skills, get_skill and read_skill_file are the lexical equivalents. This server cannot modify any content."

// NewSkillServer builds the read-only serving surface: the Skills extension
// (skills/list, skills/get, skill:// resources) plus search_skills, get_skill
// and read_skill_file tools for clients that only speak tools. None of the
// authoring tools are registered, so the surface cannot create, change or
// delete content.
func NewSkillServer(version string, catalog *Catalog) *Server {
	return NewSkillServerWith(version, catalog, ServeOptions{})
}

// NewSkillServerWith is NewSkillServer with the dynamic-loading options: the
// find_skill / load_skill / list_skill_resources tools, their session budget,
// roles, usage telemetry and live reload.
func NewSkillServerWith(version string, catalog *Catalog, opts ServeOptions) *Server {
	caps := &sdkmcp.ServerCapabilities{
		Tools:     &sdkmcp.ToolCapabilities{},
		Resources: &sdkmcp.ResourceCapabilities{ListChanged: true},
	}
	caps.AddExtension(SkillsExtensionID, map[string]any{"directoryRead": true})
	mcpServer := sdkmcp.NewServer(
		&sdkmcp.Implementation{Name: "ai-rulez-skills", Title: "AI-Rulez Skills", Version: version},
		&sdkmcp.ServerOptions{Capabilities: caps, Instructions: skillServerInstructions},
	)
	mcpServer.AddReceivingMiddleware(tolerantInitializeMiddleware())

	srv := &Server{mcpServer: mcpServer, version: version, catalog: catalog, serve: newServeState(opts)}
	mcpServer.AddReceivingMiddleware(srv.unknownResourceMiddleware())
	srv.registerSkillResources()
	srv.registerSkillTools()
	srv.registerServeTools()
	return srv
}

// Catalog returns the served skill set; nil for the authoring server.
func (s *Server) Catalog() *Catalog { return s.cat() }

func (s *Server) registerSkillResources() {
	for _, skill := range s.cat().Skills() {
		s.registerSkillFiles(skill)
	}
}

func (s *Server) readResource(_ context.Context, req *sdkmcp.ReadResourceRequest) (*sdkmcp.ReadResourceResult, error) {
	file, ok := s.cat().File(req.Params.URI)
	if !ok {
		return nil, resourceNotFound(req.Params.URI)
	}
	session := s.serve.sessionID(req.Session)
	if err := s.chargeRead(session, len(file.Content)); err != nil {
		return nil, &jsonrpc.Error{Code: jsonrpc.CodeInvalidRequest, Message: err.Error()}
	}
	contents := &sdkmcp.ResourceContents{URI: file.URI, MIMEType: file.MIME}
	if utf8.Valid(file.Content) {
		contents.Text = string(file.Content)
	} else {
		contents.Blob = file.Content
	}
	return &sdkmcp.ReadResourceResult{Contents: []*sdkmcp.ResourceContents{contents}}, nil
}

// resourceNotFound is the resource-not-found error the SDK would build
// (-32602 since SEP-2164, which the SDK's deprecated CodeResourceNotFound
// already equals), with the URI
// escaped: the SDK formats it with %q, which emits \x escapes for control
// characters, produces invalid JSON and used to end the session on marshal.
func resourceNotFound(uri string) error {
	data, err := json.Marshal(map[string]string{keyURI: uri})
	if err != nil {
		data = []byte("{}")
	}
	return &jsonrpc.Error{Code: jsonrpc.CodeInvalidParams, Message: "Resource not found", Data: data}
}

// unknownResourceMiddleware answers resources/read of a URI that is not a
// served file itself. The SDK would build that error from the raw URI with %q,
// which is invalid JSON for control characters and ended the session.
func (s *Server) unknownResourceMiddleware() sdkmcp.Middleware {
	return func(next sdkmcp.MethodHandler) sdkmcp.MethodHandler {
		return func(ctx context.Context, method string, request sdkmcp.Request) (sdkmcp.Result, error) {
			if method == methodResourcesRead {
				if req, ok := request.(*sdkmcp.ReadResourceRequest); ok && req.Params != nil {
					if _, served := s.cat().File(req.Params.URI); !served {
						return nil, resourceNotFound(req.Params.URI)
					}
				}
			}
			return next(ctx, method, request)
		}
	}
}

func (s *Server) registerSkillTools() {
	s.addTool(
		newAnnotatedTool("search_skills", "Search the served skills by name, keywords and description (lexical ranking). An empty query lists every skill.",
			newSchemaBuilder().
				String("query", "Words to look for; empty lists all skills", false).
				Number("limit", fmt.Sprintf("Maximum results (default %d, max %d)", defaultSearchLimit, maxSearchLimit), false).
				String("domain", "Only skills owned by this domain ('root' for skills in no domain)", false),
			readOnlyAnnotations(),
		),
		s.searchSkillsHandler,
	)
	s.addTool(
		newAnnotatedTool("get_skill", "Load one skill: its SKILL.md text, provenance, and the digest of every file",
			newSchemaBuilder().String("name", "Skill name or the skill:// URI of its SKILL.md", true),
			readOnlyAnnotations(),
		),
		s.getSkillHandler,
	)
	s.addTool(
		newAnnotatedTool("read_skill_file", "Read a supporting file of a served skill by its skill:// URI",
			newSchemaBuilder().String("uri", "skill://<name>/<path> URI from get_skill or skills/list", true),
			readOnlyAnnotations(),
		),
		s.readSkillFileHandler,
	)
}

func (s *Server) searchSkillsHandler(_ context.Context, req *handlers.ToolRequest) (*sdkmcp.CallToolResult, error) {
	limit := int(req.GetNumber("limit", defaultSearchLimit))
	if limit <= 0 {
		limit = defaultSearchLimit
	}
	limit = min(limit, maxSearchLimit)
	domain := req.GetString("domain", "")

	// Rank everything, filter by domain, then cut: the limit must apply to the
	// filtered list, not to hits the domain filter would drop.
	var results []map[string]any
	for _, hit := range s.cat().Search(req.GetString("query", ""), 0) {
		if domain != "" && domainLabel(hit.Skill.Domain) != domain {
			continue
		}
		results = append(results, skillSummary(hit.Skill, hit.Score))
		if len(results) == limit {
			break
		}
	}
	return handlers.ToolSuccess(map[string]any{"profile": s.cat().Profile, "count": len(results), "results": results})
}

func (s *Server) getSkillHandler(_ context.Context, req *handlers.ToolRequest) (*sdkmcp.CallToolResult, error) {
	key := req.GetString(keyName, "")
	cat := s.cat()
	skill, ok := cat.Lookup(key)
	if !ok {
		if r, refused := cat.Refusal(key); refused {
			return handlers.ToolError(fmt.Errorf("skill %q is refused (%s): %s", key, r.Code, r.Reason))
		}
		return handlers.ToolError(fmt.Errorf("no served skill %q", key))
	}
	session, _ := s.sessionInfo(req)
	if err := s.chargeRead(session, len(skill.Files[0].Content)); err != nil {
		return handlers.ToolError(err)
	}
	summary := skillSummary(skill, 0)
	summary["content"] = string(skill.Files[0].Content)
	files := make([]map[string]any, 0, len(skill.Files))
	for i := range skill.Files {
		files = append(files, fileEntry(&skill.Files[i]))
	}
	summary["files"] = files
	return handlers.ToolSuccess(summary)
}

func (s *Server) readSkillFileHandler(_ context.Context, req *handlers.ToolRequest) (*sdkmcp.CallToolResult, error) {
	uri := req.GetString("uri", "")
	file, ok := s.cat().File(uri)
	if !ok {
		return handlers.ToolError(fmt.Errorf("no served skill file %q", uri))
	}
	if !utf8.Valid(file.Content) {
		return handlers.ToolError(fmt.Errorf("%s is binary; read it with resources/read", uri))
	}
	session, _ := s.sessionInfo(req)
	if err := s.chargeRead(session, len(file.Content)); err != nil {
		return handlers.ToolError(err)
	}
	return handlers.ToolSuccess(map[string]any{
		keyURI: file.URI, keyDigest: file.Digest, keySize: file.Size, "content": string(file.Content),
	})
}

func domainLabel(domain string) string {
	if domain == "" {
		return domainRoot
	}
	return domain
}

// skillSummary is the provenance-bearing description of a skill used by the
// tools. digest covers every file of the skill; source/ref/pinned record where
// an installed skill came from so an audit log can pin exactly what was loaded.
func skillSummary(s *CatalogSkill, score int) map[string]any {
	out := map[string]any{
		keyName:       s.Name,
		keyURI:        s.URI,
		"description": s.Description,
		"domain":      domainLabel(s.Domain),
		keyDigest:     s.Digest,
		"files":       len(s.Files),
	}
	if s.Source != "" {
		out["source"] = s.Source
	}
	if s.Ref != "" {
		out["ref"] = s.Ref
		out["pinned"] = s.Pinned
	}
	if score > 0 {
		out["score"] = score
	}
	return out
}

func fileEntry(f *CatalogFile) map[string]any {
	return map[string]any{keyURI: f.URI, keyDigest: f.Digest, keySize: f.Size}
}

// skillEntry renders a skills/list or skills/get entry per SEP-2640.
// skills/list lists at most maxListedResources files per skill (skills/get
// returns them all), so a skill with a huge file tree cannot flood the reply.
func skillEntry(s *CatalogSkill, limit int) map[string]any {
	n := len(s.Files)
	if limit > 0 {
		n = min(n, limit)
	}
	resources := make([]map[string]any, 0, n)
	for i := range n {
		resources = append(resources, fileEntry(&s.Files[i]))
	}
	entry := map[string]any{keyURI: s.URI, "frontmatter": s.Frontmatter, keyResources: resources}
	if n < len(s.Files) {
		entry["resources_truncated"] = true
	}
	return entry
}

// WrapTransport returns a transport that answers skills/list and skills/get
// itself. The Go SDK dispatches only the methods it knows and rejects others
// before middleware runs, so the extension's two custom methods have to be
// served at the JSON-RPC layer; everything else passes through untouched.
func (s *Server) WrapTransport(inner sdkmcp.Transport) sdkmcp.Transport {
	if s.cat() == nil {
		return inner
	}
	return &skillsTransport{inner: inner, srv: s}
}

type skillsTransport struct {
	inner sdkmcp.Transport
	srv   *Server
}

func (t *skillsTransport) Connect(ctx context.Context) (sdkmcp.Connection, error) {
	conn, err := t.inner.Connect(ctx)
	if err != nil {
		return nil, err //nolint:wrapcheck // transport errors pass through unchanged
	}
	return &skillsConn{Connection: conn, srv: t.srv}, nil
}

type skillsConn struct {
	sdkmcp.Connection
	srv *Server
	// initialized is set once an initialize request passed to the SDK: the
	// extension methods are answered here, below the SDK's lifecycle check, so
	// they apply the same rule (no call before initialize) themselves.
	initialized atomic.Bool
}

// newProtocolVersion is the first protocol version whose requests carry their
// own protocol version in _meta and need no initialize (SEP-2575).
const newProtocolVersion = "2026-07-28"

// mayAnswer reports whether a request may be answered: after initialize, or
// when it carries the per-request protocol version of the sessionless protocol.
func (c *skillsConn) mayAnswer(params json.RawMessage) bool {
	if c.initialized.Load() {
		return true
	}
	var p struct {
		Meta map[string]any `json:"_meta"`
	}
	if len(params) == 0 || json.Unmarshal(params, &p) != nil {
		return false
	}
	version, ok := p.Meta[sdkmcp.MetaKeyProtocolVersion].(string)
	return ok && version >= newProtocolVersion
}

func (c *skillsConn) Read(ctx context.Context) (jsonrpc.Message, error) {
	for {
		msg, err := c.Connection.Read(ctx)
		if err != nil {
			return nil, err //nolint:wrapcheck // transport errors pass through unchanged
		}
		req, ok := msg.(*jsonrpc.Request)
		if ok && req.Method == methodInitialize {
			c.initialized.Store(true)
		}
		if !ok || !req.ID.IsValid() || (req.Method != methodSkillsList && req.Method != methodSkillsGet && req.Method != methodDirectoryRead) {
			return msg, nil
		}
		resp := &jsonrpc.Response{ID: req.ID}
		if !c.mayAnswer(req.Params) {
			resp.Error = &jsonrpc.Error{Code: jsonrpc.CodeInvalidRequest, Message: fmt.Sprintf("method %q is invalid during session initialization", req.Method)}
		} else if result, rpcErr := c.srv.cat().handleSkillsMethod(req.Method, req.Params); rpcErr != nil {
			resp.Error = rpcErr
		} else if raw, mErr := json.Marshal(result); mErr != nil {
			resp.Error = &jsonrpc.Error{Code: jsonrpc.CodeInternalError, Message: mErr.Error()}
		} else {
			resp.Result = raw
		}
		if err := c.Write(ctx, resp); err != nil {
			return nil, err //nolint:wrapcheck // transport errors pass through unchanged
		}
	}
}

// handleSkillsMethod computes the result of skills/list or skills/get.
func (c *Catalog) handleSkillsMethod(method string, params json.RawMessage) (result map[string]any, rpcErr *jsonrpc.Error) {
	switch method {
	case methodSkillsList:
		entries := make([]map[string]any, 0, len(c.skills))
		for _, s := range c.skills {
			entries = append(entries, skillEntry(s, maxListedResources))
		}
		return map[string]any{keyResultType: resultTypeComplete, "skills": entries}, nil
	case methodSkillsGet:
		var p struct {
			URI string `json:"uri"`
		}
		if len(params) > 0 {
			if err := json.Unmarshal(params, &p); err != nil {
				return nil, &jsonrpc.Error{Code: jsonrpc.CodeInvalidParams, Message: "params.uri must be a string"}
			}
		}
		skill, ok := c.byURI[p.URI]
		if !ok {
			return nil, &jsonrpc.Error{Code: jsonrpc.CodeInvalidParams, Message: fmt.Sprintf("not a served skill: %q", p.URI)}
		}
		return map[string]any{keyResultType: resultTypeComplete, "skill": skillEntry(skill, 0)}, nil
	case methodDirectoryRead:
		var p struct {
			URI string `json:"uri"`
		}
		if len(params) > 0 {
			if err := json.Unmarshal(params, &p); err != nil {
				return nil, &jsonrpc.Error{Code: jsonrpc.CodeInvalidParams, Message: "params.uri must be a string"}
			}
		}
		children, ok := c.directoryChildren(p.URI)
		if !ok {
			return nil, &jsonrpc.Error{Code: jsonrpc.CodeInvalidParams, Message: fmt.Sprintf("not a directory resource: %q", p.URI)}
		}
		return map[string]any{keyResultType: resultTypeComplete, keyResources: children}, nil
	}
	return nil, &jsonrpc.Error{Code: jsonrpc.CodeMethodNotFound, Message: "method not found"}
}

// directoryChildren lists the direct children of a directory inside the served
// skill tree: the skill root (skill://<name>) or one of its subdirectories.
// Files carry their resource metadata, subdirectories are inode/directory
// resources. ok is false for anything that is not such a directory, so the
// method reads only what resources/read could already return.
func (c *Catalog) directoryChildren(uri string) (children []map[string]any, ok bool) {
	rest, found := strings.CutPrefix(uri, SkillURIScheme)
	if !found || rest == "" || strings.HasSuffix(rest, "/") {
		return nil, false
	}
	name, dir, _ := strings.Cut(rest, "/")
	skill, found := c.Lookup(name)
	if !found || skill.Name != name {
		return nil, false
	}
	prefix := ""
	if dir != "" {
		prefix = dir + "/"
	}
	seenDir := map[string]bool{}
	for i := range skill.Files {
		f := &skill.Files[i]
		rel, inside := strings.CutPrefix(f.RelPath, prefix)
		if !inside || rel == "" {
			continue
		}
		ok = true
		first, _, nested := strings.Cut(rel, "/")
		childURI := SkillURIScheme + skill.Name + "/" + prefix + first
		switch {
		case nested && !seenDir[first]:
			seenDir[first] = true
			children = append(children, map[string]any{keyURI: childURI, keyName: first, "mimeType": mimeDirectory})
		case !nested:
			children = append(children, map[string]any{keyURI: childURI, keyName: first, "mimeType": f.MIME, keySize: f.Size})
		}
	}
	if children == nil {
		children = []map[string]any{}
	}
	return children, ok
}
