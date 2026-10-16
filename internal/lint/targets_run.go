package lint

import (
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
)

// isItemKind reports whether kind is a content kind rather than a JSON target kind.
func isItemKind(kind string) bool {
	switch kind {
	case kindRule, kindContext, kindSkill, kindAgent, kindCommand, kindCheck:
		return true
	}
	return false
}

// addTargetItems adds the content files of the targets to the items the run
// checks, except the ones the configuration's own tree already holds. They count
// as authored: the user named them, or a convention says they are hand-written.
func (r *runner) addTargetItems() {
	if r.targets == nil {
		return
	}
	have := make(map[string]bool, len(r.items))
	for i := range r.items {
		have[gitutil.Resolve(r.items[i].abs)] = true
	}
	top := r.tree.Top
	for i := range r.targets.Files {
		f := &r.targets.Files[i]
		if !isItemKind(f.Kind) || have[gitutil.Resolve(f.Abs)] {
			continue
		}
		have[gitutil.Resolve(f.Abs)] = true
		start := len(r.items)
		r.addItems(top, f.Kind, "", []config.ContentFile{*f.Content})
		for j := start; j < len(r.items); j++ {
			r.items[j].targetOnly = true
		}
	}
}

// scanPlainTargets runs the credential rule over the text files a user named
// that are no content kind (.env, YAML, ...): nothing else about them is judged.
func (r *runner) scanPlainTargets() {
	if r.targets == nil {
		return
	}
	for i := range r.targets.Plain {
		p := &r.targets.Plain[i]
		raw := string(p.Data)
		if _, ok := r.docs[p.Abs]; !ok {
			r.docs[p.Abs] = parseDoc(raw)
		}
		for n, line := range strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n") {
			r.scanSecrets(p.Abs, n+1, line)
		}
	}
}

// targetFiles returns the targets of one JSON kind that the location-based scan
// of the tree has not seen (seen holds absolute paths).
func (r *runner) targetFiles(kind string, seen map[string]bool) []TargetFile {
	if r.targets == nil {
		return nil
	}
	var out []TargetFile
	for _, f := range r.targets.Files {
		if f.Kind != kind || seen[f.Abs] {
			continue
		}
		seen[f.Abs] = true
		out = append(out, f)
	}
	return out
}
