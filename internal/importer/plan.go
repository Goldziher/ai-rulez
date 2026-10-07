package importer

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// Kind is the ai-rulez content kind an input construct maps to.
type Kind string

// Content kinds an importer can produce.
const (
	KindRule    Kind = "rule"
	KindContext Kind = "context"
	KindSkill   Kind = "skill"
	KindAgent   Kind = "agent"
	KindCommand Kind = "command"
	KindCheck   Kind = "check"
)

// Status says what happened to one input construct.
type Status string

// Finding statuses of the lossiness report.
const (
	StatusMapped       Status = "mapped"
	StatusApproximated Status = "approximated"
	StatusDropped      Status = "dropped"
	StatusNeedsAction  Status = "needs-action"
	StatusUnsupported  Status = "unsupported"
)

// Report codes live in their own AR9F range, apart from the AR9E0-AR9E4
// external scanner codes of the lint family. One code per lossy status; mapped
// findings carry none. CodeInvalid is not a finding: it names an input file that
// cannot be parsed at all, and appears only in the error that stops the run.
const (
	CodeInvalid      = "AR9F0"
	CodeApproximated = "AR9F1"
	CodeDropped      = "AR9F2"
	CodeNeedsAction  = "AR9F3"
	CodeUnsupported  = "AR9F4"
	CodeBlockedScan  = "AR9F5"
)

// Finding is one line of the lossiness report.
type Finding struct {
	Status Status `json:"status"`
	Source string `json:"source"`
	Field  string `json:"field,omitempty"`
	Target string `json:"target,omitempty"`
	Reason string `json:"reason,omitempty"`
	Code   string `json:"code,omitempty"`
}

func newFinding(status Status, source, field, target, reason string) Finding {
	f := Finding{Status: status, Source: source, Field: field, Target: target, Reason: reason}
	switch status {
	case StatusApproximated:
		f.Code = CodeApproximated
	case StatusDropped:
		f.Code = CodeDropped
	case StatusNeedsAction:
		f.Code = CodeNeedsAction
	case StatusUnsupported:
		f.Code = CodeUnsupported
	case StatusMapped:
	}
	return f
}

// File is one resource file of an item, relative to the item directory.
type File struct {
	Path string
	Data []byte
	// Exec marks a file the source kept executable (a script); it is written
	// with the execute bits.
	Exec bool
}

// Item is one piece of content to write below the config directory.
type Item struct {
	Kind      Kind
	Name      string
	Sources   []string
	Main      []byte
	Resources []File
	// Local items are personal: they are written below local/ (the machine-local
	// tree, which git ignores) instead of the shared content directories.
	Local bool
	hash  string
}

// localDir is the machine-local content tree inside the config directory.
const localDir = "local"

// root is the directory the item's kind directories sit in.
func (it *Item) root() string {
	if it.Local {
		return localDir + "/"
	}
	return ""
}

// Rel returns the main file path relative to the config directory.
func (it *Item) Rel() string {
	switch it.Kind {
	case KindRule:
		return it.root() + "rules/" + it.Name + ".md"
	case KindContext:
		return it.root() + "context/" + it.Name + ".md"
	case KindAgent:
		return it.root() + "agents/" + it.Name + ".md"
	case KindCommand:
		return it.root() + "commands/" + it.Name + ".md"
	case KindCheck:
		return it.root() + "checks/" + it.Name + ".md"
	case KindSkill:
		return it.root() + "skills/" + it.Name + "/SKILL.md"
	}
	return it.Name
}

// placed puts a path relative to the config directory below a domain. Local
// content keeps its local/ prefix in front: local/domains/<d>/rules/x.md.
func placed(domain, rel string) string {
	if domain == "" {
		return rel
	}
	if rest, ok := strings.CutPrefix(rel, localDir+"/"); ok {
		return localDir + "/domains/" + domain + "/" + rest
	}
	return "domains/" + domain + "/" + rel
}

// Files returns every file of the item, relative to the config directory, in
// a stable order.
func (it *Item) Files() []File {
	out := []File{{Path: it.Rel(), Data: it.Main}}
	if it.Kind == KindSkill {
		res := append([]File(nil), it.Resources...)
		sort.Slice(res, func(i, j int) bool { return res[i].Path < res[j].Path })
		for _, r := range res {
			out = append(out, File{Path: it.root() + "skills/" + it.Name + "/" + r.Path, Data: r.Data, Exec: r.Exec})
		}
	}
	return out
}

func (it *Item) computeHash() string {
	h := sha256.New()
	h.Write(it.Main)
	res := append([]File(nil), it.Resources...)
	sort.Slice(res, func(i, j int) bool { return res[i].Path < res[j].Path })
	for _, r := range res {
		h.Write([]byte{0})
		h.Write([]byte(r.Path))
		h.Write([]byte{0})
		h.Write(r.Data)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// RawFile is a file an importer planned in its final place below the config
// directory, domain included: the OKF bridge decides its own layout.
type RawFile struct {
	Path    string
	Data    []byte
	Sources []string
	// Exec marks an executable file (see File.Exec).
	Exec bool
}

// Plan is what an importer wants to write. Building one reads only. An
// importer returns it as collected; Finalize (called by Convert) orders it and
// resolves duplicates.
type Plan struct {
	Items []Item
	// Pointers are source files that only point at another file ("@AGENTS.md"):
	// nothing of them is imported, and a later generate may replace them.
	Pointers        []string
	Presets         []string
	MCPServers      []config.MCPServer
	InstalledSkills []config.InstalledSkillConfig
	// Hooks and Permissions are read from tool files; convert writes them
	// disabled unless the caller opts in (see hooks.go).
	Hooks       []config.HookGroup
	Permissions config.Permissions
	// Remotes are git sources the input names but does not hold; they are read
	// only when the caller asks to fetch (see fetch.go).
	Remotes []Remote
	// Raw are files already placed below the config directory (the okf importer).
	Raw      []RawFile
	Findings []Finding
	// fetchedText is the text of fetched files that are referenced rather than
	// copied (installed skills), keyed by display name, for the scan.
	fetchedText map[string]string

	// presetDefaulted is set when no preset was inferred and claude was chosen.
	presetDefaulted bool
	// keepNames makes Finalize report a name collision instead of renaming; the
	// collisions found are listed in collisions.
	keepNames  bool
	collisions []string
}

func (p *Plan) add(f Finding) { p.Findings = append(p.Findings, f) }

// empty reports whether the plan carries nothing to write.
func (p *Plan) empty() bool {
	return len(p.Items) == 0 && len(p.Raw) == 0 && len(p.MCPServers) == 0 && len(p.InstalledSkills) == 0 &&
		len(p.Hooks) == 0 && p.Permissions.IsEmpty()
}

// Options tune a Plan call.
type Options struct {
	// SplitHeadings splits root instruction files into one context per H2.
	SplitHeadings bool
	// BestEffort lets an importer continue past an unrecognised format version.
	BestEffort bool
	// SkipSkills names skills another importer owns (installed from a lock).
	SkipSkills map[string]bool
	// KeepNames reports a name collision between imported items instead of
	// renaming one of them.
	KeepNames bool
	// Fetch lets the importers' remote sources be read over the network; Fetcher
	// replaces the default git fetcher (tests).
	Fetch   bool
	Fetcher Fetcher
	// NativePaths limits the native importer to these project paths.
	NativePaths []string
	// Domain is the domain the import goes into; only an importer that lays out
	// its own files (okf) needs it, the others leave it to convert.
	Domain string
}

// Format reads one foreign format. Plan must not write, run commands or use
// the network; every read goes through fsys, rooted at the source directory.
type Format interface {
	Name() string
	Description() string
	// Detect lists the input files the importer recognises, sorted.
	Detect(fsys fs.FS) []string
	Plan(fsys fs.FS, opt Options) (*Plan, error)
}

// Registry returns every format importer, sorted by name.
func Registry() []Format {
	return []Format{apmImporter{}, nativeImporter{}, okfImporter{}, rulesyncImporter{}, skillsLockImporter{}, tesslImporter{}}
}

// Lookup returns the importer with the given name.
func Lookup(name string) (Format, bool) {
	for _, imp := range Registry() {
		if imp.Name() == name {
			return imp, true
		}
	}
	return nil, false
}

func shortHash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])[:6]
}

// merge folds other into p.
func (p *Plan) merge(other *Plan) {
	p.Items = append(p.Items, other.Items...)
	p.Presets = append(p.Presets, other.Presets...)
	p.MCPServers = append(p.MCPServers, other.MCPServers...)
	p.InstalledSkills = append(p.InstalledSkills, other.InstalledSkills...)
	p.Hooks = append(p.Hooks, other.Hooks...)
	p.Remotes = append(p.Remotes, other.Remotes...)
	p.Pointers = append(p.Pointers, other.Pointers...)
	p.Raw = append(p.Raw, other.Raw...)
	for k, v := range other.fetchedText {
		if p.fetchedText == nil {
			p.fetchedText = map[string]string{}
		}
		p.fetchedText[k] = v
	}
	p.Permissions.Allow = append(p.Permissions.Allow, other.Permissions.Allow...)
	p.Permissions.Ask = append(p.Permissions.Ask, other.Permissions.Ask...)
	p.Permissions.Deny = append(p.Permissions.Deny, other.Permissions.Deny...)
	p.Findings = append(p.Findings, other.Findings...)
}

// Finalize makes the plan deterministic: it drops duplicates (same content
// found twice), gives colliding names a stable suffix, de-duplicates presets,
// MCP servers and installed skills, and adds one mapped finding per item.
func (p *Plan) Finalize() {
	for i := range p.Items {
		p.Items[i].hash = p.Items[i].computeHash()
	}
	sort.SliceStable(p.Items, func(i, j int) bool {
		a, b := &p.Items[i], &p.Items[j]
		if a.Local != b.Local {
			return !a.Local
		}
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		return strings.Join(a.Sources, ",") < strings.Join(b.Sources, ",")
	})

	var out []Item
	byContent := map[string]int{}
	used := map[string]bool{}
	firstSource := map[string]string{}
	for _, it := range p.Items {
		contentKey := it.root() + string(it.Kind) + "\x00" + it.hash
		if it.Kind != KindContext {
			contentKey = it.root() + string(it.Kind) + "\x00" + it.Name + "\x00" + it.hash
		}
		if idx, ok := byContent[contentKey]; ok {
			out[idx].Sources = append(out[idx].Sources, it.Sources...)
			continue
		}
		key := it.root() + string(it.Kind) + "\x00" + it.Name
		if used[key] {
			orig := it.Name
			src := ""
			if len(it.Sources) > 0 {
				src = it.Sources[0]
			}
			if p.keepNames {
				p.collisions = append(p.collisions, fmt.Sprintf("%s %q from %s collides with %s", it.Kind, orig, src, firstSource[key]))
				continue
			}
			it.Name = orig + "-" + shortHash(src)
			if it.Kind == KindSkill {
				it.Main = []byte(setFrontmatterName(string(it.Main), it.Name))
			}
			p.add(newFinding(StatusApproximated, src, "name", it.Rel(),
				"name "+orig+" is already used by different content; renamed with a stable suffix"))
			key = it.root() + string(it.Kind) + "\x00" + it.Name
		}
		used[key] = true
		if len(it.Sources) > 0 {
			firstSource[key] = it.Sources[0]
		}
		byContent[contentKey] = len(out)
		out = append(out, it)
	}
	p.Items = out

	for i := range p.Items {
		it := &p.Items[i]
		sort.Strings(it.Sources)
		it.Sources = dedupeStrings(it.Sources)
		reason := ""
		if len(it.Sources) > 1 {
			reason = "identical content also found in " + strings.Join(it.Sources[1:], ", ")
		}
		p.add(newFinding(StatusMapped, it.Sources[0], string(it.Kind), it.Rel(), reason))
	}

	sort.Strings(p.Presets)
	p.Presets = dedupeStrings(p.Presets)
	p.MCPServers = dedupeServers(p.MCPServers)
	p.InstalledSkills = dedupeInstalled(p.InstalledSkills)
	p.Hooks = mergeHookGroups(p.Hooks)
	dedupePermissions(&p.Permissions)

	sort.SliceStable(p.Findings, func(i, j int) bool {
		a, b := p.Findings[i], p.Findings[j]
		if a.Source != b.Source {
			return a.Source < b.Source
		}
		if a.Field != b.Field {
			return a.Field < b.Field
		}
		if a.Status != b.Status {
			return a.Status < b.Status
		}
		return a.Reason < b.Reason
	})
}

func dedupeStrings(in []string) []string {
	var out []string
	for i, s := range in {
		if i > 0 && in[i-1] == s {
			continue
		}
		out = append(out, s)
	}
	return out
}

func dedupeServers(in []config.MCPServer) []config.MCPServer {
	sort.SliceStable(in, func(i, j int) bool { return in[i].Name < in[j].Name })
	var out []config.MCPServer
	for _, s := range in {
		if len(out) > 0 && out[len(out)-1].Name == s.Name {
			continue
		}
		out = append(out, s)
	}
	return out
}

func dedupeInstalled(in []config.InstalledSkillConfig) []config.InstalledSkillConfig {
	sort.SliceStable(in, func(i, j int) bool { return in[i].Name < in[j].Name })
	var out []config.InstalledSkillConfig
	for _, s := range in {
		if len(out) > 0 && out[len(out)-1].Name == s.Name {
			continue
		}
		out = append(out, s)
	}
	return out
}

func baseNoExt(p string, suffixes ...string) string {
	b := path.Base(p)
	for _, s := range suffixes {
		if strings.HasSuffix(b, s) {
			return strings.TrimSuffix(b, s)
		}
	}
	return strings.TrimSuffix(b, path.Ext(b))
}
