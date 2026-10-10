package importer

import (
	"fmt"
	"io/fs"
	"net/url"
	"path"
	"sort"
	"strings"

	"github.com/samber/oops"
	"gopkg.in/yaml.v3"

	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
)

// Microsoft APM (microsoft/apm): apm.yml declares a package and its dependencies,
// .apm/ holds the package's own primitives (instructions, agents, prompts,
// skills, context, hooks), apm_modules/ holds the installed dependencies and
// apm.lock.yaml pins them. The formats are read from the public documentation and
// from rulesync's APM-compatible reader (src/lib/apm); the layout of .apm/ is not
// verified against a release of apm itself (see docs/cli.md).

const (
	apmName     = "apm"
	apmManifest = "apm.yml"
	apmLockFile = "apm.lock.yaml"
	// rulesyncAPMLockFile is the APM-compatible lock rulesync writes next to its own.
	rulesyncAPMLockFile = "rulesync-apm.lock.yaml"
	apmPolicyFile       = "apm-policy.yml"
	apmDir              = ".apm"
	apmModulesDir       = "apm_modules"
	// maxAPMPackages bounds how many installed packages are read.
	maxAPMPackages = 200
	// maxAPMDepth bounds how deep below apm_modules a package may sit.
	maxAPMDepth = 5
)

type apmImporter struct{}

func (apmImporter) Name() string { return apmName }

func (apmImporter) Description() string {
	return "Microsoft APM project: apm.yml, .apm/ primitives, installed apm_modules/ and apm.lock.yaml"
}

func (apmImporter) Detect(fsys fs.FS) []string {
	r := newReader(fsys)
	var found []string
	for _, f := range []string{apmManifest, apmLockFile, rulesyncAPMLockFile, apmPolicyFile, apmDir, apmModulesDir} {
		if _, ok := r.exists(f); ok {
			found = append(found, f)
		}
	}
	sort.Strings(found)
	return found
}

// apmPrimitives are the directories of a package's .apm/ and the kind each maps to.
var apmPrimitives = []struct {
	dir  string
	kind Kind
}{
	{"instructions", KindRule}, {"context", KindContext}, {litAgents, KindAgent},
	{"chatmodes", KindAgent}, {"prompts", KindCommand}, {skillsDir, KindSkill},
}

// apmMetadataKeys are package metadata of apm.yml with no ai-rulez counterpart.
var apmMetadataKeys = map[string]bool{
	litName: true, litVersion: true, litDescription: true, "author": true, litLicense: true,
	"keywords": true, "tags": true, "homepage": true, "repository": true,
}

// apmTargets maps a `target` or `targets` entry of apm.yml to the presets that
// write the same files.
var apmTargets = map[string]string{
	litCopilot: litCopilot, "vscode": litCopilot, litClaude: litClaude, litCursor: litCursor,
	litOpencode: litOpencode, litCodex: litCodex, litGemini: litGemini, "windsurf": litDevin,
}

type apmPlanner struct {
	p      *Plan
	r      *reader
	n      nativeImporter
	opt    Options
	onSkip func(name, reason string)
	// lock maps "<repo url>#<virtual path>" to the commit apm.lock.yaml resolved.
	lock map[string]string
	// rootName names a skill found at the root of a fetched checkout, which has
	// no directory name of its own.
	rootName string
}

func (apmImporter) Plan(fsys fs.FS, opt Options) (*Plan, error) {
	p := &Plan{}
	r := newReader(fsys)
	r.loadGeneratedManifests()
	b := &apmPlanner{p: p, r: r, opt: opt, lock: map[string]string{}}
	b.onSkip = func(name, reason string) { p.add(newFinding(StatusDropped, name, "", "", reason)) }

	if err := b.readLock(); err != nil {
		return nil, err
	}
	doc, err := b.readManifest()
	if err != nil {
		return nil, err
	}
	b.importManifest(doc)
	b.importPackage("", "this project", true)
	b.importModules()
	b.importPolicy()
	r.flushProblems(p)
	return p, nil
}

// readYAML reads a YAML mapping; an unparsable file is an error, as for the other
// importers, because nothing sensible can be imported from it.
func (b *apmPlanner) readYAML(file string) (parsed map[string]any, found bool, err error) {
	if _, ok := b.r.exists(file); !ok {
		return nil, false, nil
	}
	data, err := b.r.read(file)
	if err != nil {
		b.p.add(newFinding(StatusDropped, file, "", "", skipReasonOr(err)))
		return nil, false, nil
	}
	var doc map[string]any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, false, oops.With("file", file).Wrapf(err, "%s is not valid YAML (%s)", file, CodeInvalid)
	}
	return doc, true, nil
}

func (b *apmPlanner) readManifest() (map[string]any, error) {
	doc, _, err := b.readYAML(apmManifest)
	return doc, err
}

// readLock reads apm.lock.yaml for the commits it resolved; the content hashes
// are never carried (their scheme differs from the ai-rulez lock). A dependency
// the lock records with per-file hashes (deployed_file_hashes) needs no action:
// ai-rulez recomputes them from the copied files. Only a legacy entry whose sole
// hash is the aggregate content_hash is reported.
func (b *apmPlanner) readLock() error {
	for _, file := range []string{apmLockFile, rulesyncAPMLockFile} {
		doc, ok, err := b.readYAML(file)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		deps := as[[]any](doc["dependencies"])
		hashes := 0
		for _, d := range deps {
			m := as[map[string]any](d)
			commit := stringOf(m["resolved_commit"])
			repo := lockRepoURL(stringOf(m["repo_url"]), stringOf(m["host"]))
			if repo == "" || !gitutil.IsCommitSHA(commit) {
				continue
			}
			b.lock[lockKey(repo, stringOf(m["virtual_path"]))] = commit
			if stringOf(m["content_hash"]) != "" && len(as[map[string]any](m["deployed_file_hashes"])) == 0 {
				hashes++
			}
		}
		if hashes > 0 {
			b.p.add(newFinding(StatusNeedsAction, file, "dependencies.content_hash", "",
				fmt.Sprintf("%d content hash(es) not carried: the scheme differs from the ai-rulez lock; run `ai-rulez lock` after converting", hashes)))
		}
	}
	return nil
}

// lockRepoURL turns a lock entry's repo_url into the canonical URL a parsed
// dependency builds, so the commit lookup matches. A newer lock records a bare
// owner/repo and a separate host field; an older one records a full https URL.
func lockRepoURL(repo, host string) string {
	repo = strings.TrimSpace(repo)
	if repo == "" {
		return ""
	}
	spec := repo
	if !strings.Contains(repo, "://") && host != "" {
		if head, _, _ := strings.Cut(repo, "/"); !strings.Contains(head, ".") {
			spec = strings.TrimSuffix(host, "/") + "/" + repo
		}
	}
	dep, reason := parseAPMString(spec)
	if reason != "" {
		return ""
	}
	return dep.url()
}

func lockKey(repoURL, virtualPath string) string {
	repoURL = strings.TrimSuffix(strings.TrimSuffix(strings.ToLower(strings.TrimSpace(repoURL)), "/"), ".git")
	return repoURL + "#" + strings.Trim(virtualPath, "/")
}

func (b *apmPlanner) importManifest(doc map[string]any) {
	if doc == nil {
		return
	}
	keys := make([]string, 0, len(doc))
	for k := range doc {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var metadata []string
	for _, k := range keys {
		switch {
		case apmMetadataKeys[k]:
			metadata = append(metadata, k)
		case k == "dependencies":
			b.importDependencies(doc[k])
		case k == "devDependencies":
			b.p.add(newFinding(StatusDropped, apmManifest, k, "", "development dependencies are not imported; move the ones the project needs into dependencies and rerun"))
		case k == "scripts":
			b.p.add(newFinding(StatusDropped, apmManifest, k, "", "apm scripts run commands of the apm runtime; ai-rulez has no equivalent and imports none"))
		case k == "target", k == litTargets:
			b.importTarget(k, doc[k])
		default:
			b.p.add(newFinding(StatusDropped, apmManifest, k, "", "apm.yml key has no ai-rulez equivalent"))
		}
	}
	if len(metadata) > 0 {
		b.p.add(newFinding(StatusDropped, apmManifest, strings.Join(metadata, ", "), "",
			"package metadata of the APM package is not carried; ai-rulez names the project after its directory"))
	}
}

// importTarget maps the `target` (a string or a list) or the newer `targets` list
// of apm.yml to presets; key is the apm.yml key the entries came from, so the
// findings name it.
func (b *apmPlanner) importTarget(key string, v any) {
	var names []string
	switch t := v.(type) {
	case string:
		names = strings.Split(t, ",")
	case []any:
		for _, e := range t {
			names = append(names, fmt.Sprint(e))
		}
	default:
		b.p.add(newFinding(StatusUnsupported, apmManifest, key, "presets",
			fmt.Sprintf("%s must be a string or a list of target names", key)))
		return
	}
	for _, n := range names {
		n = strings.ToLower(strings.TrimSpace(n))
		switch preset, ok := apmTargets[n]; {
		case n == "all" || n == "":
			b.p.add(newFinding(StatusNeedsAction, apmManifest, key, "presets", key+" all means every APM target; set `presets` in config.toml to the tools you use"))
		case ok:
			b.p.Presets = append(b.p.Presets, preset)
			b.p.add(newFinding(StatusMapped, apmManifest, key+"."+n, "presets."+preset, ""))
		default:
			b.p.add(newFinding(StatusUnsupported, apmManifest, key+"."+n, "", "no ai-rulez preset for this APM target"))
		}
	}
}

func (b *apmPlanner) importDependencies(v any) {
	deps, ok := v.(map[string]any)
	if !ok {
		b.p.add(newFinding(StatusUnsupported, apmManifest, "dependencies", "", "dependencies is not a mapping of apm and mcp lists"))
		return
	}
	keys := make([]string, 0, len(deps))
	for k := range deps {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		switch k {
		case "apm", "mcp":
			list, ok := deps[k].([]any)
			if !ok {
				b.p.add(newFinding(StatusUnsupported, apmManifest, "dependencies."+k, "",
					"dependencies."+k+" must be a list"))
				continue
			}
			for i, e := range list {
				if k == "apm" {
					b.importDependency(fmt.Sprintf("dependencies.apm[%d]", i), e)
				} else {
					b.importMCPDependency(fmt.Sprintf("dependencies.mcp[%d]", i), e)
				}
			}
		default:
			b.p.add(newFinding(StatusDropped, apmManifest, "dependencies."+k, "", "unknown dependency type"))
		}
	}
}

// importMCPDependency maps a self-defined MCP server; a bare name is a registry
// reference, which is not resolved.
func (b *apmPlanner) importMCPDependency(field string, v any) {
	switch t := v.(type) {
	case string:
		b.p.add(newFinding(StatusUnsupported, apmManifest, field, "",
			fmt.Sprintf("%q is an MCP registry reference; convert never queries a registry. Add the server to [[mcp_servers]] by hand", t)))
	case map[string]any:
		name := stringOf(t[litName])
		if name == "" {
			b.p.add(newFinding(StatusUnsupported, apmManifest, field, "", "an MCP server needs a name"))
			return
		}
		raw := map[string]any{}
		for k, val := range t {
			raw[k] = val
		}
		delete(raw, litName)
		if tr, ok := raw["transport"]; ok {
			raw["type"] = tr
			delete(raw, "transport")
		}
		if reg, ok := raw["registry"].(bool); ok && !reg {
			delete(raw, "registry")
		}
		srv, ok := mcpServerFrom(b.p, apmManifest, name, raw)
		if !ok {
			return
		}
		b.p.MCPServers = append(b.p.MCPServers, srv)
		b.p.add(newFinding(StatusMapped, apmManifest, field, "mcp_servers."+name, ""))
	default:
		b.p.add(newFinding(StatusUnsupported, apmManifest, field, "", "entry is neither a string nor a mapping"))
	}
}

// apmDep is one parsed dependency.
type apmDep struct {
	host, owner, repo, subpath, ref string
	local                           string // a project-relative directory
}

func (d apmDep) url() string { return "https://" + d.host + "/" + d.owner + "/" + d.repo }

// parseAPMDep reads the string and mapping forms of a dependency. reason is set
// when the dependency cannot be imported at all.
func parseAPMDep(v any) (dep apmDep, reason string) {
	switch t := v.(type) {
	case string:
		return parseAPMString(t)
	case map[string]any:
		src := stringOf(t["git"])
		if src == "" {
			src = stringOf(t["source"])
		}
		if src == "" {
			src = stringOf(t["url"])
		}
		if src == "" {
			return dep, "the object form needs a git (or source) field"
		}
		dep, reason = parseAPMString(src)
		if reason != "" {
			return dep, reason
		}
		if sp := stringOf(t["path"]); sp != "" {
			dep.subpath = sp
		}
		if ref := stringOf(t["ref"]); ref != "" {
			dep.ref = ref
		}
		if reason := checkSubpath(dep.subpath); reason != "" {
			return dep, reason
		}
		return dep, ""
	}
	return dep, "entry is neither a string nor a mapping"
}

func checkSubpath(p string) string {
	if p == "" {
		return ""
	}
	if strings.HasPrefix(p, "/") || strings.HasPrefix(p, "\\") || strings.Contains(p, "\\") {
		return "the path must be relative to the repository"
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == ".." {
			return "the path must not contain .. segments"
		}
	}
	return ""
}

// apmSpecial handles the entries that are not a repository reference: a path
// inside the project, or a form convert refuses. handled is false for a repository.
func apmSpecial(s string) (dep apmDep, reason string, handled bool) {
	switch {
	case s == "":
		return dep, "entry is empty", true
	case strings.HasPrefix(s, "./") || strings.HasPrefix(s, "../"):
		clean := path.Clean(s)
		if !fs.ValidPath(clean) || clean == "." {
			return dep, "a local path must stay inside the project", true
		}
		dep.local = clean
		return dep, "", true
	case strings.HasPrefix(s, "/"):
		return dep, "an absolute path dependency is outside the project", true
	case strings.HasPrefix(s, "git@"), strings.HasPrefix(s, "ssh://"):
		return dep, "SSH dependencies are not imported; use an https URL", true
	case strings.Contains(s, "@marketplace"):
		return dep, "marketplace dependencies are registry entries; convert does not resolve registries", true
	}
	return dep, "", false
}

// apmHostAndPath splits a repository reference (without its #ref) into the host
// and the owner/repo/path segments. A bare owner/repo is on github.com; a first
// segment with a dot is a host (gitlab.example.com/group/repo).
func apmHostAndPath(spec string) (host string, segs []string, reason string) {
	host = "github.com"
	https := strings.HasPrefix(spec, "https://")
	switch {
	case https:
		u, err := url.Parse(spec)
		if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" {
			return "", nil, "is not a plain https repository URL"
		}
		host, spec = strings.ToLower(u.Host), u.Path
	case strings.Contains(spec, "://"):
		return "", nil, "only https dependencies are imported"
	}
	segs = strings.Split(strings.Trim(spec, "/"), "/")
	if !https && len(segs) >= 3 && strings.Contains(segs[0], ".") {
		host, segs = strings.ToLower(segs[0]), segs[1:]
	}
	return host, segs, ""
}

func parseAPMString(s string) (dep apmDep, reason string) {
	s = strings.TrimSpace(s)
	if dep, reason, handled := apmSpecial(s); handled {
		return dep, reason
	}
	spec, ref, _ := strings.Cut(s, "#")
	dep.ref = ref
	host, segs, reason := apmHostAndPath(spec)
	if reason != "" {
		return dep, reason
	}
	if len(segs) < 2 || segs[0] == "" || segs[1] == "" {
		return dep, `is not "owner/repo"`
	}
	dep.host, dep.owner = host, strings.ToLower(segs[0])
	dep.repo = strings.TrimSuffix(segs[1], ".git")
	dep.subpath = strings.Join(segs[2:], "/")
	if reason := checkSubpath(dep.subpath); reason != "" {
		return dep, reason
	}
	if reason := gitSourceProblem(dep.url()); reason != "" {
		return dep, "repository " + reason
	}
	return dep, ""
}

func (b *apmPlanner) importDependency(field string, v any) {
	dep, reason := parseAPMDep(v)
	if reason != "" {
		b.p.add(newFinding(StatusUnsupported, apmManifest, field, "", reason))
		return
	}
	if dep.local != "" {
		b.importLocalDependency(field, dep.local)
		return
	}
	for _, dir := range apmModuleDirs(dep) {
		if isDir, ok := b.r.exists(dir); ok && isDir {
			b.p.add(newFinding(StatusApproximated, apmManifest, field, dir,
				"installed copy imported as local files; the dependency link to "+dep.url()+" is not kept, so it no longer updates"))
			b.importPackage(dir, dir, false)
			return
		}
	}
	commit := b.lock[lockKey(dep.url(), dep.subpath)]
	b.p.Remotes = append(b.p.Remotes, Remote{
		Kind: remotePackage, Origin: apmManifest + "#" + field, URL: dep.url(), Ref: dep.ref, Commit: commit, Path: dep.subpath,
	})
}

func (b *apmPlanner) importLocalDependency(field, dir string) {
	if isDir, ok := b.r.exists(dir); !ok || !isDir {
		b.p.add(newFinding(StatusNeedsAction, apmManifest, field, "", "local dependency "+dir+" is not a directory of the project"))
		return
	}
	b.p.add(newFinding(StatusApproximated, apmManifest, field, dir,
		"local package imported as local files; the dependency link is not kept"))
	b.importPackage(dir, dir, false)
}

// apmModuleDirs are the places `apm install` puts a dependency inside apm_modules.
func apmModuleDirs(d apmDep) []string {
	base := path.Join(apmModulesDir, d.owner, d.repo)
	dirs := []string{path.Join(base, d.subpath)}
	if d.host != "github.com" {
		dirs = append(dirs, path.Join(apmModulesDir, d.host, d.owner, d.repo, d.subpath))
	}
	return dirs
}

// importModules imports the installed packages that apm.yml did not name (the
// transitive ones): every directory below apm_modules that holds a package.
func (b *apmPlanner) importModules() {
	if _, ok := b.r.exists(apmModulesDir); !ok {
		return
	}
	done := map[string]bool{}
	for i := range b.p.Items {
		p := b.p.Items[i]
		for _, s := range p.Sources {
			done[s] = true
		}
	}
	count := 0
	var walk func(dir string, depth int)
	walk = func(dir string, depth int) {
		if depth > maxAPMDepth || count >= maxAPMPackages {
			return
		}
		for _, e := range b.r.dirEntries(dir, b.onSkip) {
			if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
				continue
			}
			sub := path.Join(dir, e.Name())
			if b.isPackage(sub) {
				count++
				if !b.alreadyImported(sub) {
					b.p.add(newFinding(StatusApproximated, sub, "", sub,
						"installed package imported as local files; the dependency link is not kept, so it no longer updates"))
					b.importPackage(sub, sub, false)
				}
				continue
			}
			walk(sub, depth+1)
		}
	}
	walk(apmModulesDir, 1)
}

func (b *apmPlanner) isPackage(dir string) bool {
	for _, marker := range []string{apmDir, apmManifest, litSkillMD, skillsDir, litAgents, litCommands} {
		if _, ok := b.r.exists(path.Join(dir, marker)); ok {
			return true
		}
	}
	return false
}

// alreadyImported reports whether a package directory already contributed items.
func (b *apmPlanner) alreadyImported(dir string) bool {
	for i := range b.p.Items {
		for _, s := range b.p.Items[i].Sources {
			if s == dir || strings.HasPrefix(s, dir+"/") {
				return true
			}
		}
	}
	return false
}

// importPackage imports the primitives of a package rooted at root ("" for the
// project or a fetched checkout): its .apm/ directory, else the plugin-style
// layout of a skill or agent bundle. The project root itself only has .apm/:
// its other directories are not a package.
func (b *apmPlanner) importPackage(root, label string, project bool) {
	dir := path.Join(root, apmDir)
	if isDir, ok := b.r.exists(dir); ok && isDir {
		b.importPrimitives(dir)
		return
	}
	if root == "" && project {
		return
	}
	b.importBundle(root, label)
}

func (b *apmPlanner) importPrimitives(dir string) {
	known := map[string]bool{litHooks: true}
	for _, pr := range apmPrimitives {
		known[pr.dir] = true
		sub := path.Join(dir, pr.dir)
		if _, ok := b.r.exists(sub); !ok {
			continue
		}
		switch pr.kind {
		case KindRule:
			b.n.importRules(b.p, b.r, sub, map[string]bool{}, b.onSkip)
		case KindSkill:
			b.n.importSkills(b.p, b.r, sub, b.opt, b.onSkip)
		case KindContext, KindAgent, KindCommand, KindCheck:
			b.n.importFlat(b.p, b.r, nativeSource{Path: sub, Kind: pr.kind}, b.onSkip)
		}
	}
	b.importHooksDir(path.Join(dir, litHooks))
	for _, e := range b.r.dirEntries(dir, b.onSkip) {
		if !known[e.Name()] {
			b.p.add(newFinding(StatusDropped, path.Join(dir, e.Name()), "", "", "not an APM primitive directory; ignored"))
		}
	}
}

// importBundle reads a package without .apm/: a SKILL.md at its root, or the
// skills/, agents/ and commands/ directories of a plugin.
func (b *apmPlanner) importBundle(root, label string) {
	if _, ok := b.r.exists(path.Join(root, litSkillMD)); ok {
		b.importRootSkill(root, label)
	}
	for _, s := range []struct {
		dir  string
		kind Kind
	}{{skillsDir, KindSkill}, {litAgents, KindAgent}, {litCommands, KindCommand}} {
		sub := path.Join(root, s.dir)
		if _, ok := b.r.exists(sub); !ok {
			continue
		}
		if s.kind == KindSkill {
			b.n.importSkills(b.p, b.r, sub, b.opt, b.onSkip)
			continue
		}
		b.n.importFlat(b.p, b.r, nativeSource{Path: sub, Kind: s.kind}, b.onSkip)
	}
}

// apmPackageFiles are the files of a package root that are not part of a skill.
func apmPackageFile(rel string) bool {
	lower := strings.ToLower(rel)
	switch {
	case strings.HasPrefix(rel, ".") || strings.HasPrefix(rel, apmDir+"/"):
		return true
	case rel == apmManifest, rel == apmLockFile, rel == rulesyncAPMLockFile, rel == apmPolicyFile:
		return true
	case strings.HasPrefix(lower, "readme"), strings.HasPrefix(lower, litLicense), strings.HasPrefix(lower, "changelog"):
		return true
	}
	return false
}

// importRootSkill imports a package whose SKILL.md sits at its root as one skill
// named after its frontmatter, else after the directory.
func (b *apmPlanner) importRootSkill(root, label string) {
	skillFile := path.Join(root, litSkillMD)
	data, ok := b.n.readOrReport(b.p, b.r, skillFile)
	if !ok {
		return
	}
	text := normalizeText(string(data))
	base := path.Base(root)
	if root == "" {
		base = b.rootName
	}
	name, _ := safeName(base)
	if fm, _, has := splitFrontmatter(text); has {
		meta, _ := parseFrontmatter(fm)
		if n := stringOf(meta[litName]); n != "" {
			name, _ = safeName(n)
		}
	}
	text = reviewAuxFrontmatter(b.p, skillFile, text)
	text = setFrontmatterName(text, name)
	it := Item{Kind: KindSkill, Name: name, Sources: []string{root}, Main: ensureNewline(text)}
	for _, rel := range b.r.walkFiles(root, b.onSkip) {
		if rel == litSkillMD || apmPackageFile(rel) {
			continue
		}
		file := path.Join(root, rel)
		res, err := b.r.read(file)
		if err != nil {
			b.p.add(newFinding(StatusDropped, file, "", "", skipReasonOr(err)))
			continue
		}
		it.Resources = append(it.Resources, File{Path: rel, Data: res, Exec: b.r.executable(file)})
	}
	b.p.Items = append(b.p.Items, it)
	b.p.add(newFinding(StatusApproximated, label, "", "skills/"+name, "package with a SKILL.md at its root imported as one skill"))
}

// importHooksDir reads the JSON hook files of a package; scripts they run are not
// copied, so a hook that runs one needs action.
func (b *apmPlanner) importHooksDir(dir string) {
	if _, ok := b.r.exists(dir); !ok {
		return
	}
	hb := &hookBuilder{p: b.p}
	for _, rel := range b.r.walkFiles(dir, b.onSkip) {
		file := path.Join(dir, rel)
		if !strings.HasSuffix(rel, ".json") {
			b.p.add(newFinding(StatusDropped, file, "", "", "hook scripts are not copied; a hook that runs one needs the script added to the project by hand"))
			continue
		}
		src := hookSource{file: file}
		if table, ok := hb.readTable(b.r, src, litHooks); ok {
			hb.fromEventTable(src, table, "hooks.")
		}
	}
}

func (b *apmPlanner) importPolicy() {
	if _, ok := b.r.exists(apmPolicyFile); ok {
		b.p.add(newFinding(StatusUnsupported, apmPolicyFile, "", "",
			"APM policy (allowed sources, tighten-only extends) has no ai-rulez counterpart here; see the organization policy documentation"))
	}
}
