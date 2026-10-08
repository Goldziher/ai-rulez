package generator

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/templates"
	"github.com/Goldziher/ai-rulez/v5/schema"
)

// computeSourceHash returns a stable blake3 hash over the inputs that determine
// generated output for the active profile. It includes:
//   - GeneratorSchemaVersion (so renderer changes invalidate)
//   - Resolved config metadata that affects rendering
//   - Resolved MCP servers
//   - Every ContentFile in root + domains, with name/path/content/metadata
//
// Determinism requires sorted iteration everywhere — Go maps must be visited
// in lexical key order, ContentFile slices must arrive in a stable order (the
// scanner relies on os.ReadDir's lexical-by-filename ordering; include-merged
// trees are ordered by the include resolver), and metadata serialization uses
// encoding/json which sorts map keys. Paths are normalized by hashPath so the
// checkout root never enters the hash.
func computeSourceHash(cfg *config.Config, content *config.ContentTree) string {
	var b strings.Builder
	b.WriteString("schema=" + templates.GeneratorSchemaVersion + "\n")
	b.WriteString("name=" + cfg.Name + "\n")
	b.WriteString("description=" + cfg.Description + "\n")
	b.WriteString("version=" + cfg.Version + "\n")
	if cfg.Run != nil && cfg.Run.Scope != nil {
		b.WriteString("scope=" + cfg.Run.Scope.Path + "\n")
	}
	b.WriteString("header_style=" + cfg.GetHeaderStyle() + "\n")
	_, _ = fmt.Fprintf(&b, "header_timestamp=%t\n", cfg.ShowHeaderTimestamp())

	// Effective rules mode per enabled preset, sorted, so switching modes
	// rewrites the affected files.
	modes := make([]string, 0, len(cfg.Presets))
	for _, preset := range cfg.Presets {
		name := preset.Name
		if name == "" {
			name = preset.BuiltIn
		}
		modes = append(modes, name+"="+cfg.RulesModeFor(name))
	}
	sort.Strings(modes)
	for _, mode := range modes {
		b.WriteString("rules_mode:" + mode + "\n")
	}

	// MCP servers — sorted by name
	mcpNames := make([]string, 0, len(cfg.MCPServers))
	for name := range cfg.MCPServers {
		mcpNames = append(mcpNames, name)
	}
	sort.Strings(mcpNames)
	for _, name := range mcpNames {
		serverJSON, err := json.Marshal(mcpServerForSourceHash(cfg.MCPServers[name], cfg.BaseDir))
		if err != nil {
			serverJSON = []byte("<marshal-error>")
		}
		b.WriteString("mcp:" + name + "=" + string(serverJSON) + "\n")
	}

	if cfg.HasSelfServer() {
		entryJSON, err := json.Marshal(cfg.SelfMCPServerEntry(schema.Version))
		if err != nil {
			entryJSON = []byte("<marshal-error>")
		}
		b.WriteString("mcp-self=" + string(entryJSON) + "\n")
	}

	// Plugins — sorted by name to match output rendering order
	plugins := append([]config.PluginConfig(nil), cfg.Plugins...)
	sort.SliceStable(plugins, func(i, j int) bool { return plugins[i].Name < plugins[j].Name })
	for _, p := range plugins {
		pluginJSON, err := json.Marshal(p)
		if err != nil {
			pluginJSON = []byte("<marshal-error>")
		}
		b.WriteString("plugin=" + string(pluginJSON) + "\n")
	}

	writeSourceContent(&b, cfg, content)
	return templates.HashContent(b.String())
}

// writeSourceContent writes every content category of the tree into a source
// hash input.
func writeSourceContent(b *strings.Builder, cfg *config.Config, content *config.ContentTree) {
	// Root content categories — slices already sorted by Name in the scanner
	writeContentFiles(b, "root.rules", content.Rules, cfg)
	writeContentFiles(b, "root.context", content.Context, cfg)
	writeContentFiles(b, "root.skills", content.Skills, cfg)
	writeContentFiles(b, "root.agents", content.Agents, cfg)
	writeContentFiles(b, "root.commands", content.Commands, cfg)
	writeContentFiles(b, "root.checks", content.Checks, cfg)

	// Domain content — domains visited in sorted name order
	domainNames := make([]string, 0, len(content.Domains))
	for name := range content.Domains {
		domainNames = append(domainNames, name)
	}
	sort.Strings(domainNames)
	for _, name := range domainNames {
		domain := content.Domains[name]
		_, _ = fmt.Fprintf(b, "domain:%s,builtin=%t,from_include=%t\n", name, domain.Builtin, domain.FromInclude)
		writeContentFiles(b, "domain."+name+".rules", domain.Rules, cfg)
		writeContentFiles(b, "domain."+name+".context", domain.Context, cfg)
		writeContentFiles(b, "domain."+name+".skills", domain.Skills, cfg)
		writeContentFiles(b, "domain."+name+".agents", domain.Agents, cfg)
		writeContentFiles(b, "domain."+name+".commands", domain.Commands, cfg)
		writeContentFiles(b, "domain."+name+".checks", domain.Checks, cfg)
	}
}

// computeSharedSourceHash is the Source-Hash of the shared outputs (AGENTS.md,
// .agents/skills). It covers the content and the settings that shape these files
// and deliberately not the preset list, per-preset rules modes, MCP servers or
// plugins: the files are written once for every preset that reads them, so adding
// or removing a preset must not change their provenance line unless it changes
// the file. Two inputs of that kind are included: what AGENTS.md inlines for
// presets without a rules folder, and, when some rule or context item has
// frontmatter targets, its owners (the configured presets relying on it, whose
// names and root files those targets select items by). Without targets the
// owners cannot change the file, so they stay out and the line stays stable.
func computeSharedSourceHash(cfg *config.Config, content *config.ContentTree, inlining config.AgentsMDInlining,
	owners []string,
) string {
	var b strings.Builder
	b.WriteString("schema=" + templates.GeneratorSchemaVersion + "\n")
	b.WriteString("shared=agents_md\n")
	_, _ = fmt.Fprintf(&b, "inline_scoped=%t,inline_auto_manual=%t\n", inlining.Scoped, inlining.AutoManual)
	if hasTargetedRulesOrContext(content) {
		b.WriteString("agents_md_owners=" + strings.Join(slices.Sorted(slices.Values(owners)), ",") + "\n")
	}
	b.WriteString("name=" + cfg.Name + "\n")
	b.WriteString("description=" + cfg.Description + "\n")
	b.WriteString("version=" + cfg.Version + "\n")
	if cfg.Run != nil && cfg.Run.Scope != nil {
		b.WriteString("scope=" + cfg.Run.Scope.Path + "\n")
	}
	b.WriteString("header_style=" + cfg.GetHeaderStyle() + "\n")
	_, _ = fmt.Fprintf(&b, "header_timestamp=%t\n", cfg.ShowHeaderTimestamp())
	_, _ = fmt.Fprintf(&b, "compact=%t\n", cfg.IsCompact())
	writeSourceContent(&b, cfg, content)
	return templates.HashContent(b.String())
}

// hasTargetedRulesOrContext reports whether any rule or context item restricts
// itself with frontmatter targets.
func hasTargetedRulesOrContext(content *config.ContentTree) bool {
	targeted := func(files []config.ContentFile) bool {
		return slices.ContainsFunc(files, func(f config.ContentFile) bool {
			return f.Metadata != nil && len(f.Metadata.Targets) > 0
		})
	}
	if targeted(content.Rules) || targeted(content.Context) {
		return true
	}
	for _, domain := range content.Domains {
		if targeted(domain.Rules) || targeted(domain.Context) {
			return true
		}
	}
	return false
}

// builtinPathScheme prefixes the synthetic paths used for embedded builtin
// content, which are already location-independent.
const builtinPathScheme = "builtin://"

// hashPath returns a location-independent identifier for a content file so the
// source hash stays stable across checkout roots, machines, and operating
// systems. ContentFile.Path is absolute for everything the scanner reads from
// disk, so hashing it raw made the checkout root part of the hash: the same
// tree generated from two directories produced different Source-Hash values,
// forcing a rewrite and a fresh timestamp on every relocated checkout (#166).
//
// Out-of-tree content — git includes cached under the user's home directory,
// installed skills, the temp symlink used for bare include layouts — collapses
// to the last two segments. That is exactly what rendering consumes: presets
// derive the skill id from filepath.Base(filepath.Dir(path)) via
// extractSkillID, so the directory name still busts the hash while the
// machine-specific prefix never enters it.
func hashPath(path string, cfg *config.Config) string {
	if path == "" || strings.HasPrefix(path, builtinPathScheme) {
		return path
	}

	for _, root := range []string{cfg.ConfigDir, cfg.BaseDir} {
		if root == "" {
			continue
		}
		rel, err := filepath.Rel(root, path)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		return filepath.ToSlash(rel)
	}

	return filepath.ToSlash(filepath.Join(filepath.Base(filepath.Dir(path)), filepath.Base(path)))
}

// writeContentFiles serializes a ContentFile slice into the hash builder.
// Slices reach this function in the scanner's os.ReadDir order, which is
// lexical by filename; include-merged trees are ordered by the include
// resolver instead.
func writeContentFiles(b *strings.Builder, label string, files []config.ContentFile, cfg *config.Config) {
	for i := range files {
		f := files[i]
		b.WriteString(label + ":" + f.Name + "|path=" + hashPath(f.Path, cfg) + "|content=" + f.Content)
		if f.Metadata != nil {
			metaJSON, err := json.Marshal(f.Metadata)
			if err != nil {
				metaJSON = []byte("<marshal-error>")
			}
			b.WriteString("|meta=" + string(metaJSON))
		}
		// Include skill resources in the source hash so changes to
		// references/, scripts/, or assets/ bust the SKILL.md skip-cache
		// (the resource index is derived from these and would otherwise
		// stay stale).
		//
		// Length-prefix the content so resource bytes can never spoof
		// another resource record's delimiter — without this, a reference
		// whose body happened to contain `|res=ref:other:...` could be
		// indistinguishable from two separate resources.
		for _, r := range f.Resources {
			fmt.Fprintf(b, "|res=%s:%s:len=%d:", r.Kind, r.RelPath, len(r.Content))
			b.Write(r.Content)
		}
		b.WriteByte('\n')
	}
}
