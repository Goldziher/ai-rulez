package worktreeprovision

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/samber/oops"
)

// The machine-local inputs a provisioning run may carry without naming them:
// the overlay, the local content tree and the hand-authored instruction and
// settings files a harness reads beside the generated ones. Everything else is
// carried only when the caller names it in CopyExtra. The machine-local
// manifest of an earlier generate run is generated state, not authored input,
// and is never carried.
const (
	overlayFileName   = "config.local.toml"
	localContentDir   = "local"
	claudeLocalMD     = "CLAUDE.local.md"
	agentsLocalMD     = "AGENTS.local.md"
	agentsOverrideMD  = "AGENTS.override.md"
	geminiLocalMD     = "GEMINI.local.md"
	claudeSettingsRel = ".claude/settings.local.json"
	localManifestName = ".generated-manifest.local.json"
)

// Actions of a planned copy.
const (
	actionCopy      = ActionCopy
	actionIdentical = ActionIdentical
)

// copyFile is one regular file to carry, source and destination absolute. rel
// is the destination-relative slash path, for the plan and the git checks; the
// destination path mirrors it.
type copyFile struct {
	src, dst string
	rel      string
}

// provision runs a resolved provisioning: plan the copies, check them, and
// apply unless the run is a dry run. The plan is computed before the first
// write, so a failed check leaves the destination as it was.
func provision(ctx context.Context, w *worktrees, opts *Options) (*Result, error) {
	files, err := planCopies(ctx, w, opts)
	if err != nil {
		return nil, err
	}
	res := &Result{Source: w.src, Destination: w.dst, DryRun: opts.DryRun}
	blocked := conflicting(w, files)
	if len(blocked) > 0 {
		res.Conflicts = entries(blocked, actionConflict)
		return res, oops.Hint("keep your destination edits, or delete the destination file and re-run; nothing was written").
			Errorf("%d destination file(s) differ from what the source would carry:\n%s",
				len(blocked), strings.Join(fileLines(blocked), "\n"))
	}
	for _, f := range files {
		action := actionCopy
		if identical(f) {
			action = actionIdentical
		}
		if !opts.DryRun && action == actionCopy {
			if err := copyOne(f); err != nil {
				return res, oops.Hint("the run is retryable: fix the cause and run the command again; the worktree and its branch stay").
					Errorf("carry %s: %v", f.rel, err)
			}
		}
		res.Copies = append(res.Copies, CopyEntry{Source: f.rel, Destination: f.rel, Action: action})
	}
	if err := runGenerate(ctx, w, opts, res); err != nil {
		return res, err
	}
	writePlan(opts.Out, w, res)
	return res, nil
}

// runGenerate regenerates the destination's outputs from its own committed
// configuration plus the machine-local inputs the run carried. A dry run asks
// the caller's generate for its plan lines instead.
func runGenerate(ctx context.Context, w *worktrees, opts *Options, res *Result) error {
	if opts.Generate == nil {
		return oops.Hint("this is a bug in the caller, not in your configuration").
			Errorf("no generate step was provided")
	}
	lines, err := opts.Generate(ctx, w.dst, opts.DryRun)
	if err != nil {
		return oops.Hint("the run is retryable: fix the cause and run the command again; the worktree and its branch stay").
			Errorf("generate the destination outputs: %v", err)
	}
	res.Plan = lines
	if !opts.DryRun {
		res.Generated = true
	}
	return nil
}

// planCopies collects the files the run would carry: the allowlisted
// machine-local inputs when CopyLocal, plus the explicitly named extras. It
// refuses a target git tracks or does not ignore in the destination, a symlink
// anywhere on the way, and an extra that is missing or escapes the project.
func planCopies(ctx context.Context, w *worktrees, opts *Options) ([]copyFile, error) {
	if !opts.CopyLocal && len(opts.CopyExtra) == 0 {
		return nil, nil
	}
	if opts.CopyLocal && !compatibleLayouts(w) {
		return nil, oops.Hint("copy-local needs both checkouts to hold the configuration in the same layout").
			Errorf("the destination configuration layout differs: source state dir %s, destination %s",
				relStateDir(w.src, w.srcState), relStateDir(w.dst, w.dstState))
	}
	var files []copyFile
	if opts.CopyLocal {
		entries, err := collectAllowlist(w)
		if err != nil {
			return nil, err
		}
		files = append(files, entries...)
	}
	for _, extra := range opts.CopyExtra {
		collected, err := collectExtra(w, extra)
		if err != nil {
			return nil, err
		}
		files = append(files, collected...)
	}
	if err := checkDestination(ctx, w, files); err != nil {
		return nil, err
	}
	sort.Slice(files, func(i, j int) bool { return files[i].rel < files[j].rel })
	return files, nil
}

// collectAllowlist gathers the files of the machine-local inputs that exist in
// the source checkout. Missing entries are simply not carried.
func collectAllowlist(w *worktrees) ([]copyFile, error) {
	var files []copyFile
	for _, target := range allowlisted(w.src, w.srcState) {
		info, err := os.Lstat(target)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			continue
		case err != nil:
			return nil, oops.Wrapf(err, "read %s", relTo(w.src, target))
		}
		if info.IsDir() {
			sub, err := collectDir(w, target)
			if err != nil {
				return nil, err
			}
			files = append(files, sub...)
			continue
		}
		files = append(files, plainFile(w, target))
	}
	return files, nil
}

// allowlisted returns the absolute allowlisted paths of the source checkout:
// the overlay and the local content tree in the state directory, and the
// hand-authored instruction and settings files at the project root.
func allowlisted(src string, layout StateDirs) []string {
	state := filepath.Join(src, filepath.FromSlash(layout.StateDirRel))
	return []string{
		filepath.Join(state, overlayFileName),
		filepath.Join(state, localContentDir),
		filepath.Join(src, claudeLocalMD),
		filepath.Join(src, agentsLocalMD),
		filepath.Join(src, agentsOverrideMD),
		filepath.Join(src, geminiLocalMD),
		filepath.Join(src, filepath.FromSlash(claudeSettingsRel)),
	}
}

// collectExtra gathers one caller-named extra: it must be local to the
// project, exist, and be a regular file or directory, never a symlink.
func collectExtra(w *worktrees, extra string) ([]copyFile, error) {
	if !filepath.IsLocal(filepath.Clean(extra)) || strings.TrimSpace(extra) == "" {
		return nil, oops.Hint("name each extra as a repository-relative path, one per --copy-extra").
			Errorf("the extra %q is not a relative path inside the project", extra)
	}
	src := filepath.Join(w.src, filepath.FromSlash(filepath.Clean(extra)))
	info, err := os.Lstat(src)
	if err != nil {
		return nil, oops.Hint("extras must exist in the source checkout").
			Errorf("read the extra %s: %v", extra, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, oops.Hint("a symlinked extra is never carried; name the file it points at instead").
			Errorf("the extra %s is a symlink", extra)
	}
	if info.IsDir() {
		return collectDir(w, src)
	}
	return []copyFile{plainFile(w, src)}, nil
}

// collectDir walks one directory of the source, gathering regular files and
// refusing a symlink on the way: a link could redirect the copy outside the
// worktree.
func collectDir(w *worktrees, dir string) ([]copyFile, error) {
	var files []copyFile
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("%s is a symlink", relTo(w.src, path))
		}
		if d.IsDir() {
			return nil
		}
		if !d.Type().IsRegular() {
			return fmt.Errorf("%s is not a regular file", relTo(w.src, path))
		}
		files = append(files, plainFile(w, path))
		return nil
	})
	if err != nil {
		return nil, oops.Hint("nothing inside a linked worktree is followed through a link").
			Errorf("collect %s: %v", relTo(w.src, dir), err)
	}
	return files, nil
}

// plainFile records one regular file to carry, addressed relatively on both
// sides: the destination path mirrors the source path.
func plainFile(w *worktrees, src string) copyFile {
	rel := relTo(w.src, src)
	return copyFile{src: src, dst: filepath.Join(w.dst, filepath.FromSlash(rel)), rel: rel}
}

// checkDestination refuses a copy target the destination's git tracks or does
// not ignore: a tracked path would stage a machine-local file, and an
// unignored one would pollute `git status` of every checkout of the branch.
func checkDestination(ctx context.Context, w *worktrees, files []copyFile) error {
	if len(files) == 0 {
		return nil
	}
	rels := make([]string, len(files))
	for i, f := range files {
		rels[i] = f.rel
	}
	ignored, err := w.git.IgnoredAmongContext(ctx, w.dst, rels)
	if err != nil {
		return oops.Wrapf(err, "check the destination ignore rules")
	}
	var offending []string
	for _, f := range files {
		switch {
		case w.git.IsTrackedContext(ctx, f.dst):
			offending = append(offending, f.rel+" is tracked by git in the destination")
		case !ignored[f.rel]:
			offending = append(offending, f.rel+" is not ignored by git in the destination")
		}
	}
	if len(offending) > 0 {
		return oops.Hint("the managed .gitignore block carries the machine-local inputs; run `ai-rulez generate` in the source and commit it, or ignore the paths yourself").
			Errorf("the destination cannot take these local files:\n%s", strings.Join(offending, "\n"))
	}
	return nil
}

// conflicting are the files a destination already holds with different
// content. The destination's own edits are never overwritten.
func conflicting(w *worktrees, files []copyFile) []copyFile {
	var out []copyFile
	for _, f := range files {
		if identical(f) {
			continue
		}
		if _, err := os.Lstat(f.dst); err == nil {
			out = append(out, f)
		}
	}
	return out
}

// identical reports whether the destination already holds the same bytes.
func identical(f copyFile) bool {
	existing, err := os.ReadFile(f.dst) //nolint:gosec // the destination's own file
	if err != nil {
		return false // absent or unreadable: it is a copy or a copy error, not a match
	}
	want, err := os.ReadFile(f.src) //nolint:gosec // the source's own file
	if err != nil {
		return false
	}
	return bytes.Equal(existing, want)
}

// copyOne carries one regular file with owner-only permissions, creating the
// parent directories the same way. The content is what the source holds.
func copyOne(f copyFile) error {
	if err := os.MkdirAll(filepath.Dir(f.dst), 0o700); err != nil {
		return fmt.Errorf("create the directory: %w", err)
	}
	data, err := os.ReadFile(f.src) //nolint:gosec // the source's own file
	if err != nil {
		return fmt.Errorf("read: %w", err)
	}
	if err := os.WriteFile(f.dst, data, 0o600); err != nil { //nolint:gosec // owner-only by policy
		return fmt.Errorf("write: %w", err)
	}
	return nil
}

func fileLines(files []copyFile) []string {
	lines := make([]string, len(files))
	for i, f := range files {
		lines[i] = "  " + f.rel
	}
	return lines
}

func entries(files []copyFile, action string) []CopyEntry {
	out := make([]CopyEntry, len(files))
	for i, f := range files {
		out[i] = CopyEntry{Source: f.rel, Destination: f.rel, Action: action}
	}
	return out
}

func relTo(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(rel)
}

// writePlan prints the human-readable plan: what was or would be carried, and
// the generate lines (a dry run's plan, a real run's output).
func writePlan(out io.Writer, w *worktrees, res *Result) {
	if out == nil {
		return
	}
	verb := "Provisioned"
	if res.DryRun {
		verb = "Would provision"
	}
	if len(res.Copies) > 0 {
		fmt.Fprintf(out, "%s %s from %s\n", verb, relTo(w.top, w.dst), relTo(w.top, w.src)) //nolint:errcheck // a plan line
		for _, e := range res.Copies {
			fmt.Fprintf(out, "  %s %s\n", e.Action, e.Destination) //nolint:errcheck // a plan line
		}
	} else {
		fmt.Fprintf(out, "%s %s (no machine-local inputs to carry)\n", verb, relTo(w.top, w.dst)) //nolint:errcheck // a plan line
	}
	for _, line := range res.Plan {
		fmt.Fprintln(out, line) //nolint:errcheck // a plan line
	}
}

const actionConflict = "conflict"
