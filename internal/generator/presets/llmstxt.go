package presets

import (
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/rulefiles"
	"github.com/Goldziher/ai-rulez/v5/internal/llmstxt"
)

// llms-txt preset: renders the content tree as llms.txt (https://llmstxt.org/),
// an index of the project's rules, context and skills with one line per item,
// and optionally llms-full.txt with the full text. Both files are committed
// documentation written verbatim: a generated-by banner would sit in front of
// the H1 that the format requires first. See docs/llms-txt.md.

const (
	// LLMsTxtFileName is the index file.
	LLMsTxtFileName = "llms.txt"
	// LLMsTxtFullFileName is the expanded file.
	LLMsTxtFullFileName = "llms-full.txt"

	llmsTxtMaxNote = 200
)

// LLMsTxtPresetGenerator writes llms.txt and llms-full.txt.
type LLMsTxtPresetGenerator struct{}

func (g *LLMsTxtPresetGenerator) GetName() string { return config.PresetLLMsTxt }

func (g *LLMsTxtPresetGenerator) GetOutputPaths(baseDir string) []string {
	return []string{filepath.Join(baseDir, LLMsTxtFileName)}
}

func (g *LLMsTxtPresetGenerator) Generate(content *config.ContentTree, baseDir string, cfg *config.Config) ([]config.OutputFile, error) {
	// A monorepo scope run writes scoped instruction files only, and a role
	// renders one person's slice; the index documents the whole project.
	if rulefiles.InScope(cfg) || cfg.RoleActive() {
		return nil, nil
	}
	settings := cfg.LLMsTxtSettings()
	items := llmsTxtItems(content, settings.Include, baseDir, cfg.LLMsTxtDir())

	title := firstNonEmpty(settings.Title, cfg.Name, "Project")
	summary := firstNonEmpty(settings.Summary, cfg.Description)

	doc := llmstxt.Doc{Title: title, Summary: summary}
	for _, kind := range llmsTxtKindOrder {
		section := llmstxt.Section{Name: kind.heading}
		for i := range items {
			if items[i].kind == kind.name {
				section.Links = append(section.Links, items[i].link)
			}
		}
		doc.Sections = append(doc.Sections, section)
	}

	dir := filepath.Join(baseDir, filepath.FromSlash(cfg.LLMsTxtDir()))
	outputs := []config.OutputFile{{
		Path:       filepath.Join(dir, LLMsTxtFileName),
		RawContent: []byte(doc.Render()),
		Mode:       0o644,
		Committed:  true,
	}}
	if settings.Full {
		pages := make([]llmstxt.Page, 0, len(items))
		for i := range items {
			pages = append(pages, items[i].page)
		}
		outputs = append(outputs, config.OutputFile{
			Path:       filepath.Join(dir, LLMsTxtFullFileName),
			RawContent: []byte(llmstxt.RenderFull(title, summary, pages)),
			Mode:       0o644,
			Committed:  true,
		})
	}
	return outputs, nil
}

// llmsTxtKindOrder is the order of the sections; agents and commands are the
// secondary content and share the Optional section.
var llmsTxtKindOrder = []struct{ name, heading string }{
	{"rules", "Rules"},
	{"context", "Context"},
	{"skills", "Skills"},
	{"agents", llmstxt.OptionalSection},
	{"commands", llmstxt.OptionalSection},
}

type llmsTxtItem struct {
	kind string
	link llmstxt.Link
	page llmstxt.Page
}

// llmsTxtItems lists the selected items in a stable order: by kind, then root
// content before domains (by name), then by name.
func llmsTxtItems(content *config.ContentTree, include []string, baseDir, outDir string) []llmsTxtItem {
	if content == nil {
		return nil
	}
	want := map[string]bool{}
	for _, k := range include {
		want[strings.ToLower(strings.TrimSpace(k))] = true
	}
	if len(want) == 0 {
		want["rules"], want["context"], want["skills"] = true, true, true
	}
	type source struct {
		domain string
		files  map[string][]config.ContentFile
	}
	sources := []source{{files: map[string][]config.ContentFile{
		"rules": content.Rules, "context": content.Context, "skills": content.Skills,
		"agents": content.Agents, "commands": content.Commands,
	}}}
	names := make([]string, 0, len(content.Domains))
	for name := range content.Domains {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if d := content.Domains[name]; d != nil {
			sources = append(sources, source{domain: name, files: map[string][]config.ContentFile{
				"rules": d.Rules, "context": d.Context, "skills": d.Skills, "agents": d.Agents, "commands": d.Commands,
			}})
		}
	}

	var out []llmsTxtItem
	for _, kind := range llmsTxtKindOrder {
		if !want[kind.name] {
			continue
		}
		for _, src := range sources {
			files := append([]config.ContentFile(nil), src.files[kind.name]...)
			sort.SliceStable(files, func(i, j int) bool { return files[i].Name < files[j].Name })
			for i := range files {
				out = append(out, newLLMsTxtItem(kind.name, src.domain, &files[i], baseDir, outDir))
			}
		}
	}
	return out
}

func newLLMsTxtItem(kind, domain string, f *config.ContentFile, baseDir, outDir string) llmsTxtItem {
	title := firstNonEmpty(markdownTitle(f.Content), f.Name)
	note := ""
	if f.Metadata != nil {
		note = strings.TrimSpace(f.Metadata.Extra["description"])
	}
	if note == "" {
		note = firstParagraphLine(f.Content)
	}
	note = clipRunes(strings.Join(strings.Fields(note), " "), llmsTxtMaxNote)
	if domain != "" {
		note = strings.TrimSpace("Domain " + domain + ". " + note)
	}
	source := llmsTxtRelative(baseDir, f.Path)
	return llmsTxtItem{
		kind: kind,
		link: llmstxt.Link{Title: title, URL: relativeTo(outDir, source), Note: note},
		page: llmstxt.Page{Title: title, Source: source, Body: f.Content},
	}
}

// llmsTxtRelative returns p relative to the project root, with forward slashes.
func llmsTxtRelative(baseDir, p string) string {
	if filepath.IsAbs(p) {
		if rel, err := filepath.Rel(baseDir, p); err == nil {
			p = rel
		}
	}
	return filepath.ToSlash(p)
}

// relativeTo expresses the project-relative path target from the directory dir.
func relativeTo(dir, target string) string {
	if dir == "." || dir == "" {
		return target
	}
	if rel, err := filepath.Rel(filepath.FromSlash(dir), filepath.FromSlash(target)); err == nil {
		return filepath.ToSlash(rel)
	}
	return path.Clean(target)
}

// markdownTitle returns the text of the first H1 outside fenced code.
func markdownTitle(body string) string {
	fenced := false
	for _, line := range strings.Split(body, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~") {
			fenced = !fenced
			continue
		}
		if !fenced && strings.HasPrefix(t, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(t, "# "))
		}
	}
	return ""
}

// firstParagraphLine returns the first plain prose line of body: not a
// heading, list marker, quote, fence, table or html line.
func firstParagraphLine(body string) string {
	fenced := false
	for _, line := range strings.Split(body, "\n") {
		t := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~"):
			fenced = !fenced
		case fenced, t == "":
		case strings.HasPrefix(t, "#"), strings.HasPrefix(t, ">"), strings.HasPrefix(t, "|"), strings.HasPrefix(t, "<"),
			strings.HasPrefix(t, "- "), strings.HasPrefix(t, "* "), strings.HasPrefix(t, "+ "), strings.HasPrefix(t, "---"):
		default:
			return t
		}
	}
	return ""
}

func clipRunes(s string, limit int) string {
	if r := []rune(s); len(r) > limit {
		return strings.TrimSpace(string(r[:limit-3])) + "..."
	}
	return s
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
