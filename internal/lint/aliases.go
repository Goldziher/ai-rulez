package lint

import (
	"bytes"
	"strings"
	"sync"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
)

// A generated alias is a skill, agent or command file whose bytes are exactly
// what the plugin generator renders from a definition the run knows: a bundle's
// generated copy, or a byte-identical copy of one elsewhere (a public export of
// the bundle's skills). It is that definition under another path, not a second
// one, so the duplicate rules (AR701, AR702, AR703) do not compare it with the
// definition it was rendered from; every other rule, the security rules first,
// judges it as any other file.
//
// The comparison is with what the generator renders from the definition now,
// in this run, never with a header or a provenance sidecar in the tree: those
// are text anyone can write. A copy that was edited, or that the definition has
// since moved away from (a stale bundle), renders differently and stays a
// definition of its own. A run without the renderings (WithGeneratedCopies not
// given) treats every file as a definition, as before.

// GeneratedCopy is one skill, agent or command file a plugin bundle of the
// configuration renders: its kind (skill, agent or command), the definition it
// is rendered from and the bytes the generator writes.
type GeneratedCopy struct {
	Kind    string
	Source  config.ContentFile
	Content []byte
}

// WithGeneratedCopies hands the run the plugin bundles' renderings of the
// configured skills, agents and commands, so a byte-identical copy of one is
// recognized as its definition by the duplicate rules. render is called at most
// once, and only when the run finds a duplicate to judge.
func WithGeneratedCopies(render func() []GeneratedCopy) Option {
	return func(r *runner) { r.renderCopies = render }
}

// copyIndex is the renderings of a run, keyed by kind and normalized content.
type copyIndex struct {
	once    sync.Once
	sources map[string][]string // kind + "\x00" + content -> resolved definition paths
	ids     map[string]string   // resolved definition path -> definition id, lower case
	memo    sync.Map            // item path -> resolved definition path, "" for none
}

func copyKey(kind string, content []byte) string { return kind + "\x00" + string(content) }

// normalizeEOL turns CRLF line endings into LF, so a checkout with autocrlf
// still compares equal to the rendering. Nothing else is normalized.
func normalizeEOL(data []byte) []byte {
	return bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
}

func (r *runner) copies() *copyIndex {
	r.copyIdx.once.Do(func() {
		r.copyIdx.sources, r.copyIdx.ids = map[string][]string{}, map[string]string{}
		if r.renderCopies == nil {
			return
		}
		copies := r.renderCopies()
		for i := range copies {
			c := &copies[i]
			if c.Source.Path == "" || strings.HasPrefix(c.Source.Path, "builtin://") {
				continue
			}
			src := gitutil.Resolve(c.Source.Path)
			key := copyKey(c.Kind, normalizeEOL(c.Content))
			r.copyIdx.sources[key] = append(r.copyIdx.sources[key], src)
			r.copyIdx.ids[src] = strings.ToLower(itemID(c.Kind, c.Source))
		}
	})
	return &r.copyIdx
}

// aliasSource returns the resolved path of the definition it is a generated
// alias of, or "" when it is not one: no rendering of its kind has its bytes,
// the renderings with its bytes come from more than one definition, or the
// definition has another id.
func (r *runner) aliasSource(it *item) string {
	idx := r.copies()
	if v, ok := idx.memo.Load(it.abs); ok {
		if s, isString := v.(string); isString {
			return s
		}
	}
	src := r.findAliasSource(idx, it)
	idx.memo.Store(it.abs, src)
	return src
}

func (r *runner) findAliasSource(idx *copyIndex, it *item) string {
	if len(idx.sources) == 0 || it.isDoc || isBuiltinItem(it) {
		return ""
	}
	switch it.kind {
	case kindSkill, kindAgent, kindCommand:
	default:
		return ""
	}
	data, err := readSmallFile(it.abs)
	if err != nil {
		return ""
	}
	sources := idx.sources[copyKey(it.kind, normalizeEOL(data))]
	if len(sources) == 0 {
		return ""
	}
	src := sources[0]
	for _, s := range sources[1:] {
		if s != src {
			return "" // the same bytes render from two definitions: not one item's copy
		}
	}
	if src == gitutil.Resolve(it.abs) || idx.ids[src] != strings.ToLower(itemID(it.kind, it.cf)) {
		return ""
	}
	return src
}

// sameDefinition reports whether a and b are one logical item: the same kind and
// id, where one is a generated alias of the other, or both are aliases of one
// definition. Two files that are not aliases are never the same definition,
// however alike they are.
func (r *runner) sameDefinition(a, b *item) bool {
	if a.kind != b.kind || !strings.EqualFold(itemID(a.kind, a.cf), itemID(b.kind, b.cf)) {
		return false
	}
	aSrc, bSrc := r.aliasSource(a), r.aliasSource(b)
	switch {
	case aSrc != "" && bSrc != "":
		return aSrc == bSrc
	case aSrc != "":
		return aSrc == gitutil.Resolve(b.abs)
	case bSrc != "":
		return bSrc == gitutil.Resolve(a.abs)
	}
	return false
}

// definitionListed reports whether it is a generated alias whose definition is
// among entries, so the definition stands for it.
func (r *runner) definitionListed(it *item, entries []descEntry) bool {
	src := r.aliasSource(it)
	if src == "" {
		return false
	}
	for _, e := range entries {
		if e.it != it && !isBuiltinItem(e.it) && gitutil.Resolve(e.it.abs) == src {
			return true
		}
	}
	return false
}
