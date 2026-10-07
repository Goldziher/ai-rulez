package llmstxt

import (
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// Site is the part of a zensical.toml (or mkdocs-style) project table that
// llms.txt needs.
type Site struct {
	Name        string
	Description string
	// URL is the site root without a trailing slash.
	URL string
	Nav []NavItem
}

// NavItem is a nav entry: a page (Path set) or a group (Children set).
type NavItem struct {
	Title    string
	Path     string
	Children []NavItem
}

// Nav groups and pages that go to the Optional section of the docs index, and
// pages left out of llms-full.txt because they are logs, not documentation.
var (
	optionalTitles = map[string]bool{"Proposals": true, "Changelog": true}
	fullExcluded   = map[string]bool{"Changelog": true}
)

// maxNote is the longest page description, in characters.
const maxNote = 200

// FullFileName is the file name of the concatenated docs.
const FullFileName = "llms-full.txt"

// ParseSite reads the site name, description, URL and nav from zensical.toml.
func ParseSite(data []byte) (Site, error) {
	var raw struct {
		Project struct {
			SiteName        string           `toml:"site_name"`
			SiteDescription string           `toml:"site_description"`
			SiteURL         string           `toml:"site_url"`
			Nav             []map[string]any `toml:"nav"`
		} `toml:"project"`
	}
	if err := toml.Unmarshal(data, &raw); err != nil {
		return Site{}, fmt.Errorf("parse site config: %w", err)
	}
	p := raw.Project
	if p.SiteName == "" || p.SiteURL == "" {
		return Site{}, fmt.Errorf("site config needs project.site_name and project.site_url")
	}
	nav, err := parseNav(p.Nav)
	if err != nil {
		return Site{}, err
	}
	return Site{Name: p.SiteName, Description: p.SiteDescription, URL: strings.TrimRight(p.SiteURL, "/"), Nav: nav}, nil
}

func parseNav(entries []map[string]any) ([]NavItem, error) {
	out := make([]NavItem, 0, len(entries))
	for _, entry := range entries {
		if len(entry) != 1 {
			return nil, fmt.Errorf("nav entry %v must have exactly one title", entry)
		}
		for title, value := range entry {
			switch v := value.(type) {
			case string:
				out = append(out, NavItem{Title: title, Path: v})
			case []any:
				children := make([]map[string]any, 0, len(v))
				for _, c := range v {
					m, ok := c.(map[string]any)
					if !ok {
						return nil, fmt.Errorf("nav group %q has an entry that is not a table", title)
					}
					children = append(children, m)
				}
				sub, err := parseNav(children)
				if err != nil {
					return nil, err
				}
				out = append(out, NavItem{Title: title, Children: sub})
			default:
				return nil, fmt.Errorf("nav entry %q has an unsupported value", title)
			}
		}
	}
	return out, nil
}

// PageURL is the published URL of a docs source path (directory URLs).
func (s Site) PageURL(source string) string {
	p := strings.TrimSuffix(path.Clean(source), ".md")
	if p == "index" {
		return s.URL + "/"
	}
	if strings.HasSuffix(p, "/index") {
		p = strings.TrimSuffix(p, "index")
		return s.URL + "/" + p
	}
	return s.URL + "/" + p + "/"
}

type docPage struct {
	title, source, url, description, body string
	optional, inFull                      bool
}

// BuildDocs renders /llms.txt and /llms-full.txt for a docs tree (docs is the
// docs directory, so nav paths are relative to its root). It fails on a nav
// entry whose source file is missing, so a dead nav entry cannot ship.
func BuildDocs(site Site, docs fs.FS) (index, full string, err error) {
	var pages []docPage
	var walk func(items []NavItem, optional bool) error
	walk = func(items []NavItem, optional bool) error {
		for _, it := range items {
			opt := optional || optionalTitles[it.Title]
			if len(it.Children) > 0 {
				if err := walk(it.Children, opt); err != nil {
					return err
				}
				continue
			}
			data, err := fs.ReadFile(docs, it.Path)
			if err != nil {
				return fmt.Errorf("nav page %q: %w", it.Path, err)
			}
			meta, body := splitFrontmatter(string(data))
			desc := meta["description"]
			if desc == "" {
				desc = firstSentence(body)
			}
			desc = clipWords(desc, maxNote)
			pages = append(pages, docPage{
				title: it.Title, source: it.Path, url: site.PageURL(it.Path), description: desc,
				body: dropLeadingH1(body), optional: opt, inFull: !fullExcluded[it.Title],
			})
		}
		return nil
	}
	if err := walk(site.Nav, false); err != nil {
		return "", "", err
	}

	doc := Doc{Title: site.Name, Summary: site.Description}
	// Top-level pages share the first section; each top-level group is its own.
	main := Section{Name: "Docs"}
	groups := []Section{}
	for _, it := range site.Nav {
		if optionalTitles[it.Title] {
			continue
		}
		if len(it.Children) == 0 {
			main.Links = append(main.Links, linkOf(pages, it.Path))
			continue
		}
		g := Section{Name: it.Title}
		collectLinks(&g, pages, it.Children)
		groups = append(groups, g)
	}
	doc.Sections = append(doc.Sections, main)
	doc.Sections = append(doc.Sections, groups...)
	opt := Section{Name: OptionalSection, Links: []Link{{
		Title: "Full documentation", URL: site.URL + "/" + FullFileName, Note: "Every page above concatenated into one file.",
	}}}
	for i := range pages {
		if pages[i].optional {
			opt.Links = append(opt.Links, Link{Title: pages[i].title, URL: pages[i].url, Note: pages[i].description})
		}
	}
	doc.Sections = append(doc.Sections, opt)

	var fullPages []Page
	for i := range pages {
		if pages[i].inFull {
			fullPages = append(fullPages, Page{Title: pages[i].title, Source: pages[i].url, Body: pages[i].body})
		}
	}
	return doc.Render(), RenderFull(site.Name, site.Description, fullPages), nil
}

func collectLinks(s *Section, pages []docPage, items []NavItem) {
	for _, it := range items {
		if len(it.Children) > 0 {
			collectLinks(s, pages, it.Children)
			continue
		}
		s.Links = append(s.Links, linkOf(pages, it.Path))
	}
}

func linkOf(pages []docPage, source string) Link {
	for i := range pages {
		if pages[i].source == source {
			return Link{Title: pages[i].title, URL: pages[i].url, Note: pages[i].description}
		}
	}
	return Link{}
}

// splitFrontmatter returns the flat string keys of a leading YAML block and the
// text after it.
func splitFrontmatter(src string) (meta map[string]string, body string) {
	src = strings.ReplaceAll(src, "\r\n", "\n")
	meta = map[string]string{}
	if !strings.HasPrefix(src, "---\n") {
		return meta, src
	}
	end := strings.Index(src[4:], "\n---\n")
	if end < 0 {
		return meta, src
	}
	for _, line := range strings.Split(src[4:4+end], "\n") {
		if k, v, ok := strings.Cut(line, ":"); ok && !strings.HasPrefix(line, " ") {
			meta[strings.TrimSpace(k)] = strings.Trim(strings.TrimSpace(v), `"'`)
		}
	}
	return meta, src[4+end+5:]
}

func dropLeadingH1(body string) string {
	trimmed := strings.TrimLeft(body, "\n")
	if strings.HasPrefix(trimmed, "# ") {
		if _, rest, ok := strings.Cut(trimmed, "\n"); ok {
			return rest
		}
		return ""
	}
	return body
}

var (
	mdLinkRe   = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
	mdMarkupRe = regexp.MustCompile("[`*_]+")
	sentenceRe = regexp.MustCompile(`^(.+?[.!?])(?:\s|$)`)
)

// firstSentence returns the first sentence of the first prose paragraph of a
// page: not a heading, list, table, quote, admonition, HTML or code, with
// links, emphasis and code ticks reduced to their text.
func firstSentence(body string) string {
	var para []string
	fenced := false
	flush := func() string {
		text := strings.Join(strings.Fields(strings.Join(para, " ")), " ")
		text = mdLinkRe.ReplaceAllString(text, "$1")
		text = strings.TrimSpace(mdMarkupRe.ReplaceAllString(text, ""))
		if m := sentenceRe.FindStringSubmatch(text); m != nil {
			return m[1]
		}
		return text
	}
	for _, line := range strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n") {
		t := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~"):
			fenced = !fenced
		case fenced:
		case t == "":
			if len(para) > 0 {
				return flush()
			}
		case strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t"): // indented: code or admonition body
			if len(para) > 0 {
				para = append(para, t)
			}
		case isNonProse(t):
			if len(para) > 0 {
				return flush()
			}
		default:
			para = append(para, t)
		}
	}
	if len(para) > 0 {
		return flush()
	}
	return ""
}

func isNonProse(t string) bool {
	for _, p := range []string{"#", ">", "|", "<", "- ", "* ", "+ ", "!!!", "???", "---", "=== ", "!["} {
		if strings.HasPrefix(t, p) {
			return true
		}
	}
	return len(t) > 2 && t[0] >= '0' && t[0] <= '9' && strings.Contains(t[:3], ".")
}

// clipWords cuts s to at most limit characters at a word boundary.
func clipWords(s string, limit int) string {
	r := []rune(s)
	if len(r) <= limit {
		return s
	}
	cut := string(r[:limit-3])
	if i := strings.LastIndex(cut, " "); i > 0 {
		cut = cut[:i]
	}
	return strings.TrimRight(cut, " ,;:") + "..."
}
