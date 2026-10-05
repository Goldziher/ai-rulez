package okf

import (
	"path"
	"regexp"
	"sort"
	"strings"
)

// Entry is one bullet of an index.md: `* [Title](target) - description`.
type Entry struct {
	Title       string
	Target      string
	Description string
	// Line is the 1-based line within the index body.
	Line int
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
// allows in an index (SPEC section 8).
func BuildIndexes(concepts []IndexInput, labels DirLabel) map[string][]byte {
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
		var b strings.Builder
		if dir == "" {
			b.WriteString("---\nokf_version: \"" + SpecVersion + "\"\n---\n\n")
		}
		own := byDir[dir]
		sort.Slice(own, func(i, j int) bool { return own[i].Path < own[j].Path })
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
		name := IndexFile
		if dir != "" {
			name = dir + "/" + IndexFile
		}
		out[name] = []byte(strings.TrimRight(b.String(), "\n") + "\n")
	}
	return out
}

func writeEntry(b *strings.Builder, title, target, desc string) {
	b.WriteString("* [" + escapeTitle(title) + "](" + target + ")")
	if desc = oneLine(desc); desc != "" {
		b.WriteString(" - " + desc)
	}
	b.WriteString("\n")
}

func escapeTitle(t string) string {
	t = oneLine(t)
	return strings.NewReplacer("[", "\\[", "]", "\\]").Replace(t)
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
