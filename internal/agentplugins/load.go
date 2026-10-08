package agentplugins

import (
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"path"
	"slices"
	"strings"
)

// Result is the outcome of Validate and Import.
type Result struct {
	// Spec is the Agent Plugins version plugin.json declares; empty when it
	// declares none that is supported.
	Spec string `json:"spec"`
	// Rejected is set when a conformant client rejects the whole plugin.
	Rejected bool      `json:"rejected"`
	Findings []Finding `json:"findings"`
}

// manifestFields are the only top-level plugin.json fields (§5.2).
var manifestFields = []string{
	"$schema", "name", "version", "description", "author", "homepage", "repository", keyLicense, "keywords", "extensions",
}

// Validate checks the package rooted at fsys as a conformant client loads it:
// plugin.json against the vendored official schema, skill discovery (immediate
// children of skills/ whose SKILL.md is a regular file) and the Agent Skills
// rules, mcp.json and each server entry against the schema and §7.2, and path
// containment (§4.1). Failures are bounded as the specification requires: an
// invalid skill or server is skipped and reported, an invalid mcp.json
// disables MCP, and only a plugin.json problem rejects the plugin.
//
// Symbolic links are followed only when fsys implements fs.ReadLinkFS
// (os.DirFS does); a link that leaves the root is reported, never read.
func Validate(fsys fs.FS) Result {
	_, res := Import(fsys)
	return res
}

// Import reads the package rooted at fsys into a Plugin, keeping exactly the
// components Validate accepts. It returns a nil Plugin when the plugin is
// rejected. Build(Import(Build(p))) reproduces Build(p) byte for byte.
func Import(fsys fs.FS) (*Plugin, Result) {
	l := &loader{fsys: fsys}
	p := l.load()
	sortFindings(l.findings)
	res := Result{Spec: l.spec, Rejected: l.rejected, Findings: l.findings}
	if l.rejected {
		return nil, res
	}
	return p, res
}

type loader struct {
	fsys     fs.FS
	spec     string
	rejected bool
	findings []Finding
}

func (l *loader) add(code string, sev Severity, at, format string, a ...any) {
	l.findings = append(l.findings, Finding{Code: code, Severity: sev, Path: at, Message: fmt.Sprintf(format, a...)})
}

func (l *loader) reject(code, format string, a ...any) {
	l.rejected = true
	l.add(code, SeverityError, manifestFile, format, a...)
}

func (l *loader) load() *Plugin {
	set, meta, ext := l.manifest()
	if l.rejected {
		return nil
	}
	p := &Plugin{Metadata: meta}
	p.Skills = l.skills()
	p.MCPServers = l.mcp(set)
	l.rest(p, ext)
	return p
}

// manifest loads plugin.json; any failure other than an unknown field or a
// non-object extensions rejects the plugin (§5.2, §8.1).
func (l *loader) manifest() (*schemaSet, Metadata, map[string]map[string]any) {
	data, state, err := readResolved(l.fsys, manifestFile)
	switch state {
	case fileOK:
	case fileAbsent:
		l.reject(CodeManifestMissing, "plugin.json is required at the plugin root")
	case fileEscapes:
		l.reject(CodePathEscape, "plugin.json resolves outside the plugin root")
	case fileNotRegular:
		l.reject(CodeManifestInvalid, "plugin.json is not a regular file")
	default:
		l.reject(CodeManifestInvalid, "read plugin.json: %v", err)
	}
	if l.rejected {
		return nil, Metadata{}, nil
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil || doc == nil {
		l.reject(CodeManifestInvalid, "plugin.json is not a JSON object")
		return nil, Metadata{}, nil
	}
	id, _ := doc["$schema"].(string) //nolint:errcheck // a missing or non-string $schema is reported below
	spec, ok := specFromID(id, PluginSchemaID)
	if !ok {
		l.reject(CodeUnsupportedSpec, "$schema %q is not a supported Agent Plugins manifest identifier (supported versions: %s)",
			id, strings.Join(Specs, ", "))
		return nil, Metadata{}, nil
	}
	l.spec = spec
	set, err := schemasFor(spec)
	if err != nil {
		l.reject(CodeUnsupportedSpec, "%v", err)
		return nil, Metadata{}, nil
	}
	for _, key := range slices.Sorted(maps.Keys(doc)) {
		if !slices.Contains(manifestFields, key) {
			l.add(CodeManifestUnknownField, SeverityWarning, manifestFile,
				"unknown top-level field %q is ignored; client-specific data belongs under extensions", key)
			delete(doc, key)
		}
	}
	extObject := false
	if v, present := doc["extensions"]; present {
		if _, extObject = v.(map[string]any); !extObject {
			l.add(CodeExtensionsInvalid, SeverityWarning, manifestFile, "extensions is not an object and is ignored")
			delete(doc, "extensions")
		}
	}
	if errs := schemaErrors(set.plugin, doc); len(errs) > 0 {
		l.reject(CodeManifestInvalid, "plugin.json does not match the Agent Plugins %s schema: %s", spec, strings.Join(errs, "; "))
		return nil, Metadata{}, nil
	}
	meta, ext := l.decodeManifest(data, extObject)
	return set, meta, ext
}

// decodeManifest extracts the metadata and the extension data of a manifest
// the schema accepted. Extension data keeps its JSON numbers verbatim.
func (l *loader) decodeManifest(data []byte, extObject bool) (meta Metadata, ext map[string]map[string]any) {
	var in struct {
		Name        string          `json:"name"`
		Version     string          `json:"version"`
		Description string          `json:"description"`
		Author      *Author         `json:"author"`
		Homepage    string          `json:"homepage"`
		Repository  string          `json:"repository"`
		License     string          `json:"license"`
		Keywords    []string        `json:"keywords"`
		Extensions  json.RawMessage `json:"extensions"`
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	_ = dec.Decode(&in) //nolint:errcheck // the document already matched the schema, so every known field has its type
	meta = Metadata{
		Name: in.Name, Version: in.Version, Description: in.Description, Author: in.Author,
		Homepage: in.Homepage, Repository: in.Repository, License: in.License, Keywords: in.Keywords,
	}
	if !extObject {
		return meta, nil
	}
	var raw map[string]map[string]any
	dec = json.NewDecoder(bytes.NewReader(in.Extensions))
	dec.UseNumber()
	_ = dec.Decode(&raw) //nolint:errcheck // the schema requires an object of objects
	ext = map[string]map[string]any{}
	for _, ns := range slices.Sorted(maps.Keys(raw)) {
		if !ValidNamespace(ns) {
			l.add(CodeNamespaceInvalid, SeverityWarning, manifestFile,
				"extensions key %q is not a reverse-domain namespace and is not imported", ns)
			continue
		}
		ext[ns] = raw[ns]
	}
	return meta, ext
}

// skills discovers skills in the immediate children of skills/ (§7.1).
func (l *loader) skills() []Skill {
	root, state, err := statResolved(l.fsys, skillsDir)
	switch state {
	case fileAbsent:
		return nil
	case fileEscapes:
		l.add(CodePathEscape, SeverityError, skillsDir, "skills resolves outside the plugin root; no skill is loaded")
		return nil
	case fileUnreadable:
		l.add(CodeSkillsLocation, SeverityError, skillsDir, "read skills: %v; no skill is loaded", err)
		return nil
	case fileOK, fileNotRegular:
	}
	if info, err := fs.Stat(l.fsys, root); err != nil || !info.IsDir() {
		l.add(CodeSkillsLocation, SeverityError, skillsDir, "skills is not a directory; no skill is loaded")
		return nil
	}
	entries, err := fs.ReadDir(l.fsys, root)
	if err != nil {
		l.add(CodeSkillsLocation, SeverityError, skillsDir, "read skills: %v; no skill is loaded", err)
		return nil
	}
	var out []Skill
	for _, e := range entries {
		if s, ok := l.skill(path.Join(root, e.Name()), e.Name()); ok {
			out = append(out, s)
		}
	}
	return out
}

func (l *loader) skill(dirPath, name string) (Skill, bool) {
	at := skillsDir + "/" + name
	dir, err := resolve(l.fsys, dirPath)
	switch {
	case errors.Is(err, errEscape):
		l.add(CodePathEscape, SeverityError, at, "skill directory resolves outside the plugin root; the skill is skipped")
		return Skill{}, false
	case err != nil:
		l.add(CodeUnreadable, SeverityError, at, "read skill directory: %v; the skill is skipped", err)
		return Skill{}, false
	}
	if info, err := fs.Stat(l.fsys, dir); err != nil || !info.IsDir() {
		return Skill{}, false // a file under skills/ is not a skill
	}
	mdAt := at + "/" + skillFile
	data, state, err := readResolved(l.fsys, path.Join(dir, skillFile))
	switch state {
	case fileOK:
	case fileAbsent:
		l.add(CodeSkillMissing, SeverityWarning, at,
			"directory has no SKILL.md and is not a skill; skills are discovered only in immediate children of skills/")
		return Skill{}, false
	case fileEscapes:
		l.add(CodePathEscape, SeverityError, mdAt, "SKILL.md resolves outside the plugin root; the skill is skipped")
		return Skill{}, false
	case fileNotRegular:
		l.add(CodeSkillInvalid, SeverityError, mdAt, "SKILL.md is not a regular file; the skill is skipped")
		return Skill{}, false
	default:
		l.add(CodeSkillInvalid, SeverityError, mdAt, "read SKILL.md: %v; the skill is skipped", err)
		return Skill{}, false
	}
	fatal := false
	for _, i := range checkSkill(name, data) {
		if i.fatal {
			fatal = true
			l.add(CodeSkillInvalid, SeverityError, mdAt, "%s; the skill is skipped", i.msg)
		} else {
			l.add(CodeSkillUnknownField, SeverityInfo, mdAt, "%s", i.msg)
		}
	}
	if fatal {
		return Skill{}, false
	}
	files := l.readTree(dir, at, func(rel string) bool { return rel == skillFile })
	return Skill{Name: name, SkillMD: data, Files: files}, true
}

// mcp loads mcp.json (§7.2.2): a document problem disables MCP, an entry
// problem skips that server.
func (l *loader) mcp(set *schemaSet) []MCPServer {
	data, state, err := readResolved(l.fsys, mcpFile)
	switch state {
	case fileOK:
	case fileAbsent:
		return nil
	case fileEscapes:
		l.add(CodePathEscape, SeverityError, mcpFile, "mcp.json resolves outside the plugin root; MCP is disabled")
		return nil
	case fileNotRegular:
		l.add(CodeMCPInvalid, SeverityError, mcpFile, "mcp.json is not a regular file; MCP is disabled")
		return nil
	default:
		l.add(CodeMCPInvalid, SeverityError, mcpFile, "read mcp.json: %v; MCP is disabled", err)
		return nil
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil || doc == nil {
		l.add(CodeMCPInvalid, SeverityError, mcpFile, "mcp.json is not a JSON object; MCP is disabled")
		return nil
	}
	id, _ := doc["$schema"].(string) //nolint:errcheck // a missing or non-string $schema is reported below
	spec, ok := specFromID(id, MCPSchemaID)
	switch {
	case !ok:
		l.add(CodeMCPInvalid, SeverityError, mcpFile,
			"$schema %q is not a supported Agent Plugins MCP identifier; MCP is disabled", id)
		return nil
	case spec != l.spec:
		l.add(CodeMCPSpecMismatch, SeverityError, mcpFile,
			"mcp.json targets Agent Plugins %s but plugin.json targets %s; MCP is disabled", spec, l.spec)
		return nil
	}
	servers, isObj := doc["mcpServers"].(map[string]any)
	top := maps.Clone(doc)
	if isObj {
		top["mcpServers"] = map[string]any{}
	}
	if errs := schemaErrors(set.mcp, top); len(errs) > 0 {
		l.add(CodeMCPInvalid, SeverityError, mcpFile, "mcp.json does not match the Agent Plugins %s schema: %s; MCP is disabled",
			spec, strings.Join(errs, "; "))
		return nil
	}
	var out []MCPServer
	for _, name := range slices.Sorted(maps.Keys(servers)) {
		if s, ok := l.server(set, id, name, servers[name]); ok {
			out = append(out, s)
		}
	}
	return out
}

func (l *loader) server(set *schemaSet, id, name string, entry any) (MCPServer, bool) {
	at := serverPath(name)
	single := map[string]any{"$schema": id, "mcpServers": map[string]any{name: entry}}
	if errs := schemaErrors(set.mcp, single); len(errs) > 0 {
		l.add(CodeServerInvalid, SeverityError, at, "server does not match the schema: %s; it is skipped",
			strings.Join(serverErrors(set, entry), "; "))
		return MCPServer{}, false
	}
	var ws wireServer
	raw, err := json.Marshal(entry)
	if err == nil {
		err = json.Unmarshal(raw, &ws)
	}
	if err != nil {
		l.add(CodeServerInvalid, SeverityError, at, "decode server: %v; it is skipped", err)
		return MCPServer{}, false
	}
	issues := append(checkServer(ws, modeValidate), l.containment(ws)...)
	for _, i := range issues {
		msg := i.msg
		if i.sev == SeverityError {
			msg += "; the server is skipped"
		}
		l.add(i.code, i.sev, at, "%s", msg)
	}
	if hasError(issues) {
		return MCPServer{}, false
	}
	return fromWire(name, ws), true
}

// containment checks that a stdio server's plugin-relative command and cwd
// stay inside the plugin root on disk (§4.1, §7.2.1).
func (l *loader) containment(ws wireServer) []issue {
	if ws.Type != typeStdio {
		return nil
	}
	var issues []issue
	if rel, ok := strings.CutPrefix(ws.Command, "./"); ok && commandProblem(ws.Command) == "" {
		switch _, state, err := statResolved(l.fsys, rel); state {
		case fileEscapes:
			issues = append(issues, invalid("command %q resolves outside the plugin root", ws.Command))
		case fileAbsent:
			issues = append(issues, issue{CodeCommandNotBundled, SeverityWarning,
				fmt.Sprintf("command %q is not in the package", ws.Command)})
		case fileNotRegular:
			issues = append(issues, invalid("command %q is not a regular file", ws.Command))
		case fileUnreadable:
			issues = append(issues, invalid("command %q cannot be checked: %v", ws.Command, err))
		case fileOK:
		}
	}
	rel, ok := strings.CutPrefix(ws.Cwd, "./")
	if !ok {
		rel, ok = strings.CutPrefix(ws.Cwd, phRoot+"/")
	}
	if ok && cwdProblem(ws.Cwd) == "" {
		if _, err := resolve(l.fsys, rel); err != nil {
			issues = append(issues, invalid("cwd %q resolves outside the plugin root", ws.Cwd))
		}
	}
	return issues
}

// serverErrors explains why entry fails the server union, using the variant
// its type selects.
func serverErrors(set *schemaSet, entry any) []string {
	obj, _ := entry.(map[string]any) //nolint:errcheck // a non-object entry has no type and is reported as such
	typ, _ := obj["type"].(string)   //nolint:errcheck // a missing or non-string type is reported below
	variant, ok := set.variants[typ]
	if !ok {
		return []string{fmt.Sprintf("/type: must be one of stdio, streamable-http or sse, not %v", obj["type"])}
	}
	return schemaErrors(variant, entry)
}

func fromWire(name string, ws wireServer) MCPServer {
	s := MCPServer{
		Name: name, Command: ws.Command, Args: ws.Args, Env: ws.Env, Cwd: ws.Cwd, URL: ws.URL, Headers: ws.Headers,
	}
	switch ws.Type {
	case typeStdio:
		s.Transport = TransportStdio
	case typeSSE:
		s.Transport = TransportSSE
	default:
		s.Transport = TransportHTTP
	}
	return s
}

// rest reads the extension namespace directories and the other root files.
func (l *loader) rest(p *Plugin, ext map[string]map[string]any) {
	var dirs []string
	p.Files = l.readTree(".", "", func(rel string) bool {
		first, _, nested := strings.Cut(rel, "/")
		if reservedRoot(first) {
			return true
		}
		if !nested && ValidNamespace(first) && l.isDir(first) {
			dirs = append(dirs, first)
			return true
		}
		return false
	})
	namespaces := slices.Sorted(maps.Keys(ext))
	for _, d := range dirs {
		if !slices.Contains(namespaces, d) {
			namespaces = append(namespaces, d)
		}
	}
	slices.Sort(namespaces)
	for _, ns := range namespaces {
		e := Extension{Namespace: ns, Manifest: ext[ns]}
		if slices.Contains(dirs, ns) {
			e.Files = l.readTree(ns, ns, func(string) bool { return false })
		}
		p.Extensions = append(p.Extensions, e)
	}
}

func (l *loader) isDir(name string) bool {
	info, err := fs.Lstat(l.fsys, name)
	return err == nil && info.IsDir()
}

// readTree reads the regular files under root (a resolved directory), keyed by
// slash path relative to root. at is the package path root is reported under.
// skip prunes a relative path (and, for a directory, everything below it).
// Symbolic links are followed to files inside the plugin root only.
func (l *loader) readTree(root, at string, skip func(rel string) bool) map[string][]byte {
	files := map[string][]byte{}
	walkErr := fs.WalkDir(l.fsys, root, func(p string, d fs.DirEntry, err error) error {
		rel := strings.TrimPrefix(strings.TrimPrefix(p, root), "/")
		if root == "." {
			rel = p
		}
		if p == root {
			return err
		}
		where := path.Join(at, rel)
		if err != nil {
			l.add(CodeUnreadable, SeverityError, where, "read: %v", err)
			return nil
		}
		if skip(rel) {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		switch {
		case d.IsDir():
		case d.Type()&fs.ModeSymlink != 0:
			l.readLinked(files, p, rel, where)
		case d.Type().IsRegular():
			data, err := fs.ReadFile(l.fsys, p)
			if err != nil {
				l.add(CodeUnreadable, SeverityError, where, "read: %v", err)
				return nil
			}
			files[rel] = data
		default:
			l.add(CodeUnreadable, SeverityWarning, where, "not a regular file; it is not read")
		}
		return nil
	})
	if walkErr != nil {
		l.add(CodeUnreadable, SeverityError, cmp.Or(at, "."), "read: %v", walkErr)
	}
	if len(files) == 0 {
		return nil
	}
	return files
}

func (l *loader) readLinked(files map[string][]byte, p, rel, where string) {
	data, state, err := readResolved(l.fsys, p)
	switch state {
	case fileOK:
		files[rel] = data
	case fileEscapes:
		l.add(CodePathEscape, SeverityError, where, "symbolic link resolves outside the plugin root; it is not read")
	case fileAbsent:
		l.add(CodeUnreadable, SeverityWarning, where, "symbolic link target does not exist; it is not read")
	case fileNotRegular:
		l.add(CodeUnreadable, SeverityWarning, where, "symbolic link to a directory is not followed")
	default:
		l.add(CodeUnreadable, SeverityError, where, "read: %v", err)
	}
}
