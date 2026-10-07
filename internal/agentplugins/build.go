package agentplugins

import (
	"bytes"
	"cmp"
	"encoding/json"
	"path"
	"strings"

	"github.com/kaptinlin/jsonschema"
	"github.com/samber/oops"
)

// Options configures Build.
type Options struct {
	// Spec is the Agent Plugins version to target: "1.0.0" (the default) or "1.1.0".
	Spec string
}

// wireManifest is the closed plugin.json object (§5.2). Field order is the
// output order.
type wireManifest struct {
	Schema      string                    `json:"$schema"`
	Name        string                    `json:"name"`
	Version     string                    `json:"version,omitempty"`
	Description string                    `json:"description,omitempty"`
	Author      *Author                   `json:"author,omitempty"`
	Homepage    string                    `json:"homepage,omitempty"`
	Repository  string                    `json:"repository,omitempty"`
	License     string                    `json:"license,omitempty"`
	Keywords    []string                  `json:"keywords,omitempty"`
	Extensions  map[string]map[string]any `json:"extensions,omitempty"`
}

// Build renders p as an Agent Plugins package: a map from slash path to file
// bytes. The output is deterministic (sorted keys, no timestamps) and
// independent of the order of p's slices.
//
// Content a conformant client would skip is not packaged: an invalid skill or
// MCP server is dropped and reported as an error Finding, and Build never
// writes a plugin.json or mcp.json that fails the official schema. Build
// returns an error only for input that cannot form a package at all: an
// unsupported spec, an invalid plugin name, duplicate names, an invalid
// extension namespace or an unsafe file path.
func Build(p *Plugin, opts Options) (map[string][]byte, []Finding, error) {
	spec := cmp.Or(opts.Spec, DefaultSpec)
	set, err := schemasFor(spec)
	if err != nil {
		return nil, nil, err
	}
	if p == nil {
		return nil, nil, oops.Errorf("nil plugin")
	}
	if !ValidPluginName(p.Metadata.Name) {
		return nil, nil, oops.With("name", p.Metadata.Name).Errorf(
			"plugin name %q is invalid: use 1-64 characters from a-z, 0-9, '-' and '.', "+
				"starting and ending alphanumeric, without '--' or '..'", p.Metadata.Name)
	}

	b := &builder{files: map[string][]byte{}}
	if err := b.addSkills(p.Skills); err != nil {
		return nil, nil, err
	}
	servers, err := b.mapServers(p.MCPServers)
	if err != nil {
		return nil, nil, err
	}
	manifestExt, err := b.addExtensions(p.Extensions)
	if err != nil {
		return nil, nil, err
	}
	if err := b.addRootFiles(p.Files); err != nil {
		return nil, nil, err
	}

	manifest := wireManifest{
		Schema:      PluginSchemaID(spec),
		Name:        p.Metadata.Name,
		Version:     p.Metadata.Version,
		Description: p.Metadata.Description,
		Homepage:    p.Metadata.Homepage,
		Repository:  p.Metadata.Repository,
		License:     p.Metadata.License,
		Keywords:    p.Metadata.Keywords,
		Extensions:  manifestExt,
	}
	if a := p.Metadata.Author; a != nil && *a != (Author{}) {
		manifest.Author = a
	}
	if err := b.addJSON(manifestFile, manifest, set.plugin); err != nil {
		return nil, nil, err
	}
	if len(servers) > 0 {
		doc := wireMCP{Schema: MCPSchemaID(spec), MCPServers: servers}
		if err := b.addJSON(mcpFile, doc, set.mcp); err != nil {
			return nil, nil, err
		}
	}
	sortFindings(b.findings)
	return b.files, b.findings, nil
}

type builder struct {
	files    map[string][]byte
	findings []Finding
}

func (b *builder) report(code string, sev Severity, at, msg string) {
	b.findings = append(b.findings, Finding{Code: code, Severity: sev, Path: at, Message: msg})
}

func (b *builder) addSkills(skills []Skill) error {
	seen := map[string]bool{}
	for _, s := range skills {
		if seen[s.Name] {
			return oops.With("skill", s.Name).Errorf("duplicate skill %q", s.Name)
		}
		seen[s.Name] = true
		for name := range s.Files {
			if name == skillFile {
				return oops.With("skill", s.Name).Errorf("skill %q file %q duplicates SkillMD", s.Name, name)
			}
			if !validRelPath(name) {
				return oops.With("skill", s.Name).Errorf("skill %q file %q: invalid path", s.Name, name)
			}
		}
		at := skillsDir + "/" + s.Name + "/" + skillFile
		issues := checkSkill(s.Name, s.SkillMD)
		fatal := false
		for _, i := range issues {
			sev := SeverityInfo
			code := CodeSkillUnknownField
			if i.fatal {
				sev, code, fatal = SeverityError, CodeSkillInvalid, true
			}
			b.report(code, sev, at, i.msg)
		}
		if fatal {
			continue
		}
		b.files[at] = s.SkillMD
		for name, data := range s.Files {
			b.files[path.Join(skillsDir, s.Name, name)] = data
		}
	}
	return nil
}

func (b *builder) mapServers(servers []MCPServer) (map[string]wireServer, error) {
	out := map[string]wireServer{}
	seen := map[string]bool{}
	for _, s := range servers {
		if s.Name == "" {
			return nil, oops.Errorf("MCP server name is required")
		}
		if seen[s.Name] {
			return nil, oops.With("server", s.Name).Errorf("duplicate MCP server %q", s.Name)
		}
		seen[s.Name] = true
		ws, issues, ok := toWire(s)
		at := serverPath(s.Name)
		for _, i := range issues {
			b.report(i.code, i.sev, at, i.msg)
		}
		if ok {
			out[s.Name] = ws
		}
	}
	return out, nil
}

func serverPath(name string) string { return mcpFile + "#/mcpServers/" + escapePointer(name) }

// escapePointer escapes a JSON pointer reference token (RFC 6901).
func escapePointer(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "~", "~0"), "/", "~1")
}

func (b *builder) addExtensions(exts []Extension) (map[string]map[string]any, error) {
	manifest := map[string]map[string]any{}
	seen := map[string]bool{}
	for _, e := range exts {
		if !ValidNamespace(e.Namespace) {
			return nil, oops.With("namespace", e.Namespace).Errorf(
				"extension namespace %q is not a reverse-domain identifier (e.g. com.example.client)", e.Namespace)
		}
		if seen[e.Namespace] {
			return nil, oops.With("namespace", e.Namespace).Errorf("duplicate extension namespace %q", e.Namespace)
		}
		seen[e.Namespace] = true
		if e.Manifest != nil {
			if _, err := json.Marshal(e.Manifest); err != nil {
				return nil, oops.With("namespace", e.Namespace).Wrapf(err, "encode extension %s manifest data", e.Namespace)
			}
			manifest[e.Namespace] = e.Manifest
		}
		for name, data := range e.Files {
			if !validRelPath(name) {
				return nil, oops.With("namespace", e.Namespace).Errorf("extension %s file %q: invalid path", e.Namespace, name)
			}
			b.files[e.Namespace+"/"+name] = data
		}
	}
	if len(manifest) == 0 {
		return nil, nil
	}
	return manifest, nil
}

func (b *builder) addRootFiles(files map[string][]byte) error {
	for name, data := range files {
		if !validRelPath(name) {
			return oops.With("file", name).Errorf("file %q: invalid path", name)
		}
		first, _, nested := strings.Cut(name, "/")
		if reservedRoot(first) {
			return oops.With("file", name).Errorf("file %q is at a reserved component location", name)
		}
		if nested && ValidNamespace(first) {
			return oops.With("file", name).Errorf(
				"file %q is inside the extension namespace directory %s; add it to that extension's Files", name, first)
		}
		b.files[name] = data
	}
	return nil
}

// addJSON encodes v and checks it against its schema, so Build cannot emit a
// non-conformant document.
func (b *builder) addJSON(name string, v any, schema *jsonschema.Schema) error {
	data, err := encodeJSON(v)
	if err != nil {
		return oops.With("file", name).Wrap(err)
	}
	var doc any
	if err := json.Unmarshal(data, &doc); err != nil {
		return oops.With("file", name).Wrapf(err, "decode %s", name)
	}
	if errs := schemaErrors(schema, doc); len(errs) > 0 {
		return oops.With("file", name).Errorf("%s does not match the official schema: %s", name, strings.Join(errs, "; "))
	}
	b.files[name] = data
	return nil
}

// encodeJSON encodes v with two-space indentation, a trailing newline and no
// HTML escaping, so the bytes depend only on the data.
func encodeJSON(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return nil, oops.Wrapf(err, "encode json")
	}
	return buf.Bytes(), nil
}
