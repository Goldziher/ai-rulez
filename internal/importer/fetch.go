package importer

import (
	"context"
	"fmt"
	"os"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/skillsource"
	"github.com/samber/oops"
)

// fullSHA matches a full git commit hash.
var fullSHA = regexp.MustCompile(`^[0-9a-f]{40}$`)

// splitOrigin splits "file#field" into the finding's source and field.
func splitOrigin(origin string) (source, field string) {
	source, field, _ = strings.Cut(origin, "#")
	return source, field
}

// describe names a remote source for a message: URL, subdirectory and ref.
func (r Remote) describe() string {
	s := r.URL
	if r.Path != "" {
		s += " (" + r.Path + ")"
	}
	if r.Ref != "" {
		s += " at " + r.Ref
	}
	return s
}

// label is how a fetched file is named in findings and in the scan (host, repository
// and the commit that was read), followed by the path inside it. The scheme is left
// out: a name with // in it is not a path, and tools that clean paths would mangle it.
func (r Remote) label(commit string) string {
	short := commit
	if len(short) > 12 {
		short = short[:12]
	}
	return strings.TrimPrefix(r.URL, "https://") + "@" + short
}

// reportUnfetched records a needs-action finding for every remote source: without
// --fetch convert never uses the network, so the content is not imported.
func (p *Plan) reportUnfetched() {
	for i := range p.Remotes {
		rm := p.Remotes[i]
		reason := fmt.Sprintf("%s is not fetched: convert uses the network only with --fetch; rerun with --fetch, or add the source by hand", rm.describe())
		if rm.Commit != "" {
			reason += " (the input's lock file pins it to " + rm.Commit[:12] + ")"
		}
		source, field := splitOrigin(rm.Origin)
		p.add(newFinding(StatusNeedsAction, source, field, "", reason))
	}
}

// Fetched is a remote source checked out in a local directory.
type Fetched struct {
	// Dir holds the content the source's Path selects.
	Dir string
	// Commit is the full commit hash Dir is at.
	Commit string
	// RefKind is "tag", "branch", "head" or "commit".
	RefKind string
	// Skills are the skills found directly in Dir (or Dir itself), with their files.
	Skills []skillsource.Skill
}

// Fetcher checks a remote git source out. Convert fetches only with --fetch and
// only https sources; tests supply a fake.
type Fetcher interface {
	Fetch(ctx context.Context, rm Remote) (*Fetched, error)
}

// gitFetcher fetches through the skill-source resolver: the same clone limits,
// credential handling and cache as `[[skill_sources]]`, and a branch or tag is
// resolved to the commit that is then recorded.
type gitFetcher struct {
	// cacheDir overrides the cache root; empty uses the user cache.
	cacheDir string
}

func (g gitFetcher) Fetch(ctx context.Context, rm Remote) (*Fetched, error) {
	ref := rm.Ref
	if rm.Commit != "" {
		ref = rm.Commit // the input's own lock says exactly what it had fetched
	}
	spec := skillsource.Spec{Name: "convert-" + shortHash(rm.URL+"\x00"+rm.Path), URL: rm.URL, Ref: ref, Path: rm.Path}
	res, err := skillsource.Resolve(ctx, spec, skillsource.Options{CacheDir: g.cacheDir})
	if err != nil {
		return nil, oops.Wrapf(err, "resolve %s", rm.describe())
	}
	return &Fetched{Dir: res.Dir, Commit: res.Commit, RefKind: res.RefKind, Skills: res.Skills}, nil
}

// recordRef is the ref written into an [[installed_skills]] entry: a tag or a
// commit the input named is kept, anything that moves (a branch, the default
// branch) is replaced by the commit that was read.
func recordRef(rm Remote, f *Fetched) string {
	if fullSHA.MatchString(rm.Ref) || (rm.Ref != "" && f.RefKind == "tag") {
		return rm.Ref
	}
	return f.Commit
}

// resolveRemotes imports the remote sources the importers named. Without
// opt.Fetch it only reports them; with it every source is fetched, its content is
// planned like any other input (so the scan covers it) and the commit is recorded.
func (p *Plan) resolveRemotes(ctx context.Context, opt Options) error {
	if len(p.Remotes) == 0 {
		return nil
	}
	if !opt.Fetch {
		p.reportUnfetched()
		return nil
	}
	f := opt.Fetcher
	if f == nil {
		f = gitFetcher{}
	}
	for i := range p.Remotes {
		rm := p.Remotes[i]
		if !strings.HasPrefix(strings.ToLower(rm.URL), "https://") {
			source, field := splitOrigin(rm.Origin)
			p.add(newFinding(StatusNeedsAction, source, field, "",
				rm.describe()+" is not fetched: convert fetches https sources only; add the source by hand"))
			continue
		}
		fetched, err := f.Fetch(ctx, rm)
		if err != nil {
			return oops.With("source", rm.describe()).Hint("Fix the source in the input, or rerun without --fetch to import what is on disk").
				Wrapf(err, "fetch %s", rm.Origin)
		}
		if fetched.Commit != "" && !fullSHA.MatchString(fetched.Commit) {
			return oops.With("source", rm.describe()).Errorf("fetch %s: the resolved commit %q is not a full commit hash", rm.Origin, fetched.Commit)
		}
		switch rm.Kind {
		case remoteSkills:
			p.importRemoteSkills(rm, fetched, opt)
		case remoteRules:
			p.importRemoteRules(rm, fetched)
		case remotePackage:
			p.importRemotePackage(rm, fetched, opt)
		}
	}
	p.Remotes = nil
	return nil
}

// addInstalledSkill records one fetched skill as [[installed_skills]], pinned to
// the commit that was read, and keeps its text for the scan.
func (p *Plan) addInstalledSkill(rm Remote, f *Fetched, name, skillPath string, files map[string][]byte) {
	source, field := splitOrigin(rm.Origin)
	clean, _ := safeName(name)
	for i := range p.InstalledSkills {
		if p.InstalledSkills[i].Name == clean {
			p.add(newFinding(StatusApproximated, source, field, "installed_skills."+clean,
				"skill "+clean+" is already provided by an earlier source; the first one wins"))
			return
		}
	}
	skill := config.InstalledSkillConfig{Name: clean, Source: rm.URL, Ref: recordRef(rm, f)}
	if skillPath != "skills/"+clean {
		skill.Path = skillPath
	}
	if err := config.ValidateInstalledSkillFields(&skill); err != nil {
		p.add(newFinding(StatusUnsupported, source, field, "", redactedReason(err)))
		return
	}
	p.InstalledSkills = append(p.InstalledSkills, skill)
	p.addFetchedText(rm, f, skillPath, files)
	p.add(newFinding(StatusMapped, source, field, "installed_skills."+clean, "pinned to "+skill.Ref))
}

// addFetchedText keeps the text of fetched files, named by repository, commit and
// path, so the scan covers content that is referenced rather than copied.
func (p *Plan) addFetchedText(rm Remote, f *Fetched, dir string, files map[string][]byte) {
	if p.fetchedText == nil {
		p.fetchedText = map[string]string{}
	}
	for rel, data := range files {
		if isText(data) {
			p.fetchedText[rm.label(f.Commit)+":"+path.Join(dir, rel)] = string(data)
		}
	}
}

// importRemoteSkills turns the selected skills of a rulesync source into
// [[installed_skills]] entries.
func (p *Plan) importRemoteSkills(rm Remote, f *Fetched, opt Options) {
	source, field := splitOrigin(rm.Origin)
	if rm.Path == "" || rm.Path == "." || rm.Path == "./" {
		p.add(newFinding(StatusUnsupported, source, field, "",
			"a skill at the repository root cannot be an installed skill (its path would be the repository itself); copy it into skills/"))
		return
	}
	want := map[string]bool{}
	all := len(rm.Skills) == 0
	for _, n := range rm.Skills {
		if n == "*" {
			all = true
		}
		want[n] = true
	}
	found := map[string]bool{}
	for _, sk := range f.Skills {
		if !all && !want[sk.Dir] && !want[sk.Name] {
			continue
		}
		found[sk.Dir], found[sk.Name] = true, true
		files := map[string][]byte{}
		for _, fl := range sk.Files {
			files[fl.Path] = fl.Content
		}
		p.addInstalledSkill(rm, f, sk.Dir, path.Join(rm.Path, sk.Dir), files)
	}
	var missing []string
	for _, n := range rm.Skills {
		if n != "*" && !found[n] {
			missing = append(missing, n)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		p.add(newFinding(StatusNeedsAction, source, field, "",
			fmt.Sprintf("skills not found in %s at %s: %s", rm.describe(), rm.label(f.Commit), strings.Join(missing, ", "))))
	}
}

// importRemoteRules copies the selected .md rules of a rulesync source into the
// tree: rules are text, so the copy is what the scan reads; the source link is
// not kept (rulesync does the same with its .curated rules).
func (p *Plan) importRemoteRules(rm Remote, f *Fetched) {
	source, field := splitOrigin(rm.Origin)
	sub := &Plan{}
	b := &rulesyncPlanner{p: sub, r: newReader(os.DirFS(f.Dir)), seen: map[string]int{}}
	b.onSkip = func(name, reason string) { sub.add(newFinding(StatusDropped, name, "", "", reason)) }
	all := false
	want := map[string]bool{}
	for _, n := range rm.Rules {
		if n == "*" {
			all = true
		}
		want[strings.TrimSuffix(n, ".md")] = true
	}
	entries := b.r.dirEntries(".", b.onSkip)
	found := map[string]bool{}
	for _, e := range entries {
		name := strings.TrimSuffix(e.Name(), ".md")
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") || (!all && !want[name]) {
			continue
		}
		found[name] = true
		text, ok := b.readContent(e.Name())
		if !ok {
			continue
		}
		b.importRule(e.Name(), e.Name(), text)
	}
	var missing []string
	for n := range want {
		if !found[n] {
			missing = append(missing, n)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		p.add(newFinding(StatusNeedsAction, source, field, "",
			fmt.Sprintf("rules not found in %s at %s: %s", rm.describe(), rm.label(f.Commit), strings.Join(missing, ", "))))
	}
	b.r.flushProblems(sub)
	p.absorb(rm, f, sub, "imported as local files; the link to the source is not kept, so the rule no longer updates")
}

// absorb adds a plan built from a fetched tree: its paths are re-based onto the
// repository and commit they were read from, so a finding or a scan result names
// where the text really came from.
func (p *Plan) absorb(rm Remote, f *Fetched, sub *Plan, note string) {
	prefix := rm.label(f.Commit) + ":"
	if rm.Path != "" {
		prefix += strings.Trim(rm.Path, "/") + "/"
	}
	for i := range sub.Items {
		for j, s := range sub.Items[i].Sources {
			sub.Items[i].Sources[j] = prefix + strings.TrimPrefix(s, "./")
		}
	}
	for i := range sub.Findings {
		if sub.Findings[i].Source != "" && !strings.HasPrefix(sub.Findings[i].Source, "(") {
			sub.Findings[i].Source = prefix + sub.Findings[i].Source
		}
	}
	source, field := splitOrigin(rm.Origin)
	if len(sub.Items) > 0 {
		p.add(newFinding(StatusApproximated, source, field, "", fmt.Sprintf("%s at %s: %s", rm.describe(), rm.label(f.Commit), note)))
	}
	p.merge(sub)
}

// importRemotePackage imports a fetched APM package: a single skill becomes an
// [[installed_skills]] entry, anything else is copied like an installed package.
func (p *Plan) importRemotePackage(rm Remote, f *Fetched, opt Options) {
	r := newReader(os.DirFS(f.Dir))
	source, field := splitOrigin(rm.Origin)
	_, hasAPM := r.exists(apmDir)
	if _, ok := r.exists(litSkillMD); ok && !hasAPM && rm.Path != "" {
		data, err := r.read(litSkillMD)
		if err == nil {
			name := path.Base(rm.Path)
			if fm, _, has := splitFrontmatter(string(data)); has {
				meta, _ := parseFrontmatter(fm)
				if n := stringOf(meta[litName]); n != "" {
					name = n
				}
			}
			files := map[string][]byte{litSkillMD: data}
			for _, rel := range r.walkFiles(".", func(string, string) {}) {
				if rel == litSkillMD {
					continue
				}
				if res, rerr := r.read(rel); rerr == nil {
					files[rel] = res
				}
			}
			p.addInstalledSkill(rm, f, name, rm.Path, files)
			return
		}
	}
	sub := &Plan{}
	b := &apmPlanner{p: sub, r: r, opt: opt, lock: map[string]string{}, rootName: path.Base(strings.TrimSuffix(rm.URL, ".git"))}
	b.onSkip = func(name, reason string) { sub.add(newFinding(StatusDropped, name, "", "", reason)) }
	b.importPackage("", rm.URL, false)
	b.noteTransitive(rm)
	r.flushProblems(sub)
	if len(sub.Items) == 0 && len(sub.Hooks) == 0 {
		p.add(newFinding(StatusNeedsAction, source, field, "",
			fmt.Sprintf("%s at %s holds no .apm/ primitives, SKILL.md or skills/ to import", rm.describe(), rm.label(f.Commit))))
		return
	}
	p.absorb(rm, f, sub, "package copied as local files; the dependency link is not kept, so it no longer updates")
}

// noteTransitive reports the dependencies of a fetched package: they are not
// followed.
func (b *apmPlanner) noteTransitive(rm Remote) {
	doc, ok, err := b.readYAML(apmManifest)
	if err != nil || !ok {
		return
	}
	deps := as[map[string]any](doc["dependencies"])
	if list := as[[]any](deps["apm"]); len(list) > 0 {
		b.p.add(newFinding(StatusNeedsAction, apmManifest, "dependencies.apm", "",
			fmt.Sprintf("%d transitive dependenc(ies) of %s are not followed; add them to the project's apm.yml and rerun", len(list), rm.URL)))
	}
}
