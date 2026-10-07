package okfbridge

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/Goldziher/ai-rulez/v5/internal/okf"
	"github.com/samber/oops"
	"gopkg.in/yaml.v3"
)

// Import outcomes of one target file.
const (
	StatusCreated     = "created"
	StatusUnchanged   = "unchanged"
	StatusOverwritten = "overwritten"
	StatusConflict    = "conflict"
)

// maxScanSize bounds the size of a resource that is scanned as text.
const maxScanSize = 1 << 20

// ImportOptions controls an import.
type ImportOptions struct {
	// ConfigDir is the .ai-rulez directory to write into.
	ConfigDir string
	// Into forces every concept into one kind (rules, context or skills); empty
	// chooses per concept.
	Into Kind
	// Domain places everything under domains/<Domain>/ when set.
	Domain string
	// Force overwrites files that exist and differ.
	Force bool
	// DryRun reports without writing.
	DryRun bool
	// Scan runs the security scan over the text about to be written, keyed by
	// target path. The bridge cannot import internal/lint (lint imports the
	// presets, which use this package), so the caller supplies it. nil skips the
	// scan, which only tests should do.
	Scan Scanner
}

// SecurityFinding is one finding of the pre-write scan.
type SecurityFinding struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	File     string `json:"file"`
	Line     int    `json:"line"`
	Message  string `json:"message"`
}

// SeverityError is the severity that refuses an import.
const SeverityError = "error"

// Scanner scans texts keyed by display name.
type Scanner func(texts map[string]string) []SecurityFinding

// Action is the fate of one target file.
type Action struct {
	Path   string `json:"path"`
	Kind   Kind   `json:"kind"`
	Status string `json:"status"`
	Source string `json:"source"`
}

// ImportResult summarizes an import.
type ImportResult struct {
	Actions []Action `json:"actions"`
	// Findings are OKF-level notes (AR9B1, AR9B9) about the bundle.
	Findings []okf.Finding `json:"findings,omitempty"`
	// Security are the AR0xx findings of the scan that ran before writing.
	Security []SecurityFinding `json:"security,omitempty"`
	// Skipped lists bundle files that were not imported, with the reason.
	Skipped []string `json:"skipped,omitempty"`
	// IndexStyle is the index.md scheme the bundle uses (okf.StyleBody or
	// okf.StyleFrontmatter), "" when no index lists anything. Both import alike.
	IndexStyle string `json:"index_style,omitempty"`
}

// Count returns the number of actions with a status.
func (r *ImportResult) Count(status string) int {
	n := 0
	for _, a := range r.Actions {
		if a.Status == status {
			n++
		}
	}
	return n
}

// SecurityError is returned when the scan found an error-level problem; nothing
// was written.
type SecurityError struct{ Findings []SecurityFinding }

func (e *SecurityError) Error() string {
	var errs []string
	for _, f := range e.Findings {
		if f.Severity == SeverityError {
			errs = append(errs, fmt.Sprintf("%s %s:%d %s", f.Code, f.File, f.Line, f.Message))
		}
	}
	return "refusing to import: the security scan found error-level problems:\n  " + strings.Join(errs, "\n  ")
}

type planned struct {
	rel    string
	data   []byte
	mode   fs.FileMode
	kind   Kind
	source string
}

type ownerDir struct {
	kind   Kind
	id     string
	domain string
}

// Import converts bundle b into .ai-rulez sources under opts.ConfigDir.
func Import(b *okf.Bundle, opts ImportOptions) (*ImportResult, error) {
	if opts.Domain != "" && !ValidID(opts.Domain) {
		return nil, oops.Errorf("invalid domain name %q", opts.Domain)
	}
	if opts.Into != "" && opts.Into != KindRule && opts.Into != KindContext && opts.Into != KindSkill {
		return nil, oops.Errorf("--into must be rules, context or skills, got %q", opts.Into)
	}
	res := &ImportResult{IndexStyle: b.IndexStyle()}
	p := &planner{opts: opts, res: res, taken: map[string]string{}, owners: map[string]ownerDir{}, targets: map[string]string{}}
	if err := p.rejectUnsafe(b); err != nil {
		return nil, err
	}
	for _, cp := range b.ConceptPaths() {
		p.concept(b.Concepts[cp])
	}
	p.files(b)
	p.finish()
	sort.Slice(p.out, func(i, j int) bool { return p.out[i].rel < p.out[j].rel })

	res.Security = scan(opts.Scan, p.out)
	for i := range res.Security {
		if res.Security[i].Severity == SeverityError {
			return res, &SecurityError{Findings: res.Security}
		}
	}
	if err := p.apply(); err != nil {
		return res, err
	}
	sort.Strings(res.Skipped)
	return res, nil
}

type planner struct {
	opts   ImportOptions
	res    *ImportResult
	out    []planned
	taken  map[string]string // rel path -> source, to dedupe ids
	owners map[string]ownerDir
	// targets maps a bundle path to the path (relative to the config dir) it is
	// imported to; links between imported files are rewritten through it.
	targets map[string]string
	pending []pendingBody
}

// rejectUnsafe refuses a bundle whose paths could collide (differ only in case)
// or escape it. Symlinks and oversize files are not followed or read: they are
// skipped with a warning and the rest of the bundle is imported.
func (p *planner) rejectUnsafe(b *okf.Bundle) error {
	skipped := map[string]bool{}
	for _, pr := range b.Problems {
		skipped[pr.Path] = true
		f := okf.NewFinding(okf.CodePathUnsafe, pr.Path, 0, "%s; skipped", pr.Message)
		f.Severity = okf.SeverityWarning
		p.res.Findings = append(p.res.Findings, f)
		p.skip(pr.Path, pr.Message)
	}
	for _, f := range b.Validate() {
		if f.Code == okf.CodePathUnsafe && f.Severity == okf.SeverityError && !skipped[f.Path] {
			return oops.Errorf("refusing to import a bundle with unsafe paths: %s %s", f.Path, f.Message)
		}
	}
	return nil
}

// scan runs the AR0xx security scan over everything about to be written.
func scan(scanner Scanner, files []planned) []SecurityFinding {
	if scanner == nil {
		return nil
	}
	texts := map[string]string{}
	for i := range files {
		if len(files[i].data) <= maxScanSize && utf8.Valid(files[i].data) {
			texts[files[i].rel] = string(files[i].data)
		}
	}
	return scanner(texts)
}

// pendingBody is a markdown file whose text is built once every target path is
// known, so its links can be rewritten.
type pendingBody struct {
	out      int // index into planner.out
	concept  *okf.Concept
	ext      extInfo
	kind     Kind
	id       string
	resource bool
}

type extInfo struct {
	kind, id, domain, pathKey, owner string
	metadata                         *yaml.Node
	present                          bool
}

func readExt(fm okf.Frontmatter) extInfo {
	n := fm.Lookup(okf.ExtensionKey)
	if n == nil || n.Kind != yaml.MappingNode {
		return extInfo{}
	}
	e := extInfo{present: true}
	for i := 0; i+1 < len(n.Content); i += 2 {
		k, v := n.Content[i].Value, n.Content[i+1]
		switch k {
		case keyKind:
			e.kind = strings.TrimSpace(v.Value)
		case "id":
			e.id = strings.TrimSpace(v.Value)
		case keyDomain:
			e.domain = strings.TrimSpace(v.Value)
		case "path":
			e.pathKey = strings.TrimSpace(v.Value)
		case "owner":
			e.owner = strings.TrimSpace(v.Value)
		case "metadata":
			if v.Kind == yaml.MappingNode {
				e.metadata = v
			}
		}
	}
	return e
}

func (p *planner) note(code, src string, format string, args ...any) {
	p.res.Findings = append(p.res.Findings, okf.NewFinding(code, src, 1, format, args...))
}

func (p *planner) skip(src, why string) {
	p.res.Skipped = append(p.res.Skipped, src+": "+why)
}

func (p *planner) domainDir(domain, src string) string {
	if p.opts.Domain != "" {
		domain = p.opts.Domain
	}
	if domain != "" && ValidID(domain) {
		return path.Join(dirDomains, domain)
	}
	if domain != "" {
		p.note(okf.CodeLossyMapping, src, "x-ai-rulez domain %q is not a safe name; imported at the project root", domain)
	}
	return ""
}

func (p *planner) concept(c *okf.Concept) {
	if c.Frontmatter.Err != nil {
		p.note(okf.CodeTypeInvalid, c.Path, "unparseable frontmatter, concept skipped: %v", c.Frontmatter.Err)
		p.skip(c.Path, "unparseable frontmatter")
		return
	}
	if c.Type() == "" {
		p.note(okf.CodeTypeInvalid, c.Path, "no `type`; imported as context")
	}
	ext := readExt(c.Frontmatter)
	if ext.kind == string(kindResource) {
		p.resource(c, ext)
		return
	}
	kind, ok := kindFromName(ext.kind)
	if ext.present && !ok && ext.kind != "" {
		p.note(okf.CodeLossyMapping, c.Path, "x-ai-rulez kind %q is unknown; mapped by type", ext.kind)
	}
	if !ok {
		kind = kindForType(c.Type())
	}
	if p.opts.Into != "" {
		kind = p.opts.Into
	}
	id := ext.id
	if !validImportID(id) {
		if id != "" {
			p.note(okf.CodeLossyMapping, c.Path, "x-ai-rulez id %q is not a safe name; derived from the path", id)
		}
		id = deriveID(c.Path)
	}
	dir := p.domainDir(ext.domain, c.Path)
	rel := p.unique(targetPath(path.Join(dir, string(kind)), kind, id, c.Path), c.Path)
	if kind == KindSkill || path.Base(rel) == fileCommand {
		p.owners[path.Dir(c.Path)] = ownerDir{kind: kind, id: path.Base(path.Dir(rel)), domain: dir}
	}
	p.targets[c.Path] = rel
	p.pending = append(p.pending, pendingBody{out: len(p.out), concept: c, ext: ext, kind: kind, id: id})
	p.out = append(p.out, planned{rel: rel, kind: kind, source: c.Path})
}

// finish renders every markdown file now that all target paths are known.
func (p *planner) finish() {
	for i := range p.pending {
		pb := &p.pending[i]
		out := &p.out[pb.out]
		body := p.rewriteLinks(pb.concept, out.rel)
		if pb.resource {
			out.data = []byte(body)
			continue
		}
		out.data = p.render(pb.concept, pb.ext, pb.kind, pb.id, body)
	}
}

// rewriteLinks points the links of concept c, imported to target, at the files
// the linked concepts were imported to. A link to something that was not
// imported is left as written and reported as AR9B9. Fenced code and inline code
// are not touched.
func (p *planner) rewriteLinks(c *okf.Concept, target string) string {
	srcDir := path.Dir(c.Path)
	targetDir := path.Dir(target)
	return okf.RewriteLinks(c.Body, func(dest string, line int) (string, bool) {
		_, suffix, internal := okf.SplitDest(dest)
		if !internal {
			return "", false
		}
		bundlePath, _, inside := okf.ResolveLink(srcDir, dest)
		if inside {
			if to, ok := p.targets[bundlePath]; ok {
				return okf.JoinDest(relPath(targetDir, to), suffix), true
			}
		}
		p.res.Findings = append(p.res.Findings, okf.NewFinding(okf.CodeLossyMapping, c.Path, c.BodyOffset+line,
			"link %s points at a file that was not imported; left unchanged", dest))
		return "", false
	})
}

// relPath is the slash-separated path from directory fromDir to file to, both
// relative to the same root.
func relPath(fromDir, to string) string {
	var from []string
	if fromDir != "." && fromDir != "" {
		from = strings.Split(fromDir, "/")
	}
	dst := strings.Split(to, "/")
	i := 0
	for i < len(from) && i < len(dst)-1 && from[i] == dst[i] {
		i++
	}
	parts := make([]string, 0, len(from)-i+len(dst)-i)
	for range from[i:] {
		parts = append(parts, "..")
	}
	return strings.Join(append(parts, dst[i:]...), "/")
}

// targetPath is where a concept of kind lands below base.
func targetPath(base string, kind Kind, id, source string) string {
	switch {
	case kind == KindSkill:
		return path.Join(base, id, fileSkill)
	case kind == KindCommand && path.Base(source) == fileCommand:
		return path.Join(base, id, fileCommand)
	}
	return path.Join(base, id+".md")
}

func deriveID(p string) string {
	base := strings.TrimSuffix(p, ".md")
	if b := path.Base(p); b == fileSkill || b == fileCommand {
		base = path.Dir(p)
	}
	return sanitizeID(strings.ReplaceAll(base, "/", "-"))
}

// unique makes a target path unique among the planned ones, in sorted order.
func (p *planner) unique(rel, source string) string {
	candidate := rel
	for n := 2; ; n++ {
		if _, dup := p.taken[strings.ToLower(candidate)]; !dup {
			p.taken[strings.ToLower(candidate)] = source
			return candidate
		}
		dir, file := path.Split(rel)
		if path.Base(path.Dir(rel)) != "" && (file == fileSkill || file == fileCommand) {
			candidate = fmt.Sprintf("%s-%d/%s", path.Dir(rel), n, file)
			continue
		}
		candidate = fmt.Sprintf("%s%s-%d.md", dir, strings.TrimSuffix(file, ".md"), n)
	}
}

// render builds the ai-rulez source file of a concept.
func (p *planner) render(c *okf.Concept, ext extInfo, kind Kind, id, body string) []byte {
	fields := renderFields(c, ext, kind, id)
	if len(fields) == 0 {
		return []byte(body)
	}
	head, err := okf.MarshalFrontmatter(fields)
	if err != nil {
		return []byte(body)
	}
	return append(append(head, '\n'), body...)
}

// renderFields collects the frontmatter fields of the source file of a concept.
func renderFields(c *okf.Concept, ext extInfo, kind Kind, id string) []okf.Field {
	var fields []okf.Field
	desc := c.Frontmatter.Lookup(keyDescription)
	if desc != nil && desc.Kind == yaml.ScalarNode && strings.TrimSpace(desc.Value) != "" {
		fields = append(fields, okf.Field{Key: keyDescription, Value: desc.Value})
	}
	hasName := false
	if ext.metadata != nil {
		for i := 0; i+1 < len(ext.metadata.Content); i += 2 {
			key := ext.metadata.Content[i].Value
			if key == keyDescription {
				continue
			}
			hasName = hasName || key == "name"
			fields = append(fields, okf.Field{Key: key, Value: ext.metadata.Content[i+1]})
		}
	}
	// A bundle an export wrote carries x-ai-rulez and already holds every key the
	// source had; adding name or description would make the next export differ.
	if kind == KindSkill && !ext.present {
		if !hasName {
			fields = append(fields, okf.Field{Key: "name", Value: id})
		}
		if desc == nil || strings.TrimSpace(desc.Value) == "" {
			fields = append([]okf.Field{{Key: keyDescription, Value: c.Title()}}, fields...)
		}
	}
	if memory := okfMemory(c, kind, id); memory != nil {
		fields = append(fields, okf.Field{Key: "okf", Value: memory})
	}
	return fields
}

// okfMemory collects the OKF keys that have no ai-rulez home, so an export can
// put them back. type and title are kept only when they differ from what an
// export would derive.
func okfMemory(c *okf.Concept, kind Kind, id string) *yaml.Node {
	if c.Frontmatter.Root == nil {
		return nil
	}
	mem := &yaml.Node{Kind: yaml.MappingNode, Tag: yamlMapTag}
	root := c.Frontmatter.Root
	for i := 0; i+1 < len(root.Content); i += 2 {
		k, v := root.Content[i].Value, root.Content[i+1]
		switch k {
		case keyDescription, okf.ExtensionKey:
			continue
		case keyType:
			if v.Kind == yaml.ScalarNode && (strings.TrimSpace(v.Value) == defaultType(kind) || strings.TrimSpace(v.Value) == "") {
				continue
			}
		case keyTitle:
			if v.Kind == yaml.ScalarNode && (strings.TrimSpace(v.Value) == okf.TitleFromPath(sanitizeID(id)+".md") || strings.TrimSpace(v.Value) == "") {
				continue
			}
		}
		mem.Content = append(mem.Content, root.Content[i], v)
	}
	if len(mem.Content) == 0 {
		return nil
	}
	return mem
}

func (p *planner) resource(c *okf.Concept, ext extInfo) {
	kind, _ := kindFromName(ext.owner)
	if kind != KindCommand {
		kind = KindSkill
	}
	rel := strings.TrimSpace(ext.pathKey)
	if !ValidID(ext.id) || okf.ValidatePath(rel) != nil || !resourceK[strings.SplitN(rel, "/", 2)[0]] || path.Clean(rel) != rel {
		p.note(okf.CodeLossyMapping, c.Path, "skill resource has an unsafe or missing id/path; skipped")
		p.skip(c.Path, "unsafe skill resource path")
		return
	}
	dir := p.domainDir(ext.domain, c.Path)
	target := path.Join(dir, string(kind), ext.id, rel)
	p.taken[strings.ToLower(target)] = c.Path
	p.targets[c.Path] = target
	p.pending = append(p.pending, pendingBody{out: len(p.out), concept: c, kind: kind, resource: true})
	p.out = append(p.out, planned{rel: target, kind: kind, source: c.Path})
}

// files imports the non-markdown resources that sit next to a skill or command.
func (p *planner) files(b *okf.Bundle) {
	for _, f := range sortedStrings(b.Files) {
		if strings.HasSuffix(f, ".md") {
			continue
		}
		owner, rel, ok := p.ownerOf(f)
		if !ok {
			p.skip(f, "not markdown and not a skill resource")
			continue
		}
		if !resourceK[strings.SplitN(rel, "/", 2)[0]] || okf.ValidatePath(rel) != nil {
			p.skip(f, "not in references/, scripts/ or assets/")
			continue
		}
		target := path.Join(owner.domain, string(owner.kind), owner.id, rel)
		if _, dup := p.taken[strings.ToLower(target)]; dup {
			continue
		}
		p.taken[strings.ToLower(target)] = f
		p.targets[f] = target
		p.out = append(p.out, planned{rel: target, kind: owner.kind, source: f, data: p.readFile(b, f), mode: b.Mode(f)})
	}
}

func (p *planner) ownerOf(f string) (ownerDir, string, bool) {
	for dir := path.Dir(f); ; dir = path.Dir(dir) {
		key := dir
		if dir == "." {
			key = ""
		}
		if o, ok := p.owners[key]; ok {
			rel := strings.TrimPrefix(strings.TrimPrefix(f, key), "/")
			return o, rel, true
		}
		if dir == "." || dir == "/" {
			return ownerDir{}, "", false
		}
	}
}

func (p *planner) readFile(b *okf.Bundle, name string) []byte {
	data, err := b.ReadFile(name)
	if err != nil {
		p.skip(name, err.Error())
		return nil
	}
	return data
}

// apply compares the plan with the disk and writes what is allowed.
func (p *planner) apply() error {
	var write []okf.File
	for _, f := range p.out {
		if err := okf.ValidatePath(f.rel); err != nil {
			return err
		}
		status := StatusCreated
		dest := filepath.Join(p.opts.ConfigDir, filepath.FromSlash(f.rel))
		switch info, err := os.Lstat(dest); {
		case errors.Is(err, fs.ErrNotExist):
		case err != nil:
			return oops.Wrapf(err, "inspect %s", f.rel)
		case info.Mode()&fs.ModeSymlink != 0 || !info.Mode().IsRegular():
			status = StatusConflict
		default:
			existing, err := os.ReadFile(dest)
			switch {
			case err != nil:
				return oops.Wrapf(err, "read %s", f.rel)
			case bytes.Equal(existing, f.data):
				status = StatusUnchanged
			case p.opts.Force:
				status = StatusOverwritten
			default:
				status = StatusConflict
			}
		}
		p.res.Actions = append(p.res.Actions, Action{Path: f.rel, Kind: f.kind, Status: status, Source: f.source})
		if status == StatusCreated || status == StatusOverwritten {
			write = append(write, okf.File{Path: f.rel, Data: f.data, Mode: f.mode})
		}
	}
	if p.opts.DryRun || len(write) == 0 {
		return nil
	}
	return okf.WriteFiles(p.opts.ConfigDir, write, false)
}
