package commands

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	"github.com/Goldziher/ai-rulez/v5/internal/judge"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/Goldziher/ai-rulez/v5/internal/safefs"
)

const (
	// judgeMaxFiles bounds one run: every file is a paid call.
	judgeMaxFiles = 200
	// judgeMaxReadBytes is the largest file read at all; a longer one is skipped,
	// and a file under it is cut to its head and tail before it is sent.
	judgeMaxReadBytes = 2 << 20
	// judgeWalkMaxEntries and judgeWalkMaxDepth bound the directory walk of a
	// directory that is not a git repository; the depth applies to default discovery only.
	judgeWalkMaxEntries = 100_000
	judgeWalkMaxDepth   = 8
)

// judgeSource is a file chosen for judging.
type judgeSource struct {
	rel, abs string
	// skip is a judge.Skip* kind and reason the file is not read at all: the
	// project scan already left it out (generated, over 2 MiB, binary).
	skip, reason string
}

// judgeConventionRoots are the places, besides AGENTS.md at any depth, whose
// Markdown a run without paths judges.
func judgeConventionRoots() []string {
	return []string{".agents/", ".ai-rulez/", ".config/ai-rulez/"}
}

// resolveJudgeSources picks the files to judge and returns the project root they
// are relative to. It reads no configuration: with no paths it takes AGENTS.md at
// any depth and the .agents/ and .ai-rulez/ content by convention, so it works in
// a repository that has no ai-rulez config at all and in one that has. Named
// paths take any Markdown file or directory inside the project. Generated files
// are skipped: judge the sources.
func resolveJudgeSources(ctx context.Context, cwd string, paths []string, lim lint.WalkLimits) (root string, files []judgeSource, err error) {
	cwd = gitutil.Resolve(cwd) // the tree top is symlink-resolved, so the working directory must be too
	if (gitutil.Git{}).TopLevelContext(ctx, cwd) == "" && broadRoot(cwd) {
		return "", nil, oops.
			Hint("Run it from inside a project directory").
			Errorf("refusing to judge everything below %s: it is not a project (no git repository)", cwd)
	}
	if len(paths) > 0 {
		lim.MaxDepth = 0 // a named file is judged at any depth
	}
	tree, err := lint.LoadTreeLimited(ctx, gitutil.Git{}, cwd, lim)
	if err != nil {
		return "", nil, oops.Wrapf(err, "index the project below %s", cwd)
	}
	targets, err := lint.ResolveTargets(ctx, tree, judgeTargetSpec(tree, cwd, paths))
	if err != nil {
		return "", nil, oops.Wrapf(err, "resolve what to judge")
	}
	seen := map[string]bool{}
	for _, f := range targets.Files {
		if f.Content == nil || seen[f.Rel] || (len(paths) == 0 && !isDefaultJudgeTarget(f.Rel)) {
			continue
		}
		seen[f.Rel] = true
		files = append(files, judgeSource{rel: f.Rel, abs: f.Abs})
	}
	if err := checkJudgeSourceCount(len(files), cwd, paths); err != nil {
		return "", nil, err
	}
	files = append(files, skippedJudgeSources(targets, paths, seen)...)
	sort.Slice(files, func(i, j int) bool { return files[i].rel < files[j].rel })
	return tree.Top, files, nil
}

// skippedJudgeSources are the picked files the project scan left out, with the
// reason: a file that cannot be read or is too large, a binary file, and (for
// named paths and directories) a generated file. A run with no paths leaves a
// generated file out silently, since the project's own AGENTS.md may be one.
func skippedJudgeSources(targets *lint.Targets, paths []string, seen map[string]bool) []judgeSource {
	var out []judgeSource
	add := func(rel, kind, reason string) {
		if !seen[rel] {
			seen[rel] = true
			out = append(out, judgeSource{rel: rel, skip: kind, reason: reason})
		}
	}
	for _, s := range targets.Skipped {
		kind := judge.SkipUnreadable
		switch {
		case s.Reason == "binary file":
			kind = judge.SkipBinary
		case strings.HasPrefix(s.Reason, "generated file"):
			kind = judge.SkipGenerated
		}
		if len(paths) > 0 || kind == judge.SkipUnreadable {
			add(s.Rel, kind, "not judged: "+s.Reason)
		}
	}
	if len(paths) > 0 {
		for _, rel := range targets.GeneratedRels {
			add(rel, judge.SkipGenerated, "not judged: generated file (judge its sources)")
		}
	}
	return out
}

// judgeTargetSpec is what the lint target discovery is asked for: the named
// paths, or by convention AGENTS.md and the agent content directories below the
// working directory.
func judgeTargetSpec(tree *lint.Tree, cwd string, paths []string) lint.TargetSpec {
	spec := lint.TargetSpec{Cwd: cwd, Paths: paths, Lenient: true}
	if len(paths) > 0 {
		return spec
	}
	spec.Detect = true
	if rel, err := filepath.Rel(tree.Top, cwd); err == nil && rel != "." {
		spec.Under = filepath.ToSlash(rel)
	}
	for _, dir := range []string{defaultConfigDirName, ".config/ai-rulez"} {
		abs := filepath.Join(tree.Top, filepath.FromSlash(dir))
		if info, err := os.Lstat(abs); err == nil && info.IsDir() {
			spec.Paths = append(spec.Paths, abs)
		}
	}
	return spec
}

// checkJudgeSourceCount refuses an empty selection or one over the per-run limit.
func checkJudgeSourceCount(n int, cwd string, paths []string) error {
	switch {
	case n == 0 && len(paths) == 0:
		return oops.Hint("Name the files or directories to judge, for example: ai-rulez judge docs/guide.md").
			Errorf("nothing to judge: found no AGENTS.md and no .agents/ or .ai-rulez/ content below %s", cwd)
	case n == 0:
		return oops.Hint("Judge takes Markdown instruction files; a file that carries the ai-rulez generated header is skipped, so name its sources").
			Errorf("nothing to judge: no judgeable Markdown file matches %s", strings.Join(paths, ", "))
	case n > judgeMaxFiles:
		return oops.Hint("Name fewer paths; every file is one paid model call").
			Errorf("%d files match; the limit is %d per run", n, judgeMaxFiles)
	}
	return nil
}

// isDefaultJudgeTarget reports whether rel is judged when no path is named.
func isDefaultJudgeTarget(rel string) bool {
	if rel == "AGENTS.md" || strings.HasSuffix(rel, "/AGENTS.md") {
		return true
	}
	for _, root := range judgeConventionRoots() {
		if strings.HasPrefix(rel, root) {
			return true
		}
	}
	return false
}

// prepareJudgeFiles reads each source without following a symlink and screens it
// for sending. A file that cannot be read safely is skipped with the reason, never
// sent.
func prepareJudgeFiles(sources []judgeSource, redact bool, secrets []string) []judge.Prepared {
	out := make([]judge.Prepared, 0, len(sources))
	for _, s := range sources {
		if s.skip != "" {
			out = append(out, judge.Prepared{Path: judge.SafePath(s.rel, secrets), Skip: s.skip, SkipReason: s.reason})
			continue
		}
		data, reason := readJudgeSource(s.abs)
		if reason != "" {
			out = append(out, judge.Prepared{Path: judge.SafePath(s.rel, secrets), Skip: judge.SkipUnreadable, SkipReason: reason})
			continue
		}
		out = append(out, judge.Prepare(s.rel, data, judge.PrepareOptions{MaxBytes: judge.DefaultMaxFileBytes, Redact: redact, Secrets: secrets}))
	}
	return out
}

// readJudgeSource reads one file, refusing a symlink and a file over the limit.
// The reason is empty on success.
func readJudgeSource(abs string) (data []byte, reason string) {
	f, err := safefs.OpenRegular(abs)
	if err != nil {
		return nil, "not read: it is a symlink or not a regular file (symlinks are never followed)"
	}
	defer f.Close() //nolint:errcheck // read-only
	data, err = safefs.ReadLimited(f, judgeMaxReadBytes)
	switch {
	case errors.Is(err, safefs.ErrTooLarge):
		return nil, "not read: the file is larger than 2 MiB"
	case err != nil:
		return nil, "not read: " + err.Error()
	}
	return data, ""
}

// loadJudgeRubric returns the rubric --rubric names: a built-in (builtin:<name>)
// or a file, read without following a symlink. Empty is the default rubric.
func loadJudgeRubric(spec string, secrets []string) (judge.Rubric, error) {
	switch {
	case spec == "":
		return judge.BuiltinRubric(judge.RubricInstructionQuality) //nolint:wrapcheck // typed config error
	case strings.HasPrefix(spec, judge.BuiltinPrefix):
		return judge.BuiltinRubric(strings.TrimPrefix(spec, judge.BuiltinPrefix)) //nolint:wrapcheck // typed config error
	}
	f, err := safefs.OpenRegular(spec)
	if err != nil {
		return judge.Rubric{}, oops.Wrapf(err, "read --rubric %s", spec)
	}
	defer f.Close() //nolint:errcheck // read-only
	data, err := safefs.ReadLimited(f, judge.MaxRubricBytes)
	if err != nil {
		return judge.Rubric{}, oops.Wrapf(err, "read --rubric %s", spec)
	}
	return judge.NewRubric("file:"+spec, string(data), secrets...) //nolint:wrapcheck // typed config error
}
