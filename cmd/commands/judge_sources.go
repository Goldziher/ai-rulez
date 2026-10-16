package commands

import (
	"context"
	"errors"
	"math"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	"github.com/Goldziher/ai-rulez/v5/internal/judge"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/Goldziher/ai-rulez/v5/internal/safefs"
)

const (
	// judgeMaxUnits bounds one run: every unit is up to four paid model calls.
	judgeMaxUnits = 200
	// judgeWalkMaxEntries and judgeWalkMaxDepth bound the directory walk of a directory that is not a
	// git repository.
	judgeWalkMaxEntries = 100_000
	judgeWalkMaxDepth   = 8
	// judgeMaxScanUnits bounds the units read to resolve --impact; past it the run says so and
	// narrows to the changed units.
	judgeMaxScanUnits = 2000
)

// judgeWorkspace is the project a run reads: its root, every candidate file and a safe reader.
type judgeWorkspace struct {
	root       string
	cwd        string
	tree       *lint.Tree
	candidates []string
	inGit      bool
}

// openJudgeProject locates the project of the working directory. It reads no configuration: with no
// paths it takes the files that follow an agent convention, so it works in a repository that has no
// ai-rulez config at all and in one that has.
func openJudgeWorkspace(ctx context.Context, cwd string, named bool, lim lint.WalkLimits) (*judgeWorkspace, error) {
	cwd = gitutil.Resolve(cwd) // the tree top is symlink-resolved, so the working directory must be too
	inGit := (gitutil.Git{}).TopLevelContext(ctx, cwd) != ""
	if !inGit {
		if broadRoot(cwd) {
			return nil, oops.
				Hint("Run it from inside a project directory").
				Errorf("refusing to judge everything below %s: it is not a project (no git repository)", cwd)
		}
		if !named && !hasJudgeProjectMarker(cwd) {
			return nil, oops.
				Hint("Name the files or directories to judge, for example: ai-rulez judge docs/guide.md").
				Errorf("refusing default discovery below %s: it is not a git repository and has no AGENTS.md, CLAUDE.md, .claude/, .agents/ or ai-rulez config at its root, so name the files or directories to judge", cwd)
		}
	}
	if named {
		lim.MaxDepth = 0 // a named file is judged at any depth
	}
	tree, err := lint.LoadTreeLimited(ctx, gitutil.Git{}, cwd, lim)
	if err != nil {
		return nil, oops.Wrapf(err, "index the project below %s", cwd)
	}
	return &judgeWorkspace{root: tree.Top, cwd: cwd, tree: tree, candidates: lint.Candidates(ctx, tree), inGit: inGit}, nil
}

// hasJudgeProjectMarker reports whether dir looks like a project root of its own.
func hasJudgeProjectMarker(dir string) bool {
	for _, name := range []string{config.AgentsMDFilename, judgeClaudeMD, ".claude", ".agents", ".ai-rulez/config.toml", ".config/ai-rulez/config.toml"} {
		if _, err := os.Lstat(filepath.Join(dir, filepath.FromSlash(name))); err == nil {
			return true
		}
	}
	return false
}

// rel returns the slash path of abs relative to the project root, and whether it stays inside.
func (p *judgeWorkspace) rel(abs string) (string, bool) {
	r, err := filepath.Rel(p.root, gitutil.Resolve(abs))
	if err != nil || safefs.RelEscapes(r) {
		return "", false
	}
	return filepath.ToSlash(r), true
}

// under lists the candidates below rel ("" is the whole project).
func (p *judgeWorkspace) under(rel string) []string {
	if rel == "." || rel == "" {
		return p.candidates
	}
	var out []string
	for _, c := range p.candidates {
		if c == rel || strings.HasPrefix(c, rel+"/") {
			out = append(out, c)
		}
	}
	return out
}

// resolveJudgeRefs picks the units of a run. With no paths: every unit below the working directory
// that follows a convention, nested .claude directories at any depth included. A named file or
// directory takes whatever Markdown or JSON file it holds, and a file inside a skill selects the
// whole skill.
func (p *judgeWorkspace) resolveRefs(paths []string) ([]judge.Ref, error) {
	cwdRel, ok := p.rel(p.cwd)
	if !ok {
		return nil, oops.Errorf("the working directory %s is outside the project", p.cwd)
	}
	if len(paths) == 0 {
		return judge.Discover(p.under(cwdRel), false), nil
	}
	selected, err := p.selectPaths(paths)
	if err != nil {
		return nil, err
	}
	refs := judge.Discover(p.withWholeSkills(selected), true)
	if len(refs) == 0 {
		return nil, oops.Hint("Judge takes agent instruction files (skills, agents, commands, rules, CLAUDE.md, AGENTS.md, MCP and settings files) and Markdown or JSON files named explicitly").
			Errorf("nothing to judge: no judgeable file matches %s", strings.Join(paths, ", "))
	}
	return refs, nil
}

// selectPaths turns the named files and directories into project-relative file paths.
func (p *judgeWorkspace) selectPaths(paths []string) (map[string]bool, error) {
	selected := map[string]bool{}
	for _, arg := range paths {
		abs := arg
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(p.cwd, arg)
		}
		info, err := os.Lstat(abs)
		if err != nil {
			return nil, oops.Errorf("path %q does not exist", arg)
		}
		if link, through := p.throughSymlink(abs); through {
			return nil, oops.Hint("Name the real path; judge never follows a symlink, in the path or below it").
				Errorf("path %q goes through the symlink %s", arg, link)
		}
		rel, inside := p.rel(abs)
		if !inside {
			return nil, oops.Hint("judge reads files inside the project; symlinks are never followed").Errorf("path %q is outside the project root %s", arg, p.root)
		}
		if info.IsDir() {
			for _, c := range p.under(rel) {
				selected[c] = true
			}
			continue
		}
		selected[rel] = true
	}
	return selected, nil
}

// throughSymlink reports whether abs, or any directory above it inside the project, is a symlink.
// A spelling of the project root itself (a symlinked prefix of its real path) is not one.
func (p *judgeWorkspace) throughSymlink(abs string) (link string, through bool) {
	for q := filepath.Clean(abs); ; q = filepath.Dir(q) {
		if q == p.root {
			return "", false
		}
		fi, err := os.Lstat(q)
		if err == nil && fi.Mode()&os.ModeSymlink != 0 {
			inside := strings.HasPrefix(q, p.root+string(filepath.Separator))
			if inside || gitutil.Resolve(q) != p.root {
				return q, true
			}
			return "", false
		}
		if gitutil.Resolve(q) == p.root {
			return "", false
		}
		if parent := filepath.Dir(q); parent == q {
			return "", false
		}
	}
}

// withWholeSkills returns the selected files plus every file of a skill any selected file belongs to.
func (p *judgeWorkspace) withWholeSkills(selected map[string]bool) []string {
	skillDirs := map[string]bool{}
	for _, c := range p.candidates {
		if path.Base(c) == judgeSkillFile {
			skillDirs[path.Dir(c)] = true
		}
	}
	whole := map[string]bool{}
	for f := range selected {
		for d := path.Dir(f); ; d = path.Dir(d) {
			if skillDirs[d] {
				whole[d] = true
				break
			}
			if d == "." || d == "/" {
				break
			}
		}
	}
	files := make([]string, 0, len(selected))
	for f := range selected {
		files = append(files, f)
	}
	for _, c := range p.candidates {
		if selected[c] {
			continue
		}
		for d := range whole {
			if strings.HasPrefix(c, d+"/") {
				files = append(files, c)
				break
			}
		}
	}
	return files
}

// readFile reads one file of the project without following a symlink, refusing one over the limit.
func (p *judgeWorkspace) readFile(rel string) judge.SourceFile {
	sf := judge.SourceFile{Path: rel}
	abs := filepath.Join(p.root, filepath.FromSlash(rel))
	f, err := safefs.OpenRegular(abs)
	if err != nil {
		sf.Skip, sf.SkipReason = judge.SkipUnreadable, "not read: it is a symlink or not a regular file (symlinks are never followed)"
		return sf
	}
	defer f.Close() //nolint:errcheck // read-only
	data, err := safefs.ReadLimited(f, judge.MaxFileBytes)
	switch {
	case errors.Is(err, safefs.ErrTooLarge):
		sf.Skip, sf.SkipReason, sf.Bytes = judge.SkipUnreadable, "not read: the file is larger than 2 MiB", judge.MaxFileBytes+1
	case err != nil:
		sf.Skip, sf.SkipReason = judge.SkipUnreadable, "not read: "+err.Error()
	default:
		sf.Data, sf.Bytes = data, int(min(int64(len(data)), math.MaxInt32))
	}
	return sf
}

// repo adapts the project for the attachment of conventions.
func (p *judgeWorkspace) repo() judge.Repo {
	return judge.Repo{Files: p.candidates, Read: func(rel string) (string, bool) {
		sf := p.readFile(rel)
		if sf.Skip != "" {
			return "", false
		}
		return string(sf.Data), true
	}}
}

// buildUnits reads the refs into units.
func (p *judgeWorkspace) buildUnits(refs []judge.Ref, opts judge.UnitOptions) []judge.Unit {
	units := make([]judge.Unit, 0, len(refs))
	for _, r := range refs {
		units = append(units, judge.BuildUnit(r, p.readFile, opts))
	}
	return units
}

// changedRefs narrows refs to the units that changed since base (committed, staged, unstaged and
// untracked work), and with impact adds the units a changed rule or instruction file is attached to,
// which a model has to see again though they did not change.
func (p *judgeWorkspace) changedRefs(ctx context.Context, refs []judge.Ref, base string, impact bool, opts judge.UnitOptions, attach judge.AttachOptions) (selected []judge.Ref, info judge.ChangedInfo, err error) {
	info = judge.ChangedInfo{Base: base, Impact: impact}
	if !p.inGit {
		return nil, info, oops.Hint("--changed compares with git; name the files to judge instead").Errorf("--changed needs a git repository")
	}
	changedList, err := (gitutil.Git{}).ChangedSinceContext(ctx, p.root, base)
	if err != nil {
		return nil, info, oops.Wrapf(err, "list the files changed since %s", base)
	}
	changed := make(map[string]bool, len(changedList))
	for _, c := range changedList {
		changed[c] = true
	}
	picked := map[string]bool{}
	for _, r := range refs {
		for _, m := range r.Members {
			if changed[m] {
				picked[r.Path] = true
				info.ChangedUnits++
				break
			}
		}
	}
	if impact && len(changed) > 0 && (attach.Rules || attach.ClaudeMD) {
		repo := p.impactRepo(ctx, base, changed)
		impacted := p.impacted(repo, refs, changed, picked, opts, attach)
		for _, path := range impacted {
			picked[path] = true
		}
		info.ImpactedUnits = len(impacted)
	}
	for _, r := range refs {
		if picked[r.Path] {
			selected = append(selected, r)
		}
	}
	return selected, info, nil
}

// impactRepo restores missing Markdown files at the comparison's merge base only for impact
// discovery. A deleted rule, instruction file or imported document can still change which units
// need review. The actual review keeps using p.repo(), so deleted text never reaches a model.
func (p *judgeWorkspace) impactRepo(ctx context.Context, base string, changed map[string]bool) judge.Repo {
	repo := p.repo()
	var missing []string
	for rel := range changed {
		if !strings.HasSuffix(rel, ".md") {
			continue
		}
		if _, err := os.Lstat(filepath.Join(p.root, filepath.FromSlash(rel))); os.IsNotExist(err) {
			missing = append(missing, rel)
		}
	}
	if len(missing) == 0 {
		return repo
	}
	git := gitutil.Git{}
	mergeBase, err := git.MergeBaseContext(ctx, p.root, base)
	if err != nil {
		mergeBase = base // ChangedSinceContext also falls back to base in a shallow or unrelated history.
	}
	files := append([]string(nil), repo.Files...)
	known := make(map[string]bool, len(files))
	for _, rel := range files {
		known[rel] = true
	}
	restored := map[string]string{}
	sort.Strings(missing)
	for _, rel := range missing {
		data, ok := git.ShowFileContext(ctx, p.root, mergeBase, rel)
		if !ok || len(data) > judge.MaxFileBytes {
			continue
		}
		restored[rel] = string(data)
		if !known[rel] {
			files = append(files, rel)
		}
	}
	read := repo.Read
	repo.Files = files
	repo.Read = func(rel string) (string, bool) {
		if text, ok := restored[rel]; ok {
			return text, true
		}
		return read(rel)
	}
	return repo
}

// impacted returns the paths of the units, not changed themselves, to which a changed convention
// attaches.
func (p *judgeWorkspace) impacted(repo judge.Repo, refs []judge.Ref, changed, picked map[string]bool, opts judge.UnitOptions, attach judge.AttachOptions) []string {
	if !anyConventionChanged(changed) || len(refs) > judgeMaxScanUnits {
		return nil
	}
	var out []string
	for _, r := range refs {
		if picked[r.Path] {
			continue
		}
		u := judge.BuildUnit(r, p.readFile, opts)
		if u.Status != "" && u.Status != judge.StatusWithheld {
			continue
		}
		if attachesChanged(judge.Attach(repo, u, attach).Conventions, changed) {
			out = append(out, r.Path)
		}
	}
	sort.Strings(out)
	return out
}

func anyConventionChanged(changed map[string]bool) bool {
	for c := range changed {
		base := path.Base(c)
		if strings.Contains(c, "/.claude/rules/") || strings.HasPrefix(c, ".claude/rules/") || base == judgeClaudeMD || base == "CLAUDE.local.md" || base == "AGENTS.md" || strings.HasSuffix(c, ".md") {
			return true
		}
	}
	return false
}

func attachesChanged(conventions []judge.Convention, changed map[string]bool) bool {
	for i := range conventions {
		if changed[conventions[i].Path] {
			return true
		}
	}
	return false
}
