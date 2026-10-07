package okf

import (
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"

	"github.com/samber/oops"
)

// Limits that keep a hostile bundle from exhausting memory.
const (
	maxFiles    = 50000
	maxFileSize = 8 << 20
)

// Concept is one non-reserved markdown file.
type Concept struct {
	// Path is the slash-separated bundle-relative path, including .md.
	Path        string
	Frontmatter Frontmatter
	// Body is the markdown after the frontmatter block (the whole file when there is none).
	Body string
	// BodyOffset is the number of file lines before Body, for line numbers.
	BodyOffset int
}

// ID is the concept ID: the path without .md (SPEC section 2).
func (c *Concept) ID() string { return strings.TrimSuffix(c.Path, ".md") }

// Type is the trimmed `type` value, "" when missing or not a scalar.
func (c *Concept) Type() string { return c.Frontmatter.Scalar("type") }

// Title is the frontmatter title, or one derived from the file name
// (SPEC section 4.1 lets consumers do that).
func (c *Concept) Title() string {
	if t := c.Frontmatter.Scalar(keyTitle); t != "" {
		return t
	}
	return TitleFromPath(c.Path)
}

// Description is the frontmatter description.
func (c *Concept) Description() string { return oneLine(c.Frontmatter.Scalar("description")) }

// TitleFromPath derives a display title from a file name.
func TitleFromPath(p string) string {
	base := strings.TrimSuffix(path.Base(p), ".md")
	words := strings.FieldsFunc(base, func(r rune) bool { return r == '-' || r == '_' || r == ' ' })
	for i, w := range words {
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	if len(words) == 0 {
		return base
	}
	return strings.Join(words, " ")
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// IndexFileDoc is a parsed index.md (reserved, SPEC section 8).
type IndexFileDoc struct {
	Path        string
	Frontmatter Frontmatter
	Body        string
	BodyOffset  int
	Entries     []Entry
	// Style is StyleFrontmatter or StyleBody, or "" when the file lists nothing.
	Style string
}

// LogDoc is a log.md (reserved, SPEC section 9).
type LogDoc struct {
	Path string
	Body string
}

// Problem is a load-time finding about the file system rather than content.
type Problem struct {
	Path    string
	Message string
}

// Bundle is a loaded OKF bundle.
type Bundle struct {
	// Concepts by bundle-relative path.
	Concepts map[string]*Concept
	// Indexes by bundle-relative path.
	Indexes map[string]*IndexFileDoc
	// Logs by bundle-relative path.
	Logs map[string]*LogDoc
	// Files holds every regular file path, including non-markdown resources.
	Files map[string]bool
	// Dirs holds every directory path ("" is the root).
	Dirs map[string]bool
	// Problems are unsafe entries found while loading (symlinks, oversize files).
	Problems []Problem

	fsys fs.FS
}

// ReadFile returns the bytes of a regular file of the bundle, refusing files
// larger than the load limit.
func (b *Bundle) ReadFile(name string) ([]byte, error) {
	if !b.Files[name] {
		return nil, fs.ErrNotExist
	}
	info, err := fs.Stat(b.fsys, name)
	if err != nil {
		return nil, err
	}
	if info.Size() > maxFileSize {
		return nil, oops.Errorf("%s is larger than %d bytes", name, maxFileSize)
	}
	return fs.ReadFile(b.fsys, name)
}

// Mode returns the permission bits of a file of the bundle.
func (b *Bundle) Mode(name string) fs.FileMode {
	info, err := fs.Stat(b.fsys, name)
	if err != nil {
		return 0
	}
	return info.Mode().Perm()
}

// Load reads the bundle rooted at root. It never follows symlinks and skips
// .git. Unparseable files do not fail the load; they surface as findings.
func Load(root fs.FS) (*Bundle, error) {
	b := &Bundle{
		Concepts: map[string]*Concept{}, Indexes: map[string]*IndexFileDoc{},
		Logs: map[string]*LogDoc{}, Files: map[string]bool{}, Dirs: map[string]bool{"": true}, fsys: root,
	}
	count := 0
	err := fs.WalkDir(root, ".", func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if p == "." {
			return nil
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return fs.SkipDir
			}
			b.Dirs[p] = true
			return nil
		}
		count++
		if count > maxFiles {
			return oops.Errorf("bundle has more than %d files", maxFiles)
		}
		if d.Type()&fs.ModeSymlink != 0 {
			b.Problems = append(b.Problems, Problem{Path: p, Message: "symlinks are not followed and not allowed in a bundle"})
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		b.Files[p] = true
		if !strings.HasSuffix(p, ".md") {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.Size() > maxFileSize {
			b.Problems = append(b.Problems, Problem{Path: p, Message: fmt.Sprintf("file is larger than %d bytes and was skipped", maxFileSize)})
			return nil
		}
		data, err := fs.ReadFile(root, p)
		if err != nil {
			return err
		}
		b.add(p, data)
		return nil
	})
	if err != nil {
		return nil, oops.Wrapf(err, "read bundle")
	}
	return b, nil
}

func (b *Bundle) add(p string, data []byte) {
	fm, body := SplitFrontmatter(data)
	offset := max(strings.Count(string(data), "\n")-strings.Count(body, "\n"), 0)
	switch path.Base(p) {
	case IndexFile:
		idx := &IndexFileDoc{Path: p, Frontmatter: fm, Body: body, BodyOffset: offset, Entries: ParseEntries(body)}
		switch {
		case isFrontmatterIndex(fm):
			idx.Style = StyleFrontmatter
			idx.Entries = append(frontmatterEntries(fm), idx.Entries...)
		case len(idx.Entries) > 0:
			idx.Style = StyleBody
		}
		b.Indexes[p] = idx
	case LogFile:
		b.Logs[p] = &LogDoc{Path: p, Body: body}
	default:
		b.Concepts[p] = &Concept{Path: p, Frontmatter: fm, Body: body, BodyOffset: offset}
	}
}

// IndexStyle reports the index scheme of the bundle: the style of the root
// index.md, or of the first nested index that lists something. It is "" when no
// index lists anything.
func (b *Bundle) IndexStyle() string {
	if root, ok := b.Indexes[IndexFile]; ok && root.Style != "" {
		return root.Style
	}
	for _, p := range sortedMapKeys(b.Indexes) {
		if s := b.Indexes[p].Style; s != "" {
			return s
		}
	}
	return ""
}

// ConceptPaths returns the concept paths, sorted.
func (b *Bundle) ConceptPaths() []string {
	out := make([]string, 0, len(b.Concepts))
	for p := range b.Concepts {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}
