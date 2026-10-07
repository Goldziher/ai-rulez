package okf

import (
	"path"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Entry is one bullet of an index.md: `* [Title](target) - description`.
type Entry struct {
	Title       string
	Target      string
	Description string
	// Line is the 1-based line within the index body.
	Line int
	// FileLine is the 1-based line within the file for an entry read from the
	// frontmatter; zero for a body entry.
	FileLine int
}

// fileLine is the line of the entry within the whole file; bodyOffset is the
// number of file lines before the body.
func (e Entry) fileLine(bodyOffset int) int {
	if e.FileLine > 0 {
		return e.FileLine
	}
	return bodyOffset + e.Line
}

// frontmatterStyleKeys are the keys of a frontmatter-style index.
var frontmatterStyleKeys = map[string]bool{keyTitle: true, "version": true, "entries": true}

// isFrontmatterIndex reports whether an index frontmatter uses the
// frontmatter style (title, version or entries).
func isFrontmatterIndex(fm Frontmatter) bool {
	for _, k := range fm.Keys() {
		if frontmatterStyleKeys[k] {
			return true
		}
	}
	return false
}

// frontmatterEntries reads the `entries` list of a frontmatter-style index. An
// item is a mapping with title, path (or url, link, target) and description, or a
// plain string path.
func frontmatterEntries(fm Frontmatter) []Entry {
	list := fm.Lookup("entries")
	if list == nil || list.Kind != yaml.SequenceNode {
		return nil
	}
	var out []Entry
	for _, item := range list.Content {
		e := Entry{FileLine: item.Line + 1}
		switch item.Kind {
		case yaml.ScalarNode:
			e.Target = strings.TrimSpace(item.Value)
		case yaml.MappingNode:
			for i := 0; i+1 < len(item.Content); i += 2 {
				v := strings.TrimSpace(item.Content[i+1].Value)
				switch item.Content[i].Value {
				case keyTitle:
					e.Title = v
				case "path", "url", "link", "target":
					e.Target = v
				case "description":
					e.Description = oneLine(v)
				}
			}
		}
		if e.Target != "" {
			out = append(out, e)
		}
	}
	return out
}

var entryRe = regexp.MustCompile(`^\s*[*+-]\s+\[([^\]]*)\]\(([^)\s]+)(?:\s+"[^"]*")?\)\s*(?:[-\x{2013}\x{2014}:]\s*(.*))?$`)

// ParseEntries extracts the bullets of an index body. Lines that are not
// entries (headings, prose) are ignored, as the spec leaves them free-form.
func ParseEntries(body string) []Entry {
	var out []Entry
	inFence := false
	for i, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		m := entryRe.FindStringSubmatch(strings.TrimRight(line, "\r"))
		if m == nil {
			continue
		}
		out = append(out, Entry{Title: m[1], Target: m[2], Description: strings.TrimSpace(m[3]), Line: i + 1})
	}
	return out
}

// Index styles. OKF 0.2 keeps an index in the body and allows only okf_version
// in the root frontmatter (StyleBody, the default). Another description of the
// format puts title, version and entries in the frontmatter (StyleFrontmatter).
const (
	StyleBody        = "body"
	StyleFrontmatter = "frontmatter"
	// FrontmatterIndexVersion is the `version` written by StyleFrontmatter.
	FrontmatterIndexVersion = "0.1.0"
)

// ValidIndexStyle reports whether s is a style name; "" means the default.
func ValidIndexStyle(s string) bool {
	return s == "" || s == StyleBody || s == StyleFrontmatter
}

// IndexInput describes a concept for index generation.
type IndexInput struct {
	Path        string
	Title       string
	Description string
}

// DirLabel describes a subdirectory in its parent's index.
type DirLabel map[string]string

// BuildIndexes renders the index.md of the root and of every directory that
// holds a concept, deterministically: entries are sorted by path and the output
// has no timestamps. labels maps a bundle-relative directory to its one-line
// description. The root index carries okf_version, the only frontmatter the spec
// allows in an index (SPEC section 8). With StyleFrontmatter the same entries
// are written as frontmatter (title, version, entries) instead of a body.
func BuildIndexes(concepts []IndexInput, labels DirLabel, style string) map[string][]byte {
	byDir := map[string][]IndexInput{}
	subdirs := map[string]map[string]bool{}
	dirs := map[string]bool{"": true}
	for _, c := range concepts {
		dir := path.Dir(c.Path)
		if dir == "." {
			dir = ""
		}
		byDir[dir] = append(byDir[dir], c)
		for d := dir; d != ""; {
			dirs[d] = true
			parent := path.Dir(d)
			if parent == "." {
				parent = ""
			}
			if subdirs[parent] == nil {
				subdirs[parent] = map[string]bool{}
			}
			subdirs[parent][d] = true
			d = parent
		}
	}
	out := map[string][]byte{}
	for dir := range dirs {
		name := IndexFile
		if dir != "" {
			name = dir + "/" + IndexFile
		}
		own := byDir[dir]
		sort.Slice(own, func(i, j int) bool { return own[i].Path < own[j].Path })
		if style == StyleFrontmatter {
			out[name] = frontmatterIndex(dir, own, sortedKeys(subdirs[dir]), labels)
			continue
		}
		var b strings.Builder
		if dir == "" {
			b.WriteString("---\nokf_version: \"" + SpecVersion + "\"\n---\n\n")
		}
		if len(own) > 0 {
			b.WriteString("# Concepts\n\n")
			for _, c := range own {
				writeEntry(&b, c.Title, path.Base(c.Path), c.Description)
			}
			b.WriteString("\n")
		}
		if subs := sortedKeys(subdirs[dir]); len(subs) > 0 {
			b.WriteString("# Subdirectories\n\n")
			for _, s := range subs {
				writeEntry(&b, path.Base(s), path.Base(s)+"/"+IndexFile, labels[s])
			}
			b.WriteString("\n")
		}
		out[name] = []byte(strings.TrimRight(b.String(), "\n") + "\n")
	}
	return out
}

type frontmatterEntry struct {
	Title       string `yaml:"title"`
	Path        string `yaml:"path"`
	Description string `yaml:"description,omitempty"`
}

// frontmatterIndex renders one index.md in StyleFrontmatter: the concepts of the
// directory, then its subdirectories, all as `entries` of the frontmatter.
func frontmatterIndex(dir string, own []IndexInput, subs []string, labels DirLabel) []byte {
	entries := make([]frontmatterEntry, 0, len(own)+len(subs))
	for _, c := range own {
		entries = append(entries, frontmatterEntry{Title: oneLine(c.Title), Path: path.Base(c.Path), Description: oneLine(c.Description)})
	}
	for _, s := range subs {
		entries = append(entries, frontmatterEntry{Title: path.Base(s), Path: path.Base(s) + "/" + IndexFile, Description: oneLine(labels[s])})
	}
	title := "Index"
	var fields []Field
	if dir == "" {
		fields = append(fields, Field{keyOKFVersion, SpecVersion})
	} else {
		title = TitleFromPath(path.Base(dir) + ".md")
	}
	fields = append(fields, Field{keyTitle, title}, Field{"version", FrontmatterIndexVersion}, Field{"entries", entries})
	data, err := MarshalFrontmatter(fields)
	if err != nil {
		// Every value is a string or a slice of string structs; encoding cannot fail.
		return nil
	}
	return data
}

func writeEntry(b *strings.Builder, title, target, desc string) {
	b.WriteString("* [" + escapeTitle(title) + "](" + target + ")")
	if desc = escapeText(oneLine(desc)); desc != "" {
		b.WriteString(" - " + desc)
	}
	b.WriteString("\n")
}

func escapeTitle(t string) string {
	return escapeText(oneLine(t))
}

// escapeText backslash-escapes the characters that would make a title or a
// description render as a link or as HTML in the index listing.
func escapeText(t string) string {
	return strings.NewReplacer("[", "\\[", "]", "\\]", "<", "\\<", ">", "\\>").Replace(t)
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
