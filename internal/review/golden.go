package review

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/samber/oops"
	"gopkg.in/yaml.v3"

	"github.com/Goldziher/ai-rulez/v5/internal/safefs"
)

// GoldenCase is one human-labelled case of a rubric's golden set.
type GoldenCase struct {
	ID   string
	Kind string
	// Item is the item under review; Siblings are the items it is compared with.
	Item     Item
	Siblings []Item
	// Labelers keeps each labeler's labels in file order; Adjudicated is the settled label.
	Labelers    []map[string]string
	Adjudicated map[string]string
	Probes      []string
}

// GoldenSet is a loaded golden set and the digest of everything it consists of.
type GoldenSet struct {
	Cases []GoldenCase
	// Digest covers the case files and every fixture they reference.
	Digest string
	// Dir is the directory the case paths are relative to.
	Dir string
}

// isNotExist reports a missing file or directory.
func isNotExist(err error) bool { return errors.Is(err, fs.ErrNotExist) || os.IsNotExist(err) }

func sortStrings(s []string) { sort.Strings(s) }

// goldenDir returns the directory holding *.golden.yaml: base/golden when it exists, else base.
func goldenDir(base string) string {
	if info, err := os.Lstat(filepath.Join(base, GoldenDir)); err == nil && info.IsDir() {
		return filepath.Join(base, GoldenDir)
	}
	return base
}

// LoadGolden reads the golden cases under base (a rubric directory, or any directory with
// golden/*.golden.yaml or *.golden.yaml in it). Paths in a case are relative to base. A case
// that does not lint against rb, or whose files cannot be read, is an error: a calibration
// against a half-loaded set would measure something else.
func LoadGolden(base string, rb *Rubric) (*GoldenSet, error) {
	gdir := goldenDir(base)
	entries, err := os.ReadDir(gdir)
	if err != nil {
		return nil, oops.Wrapf(err, "read golden directory %s", gdir)
	}
	set := &GoldenSet{Dir: base}
	var files []fileBytes
	seen := map[string]bool{}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".golden.yaml") {
			continue
		}
		path := filepath.Join(gdir, name)
		data, _, rerr := safefs.ReadRegularKeepMode(path)
		if rerr != nil {
			return nil, oops.Wrapf(rerr, "read golden case %s", path)
		}
		if problems := lintGoldenFile(data, rb); len(problems) > 0 {
			return nil, oops.Errorf("golden case %s is invalid: %s (run `ai-rulez rubric lint`)", name, problems[0])
		}
		var gf goldenFile
		if err := yaml.Unmarshal(data, &gf); err != nil {
			return nil, oops.Wrapf(err, "parse golden case %s", path)
		}
		if seen[gf.ID] {
			return nil, oops.Errorf("golden id %q is used twice", gf.ID)
		}
		seen[gf.ID] = true
		files = append(files, fileBytes{"case/" + name, data})
		gc := GoldenCase{ID: gf.ID, Kind: gf.Kind, Adjudicated: gf.Adjudicated, Probes: gf.Probes}
		for _, l := range gf.Labelers {
			gc.Labelers = append(gc.Labelers, l.Labels)
		}
		item, ifile, ierr := goldenItem(base, gf.Item.Path, gf.Kind)
		if ierr != nil {
			return nil, oops.Wrapf(ierr, "golden case %s", gf.ID)
		}
		files = append(files, fileBytes{"fixture/" + gf.Item.Path, ifile})
		gc.Item = item
		for _, sp := range gf.Siblings {
			sib, sfile, serr := goldenItem(base, sp, gf.Kind)
			if serr != nil {
				return nil, oops.Wrapf(serr, "golden case %s", gf.ID)
			}
			files = append(files, fileBytes{"fixture/" + sp, sfile})
			gc.Siblings = append(gc.Siblings, sib)
		}
		set.Cases = append(set.Cases, gc)
	}
	sort.Slice(set.Cases, func(i, j int) bool { return set.Cases[i].ID < set.Cases[j].ID })
	set.Digest = digestOf(dedupFiles(files))
	return set, nil
}

// dedupFiles drops repeated names (a fixture shared by several cases is hashed once).
func dedupFiles(files []fileBytes) []fileBytes {
	seen := map[string]bool{}
	var out []fileBytes
	for _, f := range files {
		if seen[f.name] {
			continue
		}
		seen[f.name] = true
		out = append(out, f)
	}
	return out
}

// goldenItem reads one fixture file as an item.
func goldenItem(base, rel, kind string) (Item, []byte, error) {
	if !safeRelPath(rel) {
		return Item{}, nil, oops.Errorf("path %q must be relative without ..", rel)
	}
	abs := filepath.Join(base, filepath.FromSlash(rel))
	data, _, err := safefs.ReadRegularKeepMode(abs)
	if err != nil {
		return Item{}, nil, oops.Wrapf(err, "read fixture %s", rel)
	}
	return ItemFromText(kind, rel, string(data)), data, nil
}

// ItemFromText builds an item from the text of its file; path names it (and gives a
// fallback name: the directory of a SKILL.md or the file name).
func ItemFromText(kind, path, text string) Item {
	fallback := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if strings.EqualFold(filepath.Base(path), "SKILL.md") || strings.EqualFold(filepath.Base(path), "COMMAND.md") {
		fallback = filepath.Base(filepath.Dir(path))
	}
	it := Item{ID: kind + ":" + fallback, Kind: kind, Name: fallback, Path: filepath.ToSlash(path), Owned: true}.WithText(text)
	it.ID = kind + ":" + it.Name
	return it
}
