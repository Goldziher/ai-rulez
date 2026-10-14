package agentssource

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/importer"
)

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
// the file stays byte for byte.
func RemoveItem(base, name string, split bool, taken map[string]bool) error {
	path := filepath.Join(base, config.AgentsMDFilename)
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
	data, err := os.ReadFile(path) //nolint:gosec // the project's own AGENTS.md
	if err != nil {
		return oops.With("path", path).Wrapf(err, "read %s", config.AgentsMDFilename)
	}
	text := string(data)
	spans := importer.H2Spans(text)
	if len(spans) != len(names) {
		return oops.With("path", path).Errorf("could not locate the sections of %s", config.AgentsMDFilename)
	}
	lines := strings.Split(text, "\n")
	span := spans[index]
	kept := append(append([]string{}, lines[:span.Start]...), lines[span.End:]...)
	out := strings.Join(kept, "\n")
	if err := os.WriteFile(path, []byte(out), 0o644); err != nil { //nolint:gosec // the project's own AGENTS.md
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
