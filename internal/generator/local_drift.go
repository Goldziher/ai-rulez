package generator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/gitignore"
	"github.com/Goldziher/ai-rulez/internal/gitutil"
	"github.com/Goldziher/ai-rulez/internal/logger"
	"github.com/Goldziher/ai-rulez/internal/templates"
)

// localPlan is the outcome of comparing what this machine would generate (shared
// config plus local overlay and content) with what a teammate without the local
// inputs generates (the shared baseline). Paths are slash-separated and relative
// to the project root.
type localPlan struct {
	baselineHash string
	localHash    string

	// localOnly: files only the merged render produces.
	localOnly []string
	// machineLocal: the localOnly files that come from the overlay (not from the
	// local/ content tree); they are ignored through .git/info/exclude because
	// the set differs per machine.
	machineLocal map[string]bool
	// inGitignore: machineLocal files that must go to the managed .gitignore
	// block instead, because no exclude file can hold them (not a repository, or
	// a path outside the work tree).
	inGitignore map[string]bool
	// drift: files both renders produce with different content.
	drift []string
	// suppressed: files only the baseline produces; they are left alone.
	suppressed []string
	// baselineFiles: every file the baseline would own, for the shared manifest.
	baselineFiles []string
	// violations: drift or local-only files that are tracked or not ignored.
	violations []localViolation
}

type localViolation struct {
	path   string
	reason string
}

// SetAllowLocalDrift lets generation write merged output over shared files even
// when a local override would change them. It is deliberately a Generator
// option, not something remote callers can set: with it, overlay values (MCP
// env, headers) may be written into files that are tracked by git.
func (g *Generator) SetAllowLocalDrift(allow bool) { g.allowLocalDrift = allow }

// SetContext sets the context used for the shared-baseline load, so a caller's
// cancellation and offline-include request (config.WithOfflineIncludes) apply to it.
func (g *Generator) SetContext(ctx context.Context) { g.ctx = ctx }

func (g *Generator) context() context.Context {
	if g.ctx != nil {
		return g.ctx
	}
	return context.Background()
}

// outputIndex maps relative path to output for files (directories skipped).
func (g *Generator) outputIndex(outputs []config.OutputFile) map[string]config.OutputFile {
	index := make(map[string]config.OutputFile, len(outputs))
	for _, o := range outputs {
		if o.IsDir {
			continue
		}
		index[filepath.ToSlash(g.convertToRelativePath(g.absOutputPath(o.Path)))] = o
	}
	return index
}

// planLocal renders the shared baseline and classifies the merged outputs. It
// returns nil when no machine-local input contributes to this run. merged is
// updated in place: LocalOnly flags and per-output Source-Hash overrides.
func (g *Generator) planLocal(profile string, merged []config.OutputFile) (*localPlan, error) {
	g.plan = nil
	g.localSkipped = false
	if !g.config.HasLocalInputs() {
		g.localSkipped = g.localInputsOnDisk()
		return nil, nil
	}
	if g.config.GeneratedAt.IsZero() {
		g.config.GeneratedAt = config.ResolveGenerationTime()
	}

	baseline, baselineHash, err := g.renderBaseline(profile)
	if err != nil {
		err = redactCredentials(err)
		// Fail closed even with --allow-local-drift: without the baseline,
		// overlay-derived outputs cannot be told apart from shared ones.
		return nil, oops.
			Hint("Fix the shared config, or run with --no-local to generate the shared view").
			Wrapf(err, "render the shared baseline to detect local drift (fix the shared config or use --no-local)")
	}

	localHash, err := g.localSourceHash(baselineHash)
	if err != nil {
		return nil, err
	}
	plan := &localPlan{
		baselineHash: baselineHash, localHash: localHash,
		machineLocal: map[string]bool{}, inGitignore: map[string]bool{},
	}
	g.classify(plan, baseline, merged)
	g.stampSourceHashes(plan, merged)
	plan.violations = g.findViolations(plan, merged)
	g.plan = plan
	return plan, nil
}

// renderBaseline renders the shared view (no overlay, no local content) from a
// separately decoded config: MCP resolution mutates servers in place, so a copy
// of the merged config would be wrong. Include fetches are shared through the
// config's memo, and unresolved MCP placeholders are tolerated because the
// secrets they name may only be needed by local servers.
func (g *Generator) renderBaseline(profile string) ([]config.OutputFile, string, error) {
	if g.config.ConfigDir == "" || g.config.ConfigFile == "" {
		return nil, "", oops.Errorf("the config file location is unknown")
	}
	cfgPath := filepath.Join(g.config.ConfigDir, g.config.ConfigFile)
	cfg, err := config.LoadConfigFromFile(g.context(), cfgPath,
		config.WithoutLocal(), config.WithIncludeMemo(g.config.IncludeMemo))
	if err != nil {
		return nil, "", err //nolint:wrapcheck // wrapped by planLocal
	}
	cfg.MCPEnvOverrides = g.config.MCPEnvOverrides
	cfg.MCPEnvFiles = g.config.MCPEnvFiles
	cfg.GeneratedAt = g.config.GeneratedAt

	bg := NewGenerator(cfg)
	bg.ctx = g.ctx
	bg.lenientMCP = true
	outputs, _, err := bg.collectOutputs(profile)
	if err != nil {
		return nil, "", err
	}
	return outputs, bg.config.SourceHash, nil
}

var urlCredentialsRe = regexp.MustCompile(`(?i)\b([a-z][a-z0-9+.-]*://)[^/\s@]+@`)

// redactCredentials returns an error whose text has URL credentials removed, for
// errors that may quote a repository address.
func redactCredentials(err error) error {
	text := err.Error()
	redacted := urlCredentialsRe.ReplaceAllString(text, "${1}<redacted>@")
	if redacted == text {
		return err
	}
	return oops.Errorf("%s", redacted)
}

// classify fills the plan from the baseline and merged outputs.
func (g *Generator) classify(plan *localPlan, baseline, merged []config.OutputFile) {
	base := g.outputIndex(baseline)
	mine := g.outputIndex(merged)

	for rel, b := range base {
		if !b.PartiallyOwned {
			plan.baselineFiles = append(plan.baselineFiles, rel)
		}
		if _, ok := mine[rel]; !ok {
			plan.suppressed = append(plan.suppressed, rel)
		}
	}
	for i := range merged {
		o := &merged[i]
		if o.IsDir {
			continue
		}
		rel := filepath.ToSlash(g.convertToRelativePath(g.absOutputPath(o.Path)))
		b, inBase := base[rel]
		switch {
		case !inBase:
			plan.localOnly = append(plan.localOnly, rel)
			// A ".local." file name (CLAUDE.local.md, <rulesdir>/x.local.md) is
			// covered by a stable pattern in the shared .gitignore. Anything else
			// (a local skill's SKILL.md, an overlay preset's output) is named after
			// machine-local content, so it is excluded per clone instead.
			if !o.LocalOnly || !stableLocalName(rel) {
				plan.machineLocal[rel] = true
			}
			o.LocalOnly = true
		case !sameOutput(b, *o):
			plan.drift = append(plan.drift, rel)
		}
	}
	for _, list := range [][]string{plan.baselineFiles, plan.localOnly, plan.drift, plan.suppressed} {
		sort.Strings(list)
	}
}

func sameOutput(a, b config.OutputFile) bool {
	return a.Content == b.Content && bytes.Equal(a.RawContent, b.RawContent) && a.Mode == b.Mode
}

// stampSourceHashes gives shared outputs the baseline Source-Hash, so a teammate
// regenerating sees no hash churn, and machine-local outputs a hash of their
// local inputs.
func (g *Generator) stampSourceHashes(plan *localPlan, merged []config.OutputFile) {
	for i := range merged {
		if merged[i].LocalOnly {
			merged[i].SourceHash = plan.localHash
		} else {
			merged[i].SourceHash = plan.baselineHash
		}
	}
}

// localSourceHash is H(shared || overlay || local content tree). The overlay
// enters the hash only as a redacted canonical form (structure and non-secret
// values): the hash is written into generated headers, and a local file's
// content already carries its own Content-Hash.
func (g *Generator) localSourceHash(baselineHash string) (string, error) {
	var b strings.Builder
	b.WriteString("local-source-hash/v2\nshared=" + baselineHash + "\n")
	if o := g.config.LocalOverlay; o != nil {
		canonical, err := json.Marshal(canonicalOverlay(nil, o.Doc))
		if err != nil {
			return "", oops.Wrapf(err, "encode the local overlay for hashing")
		}
		b.WriteString("overlay=" + string(canonical) + "\n")
	}
	if t := g.config.LocalContent; t != nil {
		writeContentFiles(&b, "local.rules", t.Rules, g.config)
		writeContentFiles(&b, "local.context", t.Context, g.config)
		writeContentFiles(&b, "local.skills", t.Skills, g.config)
		writeContentFiles(&b, "local.agents", t.Agents, g.config)
		writeContentFiles(&b, "local.commands", t.Commands, g.config)
		names := make([]string, 0, len(t.Domains))
		for name := range t.Domains {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			d := t.Domains[name]
			if d == nil {
				continue
			}
			prefix := "local.domain." + name + "."
			writeContentFiles(&b, prefix+"rules", d.Rules, g.config)
			writeContentFiles(&b, prefix+"context", d.Context, g.config)
			writeContentFiles(&b, prefix+"skills", d.Skills, g.config)
			writeContentFiles(&b, prefix+"agents", d.Agents, g.config)
			writeContentFiles(&b, prefix+"commands", d.Commands, g.config)
		}
	}
	return templates.HashContent(b.String()), nil
}

const redactedValue = "<redacted>"

// canonicalOverlay returns a copy of an overlay document that is safe to hash:
// env and header values, args, URLs (credentials and query removed), include and
// skill sources, and anything under a secret-looking key are replaced.
func canonicalOverlay(keyPath []string, v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, e := range t {
			out[k] = canonicalOverlay(append(append([]string(nil), keyPath...), k), e)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, e := range t {
			out[i] = canonicalOverlay(keyPath, e)
		}
		return out
	case string:
		return canonicalString(keyPath, t)
	}
	return v
}

func canonicalString(keyPath []string, value string) string {
	if len(keyPath) == 0 {
		return value
	}
	last := keyPath[len(keyPath)-1]
	for _, seg := range keyPath {
		if seg == "env" || seg == "headers" {
			return redactedValue
		}
	}
	switch {
	case last == "args" || config.IsSensitiveHeaderName(last):
		return redactedValue
	case last == "url" || last == "source":
		return stripURLCredentials(value)
	}
	return value
}

// stripURLCredentials keeps scheme, host and path of a URL and drops userinfo,
// query and fragment; a value that is not a URL is returned as is.
func stripURLCredentials(value string) string {
	if !strings.Contains(value, "://") {
		return value
	}
	u, err := url.Parse(value)
	if err != nil || u.Host == "" {
		return redactedValue
	}
	return u.Scheme + "://" + u.Host + u.Path
}

// findViolations returns the files whose local content would end up where other
// people can see it: tracked files, or files that are not ignored. Local-only
// files are ignored through the exclude entries this run writes, so only tracked
// ones count for them. When git cannot answer, every candidate counts as tracked.
func (g *Generator) findViolations(plan *localPlan, merged []config.OutputFile) []localViolation {
	candidates := append(append([]string(nil), plan.localOnly...), plan.drift...)
	tracked, err := gitutil.TrackedAmong(g.config.BaseDir, candidates)
	failClosed := err != nil
	if failClosed {
		logger.Warn("Could not ask git which files are tracked; treating local outputs as tracked", "error", err)
	}
	isTracked := func(rel string) bool { return failClosed || tracked[rel] }

	var out []localViolation
	for _, rel := range plan.localOnly {
		if isTracked(rel) {
			out = append(out, localViolation{rel, "local-only output is tracked by git"})
		}
	}
	ignored := g.ignoredSet(plan.drift, g.pendingIgnorePatterns(merged))
	for _, rel := range plan.drift {
		switch {
		case isTracked(rel):
			out = append(out, localViolation{rel, "shared output is tracked and would change"})
		case !ignored[rel]:
			out = append(out, localViolation{rel, "shared output would change and is not git-ignored"})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].path < out[j].path })
	return out
}

// pendingIgnorePatterns are the patterns this run is about to add to its managed
// .gitignore block.
func (g *Generator) pendingIgnorePatterns(outputs []config.OutputFile) []string {
	needed, _ := g.neededGitignorePatterns(outputs)
	return needed
}

// ignoredSet reports which of rels (project-relative) are ignored. Inside a
// repository git decides in one call, on top of which the patterns this run has
// not written yet (pending) are applied; elsewhere the generator's own matcher
// reads .gitignore and pending.
func (g *Generator) ignoredSet(rels, pending []string) map[string]bool {
	ignored := map[string]bool{}
	if len(rels) == 0 {
		return ignored
	}
	byGit, err := gitutil.IgnoredAmong(g.config.BaseDir, rels)
	if err != nil {
		logger.Warn("Could not ask git which files are ignored; using the built-in matcher", "error", err)
	}
	if byGit != nil {
		for _, rel := range rels {
			ignored[rel] = byGit[rel] || isIgnored(rel, pending)
		}
		return ignored
	}
	patterns := append([]string(nil), pending...)
	if data, readErr := os.ReadFile(filepath.Join(g.config.BaseDir, ".gitignore")); readErr == nil {
		patterns = append(patterns, gitignorePatterns(string(data))...)
	}
	for _, rel := range rels {
		ignored[rel] = isIgnored(rel, patterns)
	}
	return ignored
}

// check fails when a violation exists and drift is not allowed. The error names
// paths only; the content of local files may be secret.
func (p *localPlan) check(allow bool) error {
	if len(p.violations) == 0 || allow {
		return nil
	}
	lines := make([]string, len(p.violations))
	for i, v := range p.violations {
		lines[i] = fmt.Sprintf("%s (%s)", v.path, v.reason)
	}
	return oops.
		With("errors", lines).
		Hint("Git-ignore these files, run with --no-local to generate the shared view, "+
			"or pass --allow-local-drift to write them anyway (this can put local secrets into tracked files)").
		Errorf("local overrides would change %d shared output(s)", len(p.violations))
}

// dryRunLines describes the plan for DryRun.
func (p *localPlan) dryRunLines() []string {
	var lines []string
	for _, rel := range p.localOnly {
		lines = append(lines, "local-only: "+rel)
	}
	for _, rel := range p.drift {
		lines = append(lines, "drift: "+rel)
	}
	for _, rel := range p.suppressed {
		lines = append(lines, "suppressed: "+rel)
	}
	for _, v := range p.violations {
		lines = append(lines, fmt.Sprintf("blocked: %s (%s)", v.path, v.reason))
	}
	return lines
}

// excludeMarkers delimit this project's block in .git/info/exclude. The marker
// carries the project's absolute config directory, so projects in one
// repository and linked worktrees (which share the file) each own a distinct
// block that no other run rewrites.
func (g *Generator) excludeMarkers() (begin, end string) {
	key := gitutil.Resolve(g.manifestDir())
	return "# BEGIN ai-rulez local: " + key, "# END ai-rulez local: " + key
}

// syncMachineExcludes makes .git/info/exclude hold exactly this project's
// overlay-derived local-only paths, inside its own marked block (removed when
// there are none). Paths git's exclude file cannot express fall back to the
// managed .gitignore block. plan may be nil: a run with no local inputs removes
// a block left by an earlier run.
func (g *Generator) syncMachineExcludes(plan *localPlan) error {
	exclude := gitutil.InfoExcludePath(g.config.BaseDir)
	if exclude == "" {
		if plan != nil && len(plan.machineLocal) > 0 {
			for rel := range plan.machineLocal {
				plan.inGitignore[rel] = true
			}
			logger.Warn("Not a git repository; ignoring machine-local outputs through .gitignore instead of .git/info/exclude")
		}
		return nil
	}

	top := gitutil.TopLevel(g.config.BaseDir)
	var patterns []string
	if plan != nil {
		for rel := range plan.machineLocal {
			repoRel := gitutil.RepoRelative(top, filepath.Join(g.config.BaseDir, filepath.FromSlash(rel)))
			if repoRel == "" || strings.ContainsAny(repoRel, "\r\n") {
				// Never leave a local path unignored: the managed .gitignore takes it.
				plan.inGitignore[rel] = true
				continue
			}
			patterns = append(patterns, "/"+escapeGitPattern(repoRel))
		}
	}
	sort.Strings(patterns)

	data, err := os.ReadFile(exclude) //nolint:gosec // git's own exclude file
	if err != nil && !os.IsNotExist(err) {
		return oops.With("path", exclude).Wrapf(err, "read git exclude file")
	}
	begin, end := g.excludeMarkers()
	block := ""
	if len(patterns) > 0 {
		block = begin + "\n" + strings.Join(patterns, "\n") + "\n" + end + "\n"
	}
	content := string(data)
	updated := gitignore.ReplaceMarkedBlock(content, begin, end, block)
	if updated == content {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(exclude), 0o755); err != nil {
		return oops.With("path", exclude).Wrapf(err, "create git info directory")
	}
	// Temp file + rename: a concurrent run in another project sharing this
	// repository never reads or leaves a half-written exclude file.
	if err := config.WriteFileAtomic(exclude, []byte(updated), 0o644); err != nil {
		return oops.With("path", exclude).Wrapf(err, "write git exclude file")
	}
	return nil
}

// escapeGitPattern escapes a literal path for use in a gitignore pattern:
// wildcard and bracket characters, a leading '#' or '!', and trailing spaces
// (which git otherwise strips).
func escapeGitPattern(literal string) string {
	var b strings.Builder
	for i, r := range literal {
		switch r {
		case '\\', '[', ']', '*', '?':
			b.WriteByte('\\')
		case '#', '!':
			if i == 0 {
				b.WriteByte('\\')
			}
		}
		b.WriteRune(r)
	}
	escaped := b.String()
	trimmed := strings.TrimRight(escaped, " ")
	for range len(escaped) - len(trimmed) {
		trimmed += "\\ "
	}
	return trimmed
}

// localInputsOnDisk reports whether a config.local.* overlay or a local/ content
// tree exists even though this run did not load it (--no-local, plugin mode).
func (g *Generator) localInputsOnDisk() bool {
	dir := g.config.ConfigDir
	if dir == "" {
		return false
	}
	if matches, err := filepath.Glob(filepath.Join(dir, "config.local.*")); err == nil && len(matches) > 0 {
		return true
	}
	info, err := os.Stat(filepath.Join(dir, localSourceDirName))
	return err == nil && info.IsDir()
}

// sharedManifestFiles lists what the committed manifest records: every file the
// shared baseline owns, minus hand-written files this run left alone.
func (p *localPlan) sharedManifestFiles(skipped map[string]bool) []string {
	files := make([]string, 0, len(p.baselineFiles))
	for _, rel := range p.baselineFiles {
		if !skipped[rel] {
			files = append(files, rel)
		}
	}
	return files
}

// sourceHashFor is the Source-Hash stamped into an output's header: the output's
// own override when one is set (shared agents_md outputs, or the drift guard's
// baseline hash), the run's hash otherwise.
func (g *Generator) sourceHashFor(output config.OutputFile) string {
	if output.SourceHash != "" {
		return output.SourceHash
	}
	return g.config.SourceHash
}

// guardLocal runs the local-config checks of a generate run: classify against the
// shared baseline, refuse unless drift is allowed, and keep .git/info/exclude in
// step (also when the run has no local inputs any more, so a block left by an
// earlier run is removed; a run that skipped local inputs on purpose leaves it).
func (g *Generator) guardLocal(profile string, outputs []config.OutputFile) error {
	plan, err := g.planLocal(profile, outputs)
	if err != nil {
		return err
	}
	if plan != nil {
		if err := plan.check(g.allowLocalDrift); err != nil {
			return err
		}
	}
	if plan != nil || (!g.localSkipped && pathIsFile(g.localManifestPath())) {
		return g.syncMachineExcludes(plan)
	}
	return nil
}

// localOutputPattern is the managed-.gitignore pattern for a machine-local
// output, or "" when .git/info/exclude holds it instead: overlay-derived paths
// differ per machine, so they stay out of the shared .gitignore.
func (g *Generator) localOutputPattern(relPath string) string {
	if g.plan != nil && g.plan.machineLocal[relPath] && !g.plan.inGitignore[relPath] {
		return ""
	}
	return localGitignorePattern(relPath)
}

// stableLocalName reports whether a file's name marks it as machine-local
// (".local." before the extension), which the shared .gitignore can cover with a
// pattern that does not depend on which local content a machine has.
func stableLocalName(rel string) bool {
	return strings.Contains(path.Base(rel), ".local.")
}
