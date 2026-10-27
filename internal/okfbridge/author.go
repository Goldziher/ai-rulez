package okfbridge

import (
	"context"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/okf"
)

// RenderConcept returns data, a content file in the native layout (optional
// frontmatter and a body), as the OKF concept the source writers store: `type`,
// `title` and `x-ai-rulez` frontmatter with the former frontmatter under
// x-ai-rulez.metadata. It is what `migrate okf` and `export okf` produce, so a
// file written here is already in its migrated form. A file that carries
// x-ai-rulez is returned unchanged. domain is "" for root content.
func RenderConcept(kind Kind, domain, id string, data []byte) ([]byte, error) {
	return RenderConceptKeeping(kind, domain, id, data, nil)
}

// RenderConceptKeeping is RenderConcept for a rewrite of an existing file: the
// `type` and `title` of previous, the file's former content, are kept unless
// data declares its own. When data is a bare body without frontmatter, all of
// previous's metadata (priority, targets, description, other keys) is kept too.
func RenderConceptKeeping(kind Kind, domain, id string, data, previous []byte) ([]byte, error) {
	if fm, _ := okf.SplitFrontmatter(data); fm.Err == nil && fm.Lookup(okf.ExtensionKey) != nil {
		return data, nil
	}
	md, body, malformed := config.ParseFrontmatterChecked(string(data))
	if malformed {
		return nil, oops.Hint("Fix the YAML between the --- lines.").Errorf("the frontmatter of %s %q is not valid YAML", kind, id)
	}
	prev, _, _ := config.ParseFrontmatterChecked(string(previous))
	md = keepingMetadata(kind, id, md, prev)
	it := sourceItem{kind: kind, domain: domain, keepName: true, cf: config.ContentFile{Name: id, Content: body, Metadata: md}}
	applyKeepingIdentity(&it, md, prev)
	fields, _, err := conceptFields(it, id)
	if err != nil {
		return nil, err
	}
	head, err := okf.MarshalFrontmatter(fields)
	if err != nil {
		return nil, err
	}
	return append(append(head, '\n'), body...), nil
}

// keepingMetadata resolves the metadata a rewrite keeps: a bare body (no
// frontmatter of its own) inherits everything the previous file declared, and a
// skill's name is filled in from its id.
func keepingMetadata(kind Kind, id string, md, prev *config.Metadata) *config.Metadata {
	if md == nil && prev != nil {
		// data is a bare body: the rewrite keeps everything the file declared.
		md = prev
	}
	if kind == KindSkill {
		if md == nil {
			md = &config.Metadata{}
		}
		if md.Extra == nil {
			md.Extra = map[string]string{}
		}
		if md.Extra[keyName] == "" {
			md.Extra[keyName] = id
		}
	}
	return md
}

// applyKeepingIdentity fills it's type and title, preferring what metadata
// declares and falling back to the previous file's values.
func applyKeepingIdentity(it *sourceItem, md, prev *config.Metadata) {
	if md != nil {
		it.typ, it.title = md.OKFType, md.OKFTitle
	}
	if prev != nil {
		if it.typ == "" {
			it.typ = prev.OKFType
		}
		if it.title == "" {
			it.title = prev.OKFTitle
		}
	}
}

// RefreshIndexes rewrites the index.md files of a configuration directory so they
// list its current content, and removes generated indexes of directories that
// no longer hold any. Concept files are not touched. An index.md that holds
// prose rather than a generated listing is copied to <configDir>.bak-okf-<time>
// before it is overwritten. It is what the source writers (add, remove, init,
// the MCP tools) call after a change.
func RefreshIndexes(ctx context.Context, configDir string) error {
	return RefreshIndexesFrom(ctx, configDir, nil)
}

// RefreshIndexesFrom is RefreshIndexes for a caller that has already loaded the
// content: tree is the project's loaded content tree, from which the items read
// from configDir are listed (items merged in from includes and builtins are left
// out), so the tree is not scanned and parsed a second time. A nil tree is scanned.
func RefreshIndexesFrom(ctx context.Context, configDir string, tree *config.ContentTree) error {
	idx, items, err := indexInputs(ctx, configDir, tree)
	if err != nil {
		return err
	}
	if _, err := writeIndexes(configDir, idx, items, MigrateOptions{Write: true}); err != nil {
		return err
	}
	return removeStaleIndexes(configDir, idx)
}

// IsBundleDir reports whether configDir is an OKF bundle that ai-rulez keeps
// indexed: its root index.md is a generated listing. A directory without one, or
// whose root index.md holds prose, is left to "migrate okf".
func IsBundleDir(configDir string) bool {
	data, err := os.ReadFile(filepath.Join(configDir, okf.IndexFile)) //nolint:gosec // G304: the project's own configuration directory
	return err == nil && config.IsOKFListing(data)
}

// StaleIndexes lists the index.md files (slash paths relative to configDir) that
// RefreshIndexes would rewrite: a listed title or description no longer matches
// its concept, or an entry is missing. It is the index check "migrate okf
// --check" runs, without writing anything.
func StaleIndexes(ctx context.Context, configDir string) ([]string, error) {
	return StaleIndexesFrom(ctx, configDir, nil)
}

// StaleIndexesFrom is StaleIndexes over an already loaded content tree, see
// RefreshIndexesFrom. The index.md files themselves are still read from disk:
// they are not content items.
func StaleIndexesFrom(ctx context.Context, configDir string, tree *config.ContentTree) ([]string, error) {
	idx, items, err := indexInputs(ctx, configDir, tree)
	if err != nil {
		return nil, err
	}
	changes, _, err := planIndexes(configDir, idx, items)
	if err != nil {
		return nil, err
	}
	var stale []string
	for _, c := range changes {
		if c.Action == ActionIndex {
			stale = append(stale, c.Path)
		}
	}
	return stale, nil
}

// indexInputs returns what the index.md files of configDir list, with the items
// they were derived from. It lists the loaded tree when given one and scans
// configDir otherwise.
func indexInputs(ctx context.Context, configDir string, tree *config.ContentTree) ([]okf.IndexInput, []sourceItem, error) {
	if tree == nil {
		scanned, err := config.ScanContentTreeContext(ctx, configDir)
		if err != nil {
			return nil, nil, oops.Wrapf(err, "read %s", configDir)
		}
		tree = scanned
	}
	res := &ExportResult{Counts: map[Kind]int{}}
	items := collectItems(tree, allKindSet(), configDir, res)
	used := map[string]string{}
	claim := func(p string) string { used[strings.ToLower(p)] = p; return p }
	var idx []okf.IndexInput
	for i := range items {
		it := items[i]
		it.keepName = true
		_, in, err := renderItem(it, claim)
		if err != nil {
			return nil, nil, err
		}
		if len(in) > 0 {
			idx = append(idx, in[:1]...)
		}
	}
	return idx, items, nil
}

// removeStaleIndexes deletes the generated index.md of content directories that
// list nothing any more. A file named index.md that holds prose is content and
// stays.
func removeStaleIndexes(configDir string, idx []okf.IndexInput) error {
	live := map[string]bool{}
	for _, in := range idx {
		for d := path.Dir(in.Path); d != "." && d != ""; d = path.Dir(d) {
			live[d] = true
		}
	}
	for _, top := range []string{"rules", "context", "skills", "agents", "commands", "checks", "domains"} {
		root := filepath.Join(configDir, top)
		err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				if os.IsNotExist(err) {
					return filepath.SkipDir
				}
				return err
			}
			if d.IsDir() {
				switch d.Name() {
				case "references", "scripts", "assets":
					return filepath.SkipDir
				}
				return nil
			}
			if d.Name() != okf.IndexFile {
				return nil
			}
			rel, relErr := filepath.Rel(configDir, filepath.Dir(p))
			if relErr != nil || live[filepath.ToSlash(rel)] {
				return nil //nolint:nilerr // a path outside the tree is not ours to remove
			}
			data, readErr := os.ReadFile(p) //nolint:gosec // G122: the tree is the user's own configuration directory
			if readErr == nil && config.IsOKFListing(data) {
				return os.Remove(p) //nolint:gosec // G122: only a generated listing is removed
			}
			return nil
		})
		if err != nil {
			return oops.Wrapf(err, "remove stale indexes in %s", root)
		}
	}
	return nil
}
