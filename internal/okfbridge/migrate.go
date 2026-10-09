package okfbridge

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
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
	// BackupDir, when set with Write, receives a copy of every existing file the
	// migration rewrites (same relative paths) before the first one is touched.
	// Empty: no backup is taken, except of an index.md holding prose that an
	// index refresh replaces (see writeIndexes).
	BackupDir string
	// Clock dates that index backup; nil is the wall clock.
	Clock ambient.Clock
}

// plannedWrite is a file the migration will write once the whole plan is known.
type plannedWrite struct {
	rel  string
	path string
	data []byte
	mode os.FileMode
}

// MigrateDir converts a configuration directory, in place, to an OKF bundle: each
// rule, context, skill, agent, command and check gains the `type`, `title` and
// `x-ai-rulez` frontmatter of an exported concept, with its current frontmatter
// moved under x-ai-rulez.metadata, and every directory gets its index.md. The
// body of a file is kept byte for byte and the loader maps the frontmatter back,
// so the generated output does not change. A file that already carries
// x-ai-rulez is left alone, which makes the migration idempotent.
//
// The migration is all-or-nothing in its planning: it refuses, before writing
// anything, a rule, context file or other item whose own name is reserved in OKF
// (index.md, log.md), because the generated listing would overwrite it. Symlinks
// are never followed or rewritten; they are reported as skipped. Files are written
// atomically, after the originals were copied to opts.BackupDir.
func MigrateDir(ctx context.Context, configDir string, opts MigrateOptions) ([]MigrateChange, error) {
	tree, err := config.ScanContentTreeContext(ctx, configDir)
	if err != nil {
		return nil, oops.Wrapf(err, "read %s", configDir)
	}
	res := &ExportResult{Counts: map[Kind]int{}}
	items := collectItems(tree, allKindSet(), configDir, res)

	var changes []MigrateChange
	var writes []plannedWrite
	var reserved []string
	var idx []okf.IndexInput
	used := map[string]string{}
	claim := func(p string) string { used[strings.ToLower(p)] = p; return p }
	for i := range items {
		it := items[i]
		change, in, w, err := migrateItem(configDir, &it, claim)
		if err != nil {
			return nil, err
		}
		if isReservedName(change.Path) {
			reserved = append(reserved, change.Path)
		}
		changes = append(changes, change)
		if w != nil {
			writes = append(writes, *w)
		}
		if len(in) == 0 {
			continue
		}
		// Supporting files stay plain files in a migrated tree: the generators
		// copy them as they are, so they are not listed and get no index.
		idx = append(idx, in[:1]...)
	}
	if len(reserved) > 0 {
		sort.Strings(reserved)
		return nil, oops.
			Hint("Rename them (for example to index-notes.md) and run the migration again; nothing was changed").
			Errorf("%s would be overwritten by the generated OKF listing: %s are reserved names in an OKF bundle", configDir, strings.Join(reserved, ", "))
	}
	idxChanges, idxWrites, err := planIndexes(configDir, idx, items)
	if err != nil {
		return nil, err
	}
	changes = append(changes, idxChanges...)
	changes = append(changes, symlinkChanges(configDir)...)
	if opts.Write {
		if err := applyWrites(configDir, append(writes, idxWrites...), opts.BackupDir); err != nil {
			return nil, err
		}
	}
	return changes, nil
}

// isReservedName reports whether the file name is one OKF reserves for generated listings.
func isReservedName(rel string) bool {
	switch strings.ToLower(filepath.Base(rel)) {
	case okf.IndexFile, okf.LogFile:
		return true
	}
	return false
}

// symlinkChanges reports every markdown file or directory in the content
// directories that is a symlink: the migration never follows or rewrites one.
func symlinkChanges(configDir string) []MigrateChange {
	var changes []MigrateChange
	dirs := []string{dirDomains}
	for _, k := range AllKinds {
		dirs = append(dirs, string(k))
	}
	for _, dir := range dirs {
		_ = filepath.WalkDir(filepath.Join(configDir, dir), func(p string, d os.DirEntry, err error) error { //nolint:errcheck // a missing directory has no symlinks
			if err != nil {
				return nil //nolint:nilerr // unreadable entries are reported by the content scan
			}
			if d.Type()&os.ModeSymlink == 0 {
				return nil
			}
			if info, statErr := os.Stat(p); !strings.HasSuffix(strings.ToLower(p), ".md") && (statErr != nil || !info.IsDir()) {
				return nil //nolint:nilerr // a dangling non-markdown link is not content
			}
			rel, relErr := filepath.Rel(configDir, p)
			if relErr != nil {
				return nil //nolint:nilerr // cannot happen below configDir
			}
			changes = append(changes, MigrateChange{
				Path: filepath.ToSlash(rel), Action: ActionSkipped,
				Detail: "symlink is not followed; migrate its target by hand",
			})
			return nil
		})
	}
	return changes
}

// applyWrites copies the existing originals to backupDir, then writes every file atomically.
func applyWrites(configDir string, writes []plannedWrite, backupDir string) error {
	if backupDir != "" {
		if err := backupOriginals(writes, backupDir); err != nil {
			return err
		}
	}
	for i := range writes {
		if err := os.MkdirAll(filepath.Dir(writes[i].path), 0o755); err != nil { //nolint:gosec // G301: the project's content directory
			return oops.Wrapf(err, "create %s", filepath.Dir(writes[i].path))
		}
		if err := gitutil.WriteFileAtomic(writes[i].path, writes[i].data, writes[i].mode); err != nil {
			return oops.With("config_dir", configDir, "backup", backupDir).Wrapf(err, "write %s", writes[i].path)
		}
	}
	return nil
}

// backupOriginals copies the existing file of every write to backupDir, under
// the same relative path; a write that creates a file has nothing to copy.
func backupOriginals(writes []plannedWrite, backupDir string) error {
	for i := range writes {
		orig, err := os.ReadFile(writes[i].path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return oops.Wrapf(err, "read %s for backup", writes[i].path)
		}
		dst := filepath.Join(backupDir, filepath.FromSlash(writes[i].rel))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil { //nolint:gosec // G301: a copy of the project's own files
			return oops.Wrapf(err, "create %s", filepath.Dir(dst))
		}
		if err := gitutil.WriteFileAtomic(dst, orig, writes[i].mode); err != nil {
			return oops.Wrapf(err, "back up %s", writes[i].rel)
		}
	}
	return nil
}

// okfBackupDir is where a writer of configDir copies the originals it is about
// to replace: <configDir>.bak-okf-<timestamp>, as `migrate okf` names it.
func okfBackupDir(configDir string, now time.Time) string {
	return configDir + ".bak-okf-" + now.Format("20060102-150405")
}

// writeIndexes renders and writes the index.md files of a bundle.
func writeIndexes(configDir string, idx []okf.IndexInput, items []sourceItem, opts MigrateOptions) ([]MigrateChange, error) {
	changes, writes, err := planIndexes(configDir, idx, items)
	if err != nil {
		return nil, err
	}
	if opts.Write {
		backupDir := opts.BackupDir
		if backupDir == "" {
			// A generated listing is rebuilt from the content and needs no copy,
			// but an index.md that holds prose is the user's: keep it.
			if prose := proseIndexes(writes); len(prose) > 0 {
				if err := backupOriginals(prose, okfBackupDir(configDir, opts.Clock.Now())); err != nil {
					return nil, err
				}
			}
		}
		if err := applyWrites(configDir, writes, backupDir); err != nil {
			return nil, err
		}
	}
	return changes, nil
}

// proseIndexes are the writes that would replace an index.md which is not a
// generated OKF listing.
func proseIndexes(writes []plannedWrite) []plannedWrite {
	var out []plannedWrite
	for i := range writes {
		data, err := os.ReadFile(writes[i].path)
		if err == nil && !config.IsOKFListing(data) {
			out = append(out, writes[i])
		}
	}
	return out
}

// planIndexes renders the index.md of every directory of the bundle and plans
// those that differ from what is on disk.
func planIndexes(configDir string, idx []okf.IndexInput, items []sourceItem) ([]MigrateChange, []plannedWrite, error) {
	indexes := okf.BuildIndexes(idx, dirLabels(items), okf.StyleBody)
	paths := make([]string, 0, len(indexes))
	for p := range indexes {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	var changes []MigrateChange
	var writes []plannedWrite
	for _, p := range paths {
		change, w, err := planIndex(configDir, p, indexes[p])
		if err != nil {
			return nil, nil, err
		}
		changes = append(changes, change)
		if w != nil {
			writes = append(writes, *w)
		}
	}
	return changes, writes, nil
}

func allKindSet() map[Kind]bool {
	set := map[Kind]bool{}
	for _, k := range AllKinds {
		set[k] = true
	}
	return set
}

func migrateItem(configDir string, it *sourceItem, claim func(string) string) (MigrateChange, []okf.IndexInput, *plannedWrite, error) {
	rel, err := filepath.Rel(configDir, it.cf.Path)
	if err != nil {
		return MigrateChange{}, nil, nil, oops.Wrapf(err, "locate %s", it.cf.Path)
	}
	change := MigrateChange{Path: filepath.ToSlash(rel), Action: ActionUnchanged}
	data, err := os.ReadFile(it.cf.Path)
	if err != nil {
		return MigrateChange{}, nil, nil, oops.Wrapf(err, "read %s", it.cf.Path)
	}
	raw, ok := splitRawFrontmatter(data)
	if !ok {
		change.Action, change.Detail = ActionSkipped, "frontmatter is not closed with ---"
		return change, nil, nil, nil
	}
	fm, _ := okf.SplitFrontmatter(data)
	if fm.Err != nil {
		change.Action, change.Detail = ActionSkipped, fm.Err.Error()
		return change, nil, nil, nil //nolint:nilerr // a malformed frontmatter is reported as a skipped change, not a failure
	}
	it.typ, it.title = fm.Scalar(keyType), fm.Scalar(keyTitle)
	it.keepName = true
	pieces, in, err := renderItem(*it, claim)
	if err != nil {
		return MigrateChange{}, nil, nil, err
	}
	if pieces[0].file.Path != change.Path {
		change.Detail = fmt.Sprintf("the bundle path would be %s", pieces[0].file.Path)
	}
	if fm.Lookup(okf.ExtensionKey) != nil {
		return change, in, nil, nil
	}
	var out []byte
	if raw.present {
		out = append(append([]byte(nil), pieces[0].head[:len(pieces[0].head)-1]...), data[raw.bodyAt:]...)
	} else {
		out = append(append([]byte(nil), pieces[0].head...), data...)
	}
	change.Action = ActionConverted
	mode := os.FileMode(0o644)
	if info, err := os.Stat(it.cf.Path); err == nil {
		mode = info.Mode().Perm()
	}
	return change, in, &plannedWrite{rel: change.Path, path: it.cf.Path, data: out, mode: mode}, nil
}

func planIndex(configDir, rel string, data []byte) (MigrateChange, *plannedWrite, error) {
	target := filepath.Join(configDir, filepath.FromSlash(rel))
	change := MigrateChange{Path: rel, Action: ActionUnchanged}
	if info, err := os.Lstat(target); err == nil && info.Mode()&os.ModeSymlink != 0 {
		change.Action, change.Detail = ActionSkipped, "symlink is not followed; migrate its target by hand"
		return change, nil, nil
	}
	existing, err := os.ReadFile(target)
	if err == nil && bytes.Equal(existing, data) {
		return change, nil, nil
	}
	if err != nil && !os.IsNotExist(err) {
		return MigrateChange{}, nil, oops.Wrapf(err, "read %s", target)
	}
	change.Action = ActionIndex
	return change, &plannedWrite{rel: rel, path: target, data: data, mode: 0o644}, nil
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
