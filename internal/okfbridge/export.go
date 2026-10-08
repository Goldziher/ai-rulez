package okfbridge

import (
	"fmt"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/okf"
	"gopkg.in/yaml.v3"
)

// ExportOptions selects what an export writes.
type ExportOptions struct {
	// Include limits the kinds exported; empty means all.
	Include []Kind
	// IndexStyle is okf.StyleBody (the default, also for "") or okf.StyleFrontmatter.
	IndexStyle string
	// LocalDir is the project's own .ai-rulez directory. When set, an item whose
	// source file lies outside it (merged in from an include) is not exported,
	// with a note, the same way include and builtin domains are skipped: the
	// committed bundle documents this project, not what it pulled in. Empty
	// disables the filter.
	LocalDir string
}

// ExportResult is the bundle and what went into it.
type ExportResult struct {
	Files  []okf.File
	Counts map[Kind]int
	// Notes are things a user should know, e.g. skipped builtin domains.
	Notes []string
}

type sourceItem struct {
	kind   Kind
	domain string
	cf     config.ContentFile
	// typ and title override the defaults when set: a migrated file keeps the
	// type and title it already declares. keepName keeps a skill name equal to
	// its id in the metadata, since a migrated file is read back by the loader,
	// which does not derive it the way an import does.
	typ, title string
	keepName   bool
}

// Export renders tree as an OKF bundle. The output is deterministic: files are
// sorted, there are no timestamps, and ids are deduplicated in sorted order.
func Export(tree *config.ContentTree, opts ExportOptions) (*ExportResult, error) {
	include := map[Kind]bool{}
	kinds := opts.Include
	if len(kinds) == 0 {
		kinds = AllKinds
	}
	for _, k := range kinds {
		include[k] = true
	}
	res := &ExportResult{Counts: map[Kind]int{}}
	items := collectItems(tree, include, opts.LocalDir, res)

	used := map[string]string{}
	claim := func(p string) string {
		base, ext := p, ""
		if strings.HasSuffix(p, ".md") {
			base, ext = strings.TrimSuffix(p, ".md"), ".md"
		}
		candidate := p
		for n := 2; ; n++ {
			if _, taken := used[strings.ToLower(candidate)]; !taken {
				used[strings.ToLower(candidate)] = candidate
				return candidate
			}
			candidate = fmt.Sprintf("%s-%d%s", base, n, ext)
		}
	}

	if !okf.ValidIndexStyle(opts.IndexStyle) {
		return nil, fmt.Errorf("unknown index style %q (use %s or %s)", opts.IndexStyle, okf.StyleBody, okf.StyleFrontmatter)
	}
	var idx []okf.IndexInput
	var pieces []piece
	for i := range items {
		it := items[i]
		ps, in, err := renderItem(it, claim)
		if err != nil {
			return nil, err
		}
		pieces = append(pieces, ps...)
		idx = append(idx, in...)
		res.Counts[it.kind]++
	}
	files := rewriteExportLinks(pieces, res)
	for p, data := range okf.BuildIndexes(idx, dirLabels(items), opts.IndexStyle) {
		files = append(files, okf.File{Path: claimIndex(used, p), Data: data})
	}
	sort.SliceStable(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	res.Files = files
	return res, nil
}

// claimIndex registers an index path. Generated indexes always win their path:
// concept files never use reserved names (see resourcePath), so a rule named
// index or log lands in index_.md or log_.md and the import restores its name
// from x-ai-rulez.id.
func claimIndex(used map[string]string, p string) string {
	used[strings.ToLower(p)] = p
	return p
}

func collectItems(tree *config.ContentTree, include map[Kind]bool, localDir string, res *ExportResult) []sourceItem {
	var items []sourceItem
	foreign := 0
	add := func(domain string, lists map[Kind][]config.ContentFile) {
		for _, k := range AllKinds {
			if !include[k] {
				continue
			}
			files := append([]config.ContentFile(nil), lists[k]...)
			sort.SliceStable(files, func(i, j int) bool { return itemID(k, files[i]) < itemID(k, files[j]) })
			for i := range files {
				cf := files[i]
				if !isLocal(cf, localDir) {
					foreign++
					continue
				}
				items = append(items, loadedItem(k, domain, cf))
			}
		}
	}
	add("", map[Kind][]config.ContentFile{
		KindRule: tree.Rules, KindContext: tree.Context, KindSkill: tree.Skills,
		KindAgent: tree.Agents, KindCommand: tree.Commands, KindCheck: tree.Checks,
	})
	names := make([]string, 0, len(tree.Domains))
	for name := range tree.Domains {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		d := tree.Domains[name]
		if d == nil {
			continue
		}
		if d.Builtin || d.FromInclude {
			res.Notes = append(res.Notes, fmt.Sprintf("skipped domain %q: it comes from a builtin or an include, not from this project", name))
			continue
		}
		add(name, map[Kind][]config.ContentFile{
			KindRule: d.Rules, KindContext: d.Context, KindSkill: d.Skills,
			KindAgent: d.Agents, KindCommand: d.Commands, KindCheck: d.Checks,
		})
	}
	if foreign > 0 {
		res.Notes = append(res.Notes, fmt.Sprintf("skipped %d item(s) merged in from includes: they are not this project's own content", foreign))
	}
	return items
}

// loadedItem is a source item as the loader read it. A file that declares an OKF
// type or title keeps both, and its skill name stays in the metadata, so
// exporting a migrated tree reproduces the tree.
func loadedItem(k Kind, domain string, cf config.ContentFile) sourceItem {
	it := sourceItem{kind: k, domain: domain, cf: cf}
	if m := cf.Metadata; m != nil {
		it.typ, it.title = m.OKFType, m.OKFTitle
		it.keepName = m.OKFType != ""
	}
	return it
}

// isLocal reports whether a content file was read from localDir. Items without a
// source path (built in memory) count as local.
func isLocal(cf config.ContentFile, localDir string) bool {
	if localDir == "" || cf.Path == "" {
		return true
	}
	abs := func(p string) string {
		if a, err := filepath.Abs(p); err == nil {
			p = a
		}
		if r, err := filepath.EvalSymlinks(p); err == nil {
			p = r
		}
		return p
	}
	rel, err := filepath.Rel(abs(localDir), abs(cf.Path))
	return err == nil && filepath.IsLocal(rel)
}

// itemID is the ai-rulez identifier of an item: the directory name of a skill,
// the file stem of everything else.
func itemID(k Kind, cf config.ContentFile) string {
	if k == KindSkill {
		return config.SkillID(cf)
	}
	return cf.Name
}

func baseDir(it sourceItem) string {
	if it.domain == "" {
		return string(it.kind)
	}
	return path.Join(dirDomains, sanitizeID(it.domain), string(it.kind))
}

func hasResources(it sourceItem) bool {
	return (it.kind == KindSkill || it.kind == KindCommand) && len(it.cf.Resources) > 0
}

// piece is a file of the bundle before its links are rewritten. head and body
// are set for a markdown concept (Data is head plus the rewritten body); src is
// the path of the source file relative to the configuration directory, "" when
// unknown.
type piece struct {
	file okf.File
	head []byte
	body string
	src  string
}

// sourceRel is the path of an item's source file relative to the configuration
// directory, rebuilt from its kind, domain and id.
func sourceRel(it sourceItem) string {
	if it.cf.Path == "" {
		return ""
	}
	base := string(it.kind)
	if it.domain != "" {
		base = path.Join(dirDomains, it.domain, base)
	}
	name := path.Base(strings.ReplaceAll(it.cf.Path, "\\", "/"))
	if name == fileSkill || name == fileCommand {
		return path.Join(base, itemID(it.kind, it.cf), name)
	}
	return path.Join(base, name)
}

func renderItem(it sourceItem, claim func(string) string) ([]piece, []okf.IndexInput, error) {
	id := itemID(it.kind, it.cf)
	seg := sanitizeID(id)
	var conceptPath string
	switch {
	case it.kind == KindSkill:
		conceptPath = claim(path.Join(baseDir(it), seg, fileSkill))
	case hasResources(it):
		conceptPath = claim(path.Join(baseDir(it), seg, fileCommand))
	default:
		conceptPath = claim(resourcePath(path.Join(baseDir(it), seg+".md")))
	}
	fields, desc, err := conceptFields(it, id)
	if err != nil {
		return nil, nil, err
	}
	head, err := okf.MarshalFrontmatter(fields)
	if err != nil {
		return nil, nil, err
	}
	title := titleOf(fields)
	src := sourceRel(it)
	files := []piece{{file: okf.File{Path: conceptPath}, head: append(head, '\n'), body: it.cf.Content, src: src}}
	index := []okf.IndexInput{{Path: conceptPath, Title: title, Description: desc}}
	if hasResources(it) {
		resFiles, resIdx, err := renderResources(it, id, path.Dir(conceptPath), path.Dir(src), claim)
		if err != nil {
			return nil, nil, err
		}
		files = append(files, resFiles...)
		index = append(index, resIdx...)
	}
	return files, index, nil
}

func titleOf(fields []okf.Field) string {
	for _, f := range fields {
		if f.Key == keyTitle {
			if s, ok := f.Value.(string); ok {
				return s
			}
		}
	}
	return ""
}

// conceptFields builds the frontmatter of an item's concept and returns its
// description for the index.
func conceptFields(it sourceItem, id string) (fields []okf.Field, description string, err error) {
	okfExtra, meta := splitMetadata(it.cf.Metadata)
	if it.kind == KindSkill && !it.keepName {
		meta = dropDerivedName(meta, id)
	}
	typ, title := defaultType(it.kind), okf.TitleFromPath(sanitizeID(id)+".md")
	if it.typ != "" {
		typ = it.typ
	}
	if it.title != "" {
		title = it.title
	}
	hoisted := []okf.Field{}
	for _, kv := range okfExtra {
		switch kv.Key {
		case keyType:
			if s, ok := kv.Value.(string); ok && strings.TrimSpace(s) != "" {
				typ = s
			}
		case keyTitle:
			if s, ok := kv.Value.(string); ok && strings.TrimSpace(s) != "" {
				title = s
			}
		case keyDescription, okf.ExtensionKey:
		default:
			hoisted = append(hoisted, kv)
		}
	}
	fields = []okf.Field{{Key: keyType, Value: typ}, {Key: keyTitle, Value: title}}
	if it.cf.Metadata != nil {
		description = strings.TrimSpace(it.cf.Metadata.Extra[keyDescription])
	}
	if description != "" {
		fields = append(fields, okf.Field{Key: keyDescription, Value: description})
	}
	fields = append(fields, hoisted...)

	ext := []okf.Field{{Key: keyKind, Value: kindName(it.kind)}, {Key: "id", Value: id}}
	if it.domain != "" {
		ext = append(ext, okf.Field{Key: keyDomain, Value: it.domain})
	}
	if len(meta) > 0 {
		ext = append(ext, okf.Field{Key: "metadata", Value: fieldsNode(meta)})
	}
	fields = append(fields, okf.Field{Key: okf.ExtensionKey, Value: fieldsNode(ext)})
	return fields, description, nil
}

// dropDerivedName removes a skill `name` that equals the skill id. An import adds
// exactly that name to a skill without one, so leaving it out keeps the round trip
// from growing a metadata entry on every cycle.
func dropDerivedName(meta []okf.Field, id string) []okf.Field {
	out := meta[:0:0]
	for _, f := range meta {
		if s, ok := f.Value.(string); f.Key == keyName && ok && s == id {
			continue
		}
		out = append(out, f)
	}
	return out
}

func kindName(k Kind) string {
	return strings.TrimSuffix(string(k), "s")
}

// fieldsNode builds an ordered mapping node.
func fieldsNode(fields []okf.Field) *yaml.Node {
	n := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	for _, f := range fields {
		key := &yaml.Node{Kind: yaml.ScalarNode, Tag: tagStr, Value: f.Key}
		var val yaml.Node
		if vn, ok := f.Value.(*yaml.Node); ok {
			val = *vn
		} else if err := val.Encode(f.Value); err != nil {
			val = yaml.Node{Kind: yaml.ScalarNode, Tag: tagStr, Value: fmt.Sprint(f.Value)}
		}
		n.Content = append(n.Content, key, &val)
	}
	return n
}

// splitMetadata separates the `okf` memory key (foreign OKF keys kept by an
// import) from the metadata that goes into x-ai-rulez.metadata. The description
// is excluded from both: it becomes the OKF description.
func splitMetadata(m *config.Metadata) (okfKeys, meta []okf.Field) {
	if m == nil {
		return nil, nil
	}
	meta = typedFields(m)
	extras := make([]string, 0, len(m.Extra))
	for k := range m.Extra {
		if k != keyDescription {
			extras = append(extras, k)
		}
	}
	sort.Strings(extras)
	for _, k := range extras {
		v, ok := m.TypedExtra(k)
		if !ok {
			continue
		}
		if node, isNode := v.(*yaml.Node); isNode && k == "okf" && node.Kind == yaml.MappingNode {
			for i := 0; i+1 < len(node.Content); i += 2 {
				okfKeys = append(okfKeys, okf.Field{Key: node.Content[i].Value, Value: nodeValue(node.Content[i+1])})
			}
			continue
		}
		meta = append(meta, okf.Field{Key: k, Value: v})
	}
	return okfKeys, meta
}

// typedFields lists the metadata ai-rulez models as fields, in a fixed order.
func typedFields(m *config.Metadata) []okf.Field {
	var out []okf.Field
	add := func(key string, v any) { out = append(out, okf.Field{Key: key, Value: v}) }
	if m.Priority != "" {
		add("priority", m.Priority)
	}
	for _, l := range []struct {
		key string
		val []string
	}{
		{"targets", m.Targets}, {"aliases", m.Aliases}, {"tools", m.Tools},
		{"skills", m.Skills}, {"keywords", m.Keywords},
	} {
		if len(l.val) > 0 {
			add(l.key, l.val)
		}
	}
	for _, s := range []struct{ key, val string }{
		{"usage", m.Usage}, {"shortcut", m.Shortcut}, {"category", m.Category},
		{"effort", m.Effort}, {"activation", m.Activation},
	} {
		if s.val != "" {
			add(s.key, s.val)
		}
	}
	if len(m.Globs) > 0 {
		add("globs", m.Globs)
	}
	if len(m.Paths) > 0 {
		add("paths", m.Paths)
	}
	return out
}

// nodeValue returns a plain string for string scalars, so they are re-emitted
// with the encoder's own quoting, and the node itself for anything else.
func nodeValue(n *yaml.Node) any {
	if n.Kind == yaml.ScalarNode && n.Tag == tagStr {
		return n.Value
	}
	return n
}

func renderResources(it sourceItem, id, dir, srcDir string, claim func(string) string) ([]piece, []okf.IndexInput, error) {
	res := append([]config.SkillResource(nil), it.cf.Resources...)
	sort.Slice(res, func(i, j int) bool { return res[i].RelPath < res[j].RelPath })
	var files []piece
	var idx []okf.IndexInput
	for _, r := range res {
		rel := path.Clean(strings.ReplaceAll(r.RelPath, "\\", "/"))
		if err := okf.ValidatePath(rel); err != nil || !resourceK[strings.SplitN(rel, "/", 2)[0]] {
			return nil, nil, fmt.Errorf("%s %q has an unsupported resource path %q", kindName(it.kind), id, r.RelPath)
		}
		src := ""
		if srcDir != "" && srcDir != "." {
			src = path.Join(srcDir, rel)
		}
		// Only a lower-case .md is a concept: import reads exactly that suffix, so
		// an .MD resource travels as a plain file and survives the round trip.
		if !strings.HasSuffix(rel, ".md") {
			files = append(files, piece{file: okf.File{Path: claim(path.Join(dir, rel)), Data: r.Content, Mode: r.Mode}, src: src})
			continue
		}
		p := claim(path.Join(dir, resourcePath(rel)))
		ext := []okf.Field{{Key: keyKind, Value: string(kindResource)}, {Key: "id", Value: id}, {Key: "owner", Value: kindName(it.kind)}, {Key: "path", Value: rel}}
		if it.domain != "" {
			ext = append(ext, okf.Field{Key: keyDomain, Value: it.domain})
		}
		title := okf.TitleFromPath(rel)
		head, err := okf.MarshalFrontmatter([]okf.Field{
			{Key: keyType, Value: "Reference"}, {Key: keyTitle, Value: title},
			{Key: okf.ExtensionKey, Value: fieldsNode(ext)},
		})
		if err != nil {
			return nil, nil, err
		}
		files = append(files, piece{file: okf.File{Path: p, Mode: r.Mode}, head: append(head, '\n'), body: string(r.Content), src: src})
		idx = append(idx, okf.IndexInput{Path: p, Title: title, Description: r.Description})
	}
	return files, idx, nil
}

// resourcePath renames markdown resources whose name is reserved in OKF.
func resourcePath(rel string) string {
	switch strings.ToLower(path.Base(rel)) {
	case okf.IndexFile, okf.LogFile:
		return strings.TrimSuffix(rel, ".md") + "_.md"
	}
	return rel
}

func dirLabels(items []sourceItem) okf.DirLabel {
	labels := okf.DirLabel{}
	for i := range items {
		it := &items[i]
		if it.domain == "" {
			labels[string(it.kind)] = kindLabel(it.kind) + " exported from ai-rulez"
			continue
		}
		dom := path.Join(dirDomains, sanitizeID(it.domain))
		labels[dirDomains] = "Content of ai-rulez domains"
		labels[dom] = "Content of the " + it.domain + " domain"
		labels[path.Join(dom, string(it.kind))] = kindLabel(it.kind) + " of the " + it.domain + " domain"
	}
	return labels
}
