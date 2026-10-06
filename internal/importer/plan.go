package importer

import (
	"crypto/sha256"
	"encoding/hex"
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
}

// Item is one piece of content to write below the config directory.
type Item struct {
	Kind      Kind
	Name      string
	Sources   []string
	Main      []byte
	Resources []File
	hash      string
}

// Rel returns the main file path relative to the config directory.
func (it *Item) Rel() string {
	switch it.Kind {
	case KindRule:
		return "rules/" + it.Name + ".md"
	case KindContext:
		return "context/" + it.Name + ".md"
	case KindAgent:
		return "agents/" + it.Name + ".md"
	case KindCommand:
		return "commands/" + it.Name + ".md"
	case KindCheck:
		return "checks/" + it.Name + ".md"
	case KindSkill:
		return "skills/" + it.Name + "/SKILL.md"
	}
	return it.Name
}

// Files returns every file of the item, relative to the config directory, in
// a stable order.
func (it *Item) Files() []File {
	out := []File{{Path: it.Rel(), Data: it.Main}}
	if it.Kind == KindSkill {
		res := append([]File(nil), it.Resources...)
		sort.Slice(res, func(i, j int) bool { return res[i].Path < res[j].Path })
		for _, r := range res {
			out = append(out, File{Path: "skills/" + it.Name + "/" + r.Path, Data: r.Data})
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

// Plan is what an importer wants to write. Building one reads only. An
// importer returns it as collected; Finalize (called by Convert) orders it and
// resolves duplicates.
type Plan struct {
	Items           []Item
	Presets         []string
	MCPServers      []config.MCPServer
	InstalledSkills []config.InstalledSkillConfig
	Findings        []Finding

	// presetDefaulted is set when no preset was inferred and claude was chosen.
	presetDefaulted bool
}

func (p *Plan) add(f Finding) { p.Findings = append(p.Findings, f) }

// Options tune a Plan call.
type Options struct {
	// SplitHeadings splits root instruction files into one context per H2.
	SplitHeadings bool
	// BestEffort lets an importer continue past an unrecognised format version.
	BestEffort bool
	// SkipSkills names skills another importer owns (installed from a lock).
	SkipSkills map[string]bool
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
	return []Format{nativeImporter{}, rulesyncImporter{}, skillsLockImporter{}}
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
	for _, it := range p.Items {
		contentKey := string(it.Kind) + "\x00" + it.hash
		if it.Kind != KindContext {
			contentKey = string(it.Kind) + "\x00" + it.Name + "\x00" + it.hash
		}
		if idx, ok := byContent[contentKey]; ok {
			out[idx].Sources = append(out[idx].Sources, it.Sources...)
			continue
		}
		key := string(it.Kind) + "\x00" + it.Name
		if used[key] {
			orig := it.Name
			src := ""
			if len(it.Sources) > 0 {
				src = it.Sources[0]
			}
			it.Name = orig + "-" + shortHash(src)
			if it.Kind == KindSkill {
				it.Main = []byte(setFrontmatterName(string(it.Main), it.Name))
			}
			p.add(newFinding(StatusApproximated, src, "name", it.Rel(),
				"name "+orig+" is already used by different content; renamed with a stable suffix"))
			key = string(it.Kind) + "\x00" + it.Name
		}
		used[key] = true
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
