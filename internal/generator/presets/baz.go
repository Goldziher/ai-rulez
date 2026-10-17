package presets

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/rulefiles"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/targetmatch"
)

// Baz (baz.ai) preset.
//
// Everything here is derived from https://baz.ai/docs/agents/skills-and-instructions,
// read 2026-10-04. Documented facts the preset relies on:
//   - AGENTS.md and CLAUDE.md are discovered at any depth and a nested one is
//     scoped to its own directory and below.
//   - Skills are discovered as folders at the repository root only
//     (.claude/skills, .cursor/skills, .agent/skills, .agents/skills), and a
//     folder needs a top-level SKILL.md (or AGENTS.md).
//   - .claude/agents/*.md and .agents/*.md are discovered at the repository
//     root only; .claude/commands and .cursor/commands are always ignored.
//   - .claude/rules is not in any documented pattern, so rules written there
//     are not read.
//   - There is no precedence between guidelines: every in-scope one applies.
//   - Files are read from the default branch only (first on connect, then on
//     every push to it).
//
// NOT documented, so not encoded: size limits, whether draft PRs are reviewed
// (the settings page says drafts are off by default; see the research notes),
// and whether a .agents/skills folder is read when .claude/skills also exists
// (both are listed, so both are written when both are wanted).

const bazPresetName = "baz"

// bazClaudeAgentsDir is where Baz documents subagent files. Claude Code loads
// the same directory as subagents.
const bazClaudeAgentsDir = ".claude/agents"

// BazPresetGenerator generates what the Baz reviewer reads: AGENTS.md at the
// root and, for path-scoped rules, in the directories the globs point into;
// skills under .agents/skills; agents under .claude/agents. It writes no
// commands (Baz ignores them) and no .claude/rules (Baz does not read it).
type BazPresetGenerator struct{}

func (g *BazPresetGenerator) GetName() string {
	return bazPresetName
}

func (g *BazPresetGenerator) GetOutputPaths(baseDir string) []string {
	return []string{
		filepath.Join(baseDir, agentsMDFile),
		filepath.Join(baseDir, ".agents", "skills"),
		filepath.Join(baseDir, filepath.FromSlash(bazClaudeAgentsDir)),
	}
}

func (g *BazPresetGenerator) Generate(content *config.ContentTree, baseDir string, cfg *config.Config) ([]config.OutputFile, error) {
	codex := &CodexPresetGenerator{}
	outputs := []config.OutputFile{{
		Path:    filepath.Join(baseDir, agentsMDFile),
		Content: codex.renderAgentsMarkdownFor(content, cfg, nil),
	}}

	// Baz reads skills and agents at the repository root only, so a monorepo
	// scope run writes the scoped AGENTS.md and nothing else.
	if rulefiles.InScope(cfg) {
		return outputs, nil
	}

	nested, err := bazNestedOutputs(content, baseDir, cfg)
	if err != nil {
		return nil, err
	}
	outputs = append(outputs, nested...)

	// The claude preset already writes .claude/skills and .claude/agents, which
	// Baz reads; a second copy would be read as a second guideline.
	if !cfg.HasBuiltInPreset(presetNameClaude) {
		outputs = append(outputs, bazSkillOutputs(content, baseDir)...)
		outputs = append(outputs, bazAgentOutputs(content, baseDir)...)
	}
	return outputs, nil
}

func bazSkillOutputs(content *config.ContentTree, baseDir string) []config.OutputFile {
	var outputs []config.OutputFile
	root := filepath.Join(baseDir, ".agents", "skills")
	skillList := allSkills(content)
	for idx := range skillList {
		skill := skillList[idx]
		id := skillIDOf(skill)
		if skill.Metadata != nil && !targetmatch.Allow(skill.Metadata.Targets, []string{bazPresetName}, ".agents/skills/"+id+"/SKILL.md", "SKILL.md") {
			continue
		}
		if len(outputs) == 0 {
			outputs = append(outputs,
				config.OutputFile{Path: filepath.Join(baseDir, ".agents"), IsDir: true},
				config.OutputFile{Path: root, IsDir: true},
			)
		}
		skillDir := filepath.Join(root, id)
		outputs = append(outputs,
			config.OutputFile{Path: skillDir, IsDir: true},
			config.OutputFile{Path: filepath.Join(skillDir, "SKILL.md"), Content: renderAgentSkillFile(skill)},
		)
		outputs = append(outputs, SkillResourceOutputs(&skill, skillDir)...)
	}
	return outputs
}

func bazAgentOutputs(content *config.ContentTree, baseDir string) []config.OutputFile {
	var outputs []config.OutputFile
	dir := filepath.Join(baseDir, filepath.FromSlash(bazClaudeAgentsDir))
	agentList := allAgents(content)
	for idx := range agentList {
		agent := agentList[idx]
		id := sanitizeAgentID(agent.Name)
		if agent.Metadata != nil && !targetmatch.Allow(agent.Metadata.Targets, []string{bazPresetName}, bazClaudeAgentsDir+"/"+id+".md", id+".md") {
			continue
		}
		if len(outputs) == 0 {
			outputs = append(outputs,
				config.OutputFile{Path: filepath.Join(baseDir, ".claude"), IsDir: true},
				config.OutputFile{Path: dir, IsDir: true},
			)
		}
		outputs = append(outputs, config.OutputFile{Path: filepath.Join(dir, id+".md"), Content: renderBazAgent(agent)})
	}
	return outputs
}

// renderBazAgent renders a subagent file. Baz treats it as instruction text, so
// only the name and description frontmatter Claude Code requires is written.
func renderBazAgent(agent config.ContentFile) string {
	description := ""
	if agent.Metadata != nil {
		description = agent.Metadata.Extra[keyDescription]
	}
	var b strings.Builder
	b.WriteString("---\nname: " + sanitizeAgentID(agent.Name) + "\n")
	b.WriteString("description: " + quoteYAMLString(description) + "\n")
	b.WriteString("---\n\n")
	b.WriteString(agent.Content)
	return b.String()
}

// bazNestedActive reports whether path-scoped items are moved out of the root
// AGENTS.md into nested ones: the baz preset is configured, rules.baz_scoped is
// "nested", and this is the root run (a monorepo scope already is a nested file).
func bazNestedActive(cfg *config.Config) bool {
	return cfg.HasBuiltInPreset(bazPresetName) && !rulefiles.InScope(cfg) && cfg.BazScopedRules() == config.BazScopedNested
}

// WithoutBazNested exposes withoutBazNested for the DSL renderer in
// internal/generator/providers.
func WithoutBazNested(items []config.ContentFile, cfg *config.Config) []config.ContentFile {
	return withoutBazNested(items, cfg)
}

// withoutBazNested drops, from the items the root AGENTS.md inlines, the ones
// that bazNestedOutputs writes to a nested AGENTS.md instead. It is applied by
// every renderer of the root AGENTS.md (codex, the shared file, baz), so they
// stay byte-identical whichever writes it last.
func withoutBazNested(items []config.ContentFile, cfg *config.Config) []config.ContentFile {
	if !bazNestedActive(cfg) {
		return items
	}
	target := rulefiles.RootTarget(bazPresetName, agentsMDFile)
	skipUnscoped := bazSkipsUnscoped(cfg)
	skipRest := bazSkipsRest(cfg)
	kept := make([]config.ContentFile, 0, len(items))
	for idx := range items {
		cf := items[idx]
		// An item that baz's own target excludes is never written nested, so it
		// stays in the root file for the presets that do select it.
		if skipRest && bazRestItem(cf, cfg) {
			continue
		}
		if !rulefiles.InlineAllowed(cf, target) {
			kept = append(kept, cf)
			continue
		}
		if len(bazNestedDirs(cf, cfg)) != 0 || (skipUnscoped && bazUnscopedItem(cf, cfg)) {
			continue
		}
		kept = append(kept, cf)
	}
	return kept
}

// bazUnscopedItem reports whether baz selects a glob-activated item that has no
// nested directory to go to (a glob without a directory, or one that points
// outside the project), so the root AGENTS.md would inline it.
func bazUnscopedItem(cf config.ContentFile, cfg *config.Config) bool {
	if !bazItemSelected(cf, cfg) || cf.Metadata == nil {
		return false
	}
	return cf.Metadata.ResolveActivation().Mode == config.ActivationGlob && len(bazNestedDirs(cf, cfg)) == 0
}

// bazRestItem reports whether a glob-activated item is one baz does not select
// (unselected under baz_default = "none", or excluded by `!baz`), so it has no
// place on the baz surface.
func bazRestItem(cf config.ContentFile, cfg *config.Config) bool {
	if cf.Metadata == nil || bazItemSelected(cf, cfg) {
		return false
	}
	return cf.Metadata.ResolveActivation().Mode == config.ActivationGlob
}

// bazSkipsRest reports whether rules.baz_rest = "skip" applies: nesting is active
// and baz is the only preset writing the root AGENTS.md.
func bazSkipsRest(cfg *config.Config) bool {
	return cfg.BazRestRules() == config.BazRestSkip && bazSoleRootWriter(cfg)
}

// bazSoleRootWriter reports whether nesting is active and no other preset writes
// the root AGENTS.md, which every writer renders byte for byte.
func bazSoleRootWriter(cfg *config.Config) bool {
	if !bazNestedActive(cfg) {
		return false
	}
	for _, owner := range rulefiles.RootOwners(agentsMDFile) {
		if owner != bazPresetName && cfg.HasBuiltInPreset(owner) {
			return false
		}
	}
	return true
}

// bazSkipsUnscoped reports whether rules.baz_unscoped = "skip" applies: nesting
// is active and baz is the only preset writing the root AGENTS.md. The file is
// shared byte for byte with codex, opencode, amp, xum and the others, so while
// one of them is configured it must keep the item.
func bazSkipsUnscoped(cfg *config.Config) bool {
	return cfg.BazUnscopedRules() == config.BazUnscopedSkip && bazSoleRootWriter(cfg)
}

// BazInlinedUnscoped names the rules and context items the baz preset inlines
// into the root AGENTS.md although they are path-scoped, because no glob names a
// directory to nest them under. It is empty when rules.baz_unscoped skips them
// or nesting is off.
func BazInlinedUnscoped(content *config.ContentTree, cfg *config.Config) []string {
	if content == nil || !bazNestedActive(cfg) || bazSkipsUnscoped(cfg) {
		return nil
	}
	target := rulefiles.RootTarget(bazPresetName, agentsMDFile)
	var names []string
	items := append(rulefiles.FilterInline(allInlineRules(content), target), rulefiles.FilterInline(allInlineContext(content), target)...)
	for idx := range items {
		if bazUnscopedItem(items[idx], cfg) {
			names = append(names, items[idx].Name)
		}
	}
	sort.Strings(names)
	return names
}

// BazConfigDirItems names the path-scoped rules and context items baz selects
// whose globs point into the configuration directory, .git/ or another nested
// ai-rulez root, where a nested AGENTS.md is never written (it would be read back
// as a source file or collide with that root's own). They stay
// in the root AGENTS.md, or are left out of every AGENTS.md when
// rules.baz_unscoped = "skip" applies. skipped reports which.
func BazConfigDirItems(content *config.ContentTree, cfg *config.Config) (names []string, skipped bool) {
	if content == nil || !bazNestedActive(cfg) {
		return nil, false
	}
	target := rulefiles.RootTarget(bazPresetName, agentsMDFile)
	items := append(rulefiles.FilterInline(allInlineRules(content), target), rulefiles.FilterInline(allInlineContext(content), target)...)
	for idx := range items {
		cf := items[idx]
		if !bazItemSelected(cf, cfg) || cf.Metadata == nil || cf.Metadata.ResolveActivation().Mode != config.ActivationGlob {
			continue
		}
		if len(bazNestedDirs(cf, cfg)) != 0 {
			continue
		}
		for _, glob := range cf.Metadata.ResolveActivation().Globs {
			if strings.HasPrefix(glob, "!") {
				continue
			}
			if dir := bazScopeDir(glob, cfg); bazConfigDir(dir, cfg) || (dir != "" && bazInNestedRoot(dir, cfg)) {
				names = append(names, cf.Name)
				break
			}
		}
	}
	sort.Strings(names)
	return names, bazSkipsUnscoped(cfg)
}

// bazConfigDir reports whether dir is, or is below, a directory a nested
// AGENTS.md is never written to: .git, .ai-rulez or the configuration directory.
func bazConfigDir(dir string, cfg *config.Config) bool {
	for _, segment := range strings.Split(path.Clean(dir), "/") {
		if segment == ".git" || segment == ".ai-rulez" || (cfg.ConfigDirName != "" && segment == cfg.ConfigDirName) {
			return true
		}
	}
	return false
}

// bazInNestedRoot reports whether dir is, or is below, another ai-rulez root
// nested under the project (a directory holding its own configuration). That
// root owns the files below it: a root-run AGENTS.md there would be reported as
// an orphan by the nested run and overwrite its own, so the item stays in the
// root file and every generate mode renders the same bytes (#395).
func bazInNestedRoot(dir string, cfg *config.Config) bool {
	cur := cfg.BaseDir
	for _, segment := range strings.Split(path.Clean(dir), "/") {
		cur = filepath.Join(cur, segment)
		if config.ResolveConfigDirName(cur) != "" {
			return true
		}
	}
	return false
}

// bazNestedDirs returns the directories (relative to the project root, slash
// separated, shortest first) whose AGENTS.md should carry a path-scoped item,
// or nil when it stays in the root file. The item moves only when every
// positive glob points into an existing directory, so it never half-moves.
func bazNestedDirs(cf config.ContentFile, cfg *config.Config) []string {
	if !bazItemSelected(cf, cfg) {
		return nil
	}
	act := cf.Metadata.ResolveActivation()
	if act.Mode != config.ActivationGlob {
		return nil
	}
	var dirs []string
	for _, glob := range act.Globs {
		if strings.HasPrefix(glob, "!") {
			continue
		}
		found := false
		for _, expanded := range rulefiles.ExpandBraces(glob) {
			dir := bazScopeDir(expanded, cfg)
			if dir == "" || !bazDirUsable(dir, cfg) {
				return nil
			}
			dirs = append(dirs, dir)
			found = true
		}
		if !found {
			return nil
		}
	}
	return bazCollapseDirs(minimalDirs(dirs), cfg)
}

// bazItemSelected reports whether an item may reach the baz preset's
// path-scoped surface. A `!baz` target always excludes it. With
// rules.baz_default = "none" only an item that opts in qualifies: a target
// selecting baz, `baz: true` in its frontmatter, or a name listed in
// rules.baz.include (an item with no targets does not). The default ("all")
// selects every item that is not excluded.
func bazItemSelected(cf config.ContentFile, cfg *config.Config) bool {
	var targets []string
	if cf.Metadata != nil {
		targets = cf.Metadata.Targets
	}
	if targetmatch.Denied(targets, []string{bazPresetName}, agentsMDFile) {
		return false
	}
	if cfg.BazDefaultRules() != config.BazDefaultNone {
		return true
	}
	if cf.Metadata != nil && strings.EqualFold(strings.TrimSpace(cf.Metadata.Extra["baz"]), "true") {
		return true
	}
	if cfg.BazIncludes(cf.Name) || cfg.BazIncludes(path.Base(filepath.ToSlash(cf.Path))) {
		return true
	}
	return targetmatch.Match(targets, []string{bazPresetName}, agentsMDFile)
}

// bazCollapseDirs collapses the directories of one item to the directory that
// covers all of them, so it is written once instead of a copy in each. That
// happens when rules.baz_dedupe is on, or when the item would produce more than
// rules.baz_dedupe_max copies. The covering directory is their deepest shared
// parent; when they share none beyond the project root and the item exceeds
// baz_dedupe_max, the item moves to the root AGENTS.md (nil, "not nested") unless
// rules.baz_unscoped = "skip" keeps the root file free of path-scoped items. It
// returns dirs unchanged when neither setting applies or no covering directory
// is usable.
func bazCollapseDirs(dirs []string, cfg *config.Config) []string {
	if len(dirs) <= 1 {
		return dirs
	}
	overMax := cfg.BazDedupeMaxCopies() > 0 && len(dirs) > cfg.BazDedupeMaxCopies()
	if !cfg.BazDedupeEnabled() && !overMax {
		return dirs
	}
	parent := commonDir(dirs)
	if parent == "" {
		if overMax && cfg.BazUnscopedRules() != config.BazUnscopedSkip {
			return nil
		}
		return dirs
	}
	if !bazDirUsable(parent, cfg) {
		return dirs
	}
	return []string{parent}
}

// commonDir returns the deepest directory all dirs sit under: their longest
// common slash-separated prefix, or "" when they share none beyond the root.
func commonDir(dirs []string) string {
	parts := strings.Split(dirs[0], "/")
	for _, dir := range dirs[1:] {
		segs := strings.Split(dir, "/")
		n := 0
		for n < len(parts) && n < len(segs) && parts[n] == segs[n] {
			n++
		}
		parts = parts[:n]
	}
	return strings.Join(parts, "/")
}

// globStaticDir is the directory a glob is rooted in: the leading path segments
// without wildcard characters, excluding the last segment unless the glob ends
// in a slash ("src/api/*.py" and "src/api/**" give "src/api", "*.py" gives "").
// A glob that escapes the project ("../x") gives "".
func globStaticDir(glob string) string {
	glob = strings.TrimPrefix(strings.TrimPrefix(glob, "./"), "/")
	segments := strings.Split(glob, "/")
	var dir []string
	for _, seg := range segments[:len(segments)-1] {
		if seg == ".." {
			return ""
		}
		if seg == "" || seg == "." || strings.ContainsAny(seg, "*?[{\\") {
			break
		}
		dir = append(dir, seg)
	}
	return strings.Join(dir, "/")
}

// bazScopeDir is globStaticDir, except that a glob whose last segment is a
// wildcard-free existing directory ("src/api") scopes to that directory: a bare
// directory means everything below it, so its parent would over-scope.
func bazScopeDir(glob string, cfg *config.Config) string {
	trimmed := strings.TrimPrefix(strings.TrimPrefix(glob, "./"), "/")
	last := trimmed[strings.LastIndex(trimmed, "/")+1:]
	if last != "" && !strings.ContainsAny(last, "*?[{\\") && last != ".." && last != "." {
		if dir := globStaticDir(trimmed + "/"); dir == strings.TrimSuffix(trimmed, "/") {
			if info, err := os.Stat(filepath.Join(cfg.BaseDir, filepath.FromSlash(dir))); err == nil && info.IsDir() {
				return dir
			}
		}
	}
	return globStaticDir(glob)
}

// bazDirUsable reports whether a nested AGENTS.md can be written to dir: it is an
// existing directory inside the project that no [[scopes]] entry already owns.
func bazDirUsable(dir string, cfg *config.Config) bool {
	if dir == "" || dir == "." || path.IsAbs(dir) || strings.HasPrefix(path.Clean(dir), "..") {
		return false
	}
	// A nested AGENTS.md inside a configuration directory would be read back as
	// a source file (a rule or agent named AGENTS) on the next run; .git holds
	// no project files.
	if bazConfigDir(dir, cfg) || bazInNestedRoot(dir, cfg) {
		return false
	}
	for _, scope := range cfg.Scopes {
		if path.Clean(filepath.ToSlash(scope.Path)) == path.Clean(dir) {
			return false
		}
	}
	info, err := os.Stat(filepath.Join(cfg.BaseDir, filepath.FromSlash(dir)))
	return err == nil && info.IsDir()
}

// minimalDirs sorts and de-duplicates dirs and drops those below another entry:
// a file in "src/api" is already covered by the scope of "src".
func minimalDirs(dirs []string) []string {
	sort.Strings(dirs)
	var out []string
	for _, dir := range dirs {
		if len(out) > 0 && (out[len(out)-1] == dir || strings.HasPrefix(dir, out[len(out)-1]+"/")) {
			continue
		}
		out = append(out, dir)
	}
	return out
}

// bazNestedOutputs writes <dir>/AGENTS.md for every directory that path-scoped
// rules and context point into (see bazNestedDirs). Baz scopes a nested
// AGENTS.md to its directory and below, so the item applies to the files the
// glob names without a root file loading it for unrelated changes.
func bazNestedOutputs(content *config.ContentTree, baseDir string, cfg *config.Config) ([]config.OutputFile, error) {
	if !bazNestedActive(cfg) {
		return nil, nil
	}
	target := rulefiles.RootTarget(bazPresetName, agentsMDFile)
	rules := map[string][]config.ContentFile{}
	contexts := map[string][]config.ContentFile{}
	ruleList := rulefiles.FilterInline(allInlineRules(content), target)
	for idx := range ruleList {
		rule := ruleList[idx]
		for _, dir := range bazNestedDirs(rule, cfg) {
			rules[dir] = append(rules[dir], rule)
		}
	}
	ctxList := rulefiles.FilterInline(allInlineContext(content), target)
	for idx := range ctxList {
		ctx := ctxList[idx]
		for _, dir := range bazNestedDirs(ctx, cfg) {
			contexts[dir] = append(contexts[dir], ctx)
		}
	}
	dirs := make([]string, 0, len(rules)+len(contexts))
	for dir := range rules {
		dirs = append(dirs, dir)
	}
	for dir := range contexts {
		if _, ok := rules[dir]; !ok {
			dirs = append(dirs, dir)
		}
	}
	sort.Strings(dirs)

	outputs := make([]config.OutputFile, 0, len(dirs))
	for _, dir := range dirs {
		rel := dir + "/AGENTS.md"
		var b strings.Builder
		b.WriteString(generateCodexPresetHeader(cfg, rel, len(rules[dir]), 0, 0))
		b.WriteString("# " + dir + "\n\n")
		fmt.Fprintf(&b, "Instructions for `%s/` and everything below it.\n\n", dir)
		opts := rulefiles.InlineOpts{Diag: cfg.Diag, Compact: cfg.IsCompact(), AppliesTo: true}
		rulefiles.WriteInlineRules(&b, rules[dir], opts, nil)
		rulefiles.WriteInlineContext(&b, contexts[dir], opts, nil)
		outputs = append(outputs, config.OutputFile{
			Path:    filepath.Join(baseDir, filepath.FromSlash(rel)),
			Content: b.String(),
		})
	}
	return outputs, nil
}
