package agentssource

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/importer"
)

// utf8BOM is the byte order mark some editors write at the start of a file.
const utf8BOM = "\ufeff"

// ItemNames lists the context items the root AGENTS.md of the project at base
// yields, named as Read names them: one item, or one per H2 heading with split.
// taken holds the names of the files under .agents/context (an item never
// takes one of them). A missing or generated AGENTS.md yields none.
func ItemNames(base string, split bool, taken map[string]bool) []string {
	items := importer.ReadAgentsMD(os.DirFS(base), importer.Options{SplitHeadings: split}).Items
	return itemNames(items, copyOf(taken))
}

// RemoveItem deletes the AGENTS.md item called name from the file. With split
// the item is a section (its heading line through the line before the next
// heading, or the text before the first heading); without it the item is the
// whole file, which is the author's own and is left alone. Everything else in
// the file stays byte for byte (line endings, a leading BOM and the mode
// included). The file is replaced atomically, never through a symlink, and not
// at all when it changed while the section was being cut out.
func RemoveItem(base, name string, split bool, taken map[string]bool) error {
	return removeItem(base, name, split, taken, nil)
}

// removeItem is RemoveItem with a hook that runs after the new text is written
// to its temporary file and before it replaces AGENTS.md; tests use it to change
// the file at that moment.
func removeItem(base, name string, split bool, taken map[string]bool, beforeReplace func()) error {
	sec, err := openSection(base, name, split, taken, "removed")
	if err != nil {
		return err
	}
	lines := sec.lines
	kept := append(append([]string{}, lines[:sec.span.Start]...), lines[sec.span.End:]...)
	return replaceUnchanged(sec.path, sec.data, []byte(sec.bom+strings.Join(kept, "\n")), sec.mode, "removed", beforeReplace)
}

// ReadItem returns the text of the AGENTS.md item called name: the section
// (its heading line through the line before the next heading, or the text
// before the first heading) with split, the whole file without it.
func ReadItem(base, name string, split bool, taken map[string]bool) (string, error) {
	path := filepath.Join(base, config.AgentsMDFilename)
	if !slices.Contains(ItemNames(base, split, taken), name) {
		return "", oops.With("path", path).With("name", name).Errorf("%s has no item %q", config.AgentsMDFilename, name)
	}
	if !split {
		data, err := os.ReadFile(path) //nolint:gosec // the project's own AGENTS.md
		if err != nil {
			return "", oops.With("path", path).Wrapf(err, "read %s", config.AgentsMDFilename)
		}
		return string(data), nil
	}
	sec, err := openSection(base, name, split, taken, "read")
	if err != nil {
		return "", err
	}
	return sec.sectionText(), nil
}

// UpdateItem replaces the section of AGENTS.md called name with content, which
// is the whole new section as ReadItem returns it (heading line included).
// Everything else in the file stays byte for byte, and the write is atomic like
// RemoveItem's. Without split the item is the whole file, which is the author's
// own and is left alone. Content that would not parse back as that one section
// (a missing heading, another "## " heading) is refused before anything is written.
func UpdateItem(base, name, content string, split bool, taken map[string]bool) error {
	return updateItem(base, name, content, split, taken, nil)
}

func updateItem(base, name, content string, split bool, taken map[string]bool, beforeReplace func()) error {
	if strings.TrimSpace(content) == "" {
		return oops.With("name", name).
			Hint("Use remove context to delete a section.").
			Errorf("the new content of %s item %q is empty", config.AgentsMDFilename, name)
	}
	sec, err := openSection(base, name, split, taken, "updated")
	if err != nil {
		return err
	}
	eol := "\n"
	if strings.Contains(sec.text, "\r\n") {
		eol = "\r\n"
	}
	lines := sec.lines
	body := newSectionLines(content, eol, sec.span.End == len(lines), strings.HasSuffix(sec.text, "\n"))
	merged := append(append(append([]string{}, lines[:sec.span.Start]...), body...), lines[sec.span.End:]...)
	text := strings.Join(merged, "\n")
	spans, err := importer.H2Spans(text)
	if err != nil || len(spans) != len(sec.spans) || spans[sec.index].Header != sec.span.Header {
		return oops.With("path", sec.path).With("name", name).
			Hint("Send the whole section: its \"## \" heading line unchanged, then its text, and no other \"## \" heading.").
			Errorf("the new content would not stay one section of %s", config.AgentsMDFilename)
	}
	return replaceUnchanged(sec.path, sec.data, []byte(sec.bom+text), sec.mode, "updated", beforeReplace)
}

// newSectionLines splits the new content of a section into the file lines that
// replace it. A line gets its "\r" (for a CRLF file) only when a "\n" follows it,
// so the final line of a file without a trailing newline stays as the file had
// it. last says the section runs to the end of the file; endsWithNewline says the
// file ends with a newline, which the new last section keeps (or does not gain).
func newSectionLines(content, eol string, last, endsWithNewline bool) []string {
	text := strings.ReplaceAll(content, "\r\n", "\n")
	if last && !endsWithNewline {
		text = strings.TrimRight(text, "\n")
	} else {
		text = strings.TrimSuffix(text, "\n")
	}
	body := strings.Split(text, "\n")
	if last && endsWithNewline {
		body = append(body, "") // the empty element after the file's final newline
	}
	for i := range body {
		body[i] = strings.TrimSuffix(body[i], "\r")
		if eol == "\r\n" && (i < len(body)-1 || !last) {
			body[i] += "\r"
		}
	}
	return body
}

// section is one split AGENTS.md item located in the file as it is now.
type section struct {
	path  string
	mode  os.FileMode
	data  []byte // the file as read
	bom   string // a leading BOM, put back at the start of what is written
	text  string // data without the BOM
	lines []string
	spans []importer.H2Span
	span  importer.H2Span
	index int
}

// sectionText is the text of the section: its lines, each ended by the newline
// that followed it in the file.
func (s *section) sectionText() string {
	text := strings.Join(s.lines[s.span.Start:s.span.End], "\n")
	if s.span.End < len(s.lines) {
		text += "\n"
	}
	return text
}

// openSection locates the split item called name. verb ("removed", "updated",
// "read") words the refusal for a file that is one item; a symlinked AGENTS.md
// is never edited.
func openSection(base, name string, split bool, taken map[string]bool, verb string) (*section, error) {
	path := filepath.Join(base, config.AgentsMDFilename)
	if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return nil, oops.With("path", path).
			Errorf("%s is a symlink and is not edited; edit its target by hand", config.AgentsMDFilename)
	}
	names := ItemNames(base, split, taken)
	index := slices.Index(names, name)
	if index < 0 {
		return nil, oops.With("path", path).With("name", name).Errorf("%s has no item %q", config.AgentsMDFilename, name)
	}
	if !split {
		return nil, oops.
			With("path", path).
			Hint("AGENTS.md is your own file: edit it by hand, or set agents_md_split = true to address its sections one by one.").
			Errorf("%s is one item and is not %s here", config.AgentsMDFilename, verb)
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, oops.With("path", path).Wrapf(err, "read %s", config.AgentsMDFilename)
	}
	data, err := os.ReadFile(path) //nolint:gosec // the project's own AGENTS.md
	if err != nil {
		return nil, oops.With("path", path).Wrapf(err, "read %s", config.AgentsMDFilename)
	}
	text := string(data)
	bom := ""
	if strings.HasPrefix(text, utf8BOM) {
		bom, text = utf8BOM, strings.TrimPrefix(text, utf8BOM) // put back at the start of what remains
	}
	spans, err := importer.H2Spans(text)
	if err != nil {
		return nil, oops.With("path", path).Wrapf(err, "locate the sections of %s", config.AgentsMDFilename)
	}
	if len(spans) != len(names) {
		return nil, oops.With("path", path).Errorf("could not locate the sections of %s", config.AgentsMDFilename)
	}
	return &section{
		path: path, mode: info.Mode().Perm(), data: data, bom: bom, text: text,
		lines: strings.Split(text, "\n"), spans: spans, span: spans[index], index: index,
	}, nil
}

// replaceUnchanged writes out to a temporary file next to path and renames it
// over path, but only when path still holds before: a file that changed in the
// meantime is left alone. The mode of the original is kept. The check and the
// rename are not one atomic step, so an edit in that instant can still be lost.
func replaceUnchanged(path string, before, out []byte, mode os.FileMode, verb string, beforeReplace func()) (err error) {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+config.AgentsMDFilename+".*")
	if err != nil {
		return oops.With("path", path).Wrapf(err, "write %s", config.AgentsMDFilename)
	}
	defer func() {
		if err != nil {
			_ = os.Remove(tmp.Name()) //nolint:errcheck // best effort cleanup of our own temporary file
		}
	}()
	if _, err = tmp.Write(out); err != nil {
		_ = tmp.Close() //nolint:errcheck // the write error is the one reported
		return oops.With("path", path).Wrapf(err, "write %s", config.AgentsMDFilename)
	}
	if err = tmp.Close(); err != nil {
		return oops.With("path", path).Wrapf(err, "write %s", config.AgentsMDFilename)
	}
	if err = os.Chmod(tmp.Name(), mode); err != nil {
		return oops.With("path", path).Wrapf(err, "write %s", config.AgentsMDFilename)
	}
	if beforeReplace != nil {
		beforeReplace()
	}
	now, readErr := os.ReadFile(path) //nolint:gosec // the project's own AGENTS.md
	if readErr != nil || !bytes.Equal(now, before) {
		err = oops.With("path", path).
			Errorf("%s changed while the section was being %s; nothing was written, try again", config.AgentsMDFilename, verb)
		return err
	}
	if err = os.Rename(tmp.Name(), path); err != nil {
		return oops.With("path", path).Wrapf(err, "write %s", config.AgentsMDFilename)
	}
	return nil
}

func copyOf(m map[string]bool) map[string]bool {
	out := make(map[string]bool, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
