package okfbridge

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/okf"
)

// Migrate actions.
const (
	ActionConverted = "converted"
	ActionIndex     = "index"
	ActionUnchanged = "unchanged"
	ActionSkipped   = "skipped"
)

// MigrateChange is one file a migration rewrote, wrote or left alone.
type MigrateChange struct {
	// Path is relative to the configuration directory, slash separated.
	Path   string
	Action string
	Detail string
}

// Pending reports whether the change still has to be written.
func (c MigrateChange) Pending() bool {
	return c.Action == ActionConverted || c.Action == ActionIndex
}

// MigrateOptions configures MigrateDir.
type MigrateOptions struct {
	// Write applies the changes; false only reports them.
	Write bool
}

// MigrateDir converts a configuration directory, in place, to an OKF bundle: each
// rule, context, skill, agent, command and check gains the `type`, `title` and
// `x-ai-rulez` frontmatter of an exported concept, with its current frontmatter
// moved under x-ai-rulez.metadata, and every directory gets its index.md. The
// body of a file is kept byte for byte and the loader maps the frontmatter back,
// so the generated output does not change. A file that already carries
// x-ai-rulez is left alone, which makes the migration idempotent.
func MigrateDir(ctx context.Context, configDir string, opts MigrateOptions) ([]MigrateChange, error) {
	tree, err := config.ScanContentTreeContext(ctx, configDir)
	if err != nil {
		return nil, oops.Wrapf(err, "read %s", configDir)
	}
	res := &ExportResult{Counts: map[Kind]int{}}
	items := collectItems(tree, allKindSet(), configDir, res)

	var changes []MigrateChange
	var idx []okf.IndexInput
	used := map[string]string{}
	claim := func(p string) string { used[strings.ToLower(p)] = p; return p }
	for i := range items {
		it := items[i]
		change, in, err := migrateItem(configDir, &it, claim, opts)
		if err != nil {
			return nil, err
		}
		changes = append(changes, change)
		if len(in) == 0 {
			continue
		}
		// Supporting files stay plain files in a migrated tree: the generators
		// copy them as they are, so they are not listed and get no index.
		idx = append(idx, in[:1]...)
	}
	idxChanges, err := writeIndexes(configDir, idx, items, opts)
	if err != nil {
		return nil, err
	}
	return append(changes, idxChanges...), nil
}

// writeIndexes renders the index.md of every directory of the bundle and writes
// those that differ from what is on disk.
func writeIndexes(configDir string, idx []okf.IndexInput, items []sourceItem, opts MigrateOptions) ([]MigrateChange, error) {
	indexes := okf.BuildIndexes(idx, dirLabels(items), okf.StyleBody)
	paths := make([]string, 0, len(indexes))
	for p := range indexes {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	var changes []MigrateChange
	for _, p := range paths {
		change, err := migrateIndex(configDir, p, indexes[p], opts)
		if err != nil {
			return nil, err
		}
		changes = append(changes, change)
	}
	return changes, nil
}

func allKindSet() map[Kind]bool {
	set := map[Kind]bool{}
	for _, k := range AllKinds {
		set[k] = true
	}
	return set
}

func migrateItem(configDir string, it *sourceItem, claim func(string) string, opts MigrateOptions) (MigrateChange, []okf.IndexInput, error) {
	rel, err := filepath.Rel(configDir, it.cf.Path)
	if err != nil {
		return MigrateChange{}, nil, oops.Wrapf(err, "locate %s", it.cf.Path)
	}
	change := MigrateChange{Path: filepath.ToSlash(rel), Action: ActionUnchanged}
	data, err := os.ReadFile(it.cf.Path)
	if err != nil {
		return MigrateChange{}, nil, oops.Wrapf(err, "read %s", it.cf.Path)
	}
	raw, ok := splitRawFrontmatter(data)
	if !ok {
		change.Action, change.Detail = ActionSkipped, "frontmatter is not closed with ---"
		return change, nil, nil
	}
	fm, _ := okf.SplitFrontmatter(data)
	if fm.Err != nil {
		change.Action, change.Detail = ActionSkipped, fm.Err.Error()
		return change, nil, nil
	}
	it.typ, it.title = fm.Scalar(keyType), fm.Scalar(keyTitle)
	it.keepName = true
	id := itemID(it.kind, it.cf)
	pieces, in, err := renderItem(*it, claim)
	if err != nil {
		return MigrateChange{}, nil, err
	}
	if pieces[0].file.Path != change.Path {
		change.Detail = fmt.Sprintf("the bundle path would be %s", pieces[0].file.Path)
	}
	if fm.Lookup(okf.ExtensionKey) != nil {
		return change, in, nil
	}
	var out []byte
	if raw.present {
		out = append(append([]byte(nil), pieces[0].head[:len(pieces[0].head)-1]...), data[raw.bodyAt:]...)
	} else {
		out = append(append([]byte(nil), pieces[0].head...), data...)
	}
	change.Action = ActionConverted
	if opts.Write {
		if err := writeKeepingMode(it.cf.Path, out); err != nil {
			return MigrateChange{}, nil, oops.With("id", id).Wrapf(err, "write %s", it.cf.Path)
		}
	}
	return change, in, nil
}

func migrateIndex(configDir, rel string, data []byte, opts MigrateOptions) (MigrateChange, error) {
	target := filepath.Join(configDir, filepath.FromSlash(rel))
	change := MigrateChange{Path: rel, Action: ActionUnchanged}
	existing, err := os.ReadFile(target)
	if err == nil && bytes.Equal(existing, data) {
		return change, nil
	}
	if err != nil && !os.IsNotExist(err) {
		return MigrateChange{}, oops.Wrapf(err, "read %s", target)
	}
	change.Action = ActionIndex
	if opts.Write {
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return MigrateChange{}, oops.Wrapf(err, "create %s", filepath.Dir(target))
		}
		if err := os.WriteFile(target, data, 0o644); err != nil {
			return MigrateChange{}, oops.Wrapf(err, "write %s", target)
		}
	}
	return change, nil
}

func writeKeepingMode(p string, data []byte) error {
	mode := os.FileMode(0o644)
	if info, err := os.Stat(p); err == nil {
		mode = info.Mode().Perm()
	}
	return os.WriteFile(p, data, mode)
}

// rawFrontmatter locates a file's frontmatter block.
type rawFrontmatter struct {
	present bool
	// bodyAt is the offset of the first byte after the closing --- line.
	bodyAt int
}

// splitRawFrontmatter finds the closing --- line of a leading frontmatter block
// the way the loader does. ok is false when a block opens and never closes.
func splitRawFrontmatter(data []byte) (rawFrontmatter, bool) {
	if !bytes.HasPrefix(data, []byte("---\n")) && !bytes.HasPrefix(data, []byte("---\r\n")) {
		return rawFrontmatter{}, true
	}
	offset := bytes.IndexByte(data, '\n') + 1
	for offset < len(data) {
		end := bytes.IndexByte(data[offset:], '\n')
		line, next := data[offset:], len(data)
		if end >= 0 {
			line, next = data[offset:offset+end], offset+end+1
		}
		if strings.TrimSpace(string(line)) == "---" {
			return rawFrontmatter{present: true, bodyAt: next}, true
		}
		offset = next
	}
	return rawFrontmatter{}, false
}
