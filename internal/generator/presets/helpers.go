package presets

import (
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/logger"
	"github.com/Goldziher/ai-rulez/internal/markdown"
)

const agentDelegationBuiltin = "agent-delegation"

// sortedDomainNames returns domain names from a ContentTree in lexical order.
// Go map iteration is randomized, so callers that flatten domains into output
// must sort first to keep generation idempotent.
func sortedDomainNames(content *config.ContentTree) []string {
	names := make([]string, 0, len(content.Domains))
	for name := range content.Domains {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// configFileName returns the actual config filename from the Config,
// falling back to "config.toml" (V4 default) if not set.
func configFileName(cfg *config.Config) string {
	if cfg.ConfigFile != "" {
		return cfg.ConfigFile
	}
	return "config.toml"
}

// domainRank ranks a domain by content-source precedence for deduplication:
// on-disk domains (0) beat include-sourced domains (1) beat builtins (2).
// A lower rank wins when two domains define a rule/context of the same name.
func domainRank(domain *config.Domain) int {
	switch {
	case domain.Builtin:
		return 2
	case domain.FromInclude:
		return 1
	default:
		return 0
	}
}

// domainNamesByPrecedence returns domain names ordered by source precedence
// (on-disk, then include, then builtin), alphabetical within each rank. Callers
// that dedupe combined rules/context rely on this order so the highest-priority
// source is encountered first and wins keep-first deduplication.
func domainNamesByPrecedence(content *config.ContentTree) []string {
	names := sortedDomainNames(content)
	sort.SliceStable(names, func(i, j int) bool {
		return domainRank(content.Domains[names[i]]) < domainRank(content.Domains[names[j]])
	})
	return names
}

// combineContentFiles combines multiple ContentFile slices into a single
// alphabetically-sorted slice. Sorting the merged result keeps generated
// output stable when content comes from multiple sources (root + domains).
func combineContentFiles(slices ...[]config.ContentFile) []config.ContentFile {
	var total int
	for _, slice := range slices {
		total += len(slice)
	}

	result := make([]config.ContentFile, 0, total)
	for _, slice := range slices {
		result = append(result, slice...)
	}
	sort.SliceStable(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result
}

// dedupeByName keeps the first ContentFile for each Name, preserving input
// order. Callers pass content in precedence order (highest first) so the
// surviving copy is the highest-priority source. The dropped names (later
// duplicates) are returned for diagnostics.
func dedupeByName(files []config.ContentFile) (kept []config.ContentFile, dropped []string) {
	seen := make(map[string]struct{}, len(files))
	kept = make([]config.ContentFile, 0, len(files))
	for _, file := range files {
		if _, ok := seen[file.Name]; ok {
			dropped = append(dropped, file.Name)
			continue
		}
		seen[file.Name] = struct{}{}
		kept = append(kept, file)
	}
	return kept, dropped
}

// combineDedupedContentFiles concatenates the given slices in caller-supplied
// precedence order (highest first), removes duplicate Names keeping the
// highest-precedence occurrence, then sorts alphabetically for stable output.
// Deduplication happens before the sort so input order — not name order —
// decides the winner. Used for inline rules/context where the same name from a
// builtin and an include/root must collapse to a single entry.
func combineDedupedContentFiles(slices ...[]config.ContentFile) []config.ContentFile {
	var combined []config.ContentFile
	for _, slice := range slices {
		combined = append(combined, slice...)
	}
	kept, _ := dedupeByName(combined)
	sort.SliceStable(kept, func(i, j int) bool {
		return kept[i].Name < kept[j].Name
	})
	return kept
}

// allInlineRules returns the deduplicated, precedence-ordered rules for inline
// rendering — root rules plus every domain's rules, collapsing duplicate names
// (e.g. a builtin and an include both defining "commit-messages") to a single
// highest-precedence entry. Every preset that renders a "## Rules" section
// funnels through this so deduplication is applied uniformly.
func allInlineRules(content *config.ContentTree) []config.ContentFile {
	return combineDedupedContentFiles(content.Rules, getAllDomainRules(content))
}

// allInlineContext is the context counterpart to allInlineRules.
func allInlineContext(content *config.ContentTree) []config.ContentFile {
	return combineDedupedContentFiles(content.Context, getAllDomainContext(content))
}

// getAllDomainRules extracts all rules from all domains in precedence order.
func getAllDomainRules(content *config.ContentTree) []config.ContentFile {
	var rules []config.ContentFile
	for _, name := range domainNamesByPrecedence(content) {
		rules = append(rules, content.Domains[name].Rules...)
	}
	return rules
}

// getAllDomainContext extracts all context from all domains in precedence order,
// excluding the agent-delegation builtin (rendered separately in the Agents section).
func getAllDomainContext(content *config.ContentTree) []config.ContentFile {
	var context []config.ContentFile
	for _, name := range domainNamesByPrecedence(content) {
		if name == agentDelegationBuiltin {
			continue
		}
		context = append(context, content.Domains[name].Context...)
	}
	return context
}

// getAllDomainSkills extracts all skills from all domains in sorted domain order.
func getAllDomainSkills(content *config.ContentTree) []config.ContentFile {
	var skills []config.ContentFile
	for _, name := range sortedDomainNames(content) {
		skills = append(skills, content.Domains[name].Skills...)
	}
	return skills
}

// getAllDomainAgents extracts all agents from all domains in sorted domain order.
func getAllDomainAgents(content *config.ContentTree) []config.ContentFile {
	var agents []config.ContentFile
	for _, name := range sortedDomainNames(content) {
		agents = append(agents, content.Domains[name].Agents...)
	}
	return agents
}

// getAllDomainCommands extracts all commands from all domains in sorted domain order.
func getAllDomainCommands(content *config.ContentTree) []config.ContentFile {
	var commands []config.ContentFile
	for _, name := range sortedDomainNames(content) {
		commands = append(commands, content.Domains[name].Commands...)
	}
	return commands
}

// renderAgentsSection writes an "## Agents" section listing all available
// subagents with delegation and parallelization instructions.
// The section is only rendered when the agent-delegation builtin is enabled
// and at least one agent exists.
func renderAgentsSection(builder *strings.Builder, content *config.ContentTree, agents []config.ContentFile) {
	delegationDomain, ok := content.Domains[agentDelegationBuiltin]
	if !ok || len(agents) == 0 {
		return
	}

	builder.WriteString("## Agents\n\n")

	// Render instruction text from the builtin's context files
	for _, ctx := range delegationDomain.Context {
		processedContent := markdown.ProcessEmbeddedContent(ctx.Content)
		builder.WriteString(processedContent)
		builder.WriteString("\n\n")
	}

	for _, agent := range agents {
		description := ""
		if agent.Metadata != nil {
			description = agent.Metadata.Extra["description"]
		}
		builder.WriteString("- **")
		builder.WriteString(agent.Name)
		builder.WriteString("**")
		if description != "" {
			builder.WriteString(": ")
			builder.WriteString(description)
		}
		builder.WriteString("\n")
	}
	builder.WriteString("\n")
}

// filterContentByExplicitTargets returns only content files that have explicit
// targets matching the given output path. Content with no targets is excluded,
// since untargeted content is already present in the main output file (e.g. CLAUDE.md).
func filterContentByExplicitTargets(files []config.ContentFile, outputPath, baseDir string) []config.ContentFile {
	var result []config.ContentFile
	for _, f := range files {
		if f.Metadata != nil && len(f.Metadata.Targets) > 0 {
			if shouldIncludeInOutput(f.Metadata.Targets, outputPath, baseDir) {
				result = append(result, f)
			}
		}
	}
	return result
}

// sanitizeAgentID lowercases the name and replaces spaces/underscores with
// hyphens — produces filesystem-safe stems for agent and command-as-skill
// output paths. Distinct from sanitizeName, which also strips
// non-alphanumeric runes.
func sanitizeAgentID(name string) string {
	id := strings.ToLower(name)
	id = strings.ReplaceAll(id, " ", "-")
	id = strings.ReplaceAll(id, "_", "-")
	return id
}

// extractSkillID extracts the canonical skill ID from a skill's source path.
// Source paths take the form `.../skills/{skill-id}/SKILL.md`; the parent
// directory name is the ID. Used by every preset that emits per-skill files.
func extractSkillID(skillPath string) string {
	dir := filepath.Dir(skillPath)
	return filepath.Base(dir)
}

// shouldIncludeInOutput decides whether a content file should land in a given
// output path. Returns true when targets is empty (no restriction) or any
// target matches the output via targetMatchesOutput.
func shouldIncludeInOutput(targets []string, outputPath, baseDir string) bool {
	if len(targets) == 0 {
		return true
	}
	candidates := buildOutputPathCandidates(outputPath, baseDir)
	for _, target := range targets {
		if targetMatchesOutput(target, candidates) {
			return true
		}
	}
	return false
}

func buildOutputPathCandidates(outputPath, baseDir string) []string {
	candidateSet := make(map[string]struct{})
	addCandidate := func(value string) {
		normalized := normalizeTargetPath(value)
		if normalized == "" {
			return
		}
		candidateSet[normalized] = struct{}{}
	}

	addCandidate(outputPath)
	addCandidate(filepath.Base(outputPath))

	baseDir = normalizeTargetPath(baseDir)
	outputPath = normalizeTargetPath(outputPath)
	if baseDir != "" {
		prefix := baseDir + "/"
		if strings.HasPrefix(outputPath, prefix) {
			addCandidate(strings.TrimPrefix(outputPath, prefix))
		}
	}

	candidates := make([]string, 0, len(candidateSet))
	for candidate := range candidateSet {
		candidates = append(candidates, candidate)
	}
	return candidates
}

func normalizeTargetPath(raw string) string {
	if raw == "" {
		return ""
	}
	hasTrailingSlash := strings.HasSuffix(raw, "/") || strings.HasSuffix(raw, "\\")
	normalized := filepath.ToSlash(filepath.Clean(raw))
	normalized = strings.TrimPrefix(normalized, "./")
	if normalized == "." {
		return ""
	}
	if hasTrailingSlash && normalized != "/" {
		normalized += "/"
	}
	return normalized
}

func targetMatchesOutput(target string, outputCandidates []string) bool {
	target = normalizeTargetPath(target)
	if target == "" {
		return false
	}
	if target == presetNameClaude {
		for _, candidate := range outputCandidates {
			if candidate == fileClaudeMD || strings.HasPrefix(candidate, ".claude/") {
				return true
			}
		}
		return false
	}
	if strings.HasSuffix(target, "/") {
		prefix := strings.TrimSuffix(target, "/")
		for _, candidate := range outputCandidates {
			if candidate == prefix || strings.HasPrefix(candidate, prefix+"/") {
				return true
			}
		}
		return false
	}
	for _, candidate := range outputCandidates {
		if candidate == target {
			return true
		}
		if strings.ContainsAny(target, "*?[") {
			matched, err := path.Match(target, candidate)
			if err == nil && matched {
				return true
			}
		}
	}
	return false
}

// CombineContentFiles is the exported variant of combineContentFiles, called
// from the providers package (internal/generator/providers) which composes
// content from the same root+domain sources during DSL-backed rendering.
func CombineContentFiles(slices ...[]config.ContentFile) []config.ContentFile {
	return combineContentFiles(slices...)
}

// CombineDedupedContentFiles is the exported variant of
// combineDedupedContentFiles, used by the DSL renderer for inline rules and
// context sections where duplicate names (e.g. a builtin and an include both
// defining "commit-messages") must collapse to a single entry.
func CombineDedupedContentFiles(slices ...[]config.ContentFile) []config.ContentFile {
	return combineDedupedContentFiles(slices...)
}

// AllInlineRules / AllInlineContext expose the deduplicated inline collectors
// for the DSL renderer in internal/generator/providers.
func AllInlineRules(content *config.ContentTree) []config.ContentFile {
	return allInlineRules(content)
}

func AllInlineContext(content *config.ContentTree) []config.ContentFile {
	return allInlineContext(content)
}

// WarnDuplicateContent logs a warning for each rule or context name defined by
// more than one source. The lower-precedence copies are dropped from generated
// output by allInlineRules/allInlineContext; surfacing the collapse during both
// `generate` and `validate` lets authors notice overlap and bloat. Shared by the
// generator and the validate command so the message and precedence stay aligned.
func WarnDuplicateContent(content *config.ContentTree) {
	if content == nil {
		return
	}
	warn := func(kind string, dups []DuplicateContent) {
		for _, dup := range dups {
			logger.Warn("Duplicate "+kind+" collapsed",
				"name", dup.Name,
				"kept", dup.Winner,
				"dropped", strings.Join(dup.Losers, ", "))
		}
	}
	warn("rule", FindDuplicateContent(content.Rules, getAllDomainRules(content)))
	warn("context", FindDuplicateContent(content.Context, getAllDomainContext(content)))
}

// DuplicateContent describes a content name defined by more than one source,
// naming the surviving (highest-precedence) source path and the dropped ones.
type DuplicateContent struct {
	Name   string
	Winner string
	Losers []string
}

// FindDuplicateContent reports names that appear in more than one of the given
// precedence-ordered slices. The first occurrence of a name is the winner; the
// rest are losers. Used to warn authors that lower-priority copies (typically
// builtins) were overridden and dropped from the generated output.
func FindDuplicateContent(slices ...[]config.ContentFile) []DuplicateContent {
	type entry struct {
		winner string
		losers []string
	}
	order := make([]string, 0)
	entries := make(map[string]*entry)
	for _, slice := range slices {
		for _, file := range slice {
			if existing, ok := entries[file.Name]; ok {
				existing.losers = append(existing.losers, file.Path)
				continue
			}
			order = append(order, file.Name)
			entries[file.Name] = &entry{winner: file.Path}
		}
	}

	var duplicates []DuplicateContent
	for _, name := range order {
		e := entries[name]
		if len(e.losers) == 0 {
			continue
		}
		duplicates = append(duplicates, DuplicateContent{
			Name:   name,
			Winner: e.winner,
			Losers: e.losers,
		})
	}
	return duplicates
}

// GetAllDomainRules / GetAllDomainContext / GetAllDomainSkills /
// GetAllDomainAgents / GetAllDomainCommands expose the per-type domain
// collectors for the DSL renderer in internal/generator/providers.
func GetAllDomainRules(content *config.ContentTree) []config.ContentFile {
	return getAllDomainRules(content)
}

func GetAllDomainContext(content *config.ContentTree) []config.ContentFile {
	return getAllDomainContext(content)
}

func GetAllDomainSkills(content *config.ContentTree) []config.ContentFile {
	return getAllDomainSkills(content)
}

func GetAllDomainAgents(content *config.ContentTree) []config.ContentFile {
	return getAllDomainAgents(content)
}

func GetAllDomainCommands(content *config.ContentTree) []config.ContentFile {
	return getAllDomainCommands(content)
}

// RenderAgentsSectionExported exposes renderAgentsSection for the DSL
// renderer's `agents_delegation` root section.
func RenderAgentsSectionExported(builder *strings.Builder, content *config.ContentTree, agents []config.ContentFile) {
	renderAgentsSection(builder, content, agents)
}

// FilterContentByExplicitTargetsExported exposes filterContentByExplicitTargets
// for the DSL renderer's `targeted_rules` / `targeted_context` body sections.
func FilterContentByExplicitTargetsExported(files []config.ContentFile, outputPath, baseDir string) []config.ContentFile {
	return filterContentByExplicitTargets(files, outputPath, baseDir)
}

// ConfigFileName exposes configFileName for the DSL renderer's `header`
// root section, which needs to embed the source config filename.
func ConfigFileName(cfg *config.Config) string {
	return configFileName(cfg)
}

// sanitizeName removes special characters from names for use in filenames
func sanitizeName(name string) string {
	// Replace spaces and special chars with dashes
	replacer := strings.NewReplacer(
		" ", "-",
		"_", "-",
		"/", "-",
		"\\", "-",
	)
	sanitized := replacer.Replace(name)
	// Remove any remaining non-alphanumeric chars except dashes
	var builder strings.Builder
	for _, r := range sanitized {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' {
			builder.WriteRune(r)
		}
	}
	return strings.Trim(builder.String(), "-")
}
