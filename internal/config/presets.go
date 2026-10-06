package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/generator/jsonmerge"
)

// PresetGenerator defines the interface for preset generators
type PresetGenerator interface {
	Generate(content *ContentTree, baseDir string, config *Config) ([]OutputFile, error)
	GetOutputPaths(baseDir string) []string
	GetName() string
}

// OutputFile represents a generated output file or directory.
//
// When RawContent is non-nil, the file is written verbatim — no header
// injection, no hash bookkeeping, no trailing-newline normalization. Use this
// for skill supporting files (references, scripts, assets) where any
// ai-rulez-generated marker would corrupt the payload (e.g. Python scripts,
// binary assets).
//
// When RawContent is nil, Content is rendered through the standard
// generation pipeline (header banner + content/source hashes).
//
// Mode is the file permission bits to apply on write (0o644 if zero). Used
// to preserve the executable bit on bundled skill scripts so the agent can
// invoke them directly. Only honored on the raw-content write path.
//
// LocalOnly marks an output rendered from machine-local override content
// (.ai-rulez/local/ → CLAUDE.local.md, AGENTS.local.md, ...). Such outputs are
// kept out of git unconditionally — even when config gitignore is disabled —
// because committing them would leak machine-local configuration. A ".local."
// name is covered by a pattern in the managed .gitignore block; a path that
// exists only on this machine goes to .git/info/exclude (the .gitignore block
// when that is not possible).
type OutputFile struct {
	Path       string
	Content    string
	RawContent []byte
	Mode       os.FileMode
	IsDir      bool
	LocalOnly  bool
	// SourceHash, when set, replaces the run's Source-Hash in this output's
	// header. The agents_md shared outputs set their own (a hash of shared
	// content only); the drift guard stamps machine-local outputs with a hash of
	// their local inputs and every other output without one with the baseline hash.
	SourceHash string
	// PartiallyOwned marks a file ai-rulez only contributes some of the content
	// to — a settings document where it owns one top-level key and the consumer
	// hand-authors and tracks the rest. Such a file must not be added to the
	// managed .gitignore block, and must not be deleted as stale when it stops
	// being generated: both would destroy or hide user-authored data (#185).
	PartiallyOwned bool
	// Committed marks an output that must stay tracked in git even when
	// gitignore management is on: hosted reviewers (Bugbot, Kilo, Rovo Dev, ...)
	// only read committed files, so ignoring a check output would hide it.
	Committed bool
	// Sensitive marks an output that carries resolved MCP secrets (env values or
	// headers). It is written owner-only (0600), and an existing file is
	// tightened to that mode.
	Sensitive bool
	// OmitsRules marks a root instruction file that leaves out the rules and
	// context its tool reads from its own rules folder (junie's AGENTS.md in the
	// split rules mode). When another preset writes the same path with every rule
	// inlined, that file is the complete one and replaces this.
	OmitsRules bool
	// MergeClaims records what ai-rulez wrote into a PartiallyOwned (or merged)
	// JSON document, so it can take exactly that back out on clean or when the
	// preset or server that wrote it goes away.
	MergeClaims []jsonmerge.Claim
	// Merge, when set, lets two presets that write one document with different
	// content be combined instead of reported as a conflict: each preset's owned
	// keys are unioned and the document rendered once.
	Merge *MergeSource
}

// MergeFormatOwnedHooks is the MergeSource.Format of a hooks file ai-rulez writes
// whole (Copilot's .github/hooks/ai-rulez.json) rather than merges into.
const MergeFormatOwnedHooks = "owned-hooks"

// MergeSource is what an output was rendered from: the document (Path, the file
// the merge read), its format (a docmerge format, or MergeFormatOwnedHooks) and
// the keys ai-rulez owns in it.
type MergeSource struct {
	Path   string
	Format string
	Owned  []jsonmerge.OwnedKey
}

// LocalRootProvider is implemented by preset generators that emit a single
// markdown root instructions file and therefore support a machine-local ".local"
// variant of it (CLAUDE.md → CLAUDE.local.md). LocalRootFile returns the local
// variant's path relative to the output base dir, or "" when the preset has no
// single-file markdown root (e.g. cursor, devin) and so has no local variant.
type LocalRootProvider interface {
	LocalRootFile() string
}

// LocalRootStandIn is implemented by LocalRootProviders whose local file is not
// the ".local" variant of the root file it stands in for (a file in a rules
// folder). LocalRootStandsIn returns that root file, which frontmatter targets
// are matched against.
type LocalRootStandIn interface {
	LocalRootStandsIn() string
}

// LocalRootRenderer is implemented by presets whose machine-local root file is
// not the generic markdown override (Copilot writes a path-specific instructions
// file with an applyTo frontmatter). RenderLocalRoot builds that file from the
// local rules the preset keeps inline and the local context; local is the
// profile-selected local content tree.
type LocalRootRenderer interface {
	RenderLocalRoot(local *ContentTree, rules []ContentFile, baseDir string, cfg *Config) (OutputFile, error)
}

// LocalRuleProvider is implemented by preset generators that write one native
// rule file per rule. LocalRuleOutputs plans machine-local rules with the same
// routing the preset applies to shared rules and renders the ones it routes to
// rule files as personal overrides, "<rulesdir>/<id>.local<ext>", which the tool
// loads natively. The outputs are LocalOnly. Rules the routing keeps inline are
// returned in inline for the local root file. Local rule IDs never contain a
// path separator (local rules are not scoped).
type LocalRuleProvider interface {
	LocalRuleOutputs(rules []ContentFile, baseDir string, cfg *Config) (files []OutputFile, inline []ContentFile, err error)
}

// GeneratePresets generates all configured presets for a config
func GeneratePresets(cfg *Config) (map[string][]OutputFile, error) {
	if cfg.Content == nil {
		return nil, ErrNoContent
	}

	results := make(map[string][]OutputFile)
	cfg.WarnDeliveryFallbacks()

	for _, preset := range cfg.Presets {
		var outputs []OutputFile
		var generator PresetGenerator
		var err error

		switch {
		case preset.IsBuiltIn():
			generator, err = cfg.Registry.Generator(preset.BuiltIn)
		case preset.Provider != "":
			if cfg.Registry == nil || cfg.Registry.Provider == nil {
				return nil, fmt.Errorf("provider preset generator factory not initialized")
			}
			generator, err = cfg.Registry.Provider(preset, cfg.BaseDir, cfg.View())
		default:
			if cfg.Registry == nil || cfg.Registry.Custom == nil {
				return nil, fmt.Errorf("custom preset generator factory not initialized")
			}
			generator = cfg.Registry.Custom(preset)
		}
		if err != nil {
			return nil, fmt.Errorf("resolve preset %s: %w", preset.GetName(), err)
		}
		if owner, ok := generator.(RulesDirOwner); ok && !preset.IsBuiltIn() {
			cfg.AddRulesDir(owner.SplitRulesDir())
		}

		outputs, err = generator.Generate(cfg.ContentForPreset(preset.GetName()), cfg.BaseDir, cfg)
		if err != nil {
			return nil, fmt.Errorf("generate preset %s: %w", preset.GetName(), err)
		}

		results[preset.GetName()] = outputs
		// Stamp the preset name onto every output. DSL-backed providers have
		// already registered theirs with an exact kind and a per-section
		// breakdown; this pass covers the hand-written generators, whose kind is
		// inferred from the path. No-op when analysis is disabled.
		cfg.Analysis.Attribute(preset.GetName(), cfg.BaseDir, outputs)
	}

	return results, nil
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

// extractSkillID extracts the skill ID from a skill's path
func extractSkillID(skillPath string) string {
	// Path format: .../skills/{skill-id}/SKILL.md
	dir := filepath.Dir(skillPath)
	return filepath.Base(dir)
}

// combineContent combines multiple ContentFile slices
func combineContent(slices ...[]ContentFile) []ContentFile {
	var total int
	for _, slice := range slices {
		total += len(slice)
	}

	result := make([]ContentFile, 0, total)
	for _, slice := range slices {
		result = append(result, slice...)
	}
	return result
}
