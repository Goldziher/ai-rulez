package importer

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"

	"github.com/tailscale/hujson"
)

const (
	rulesyncName       = "rulesync"
	rulesyncConfigFile = "rulesync.jsonc"
	rulesyncLocalFile  = "rulesync.local.jsonc"
	rulesyncDir        = ".rulesync"
	rulesyncLockFile   = "rulesync.lock"
	rulesyncNPMLock    = "rulesync-npm.lock.json"
	rulesyncIgnoreFile = ".rulesyncignore"
)

// The on-disk format is read from the rulesync source tree (docs/reference/file-formats.md
// and src/constants/rulesync-paths.ts of dyoshikawa/rulesync): a rulesync.jsonc at the
// project root, and an input root (default .rulesync/) holding rules/, commands/,
// subagents/, skills/<name>/, checks/, mcp.jsonc (or the legacy mcp.json),
// hooks.jsonc, permissions.jsonc and .aiignore.

type rulesyncImporter struct{}

func (rulesyncImporter) Name() string { return rulesyncName }

func (rulesyncImporter) Description() string {
	return "rulesync project: rulesync.jsonc and .rulesync/ rules, commands, subagents, skills, checks, mcp.jsonc and ignore"
}

// rulesyncInputs are the paths below an input root that Detect lists.
var rulesyncInputs = []string{
	"rules", "commands", "subagents", "skills", "checks", "mcp.jsonc", "mcp.json", ".mcp.json",
	"hooks.jsonc", "hooks.json", "permissions.jsonc", "permissions.json", ".aiignore",
}

func (rulesyncImporter) Detect(fsys fs.FS) []string {
	r := newReader(fsys)
	var found []string
	for _, f := range []string{rulesyncConfigFile, rulesyncLocalFile, rulesyncLockFile, rulesyncNPMLock, rulesyncIgnoreFile} {
		if _, ok := r.exists(f); ok {
			found = append(found, f)
		}
	}
	cfg, _ := readRulesyncConfig(r)
	for _, root := range rulesyncRoots(cfg, nil) {
		for _, in := range rulesyncInputs {
			if _, ok := r.exists(path.Join(root, in)); ok {
				found = append(found, path.Join(root, in))
			}
		}
	}
	sort.Strings(found)
	return dedupeStrings(found)
}

// rulesyncConfig is the part of rulesync.jsonc the importer reads.
type rulesyncConfig struct {
	raw map[string]json.RawMessage
}

// readRulesyncConfig parses rulesync.jsonc. A missing file is not an error; an
// unparsable one is.
func readRulesyncConfig(r *reader) (*rulesyncConfig, error) {
	if _, ok := r.exists(rulesyncConfigFile); !ok {
		return nil, nil
	}
	data, err := r.read(rulesyncConfigFile)
	if err != nil {
		return nil, fmt.Errorf("%s: %s", rulesyncConfigFile, skipReasonOr(err))
	}
	std, err := hujson.Standardize(data)
	if err != nil {
		return nil, fmt.Errorf("%s is not valid JSONC (%s): %w", rulesyncConfigFile, CodeInvalid, err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(std, &raw); err != nil {
		return nil, fmt.Errorf("%s is not a JSON object (%s): %w", rulesyncConfigFile, CodeInvalid, err)
	}
	return &rulesyncConfig{raw: raw}, nil
}

// rulesyncRoots returns the input roots, in order: `inputRoots`, else the
// parent-style `inputRoot`, else .rulesync. A root that is absolute or leaves
// the project is refused (and reported when p is set).
func rulesyncRoots(cfg *rulesyncConfig, p *Plan) []string {
	var roots []string
	if cfg != nil {
		var list []string
		if raw, ok := cfg.raw["inputRoots"]; ok {
			_ = json.Unmarshal(raw, &list)
		}
		if len(list) == 0 {
			var one string
			if raw, ok := cfg.raw["inputRoot"]; ok && json.Unmarshal(raw, &one) == nil && one != "" {
				list = []string{path.Join(one, rulesyncDir)}
			}
		}
		for _, entry := range list {
			clean := path.Clean(strings.ReplaceAll(entry, "\\", "/"))
			if !fs.ValidPath(clean) || strings.HasPrefix(entry, "/") {
				if p != nil {
					p.add(newFinding(StatusUnsupported, rulesyncConfigFile, "inputRoots", "",
						fmt.Sprintf("input root %q is outside the source directory and was not read", entry)))
				}
				continue
			}
			roots = append(roots, clean)
		}
	}
	if len(roots) == 0 {
		roots = []string{rulesyncDir}
	}
	return roots
}

func (rulesyncImporter) Plan(fsys fs.FS, opt Options) (*Plan, error) {
	p := &Plan{}
	r := newReader(fsys)
	r.loadGeneratedManifests()
	cfg, err := readRulesyncConfig(r)
	if err != nil {
		return nil, err
	}
	b := &rulesyncPlanner{p: p, r: r, seen: map[string]int{}}
	b.onSkip = func(name, reason string) { p.add(newFinding(StatusDropped, name, "", "", reason)) }

	b.importConfig(cfg)
	for _, root := range rulesyncRoots(cfg, p) {
		b.importRoot(root)
	}
	b.importProjectFiles()
	r.flushProblems(p)
	return p, nil
}

// rulesyncPlanner holds the state of one rulesync import.
type rulesyncPlanner struct {
	p      *Plan
	r      *reader
	seen   map[string]int // kind\x00name -> index in p.Items, so a later input root overrides
	onSkip func(name, reason string)
}

func (b *rulesyncPlanner) addItem(it Item) {
	key := string(it.Kind) + "\x00" + it.Name
	if i, ok := b.seen[key]; ok {
		b.p.add(newFinding(StatusApproximated, it.Sources[0], "name", it.Rel(),
			"overrides the item of the same name from "+b.p.Items[i].Sources[0]+" (a later input root wins)"))
		b.p.Items[i] = it
		return
	}
	b.seen[key] = len(b.p.Items)
	b.p.Items = append(b.p.Items, it)
}

var rulesyncRootEntries = map[string]bool{
	"rules": true, "commands": true, "subagents": true, "skills": true, "checks": true,
	"mcp.jsonc": true, "mcp.json": true, ".mcp.json": true, "hooks.jsonc": true, "hooks.json": true,
	"permissions.jsonc": true, "permissions.json": true, ".aiignore": true,
}

func (b *rulesyncPlanner) importRoot(root string) {
	isDir, ok := b.r.exists(root)
	if !ok || !isDir {
		return
	}
	for _, e := range b.r.dirEntries(root, b.onSkip) {
		if !rulesyncRootEntries[e.Name()] {
			b.p.add(newFinding(StatusDropped, path.Join(root, e.Name()), "", "", "not a rulesync input; ignored"))
		}
	}
	b.importRules(root)
	b.importFlatKind(root, "commands", KindCommand)
	b.importFlatKind(root, "subagents", KindAgent)
	b.importFlatKind(root, "checks", KindCheck)
	b.importSkills(root)
	b.importMCP(root)
	b.importHooksAndPermissions(root)
	b.importIgnore(path.Join(root, ".aiignore"))
}

// importProjectFiles reports the project-level files outside the input root.
func (b *rulesyncPlanner) importProjectFiles() {
	b.importIgnore(rulesyncIgnoreFile)
	if _, ok := b.r.exists(rulesyncLocalFile); ok {
		b.p.add(newFinding(StatusNeedsAction, rulesyncLocalFile, "", "",
			"machine-local rulesync overrides are not imported; put personal settings in .ai-rulez/config.local.toml"))
	}
	for _, lock := range []string{rulesyncLockFile, rulesyncNPMLock} {
		if _, ok := b.r.exists(lock); ok {
			b.p.add(newFinding(StatusNeedsAction, lock, "", "",
				"pins remote sources by commit; not carried because remote sources are not fetched. After adding them as [[includes]] or [[installed_skills]], run `ai-rulez lock`"))
		}
	}
}

// importIgnore reports an ignore file: ai-rulez has no ignore feature, and
// rulesync deprecates it in favour of permissions (read deny).
func (b *rulesyncPlanner) importIgnore(file string) {
	if _, ok := b.r.exists(file); !ok {
		return
	}
	data, err := b.r.read(file)
	if err != nil {
		b.p.add(newFinding(StatusDropped, file, "", "", skipReasonOr(err)))
		return
	}
	n := 0
	for _, line := range strings.Split(normalizeText(string(data)), "\n") {
		if line = strings.TrimSpace(line); line != "" && !strings.HasPrefix(line, "#") {
			n++
		}
	}
	b.p.add(newFinding(StatusDropped, file, "", "", fmt.Sprintf(
		"%d ignore pattern(s) not imported: ignore files are deprecated upstream and ai-rulez has no ignore feature; deny reads with [permissions] instead", n)))
}
