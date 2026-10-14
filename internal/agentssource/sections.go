package agentssource

import (
	"bytes"
	"os"
	"path/filepath"
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
	path := filepath.Join(base, config.AgentsMDFilename)
	if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return oops.With("path", path).
			Errorf("%s is a symlink and is not edited; edit its target by hand", config.AgentsMDFilename)
	}
	names := ItemNames(base, split, taken)
	index := -1
	for i, n := range names {
		if n == name {
			index = i
		}
	}
	if index < 0 {
		return oops.With("path", path).With("name", name).Errorf("%s has no item %q", config.AgentsMDFilename, name)
	}
	if !split {
		return oops.
			With("path", path).
			Hint("AGENTS.md is your own file: edit or delete it by hand, or set agents_md_split = true to address its sections one by one.").
			Errorf("%s is one item and is not removed here", config.AgentsMDFilename)
	}
	info, err := os.Stat(path)
	if err != nil {
		return oops.With("path", path).Wrapf(err, "read %s", config.AgentsMDFilename)
	}
	data, err := os.ReadFile(path) //nolint:gosec // the project's own AGENTS.md
	if err != nil {
		return oops.With("path", path).Wrapf(err, "read %s", config.AgentsMDFilename)
	}
	text := string(data)
	bom := ""
	if strings.HasPrefix(text, utf8BOM) {
		bom, text = utf8BOM, strings.TrimPrefix(text, utf8BOM) // put back at the start of what remains
	}
	spans, err := importer.H2Spans(text)
	if err != nil {
		return oops.With("path", path).Wrapf(err, "locate the sections of %s", config.AgentsMDFilename)
	}
	if len(spans) != len(names) {
		return oops.With("path", path).Errorf("could not locate the sections of %s", config.AgentsMDFilename)
	}
	lines := strings.Split(text, "\n")
	span := spans[index]
	kept := append(append([]string{}, lines[:span.Start]...), lines[span.End:]...)
	out := bom + strings.Join(kept, "\n")
	return replaceUnchanged(path, data, []byte(out), info.Mode().Perm(), beforeReplace)
}

// replaceUnchanged writes out to a temporary file next to path and renames it
// over path, but only when path still holds before: a file that changed in the
// meantime is left alone. The mode of the original is kept. The check and the
// rename are not one atomic step, so an edit in that instant can still be lost.
func replaceUnchanged(path string, before, out []byte, mode os.FileMode, beforeReplace func()) (err error) {
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
			Errorf("%s changed while the section was being removed; nothing was written, try again", config.AgentsMDFilename)
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
