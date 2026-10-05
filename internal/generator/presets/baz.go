package presets

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/generator/rulefiles"
	"github.com/Goldziher/ai-rulez/internal/generator/targetmatch"
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

func init() {
	config.RegisterPreset(bazPresetName, &BazPresetGenerator{})
}

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
		filepath.Join(baseDir, agentsFileName),
		filepath.Join(baseDir, ".agents", "skills"),
		filepath.Join(baseDir, filepath.FromSlash(bazClaudeAgentsDir)),
	}
}

func (g *BazPresetGenerator) Generate(content *config.ContentTree, baseDir string, cfg *config.Config) ([]config.OutputFile, error) {
	codex := &CodexPresetGenerator{}
	outputs := []config.OutputFile{{
		Path:    filepath.Join(baseDir, agentsFileName),
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
	for _, skill := range allSkills(content) {
		id := extractSkillID(skill.Path)
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
	for _, agent := range allAgents(content) {
		id := sanitizeAgentID(agent.Name)
		if agent.Metadata != nil && !targetmatch.Allow(agent.Metadata.Targets, []string{bazPresetName}, bazClaudeAgentsDir+"/"+id+extMarkdown, id+extMarkdown) {
			continue
		}
		if len(outputs) == 0 {
			outputs = append(outputs,
				config.OutputFile{Path: filepath.Join(baseDir, ".claude"), IsDir: true},
				config.OutputFile{Path: dir, IsDir: true},
			)
		}
		outputs = append(outputs, config.OutputFile{Path: filepath.Join(dir, id+extMarkdown), Content: renderBazAgent(agent)})
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
	target := rulefiles.RootTarget(bazPresetName, agentsFileName)
	kept := make([]config.ContentFile, 0, len(items))
	for _, cf := range items {
		// An item that baz's own target excludes is never written nested, so it
		// stays in the root file for the presets that do select it.
		if !rulefiles.InlineAllowed(cf, target) || len(bazNestedDirs(cf, cfg)) == 0 {
			kept = append(kept, cf)
		}
	}
	return kept
}

// bazNestedDirs returns the directories (relative to the project root, slash
// separated, shortest first) whose AGENTS.md should carry a path-scoped item,
// or nil when it stays in the root file. The item moves only when every
// positive glob points into an existing directory, so it never half-moves.
func bazNestedDirs(cf config.ContentFile, cfg *config.Config) []string {
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
	return minimalDirs(dirs)
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
	for _, segment := range strings.Split(path.Clean(dir), "/") {
		if segment == ".git" || segment == ".ai-rulez" || (cfg.ConfigDirName != "" && segment == cfg.ConfigDirName) {
			return false
		}
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
	target := rulefiles.RootTarget(bazPresetName, agentsFileName)
	rules := map[string][]config.ContentFile{}
	contexts := map[string][]config.ContentFile{}
	for _, rule := range rulefiles.FilterInline(allInlineRules(content), target) {
		for _, dir := range bazNestedDirs(rule, cfg) {
			rules[dir] = append(rules[dir], rule)
		}
	}
	for _, ctx := range rulefiles.FilterInline(allInlineContext(content), target) {
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
		opts := rulefiles.InlineOpts{Compact: cfg.IsCompact(), AppliesTo: true}
		rulefiles.WriteInlineRules(&b, rules[dir], opts, nil)
		rulefiles.WriteInlineContext(&b, contexts[dir], opts, nil)
		outputs = append(outputs, config.OutputFile{
			Path:    filepath.Join(baseDir, filepath.FromSlash(rel)),
			Content: b.String(),
		})
	}
	return outputs, nil
}
