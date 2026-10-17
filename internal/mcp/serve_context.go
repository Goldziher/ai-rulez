package mcp

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path"
	"strings"
	"unicode/utf8"

	"github.com/Goldziher/ai-rulez/v5/internal/mcp/handlers"
	"github.com/Goldziher/ai-rulez/v5/internal/repoindex"
	"github.com/Goldziher/ai-rulez/v5/internal/skillsearch"
	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Nested context: with repository discovery on, the instruction files, rules
// and agents the repository keeps at any depth are served beside the skills.
// find_context searches summaries; load_context and the context:// resources
// read a body from the repository when they are called, never from the index.

const (
	toolFindContext = "find_context"
	toolLoadContext = "load_context"

	// contextScheme is the URI scheme of a context resource: context://<scope>/<name>.
	contextScheme = "context://"
	uriContext    = contextScheme + "{+scope}"

	argURI = "uri"

	// maxSummaryScopes bounds the scopes the server instructions name.
	maxSummaryScopes = 8
)

// errContextNotFound marks a context that is not indexed.
var errContextNotFound = errors.New("no served context")

// servedAsContext reports whether a kind of item is served by find_context.
// Commands are indexed and counted, not served.
func servedAsContext(k repoindex.Kind) bool {
	return k == repoindex.KindInstruction || k == repoindex.KindRule || k == repoindex.KindAgent
}

func contextKinds() []string {
	return []string{string(repoindex.KindInstruction), string(repoindex.KindRule), string(repoindex.KindAgent)}
}

// contextURI is the resource URI of an item.
func contextURI(it *repoindex.Item) string {
	segments := strings.Split(it.Scope, "/")
	for i, seg := range segments {
		segments[i] = url.PathEscape(seg)
	}
	return contextScheme + strings.Join(segments, "/") + "/" + url.PathEscape(it.Name)
}

// parseContextURI splits context://<scope>/<name>; the scope is validated like
// every scope argument, so a URI cannot climb out of the project.
func parseContextURI(uri string) (scope, name string, err error) {
	rest, ok := strings.CutPrefix(uri, contextScheme)
	if !ok {
		return "", "", fmt.Errorf("%q is not a context:// URI", uri)
	}
	cut := strings.LastIndex(rest, "/")
	if cut <= 0 || cut == len(rest)-1 {
		return "", "", fmt.Errorf("a context URI is context://<scope>/<name>, got %q", uri)
	}
	rawScope, rawName := rest[:cut], rest[cut+1:]
	if scope, err = url.PathUnescape(rawScope); err != nil {
		return "", "", fmt.Errorf("context URI scope: %w", err)
	}
	if name, err = url.PathUnescape(rawName); err != nil {
		return "", "", fmt.Errorf("context URI name: %w", err)
	}
	if scope, err = repoindex.CleanPath(scope); err != nil {
		return "", "", fmt.Errorf("context URI scope: %w", err)
	}
	return scope, name, nil
}

// allowsContext applies --allow and --deny to an item of context: a pattern
// matches the item's name or scope/name. Domains are skill-only.
func (f SkillFilter) allowsContext(it *repoindex.Item) bool {
	ids := []string{it.Name, it.Scope + "/" + it.Name}
	for _, id := range ids {
		if matchesAny(f.Deny, id) {
			return false
		}
	}
	if len(f.Allow) == 0 {
		return true
	}
	for _, id := range ids {
		if matchesAny(f.Allow, id) {
			return true
		}
	}
	return false
}

// screen judges an item of the index with its content in hand: allow and deny,
// then the same security scan skills go through. A skill or command passes.
func (d *discovery) screen(it repoindex.Item, content []byte) (keep bool, reason string) {
	if !servedAsContext(it.Kind) {
		return true, ""
	}
	if !d.st.Filter.allowsContext(&it) {
		return false, ""
	}
	adm := d.admission()
	adm.Verdicts = d.verdicts() // only the build may rely on a verdict; the view's admission scans every time
	if r := scanContext(adm, &it, content); r != nil {
		return false, fmt.Sprintf("%s: %s", r.Code, r.Reason)
	}
	return true, ""
}

// admission is the scan setup of this discovery: the project's lint settings
// and the default trust of content authored in the project.
func (d *discovery) admission() Admission {
	return Admission{Config: d.cfg, DefaultTrust: defaultTrust(d.cfg), Log: d.st.Log}
}

// scanContext runs the served-skill security scan over one item of context.
func scanContext(adm Admission, it *repoindex.Item, content []byte) *Refusal {
	probe := &CatalogSkill{Name: it.Name, Files: []CatalogFile{{RelPath: path.Base(it.Path), Content: content}}}
	return adm.scanAs(probe, contextSubject(contextURI(it)))
}

// contextCandidates lists the items of context a query selects: what a path
// resolves to (nearest scope first, shadowed agents dropped), one scope, or all.
func (v *repoView) contextCandidates(q scopeQuery) (items []repoindex.Item, scopes []string, err error) {
	var all []repoindex.Item
	switch {
	case q.Path != "":
		res, rerr := v.index.Resolve(q.Path)
		if rerr != nil {
			return nil, nil, rerr //nolint:wrapcheck // names the path
		}
		scopes = res.Scopes
		for i := range res.Items {
			if res.Items[i].ShadowedBy == "" {
				all = append(all, res.Items[i].Item)
			}
		}
	case q.Scope != "":
		if all, err = v.index.Within(q.Scope, q.Recursive); err != nil {
			return nil, nil, err //nolint:wrapcheck // names the scope
		}
	default:
		all = v.index.Items()
	}
	for _, it := range all {
		if servedAsContext(it.Kind) && (q.Path == "" || ruleAppliesTo(&it, q.Path)) {
			items = append(items, it)
		}
	}
	return items, scopes, nil
}

// rankContext orders items by how well their name, scope, kind and description
// match a task; an item with no matching term is left out.
func rankContext(items []repoindex.Item, task string) []repoindex.Item {
	docs := make([]skillsearch.Doc, len(items))
	for i := range items {
		it := &items[i]
		docs[i] = skillsearch.Doc{Name: it.Name, Description: it.Description, Keywords: []string{it.Scope, string(it.Kind)}}
	}
	ranked := skillsearch.Rank(docs, task)
	out := make([]repoindex.Item, 0, len(ranked))
	for _, h := range ranked {
		out = append(out, items[h.Index])
	}
	return out
}

// contextSummary is the description of an item as find_context returns it.
func contextSummary(it *repoindex.Item) map[string]any {
	out := map[string]any{
		keyName: it.Name, keyKind: string(it.Kind), argScope: it.Scope, "source": it.Source,
		keyDescription: it.Description, keyURI: contextURI(it), "hash": it.Hash,
	}
	if only := it.Only; only != nil {
		if len(only.Paths) > 0 {
			out["paths"] = only.Paths
		}
		if len(only.Globs) > 0 {
			out["globs"] = only.Globs
		}
	}
	return out
}

type findContextIn struct {
	Task      string `json:"task,omitempty" jsonschema:"Words to rank the instruction files, rules and agents by (their name, scope, kind and description); omit to list what applies"`
	Path      string `json:"path,omitempty" jsonschema:"A project-relative path of a file you are working on: the context of its scope and the ancestor scopes, nearest first; a rule with paths or globs in its frontmatter only if the path matches. Not with scope"`
	Scope     string `json:"scope,omitempty" jsonschema:"One scope (a directory, '.' for the root). Not with path"`
	Recursive bool   `json:"recursive,omitempty" jsonschema:"With scope: also the scopes below it"`
	Kind      string `json:"kind,omitempty" jsonschema:"Only this kind of context"`
	Limit     int    `json:"limit,omitempty" jsonschema:"Maximum results (default 10, max 50); total reports the count before the cut"`
}

type loadContextIn struct {
	Scope       string `json:"scope,omitempty" jsonschema:"The scope from find_context (with name)"`
	Name        string `json:"name,omitempty" jsonschema:"The name from find_context (with scope)"`
	URI         string `json:"uri,omitempty" jsonschema:"The context:// URI from find_context, instead of scope and name"`
	BudgetBytes int    `json:"budget_bytes,omitempty" jsonschema:"Return at most this many bytes of the file (default: the whole file)"`
}

type contextSearchOut struct {
	Count   int      `json:"count" jsonschema:"Number of results"`
	Total   int      `json:"total" jsonschema:"Matches before the limit"`
	Results []any    `json:"results,omitempty" jsonschema:"Instruction files, rules and agents: name, kind, scope, source, description, uri, hash"`
	Path    string   `json:"path,omitempty" jsonschema:"The path that was resolved"`
	Scopes  []string `json:"scopes,omitempty" jsonschema:"The scopes the path resolved to, nearest first"`
	Scope   string   `json:"scope,omitempty" jsonschema:"The scope that was listed"`
	Hint    string   `json:"hint,omitempty" jsonschema:"What to try when nothing matched"`
}

type contextOut struct {
	Name      string `json:"name,omitempty" jsonschema:"Item name"`
	Kind      string `json:"kind,omitempty" jsonschema:"instruction, rule or agent"`
	Scope     string `json:"scope,omitempty" jsonschema:"Scope the item lives in"`
	URI       string `json:"uri,omitempty" jsonschema:"context:// URI"`
	Content   string `json:"content,omitempty" jsonschema:"File text, read from the repository now"`
	Hash      string `json:"hash,omitempty" jsonschema:"Hash of the content just read"`
	Truncated bool   `json:"truncated,omitempty" jsonschema:"True when the content was cut at budget_bytes"`
}

func (s *Server) registerContextTools() {
	s.noteKnown(toolFindContext, toolLoadContext)
	if s.cat().repository() == nil {
		return
	}
	addTool[findContextIn](s, toolSpec{
		name: toolFindContext, title: "Find Context",
		description: "Find the instruction files (AGENTS.md, CLAUDE.md, GEMINI.md), rules and agents a repository keeps at any depth. Pass path for what applies to a file (its scope and the ancestors', nearest first; a rule with paths or globs only when the file matches) or scope for one directory; add task to rank. Returns summaries (name, scope, kind, description, uri); read one with load_context. Call this before changing code in an unfamiliar package.",
		annotations: readOnlyAnnotations(), output: contextSearchOut{}, enums: map[string][]string{keyKind: contextKinds()},
	}, s.findContextHandler)
	addTool[loadContextIn](s, toolSpec{
		name: toolLoadContext, title: "Load Context",
		description: "Load one instruction file, rule or agent found by find_context: its text, read from the repository when you call (never from the index), with the hash of what was read. Each session has a byte budget shared with load_skill.",
		annotations: readOnlyAnnotations(), output: contextOut{},
	}, s.loadContextHandler)
	s.mcpServer.AddResourceTemplate(&sdkmcp.ResourceTemplate{
		URITemplate: uriContext, Name: "context", Title: "Repository Context", MIMEType: mimeMarkdown,
		Description: "One instruction file, rule or agent of the repository: context://<scope>/<name> (the scope '.' is the project root); find_context lists them",
		Annotations: &sdkmcp.Annotations{Audience: []sdkmcp.Role{roleAssistant}, Priority: 0.6},
	}, s.readContextResource)
}

func (s *Server) findContextHandler(_ context.Context, req *handlers.ToolRequest) (*sdkmcp.CallToolResult, error) {
	view := s.cat().repository()
	if view == nil {
		return handlers.ToolError(errNeedsDiscovery)
	}
	q, err := scopeQueryOf(req)
	if err != nil {
		return handlers.ToolError(err)
	}
	kind := req.GetString(keyKind, "")
	if kind != "" && !containsString(contextKinds(), kind) {
		return handlers.ToolError(fmt.Errorf("kind %q is not one of %s", kind, strings.Join(contextKinds(), ", ")))
	}
	limit, err := limitArg(req, "limit", defaultSearchLimit)
	if err != nil {
		return handlers.ToolError(err)
	}
	limit = min(limit, maxSearchLimit)
	items, scopes, err := view.contextCandidates(q)
	if err != nil {
		return handlers.ToolError(err)
	}
	if kind != "" {
		items = filterItems(items, func(it *repoindex.Item) bool { return string(it.Kind) == kind })
	}
	task := strings.TrimSpace(req.GetString("task", ""))
	if task != "" {
		items = rankContext(items, task)
	}
	total := len(items)
	results := make([]any, 0, min(total, limit))
	for i := range items[:min(total, limit)] {
		results = append(results, contextSummary(&items[i]))
	}
	out := map[string]any{keyCount: len(results), "total": total, keyResults: results}
	switch {
	case q.Path != "":
		out[argPath], out["scopes"] = q.Path, scopes
	case q.Scope != "":
		out[argScope] = q.Scope
	}
	if total == 0 {
		out["hint"] = "no served instruction file, rule or agent matches; try a broader path or scope, or different words"
	}
	return handlers.ToolSuccess(out)
}

func filterItems(items []repoindex.Item, keep func(*repoindex.Item) bool) []repoindex.Item {
	out := items[:0:0]
	for i := range items {
		if keep(&items[i]) {
			out = append(out, items[i])
		}
	}
	return out
}

// contextRef reads the scope and name a load_context call names, from its uri or
// from scope and name.
func contextRef(req *handlers.ToolRequest) (scope, name string, err error) {
	if uri := req.GetString(argURI, ""); uri != "" {
		return parseContextURI(uri)
	}
	rawScope, name := req.GetString(argScope, ""), strings.TrimSpace(req.GetString(keyName, ""))
	switch {
	case rawScope == "":
		return "", "", fmt.Errorf("scope is required with name (or pass uri): use the scope from find_context")
	case name == "":
		return "", "", fmt.Errorf("name is required with scope (or pass uri): use the name from find_context")
	}
	if scope, err = repoindex.CleanPath(rawScope); err != nil {
		return "", "", fmt.Errorf("scope: %w", err)
	}
	return scope, name, nil
}

// loaded is an item of context with the body read for it.
type loaded struct {
	item    repoindex.Item
	content []byte
}

// load finds an item of context and reads its body from the repository now. The
// body is scanned again: a file can change after the index was built, and the
// index never vouches for it.
func (v *repoView) load(scope, name string) (loaded, error) {
	if name == ".." || strings.ContainsAny(name, "/\\") {
		return loaded{}, fmt.Errorf("name %q must be a plain name from find_context, not a path", name)
	}
	it, found := v.index.Lookup(scope, name)
	if !found || !servedAsContext(it.Kind) {
		if reason, refused := v.refused[refusedKey(scope, name)]; refused {
			return loaded{}, fmt.Errorf("context %q in %q is refused (%s)", name, scope, reason)
		}
		return loaded{}, fmt.Errorf("%w %q in scope %q; call find_context to see what is available", errContextNotFound, name, scope)
	}
	content, ok, err := repoindex.ReadRegular(v.root, it.Path, repoindex.MaxFileBytes)
	switch {
	case err != nil:
		return loaded{}, fmt.Errorf("read %s: %w", it.Path, err)
	case !ok:
		return loaded{}, fmt.Errorf("%w %q in scope %q: the file is gone or is not a regular file", errContextNotFound, name, scope)
	case v.generated != nil && v.generated(joinRoot(v.root, it.Path), content):
		return loaded{}, fmt.Errorf("context %q in %q is refused (generated by ai-rulez)", name, scope)
	}
	if r := scanContext(v.adm, &it, content); r != nil {
		return loaded{}, fmt.Errorf("context %q in %q is refused (%s): %s", name, scope, r.Code, r.Reason)
	}
	return loaded{item: it, content: content}, nil
}

func (s *Server) loadContextHandler(_ context.Context, req *handlers.ToolRequest) (*sdkmcp.CallToolResult, error) {
	view := s.cat().repository()
	if view == nil {
		return handlers.ToolError(errNeedsDiscovery)
	}
	scope, name, err := contextRef(req)
	if err != nil {
		return handlers.ToolError(err)
	}
	budget, err := limitArg(req, "budget_bytes", 0)
	if err != nil {
		return handlers.ToolError(err)
	}
	got, err := view.load(scope, name)
	if err != nil {
		return handlers.ToolError(err)
	}
	if !utf8.Valid(got.content) {
		return handlers.ToolError(fmt.Errorf("%s is not text", got.item.Path))
	}
	text, truncated := string(got.content), false
	if budget > 0 && len(text) > budget {
		text, truncated = truncateUTF8(text, budget), true
	}
	session, _ := s.sessionInfo(req)
	remaining, ok := s.serve.charge(session, len(text))
	if !ok {
		return handlers.ToolError(fmt.Errorf("session budget exhausted: loading %d bytes would exceed the %d-byte cap (%d bytes left); pass a smaller budget_bytes or restart the session",
			len(text), s.serve.opts.budget(), remaining))
	}
	out := contextSummary(&got.item)
	hash := repoindex.Hash(got.content)
	out["hash"], out["content"], out["bytes"], out["truncated"] = hash, text, len(text), truncated
	if hash != got.item.Hash {
		out["changed_since_index"] = true
	}
	if truncated {
		out["total_bytes"] = len(got.content)
	}
	if remaining >= 0 {
		out["budget_remaining_bytes"] = remaining
	}
	return handlers.ToolSuccess(out)
}

// readContextResource serves a context:// resource: the body, read now, under
// the same scan and session budget as load_context.
func (s *Server) readContextResource(_ context.Context, req *sdkmcp.ReadResourceRequest) (*sdkmcp.ReadResourceResult, error) {
	uri := req.Params.URI
	view := s.cat().repository()
	if view == nil {
		return nil, resourceNotFound(uri)
	}
	scope, name, err := parseContextURI(uri)
	if err != nil {
		return nil, &jsonrpc.Error{Code: jsonrpc.CodeInvalidParams, Message: err.Error()}
	}
	got, err := view.load(scope, name)
	switch {
	case errors.Is(err, errContextNotFound):
		return nil, resourceNotFound(uri)
	case err != nil:
		return nil, &jsonrpc.Error{Code: jsonrpc.CodeInvalidRequest, Message: err.Error()}
	}
	if err := s.chargeRead(s.serve.sessionID(req.Session), len(got.content)); err != nil {
		return nil, &jsonrpc.Error{Code: jsonrpc.CodeInvalidRequest, Message: err.Error()}
	}
	contents := &sdkmcp.ResourceContents{URI: uri, MIMEType: mimeMarkdown}
	if utf8.Valid(got.content) {
		contents.Text = string(got.content)
	} else {
		contents.Blob = got.content
	}
	return &sdkmcp.ReadResourceResult{Contents: []*sdkmcp.ResourceContents{contents}}, nil
}

// scopeSummary is the sentence of the server instructions that tells a planner
// the nested content exists: the first maxSummaryScopes scopes by depth, then
// the count of the rest. Empty when discovery did not index the repository.
func scopeSummary(v *repoView) string {
	if v == nil {
		return ""
	}
	var described []string
	for _, s := range v.index.Scopes() {
		if parts := countParts(s.Counts); len(parts) > 0 {
			described = append(described, fmt.Sprintf("%s (%s)", s.Path, strings.Join(parts, ", ")))
		}
	}
	if len(described) == 0 {
		return ""
	}
	shown := described[:min(len(described), maxSummaryScopes)]
	list := strings.Join(shown, "; ")
	if more := len(described) - len(shown); more > 0 {
		list += fmt.Sprintf("; and %d more", more)
	}
	return " The repository keeps skills, instruction files, rules and agents in nested scopes: " + list + ". " +
		"find_skill and find_context take path (what applies to a file) or scope; load_context reads one instruction file, rule or agent (context://<scope>/<name>)."
}

// countParts renders per-kind counts in a stable order: "2 skills", "1 instruction".
func countParts(counts map[repoindex.Kind]int) []string {
	var parts []string
	for _, k := range []repoindex.Kind{
		repoindex.KindSkill, repoindex.KindInstruction, repoindex.KindRule, repoindex.KindAgent, repoindex.KindCommand,
	} {
		switch n := counts[k]; n {
		case 0:
		case 1:
			parts = append(parts, fmt.Sprintf("1 %s", k))
		default:
			parts = append(parts, fmt.Sprintf("%d %ss", n, k))
		}
	}
	return parts
}
