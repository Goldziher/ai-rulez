package generator

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/generator/plugin"
	"github.com/Goldziher/ai-rulez/internal/generator/presets"     // Register remaining legacy preset generators
	_ "github.com/Goldziher/ai-rulez/internal/generator/providers" // Register DSL-backed preset generators (overrides legacy registrations where they overlap)
	"github.com/Goldziher/ai-rulez/internal/gitignore"
	"github.com/Goldziher/ai-rulez/internal/logger"
	"github.com/Goldziher/ai-rulez/internal/templates"
	"github.com/samber/oops"
)

const defaultProfileName = "default"
const generatedManifestName = ".generated-manifest.json"

// Generator handles configuration generation
type Generator struct {
	config *config.Config
}

type generatedManifest struct {
	Version string   `json:"version"`
	Files   []string `json:"files"`
}

// NewGenerator creates a new generator
func NewGenerator(cfg *config.Config) *Generator {
	return &Generator{
		config: cfg,
	}
}

// Generate generates all outputs for the specified profile
func (g *Generator) Generate(profile string) error {
	flatOutputs, activeProfile, err := g.collectOutputs(profile)
	if err != nil {
		return err
	}

	logger.Info("Generating with configuration", "profile", activeProfile)

	if err := g.ensureSecretOutputsIgnored(flatOutputs); err != nil {
		return err
	}

	staleFiles := g.staleManifestFiles(flatOutputs)
	g.removeStaleManifestFiles(staleFiles)

	// Write all output files
	if err := g.writeOutputs(flatOutputs); err != nil {
		return err
	}

	if err := g.writeGeneratedManifest(flatOutputs); err != nil {
		logger.Warn("Failed to write generated manifest", "error", err)
	}

	// Update gitignore if enabled
	if g.config.ShouldUpdateGitignore() {
		if err := g.updateGitignore(flatOutputs); err != nil {
			logger.Warn("Failed to update .gitignore", "error", err)
		}
	}

	logger.Info("Generation complete", "files", len(flatOutputs))

	return nil
}

// GeneratePlugin packages the project into distributable plugin bundles plus a
// marketplace index for the runtimes named in the [plugin] block. Unlike the
// normal generate path, plugin outputs are written verbatim (RawContent) and do
// not participate in the generated-manifest / stale-file bookkeeping.
func (g *Generator) GeneratePlugin(profile string) error {
	outputs, err := g.collectPluginOutputs(profile)
	if err != nil {
		return err
	}
	if err := g.writeOutputs(outputs); err != nil {
		return err
	}
	logger.Info("Plugin generation complete", "files", len(outputs))
	return nil
}

// VerifyPlugin verifies the generated plugin bundles against their provenance
// sidecars without regenerating or modifying files.
func (g *Generator) VerifyPlugin(profile string) error {
	expected, err := g.collectPluginOutputs(profile)
	if err != nil {
		return oops.Wrapf(err, "render expected plugin outputs")
	}
	for _, output := range expected {
		if output.IsDir {
			continue
		}
		actual, readErr := os.ReadFile(output.Path)
		if readErr != nil {
			return oops.With("path", output.Path).Wrapf(readErr, "read generated plugin output")
		}
		expectedBytes := output.RawContent
		if expectedBytes == nil {
			expectedBytes = []byte(output.Content)
		}
		if !bytes.Equal(actual, expectedBytes) {
			return oops.With("path", output.Path).
				Hint("Run ai-rulez generate --plugin and commit the regenerated output").
				Errorf("generated plugin output is stale")
		}
	}
	if marketplace := g.config.Marketplace; marketplace != nil && len(marketplace.Members) > 0 {
		for _, member := range marketplace.Members {
			if err := plugin.VerifyProvenance(filepath.Join(g.config.BaseDir, member)); err != nil {
				return oops.With("member", member).Wrapf(err, "verify member plugin bundle")
			}
		}
	}
	return plugin.VerifyProvenance(g.config.BaseDir)
}

// DryRunPlugin returns the plugin generation plan without writing files.
func (g *Generator) DryRunPlugin(profile string) ([]string, error) {
	outputs, err := g.collectPluginOutputs(profile)
	if err != nil {
		return nil, err
	}
	lines := make([]string, 0, len(outputs)+1)
	lines = append(lines, "plugin bundle:")
	for _, output := range outputs {
		lines = append(lines, "write-file: "+g.convertToRelativePath(g.absOutputPath(output.Path)))
	}
	return lines, nil
}

// collectPluginOutputs resolves the content tree and MCP servers, builds the
// plugin manifest, and renders all requested runtime bundles + marketplace. When
// the config is a monorepo root ([marketplace].members set), it instead renders
// each member's bundle plus the aggregate marketplace index.
func (g *Generator) collectPluginOutputs(profile string) ([]config.OutputFile, error) {
	if mkt := g.config.Marketplace; mkt != nil && len(mkt.Members) > 0 {
		return g.collectMonorepoOutputs(mkt)
	}

	if g.config.Plugin == nil {
		return nil, oops.
			Hint("Add a [plugin] block (or a [marketplace] with members) to your config").
			Errorf("no [plugin] block configured; nothing to generate with --plugin")
	}

	manifest, err := g.buildPluginManifest(profile)
	if err != nil {
		return nil, err
	}
	return plugin.Generate(manifest, g.config.BaseDir)
}

// buildPluginManifest resolves the content tree and MCP servers for the active
// profile and builds the plugin manifest for the current config.
func (g *Generator) buildPluginManifest(profile string) (*plugin.Manifest, error) {
	if err := g.resolveMCPEnv(); err != nil {
		return nil, err
	}

	activeProfile := g.resolveProfile(profile)
	contentTree, err := g.getContentForProfile(activeProfile)
	if err != nil {
		return nil, err
	}
	if g.config.Plugin.ContentRoot != "" {
		contentTree, err = config.ScanContentTree(filepath.Join(g.config.BaseDir, g.config.Plugin.ContentRoot))
		if err != nil {
			return nil, oops.With("content_root", g.config.Plugin.ContentRoot).Wrapf(err, "scan plugin content")
		}
	}

	mcpServers := g.collectMCPServersForContent(contentTree)

	tempCfg := *g.config
	tempCfg.Content = contentTree
	tempCfg.MCPServers = mcpServers

	return plugin.BuildManifest(&tempCfg, contentTree)
}

// collectMonorepoOutputs renders every member plugin under its source directory
// and emits the aggregate root marketplace index. Each member is an independent
// ai-rulez project loaded from <baseDir>/<member>.
func (g *Generator) collectMonorepoOutputs(mkt *config.MarketplaceAuthoring) ([]config.OutputFile, error) {
	var outputs []config.OutputFile
	entries := make([]plugin.MemberEntry, 0, len(mkt.Members))

	for _, member := range mkt.Members {
		memberDir := filepath.Join(g.config.BaseDir, member)
		memberCfg, err := config.LoadConfig(context.Background(), memberDir)
		if err != nil {
			return nil, oops.With("member", member).Wrapf(err, "load monorepo member config")
		}
		if memberCfg.Plugin == nil {
			return nil, oops.
				With("member", member).
				Hint("Each monorepo member must define its own [plugin] block").
				Errorf("monorepo member %q has no [plugin] block", member)
		}

		memberGen := NewGenerator(memberCfg)
		manifest, err := memberGen.buildPluginManifest("")
		if err != nil {
			return nil, oops.With("member", member).Wrapf(err, "build member manifest")
		}

		memberOutputs, err := plugin.GenerateMember(manifest, memberCfg.BaseDir)
		if err != nil {
			return nil, oops.With("member", member).Wrapf(err, "generate member bundle")
		}
		outputs = append(outputs, memberOutputs...)

		entries = append(entries, plugin.MemberEntry{
			Name:        manifest.Name,
			Description: manifest.Description,
			Source:      "./" + filepath.ToSlash(member),
			Category:    manifest.Category,
		})
	}

	market := plugin.ResolveMarketInfo(mkt)
	marketplaceOutput, err := plugin.RenderMonorepoMarketplace(market, entries, g.config.BaseDir)
	if err != nil {
		return nil, oops.Wrapf(err, "render monorepo marketplace")
	}
	codexMarketplaceOutput, err := plugin.RenderCodexMonorepoMarketplace(market, entries, g.config.BaseDir)
	if err != nil {
		return nil, oops.Wrapf(err, "render Codex monorepo marketplace")
	}
	rootOutputs, err := plugin.AddProvenance(
		[]config.OutputFile{marketplaceOutput, codexMarketplaceOutput},
		g.config.BaseDir,
	)
	if err != nil {
		return nil, oops.Wrapf(err, "add marketplace provenance")
	}
	return append(outputs, rootOutputs...), nil
}

// DryRun returns an inspectable generation plan without writing or deleting files.
func (g *Generator) DryRun(profile string) ([]string, error) {
	flatOutputs, activeProfile, err := g.collectOutputs(profile)
	if err != nil {
		return nil, err
	}

	lines := []string{fmt.Sprintf("profile: %s", activeProfile)}
	for _, output := range flatOutputs {
		relPath := g.convertToRelativePath(g.absOutputPath(output.Path))
		if output.IsDir {
			lines = append(lines, "create-dir: "+relPath)
		} else {
			lines = append(lines, "write-file: "+relPath)
		}
	}
	for _, stale := range g.staleManifestFiles(flatOutputs) {
		lines = append(lines, "delete-stale: "+g.convertToRelativePath(stale))
	}
	return lines, nil
}

func (g *Generator) collectOutputs(profile string) ([]config.OutputFile, string, error) {
	if err := g.resolveMCPEnv(); err != nil {
		return nil, "", err
	}

	activeProfile := g.resolveProfile(profile)

	contentTree, err := g.getContentForProfile(activeProfile)
	if err != nil {
		return nil, "", err
	}

	logger.Debug("Content scanned",
		"rules", len(contentTree.Rules),
		"context", len(contentTree.Context),
		"skills", len(contentTree.Skills),
		"agents", len(contentTree.Agents),
		"domains", len(contentTree.Domains))

	presets.WarnDuplicateContent(contentTree)

	// Collect MCP servers based on the resolved content tree
	mcpServers := g.collectMCPServersForContent(contentTree)

	// Create a temporary config with the filtered content and MCP servers
	tempCfg := *g.config
	tempCfg.Content = contentTree
	tempCfg.MCPServers = mcpServers

	// Compute a single source hash covering all profile-relevant inputs.
	// Embedded in every output's header so subsequent runs can detect "no source
	// change" cheaply, and combined with Content-Hash for skip decisions.
	// We set it on both tempCfg (used by preset rendering) and g.config (used
	// by writeOutput's skip decision and hash injection).
	sourceHash := computeSourceHash(&tempCfg, contentTree)
	tempCfg.SourceHash = sourceHash
	g.config.SourceHash = sourceHash

	// Generate outputs for all presets using the existing infrastructure
	allOutputs, err := config.GeneratePresets(&tempCfg)
	if err != nil {
		return nil, "", oops.Wrapf(err, "generate presets")
	}

	// Auto-generate MCP output if servers exist. The MCP preset is now
	// DSL-driven (internal/generator/providers/builtin/mcp.toml); fetch it
	// from the registry rather than instantiating a hand-written generator.
	if len(mcpServers) > 0 {
		mcpGen, err := config.GetPresetGenerator("mcp")
		if err != nil {
			logger.Warn("Failed to resolve MCP preset generator", "error", err)
		} else {
			mcpOutputs, err := mcpGen.Generate(contentTree, g.config.BaseDir, &tempCfg)
			if err != nil {
				logger.Warn("Failed to generate MCP output", "error", err)
			} else if len(mcpOutputs) > 0 {
				allOutputs["mcp"] = mcpOutputs
				logger.Debug("Auto-generated MCP output", "count", len(mcpOutputs))
			}
		}
	}

	// Flatten outputs for writing, detecting conflicts and deduplicating
	flatOutputs := flattenPresetOutputs(allOutputs)

	scopedOutputs, err := g.generateScopedOutputs(activeProfile)
	if err != nil {
		return nil, "", err
	}
	flatOutputs = append(flatOutputs, scopedOutputs...)

	return flatOutputs, activeProfile, nil
}

// resolveProfile determines which profile to use
func (g *Generator) resolveProfile(profile string) string {
	// 1. Use provided profile if specified
	if profile != "" {
		return profile
	}

	// 2. Use default profile from config if specified
	if g.config.Default != "" {
		return g.config.Default
	}

	// 3. Use "default" as fallback
	return defaultProfileName
}

// getContentForProfile returns the content tree for a specific profile
func (g *Generator) getContentForProfile(profile string) (*config.ContentTree, error) {
	// Guard against nil content to avoid panics when Generator is created
	// with a Config that hasn't been fully loaded.
	if g.config.Content == nil {
		return nil, config.ErrNoContent
	}

	// Special case: "default" profile
	if profile == defaultProfileName {
		// If the user explicitly defined profiles["default"], honor it like any
		// other named profile rather than applying the built-in fallback logic.
		if g.config.HasProfile(defaultProfileName) {
			return g.config.GetContentForProfile(defaultProfileName)
		}

		// When no profiles are defined, "default" should include all content
		// (root + all domains). This is important for consumers that rely on
		// includes and don't define their own profiles.
		if len(g.config.Profiles) == 0 {
			// Shallow-copy domains to avoid exposing internal map for mutation.
			domainsCopy := make(map[string]*config.Domain, len(g.config.Content.Domains))
			for name, domain := range g.config.Content.Domains {
				domainsCopy[name] = domain
			}

			return &config.ContentTree{
				Rules:    g.config.Content.Rules,
				Context:  g.config.Content.Context,
				Skills:   g.config.Content.Skills,
				Agents:   g.config.Content.Agents,
				Commands: g.config.Content.Commands,
				Domains:  domainsCopy,
			}, nil
		}

		// When profiles are defined in the config, keep the previous
		// behavior where the built-in "default" profile only sees
		// root content and (optionally) built-in domains. FromInclude
		// domains are always included regardless of profile definitions.
		defaultDomains := make(map[string]*config.Domain)
		for name, domain := range g.config.Content.Domains {
			if domain.Builtin || domain.FromInclude {
				defaultDomains[name] = domain
			}
		}
		return &config.ContentTree{
			Rules:    g.config.Content.Rules,
			Context:  g.config.Content.Context,
			Skills:   g.config.Content.Skills,
			Agents:   g.config.Content.Agents,
			Commands: g.config.Content.Commands,
			Domains:  defaultDomains,
		}, nil
	}

	// Check if profile exists
	if !g.config.HasProfile(profile) {
		availableProfiles := make([]string, 0, len(g.config.Profiles))
		for name := range g.config.Profiles {
			availableProfiles = append(availableProfiles, name)
		}
		sort.Strings(availableProfiles)

		return nil, oops.
			With("profile", profile).
			With("available_profiles", availableProfiles).
			Hint(fmt.Sprintf(
				"Available profiles: %v\nUse 'default' for the built-in profile (all content when no profiles are defined; root content plus builtin and FromInclude domains when profiles are defined).",
				availableProfiles,
			)).
			Errorf("profile not found: %s", profile)
	}

	// Get content for the profile (includes root + specified domains)
	return g.config.GetContentForProfile(profile)
}

// collectMCPServersForContent collects MCP servers for the resolved content tree.
// Only include enabled servers from root config.
func (g *Generator) collectMCPServersForContent(content *config.ContentTree) map[string]*config.MCPServer {
	collected := make(map[string]*config.MCPServer)

	// Include root servers (if enabled)
	for name, server := range g.config.MCPServers {
		if server.IsEnabled() {
			collected[name] = server
		}
	}

	return collected
}

func (g *Generator) generateScopedOutputs(activeProfile string) ([]config.OutputFile, error) {
	var result []config.OutputFile
	for _, scope := range g.config.Scopes {
		if scope.Path == "" {
			return nil, oops.Errorf("scope path is required")
		}
		scopeProfile := scope.Profile
		if scopeProfile == "" {
			scopeProfile = activeProfile
		}
		scopeContent, err := g.getContentForProfile(scopeProfile)
		if err != nil {
			return nil, oops.With("scope", scope.Name).With("path", scope.Path).Wrapf(err, "resolve scope profile")
		}
		scopeCfg := *g.config
		scopeCfg.BaseDir = filepath.Join(g.config.BaseDir, scope.Path)
		scopeCfg.Content = scopeContent
		scopeCfg.MCPServers = g.collectMCPServersForContent(scopeContent)
		scopeCfg.Presets = scopedPresets(scope.Presets)
		scopeCfg.SourceHash = computeSourceHash(&scopeCfg, scopeContent)

		outputsByPreset, err := config.GeneratePresets(&scopeCfg)
		if err != nil {
			return nil, oops.With("scope", scope.Name).With("path", scope.Path).Wrapf(err, "generate scoped presets")
		}
		result = append(result, flattenPresetOutputs(outputsByPreset)...)
	}
	return result, nil
}

func scopedPresets(names []string) []config.Preset {
	if len(names) == 0 {
		names = []string{"claude", "codex"}
	}
	result := make([]config.Preset, 0, len(names))
	for _, name := range names {
		result = append(result, config.Preset{BuiltIn: name})
	}
	return result
}

// writeOutputs writes all output files to disk
// flattenPresetOutputs merges outputs from all presets, deduplicating directories
// and detecting file conflicts (last write wins with a warning).
func flattenPresetOutputs(allOutputs map[string][]config.OutputFile) []config.OutputFile {
	var flatOutputs []config.OutputFile
	seenPaths := make(map[string]string) // path -> first preset that claimed it
	seenDirs := make(map[string]bool)
	presetNames := make([]string, 0, len(allOutputs))
	for presetName := range allOutputs {
		presetNames = append(presetNames, presetName)
	}
	sort.Strings(presetNames)
	for _, presetName := range presetNames {
		outputs := allOutputs[presetName]
		logger.Debug("Generated outputs for preset", "preset", presetName, "count", len(outputs))
		for _, output := range outputs {
			if output.IsDir {
				if !seenDirs[output.Path] {
					seenDirs[output.Path] = true
					flatOutputs = append(flatOutputs, output)
				}
				continue
			}
			if prev, ok := seenPaths[output.Path]; ok {
				// Multiple presets emitting the same path (e.g. cursor + copilot + auto-mcp
				// all writing .mcp.json) is the expected case, not a configuration error.
				logger.Debug("Multiple presets write to the same file, last write wins",
					"path", output.Path, "presets", prev+" and "+presetName)
				// Replace the existing entry with the latest version
				for i, existing := range flatOutputs {
					if existing.Path == output.Path {
						flatOutputs[i] = output
						break
					}
				}
				seenPaths[output.Path] = presetName
			} else {
				seenPaths[output.Path] = presetName
				flatOutputs = append(flatOutputs, output)
			}
		}
	}
	return flatOutputs
}

func (g *Generator) writeOutputs(outputs []config.OutputFile) error {
	for _, output := range outputs {
		if err := g.writeOutput(output); err != nil {
			return oops.
				With("path", output.Path).
				Wrapf(err, "write output file")
		}
	}
	return nil
}

func (g *Generator) absOutputPath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(g.config.BaseDir, path)
}

// writeOutput writes a single output file or creates a directory.
//
// Skip decision: we hash the freshly-rendered body and compare two values
// embedded in the existing file's header — the Source-Hash (changes when any
// source input changes) and the Content-Hash (changes when this output's body
// changes). The comparison is against the in-header hashes, never against the
// on-disk body, so skipping is robust to formatters or other tools that touch
// the body after we wrote it. We only skip when BOTH match: a Source-Hash
// mismatch alone forces a rewrite to refresh stale provenance metadata even
// when the body would round-trip identically.
//
// On write, output is normalized to end with exactly one trailing newline so
// formatters that enforce that convention (end-of-file-fixer, etc.) don't
// spuriously modify the file after generation.
func (g *Generator) writeOutput(output config.OutputFile) error {
	absPath := g.absOutputPath(output.Path)

	if output.IsDir {
		if err := os.MkdirAll(absPath, 0o755); err != nil {
			return oops.
				With("dir", absPath).
				Hint(fmt.Sprintf("Check directory permissions for: %s", absPath)).
				Wrapf(err, "create directory")
		}
		logger.Debug("Created directory", "path", output.Path)
		return nil
	}

	// Raw mode: write bytes verbatim. Used for skill resources (references,
	// scripts, assets) where the standard header banner would corrupt the
	// payload (e.g. Python scripts) or break binary files.
	if output.RawContent != nil {
		return writeRawOutput(absPath, output)
	}

	body := stripHeader(output.Content, output.Path)
	contentHash := templates.HashContent(body)

	existingContentHash, existingSourceHash := extractStoredHashes(absPath)
	if existingContentHash != "" && existingContentHash == contentHash &&
		existingSourceHash == g.config.SourceHash {
		logger.Debug("Skipped unchanged file",
			"path", output.Path,
			"content_hash", contentHash,
			"source_hash", g.config.SourceHash)
		return nil
	}

	finalContent := injectHashes(output.Content, output.Path, contentHash, g.config.SourceHash)
	finalContent = normalizeTrailingNewline(finalContent)

	dir := filepath.Dir(absPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return oops.
			With("dir", dir).
			With("path", absPath).
			Hint(fmt.Sprintf("Check directory permissions for: %s", dir)).
			Wrapf(err, "create parent directory")
	}

	if err := os.WriteFile(absPath, []byte(finalContent), 0o644); err != nil {
		return oops.
			With("path", absPath).
			Hint(fmt.Sprintf("Check write permissions for: %s", absPath)).
			Wrapf(err, "write file")
	}

	logger.Debug("Wrote file", "path", output.Path, "size", len(finalContent))
	return nil
}

// writeRawOutput writes an OutputFile with non-nil RawContent verbatim,
// preserving Mode (defaulting to 0o644). Skips the write when both the
// existing bytes and mode already match on disk so unchanged bundled
// assets don't dirty the working tree on every regeneration.
func writeRawOutput(absPath string, output config.OutputFile) error {
	mode := output.Mode.Perm()
	if mode == 0 {
		mode = 0o644
	}

	if rawWriteCanSkip(absPath, output.RawContent, mode) {
		logger.Debug("Skipped unchanged raw file", "path", output.Path)
		return nil
	}

	dir := filepath.Dir(absPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return oops.
			With("dir", dir).
			With("path", absPath).
			Hint(fmt.Sprintf("Check directory permissions for: %s", dir)).
			Wrapf(err, "create parent directory")
	}
	if err := os.WriteFile(absPath, output.RawContent, mode); err != nil {
		return oops.
			With("path", absPath).
			Hint(fmt.Sprintf("Check write permissions for: %s", absPath)).
			Wrapf(err, "write file")
	}
	// os.WriteFile only applies the mode on file creation. To handle mode
	// changes on subsequent regenerations, chmod explicitly.
	if err := os.Chmod(absPath, mode); err != nil {
		return oops.
			With("path", absPath).
			With("mode", mode).
			Wrapf(err, "set file mode")
	}
	logger.Debug("Wrote raw file", "path", output.Path, "size", len(output.RawContent), "mode", mode)
	return nil
}

func rawWriteCanSkip(absPath string, payload []byte, mode os.FileMode) bool {
	existing, err := os.ReadFile(absPath)
	if err != nil || !bytes.Equal(existing, payload) {
		return false
	}
	info, err := os.Stat(absPath)
	if err != nil {
		return false
	}
	return info.Mode().Perm() == mode
}

// normalizeTrailingNewline ensures the file ends with exactly one '\n'.
// Empty content stays empty.
func normalizeTrailingNewline(content string) string {
	if content == "" {
		return content
	}
	trimmed := strings.TrimRight(content, "\n")
	return trimmed + "\n"
}

// extractContentHash returns just the Content-Hash header value (or empty if absent).
// Kept for tests and consumers that don't care about Source-Hash.
func extractContentHash(filePath string) string {
	contentHash, _ := extractStoredHashes(filePath)
	return contentHash
}

// extractStoredHashes scans the header of an existing file and returns the
// Content-Hash and Source-Hash values found there (empty strings if missing).
// It reads up to 60 lines to accommodate detailed headers.
func extractStoredHashes(filePath string) (contentHash, sourceHash string) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", ""
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for i := 0; i < 60 && scanner.Scan(); i++ {
		line := strings.TrimSpace(scanner.Text())
		// Strip common comment prefixes so we can find hash lines
		// regardless of comment style (HTML, hash, slash, semicolon).
		stripped := line
		for _, prefix := range []string{"<!--", "-->", "//", "#", ";", "/*", "*/"} {
			stripped = strings.TrimPrefix(stripped, prefix)
		}
		stripped = strings.TrimSpace(stripped)

		switch {
		case strings.HasPrefix(stripped, "Content-Hash: "):
			contentHash = strings.TrimPrefix(stripped, "Content-Hash: ")
		case strings.HasPrefix(stripped, "Source-Hash: "):
			sourceHash = strings.TrimPrefix(stripped, "Source-Hash: ")
		}
		if contentHash != "" && sourceHash != "" {
			return contentHash, sourceHash
		}
	}
	return contentHash, sourceHash
}

// stripHeader removes the header comment from generated content, returning
// only the body. The header format depends on the output file extension.
//
// Detection runs in the same priority order as injectHashes so that the body
// passed to the body hash is symmetric with the body the injection sees.
// Files may have multiple header layers (e.g. windsurf rule files have
// trigger frontmatter THEN a generated-file banner) — we strip them all,
// otherwise the banner's per-run timestamp would leak into the body hash.
func stripHeader(content, outputPath string) string {
	ext := strings.ToLower(filepath.Ext(outputPath))

	// 1. YAML frontmatter (skill/agent files, plus rule files that prepend
	// trigger frontmatter). Strip and continue — there may be a banner after.
	if strings.HasPrefix(content, "---\n") {
		rest := content[len("---\n"):]
		if idx := strings.Index(rest, "\n---\n"); idx >= 0 {
			content = rest[idx+len("\n---\n"):]
			// Trim a single leading blank line so banner detection works
			// against either "---\n\n<!--" or "---\n<!--" forms.
			content = strings.TrimPrefix(content, "\n")
		}
	}

	switch ext {
	case ".md", ".markdown", ".mdx", ".html":
		// HTML comment banner: strip everything up to and including "-->\n\n".
		// We do NOT strip line-prefix comments here because "# Heading" in
		// markdown is a heading, not a comment, and stripping it would
		// truncate the body.
		if idx := strings.Index(content, "-->\n\n"); idx >= 0 {
			return content[idx+len("-->\n\n"):]
		}
		if idx := strings.Index(content, "-->\n"); idx >= 0 {
			return content[idx+len("-->\n"):]
		}
		return content
	default:
		// Line-prefix comments (#, //, ;) — also covers .mdc which uses
		// "# title\n\nbody" with the title acting as a heading-shaped header.
		lines := strings.Split(content, "\n")
		i := 0
		for i < len(lines) {
			trimmed := strings.TrimSpace(lines[i])
			if trimmed == "" {
				i++
				break
			}
			isComment := strings.HasPrefix(trimmed, "#") ||
				strings.HasPrefix(trimmed, "//") ||
				strings.HasPrefix(trimmed, ";")
			if !isComment {
				break
			}
			i++
		}
		if i >= len(lines) {
			return ""
		}
		return strings.Join(lines[i:], "\n")
	}
}

// injectContentHash inserts a Content-Hash line into the header of the
// generated content. Backward-compatible thin wrapper around injectHashes.
func injectContentHash(content, outputPath, hash string) string {
	return injectHashes(content, outputPath, hash, "")
}

// injectHashes inserts Content-Hash and (optionally) Source-Hash lines into
// the header of the generated content. It locates the header closing marker
// and inserts the hash lines before it. If sourceHash is empty, only
// Content-Hash is injected (e.g., from older callers).
func injectHashes(content, outputPath, contentHash, sourceHash string) string {
	ext := strings.ToLower(filepath.Ext(outputPath))

	hashBlock := func(linePrefix string) string {
		var b strings.Builder
		b.WriteString(linePrefix)
		b.WriteString("Content-Hash: ")
		b.WriteString(contentHash)
		if sourceHash != "" {
			b.WriteByte('\n')
			b.WriteString(linePrefix)
			b.WriteString("Source-Hash: ")
			b.WriteString(sourceHash)
		}
		return b.String()
	}

	// 1. YAML frontmatter (skill/agent files): inject as YAML comment lines
	// inside the frontmatter, before the closing "---". YAML parsers ignore
	// comments, so consumers see the same parsed fields.
	if strings.HasPrefix(content, "---\n") {
		rest := content[len("---\n"):]
		if idx := strings.Index(rest, "\n---\n"); idx >= 0 {
			closeIdx := len("---\n") + idx
			return content[:closeIdx] + "\n" + hashBlock("# ") + content[closeIdx:]
		}
	}

	// 2. HTML comment banner — only for true markdown/HTML extensions.
	switch ext {
	case ".md", ".markdown", ".mdx", ".html":
		marker := "\n-->\n"
		if idx := strings.Index(content, marker); idx >= 0 {
			return content[:idx] + "\n" + hashBlock("") + marker + content[idx+len(marker):]
		}
		return content
	}

	// 3. Line-prefix comments (yaml, ini, .mdc title-as-header, etc).
	prefix := "# "
	switch ext {
	case ".json", ".jsonc", ".go", ".js", ".ts", ".tsx", ".jsx", ".java", ".c", ".cc", ".cpp", ".cs":
		prefix = "// "
	case ".ini":
		prefix = "; "
	}

	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) == "" && i > 0 {
			prevTrimmed := strings.TrimSpace(lines[i-1])
			isComment := strings.HasPrefix(prevTrimmed, "#") ||
				strings.HasPrefix(prevTrimmed, "//") ||
				strings.HasPrefix(prevTrimmed, ";")
			if isComment {
				hashLines := strings.Split(hashBlock(prefix), "\n")
				result := make([]string, 0, len(lines)+len(hashLines))
				result = append(result, lines[:i]...)
				result = append(result, hashLines...)
				result = append(result, lines[i:]...)
				return strings.Join(result, "\n")
			}
		}
	}
	return content
}

// computeSourceHash returns a stable blake3 hash over the inputs that determine
// generated output for the active profile. It includes:
//   - GeneratorSchemaVersion (so renderer changes invalidate)
//   - Resolved config metadata that affects rendering
//   - Resolved MCP servers
//   - Every ContentFile in root + domains, sorted, with name/path/content/metadata
//
// Determinism requires sorted iteration everywhere — Go maps must be visited
// in lexical key order, ContentFile slices must already be sorted by Name
// (the scanner does this), and metadata serialization uses encoding/json which
// sorts map keys.
func computeSourceHash(cfg *config.Config, content *config.ContentTree) string {
	var b strings.Builder
	b.WriteString("schema=" + templates.GeneratorSchemaVersion + "\n")
	b.WriteString("name=" + cfg.Name + "\n")
	b.WriteString("description=" + cfg.Description + "\n")
	b.WriteString("version=" + cfg.Version + "\n")
	b.WriteString("header_style=" + cfg.GetHeaderStyle() + "\n")

	// MCP servers — sorted by name
	mcpNames := make([]string, 0, len(cfg.MCPServers))
	for name := range cfg.MCPServers {
		mcpNames = append(mcpNames, name)
	}
	sort.Strings(mcpNames)
	for _, name := range mcpNames {
		serverJSON, err := json.Marshal(mcpServerForSourceHash(cfg.MCPServers[name]))
		if err != nil {
			serverJSON = []byte("<marshal-error>")
		}
		b.WriteString("mcp:" + name + "=" + string(serverJSON) + "\n")
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

	// Root content categories — slices already sorted by Name in the scanner
	writeContentFiles(&b, "root.rules", content.Rules)
	writeContentFiles(&b, "root.context", content.Context)
	writeContentFiles(&b, "root.skills", content.Skills)
	writeContentFiles(&b, "root.agents", content.Agents)
	writeContentFiles(&b, "root.commands", content.Commands)

	// Domain content — domains visited in sorted name order
	domainNames := make([]string, 0, len(content.Domains))
	for name := range content.Domains {
		domainNames = append(domainNames, name)
	}
	sort.Strings(domainNames)
	for _, name := range domainNames {
		domain := content.Domains[name]
		_, _ = fmt.Fprintf(&b, "domain:%s,builtin=%t,from_include=%t\n", name, domain.Builtin, domain.FromInclude)
		writeContentFiles(&b, "domain."+name+".rules", domain.Rules)
		writeContentFiles(&b, "domain."+name+".context", domain.Context)
		writeContentFiles(&b, "domain."+name+".skills", domain.Skills)
		writeContentFiles(&b, "domain."+name+".agents", domain.Agents)
		writeContentFiles(&b, "domain."+name+".commands", domain.Commands)
	}

	return templates.HashContent(b.String())
}

// writeContentFiles serializes a ContentFile slice into the hash builder.
// Slices reach this function pre-sorted by Name (scanner guarantee).
func writeContentFiles(b *strings.Builder, label string, files []config.ContentFile) {
	for _, f := range files {
		b.WriteString(label + ":" + f.Name + "|path=" + f.Path + "|content=" + f.Content)
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

func (g *Generator) manifestPath() string {
	configDir := g.config.ConfigDir
	if configDir == "" {
		configDir = filepath.Join(g.config.BaseDir, ".ai-rulez")
	}
	return filepath.Join(configDir, generatedManifestName)
}

func (g *Generator) readGeneratedManifest() generatedManifest {
	data, err := os.ReadFile(g.manifestPath())
	if err != nil {
		return generatedManifest{}
	}
	var manifest generatedManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		logger.Warn("Ignoring invalid generated manifest", "path", g.manifestPath(), "error", err)
		return generatedManifest{}
	}
	return manifest
}

func (g *Generator) writeGeneratedManifest(outputs []config.OutputFile) error {
	files := make([]string, 0, len(outputs))
	for _, output := range outputs {
		if output.IsDir {
			continue
		}
		files = append(files, filepath.ToSlash(g.convertToRelativePath(g.absOutputPath(output.Path))))
	}
	sort.Strings(files)

	manifest := generatedManifest{Version: "1", Files: files}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return oops.Wrapf(err, "marshal generated manifest")
	}
	data = append(data, '\n')

	path := g.manifestPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return oops.With("dir", filepath.Dir(path)).Wrapf(err, "create manifest directory")
	}
	return os.WriteFile(path, data, 0o644)
}

func (g *Generator) staleManifestFiles(outputs []config.OutputFile) []string {
	previous := g.readGeneratedManifest()
	if len(previous.Files) == 0 {
		return nil
	}

	next := make(map[string]bool)
	for _, output := range outputs {
		if output.IsDir {
			continue
		}
		next[filepath.ToSlash(g.convertToRelativePath(g.absOutputPath(output.Path)))] = true
	}

	var stale []string
	for _, relPath := range previous.Files {
		if next[relPath] {
			continue
		}
		absPath := filepath.Join(g.config.BaseDir, filepath.FromSlash(relPath))
		if !isUnderBaseDir(g.config.BaseDir, absPath) {
			logger.Warn("Skipping generated manifest path outside project", "path", relPath)
			continue
		}
		if _, err := os.Stat(absPath); err == nil {
			stale = append(stale, absPath)
		}
	}
	sort.Strings(stale)
	return stale
}

func isUnderBaseDir(baseDir, path string) bool {
	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		return false
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(absBase, absPath)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != "..")
}

func (g *Generator) removeStaleManifestFiles(files []string) {
	for _, file := range files {
		g.removeStaleFile(file)
	}
}

// removeStaleFile removes a single stale file.
func (g *Generator) removeStaleFile(filePath string) {
	if err := os.Remove(filePath); err != nil {
		logger.Warn("Failed to remove stale file", "path", filePath, "error", err)
	} else {
		logger.Debug("Removed stale file", "path", filePath)
	}
}

// collectGitignorePaths collects unique patterns to add to .gitignore.
func (g *Generator) collectGitignorePaths(outputs []config.OutputFile) map[string]bool {
	paths := make(map[string]bool)
	for _, output := range outputs {
		relPath := filepath.ToSlash(g.convertToRelativePath(g.absOutputPath(output.Path)))
		if g.shouldSkipPath(relPath) {
			continue
		}
		if pattern := gitignorePatternForOutput(relPath, output.IsDir); pattern != "" {
			paths[pattern] = true
		}
	}

	// The generated manifest sits inside the config dir and is rewritten on
	// every `generate`; include it explicitly so the managed fence covers it.
	if manifestRel := filepath.ToSlash(g.convertToRelativePath(g.manifestPath())); manifestRel != "" {
		paths[manifestRel] = true
	}

	return paths
}

func gitignorePatternForOutput(relPath string, isDir bool) string {
	relPath = strings.TrimPrefix(filepath.ToSlash(relPath), "./")
	if relPath == ".github" || strings.HasSuffix(relPath, "/.github") {
		return ""
	}
	for _, dir := range generatedAssistantDirs {
		trimmedDir := strings.TrimSuffix(dir, "/")
		if relPath == trimmedDir || strings.HasPrefix(relPath, dir) {
			return dir
		}
		if idx := strings.Index(relPath, "/"+dir); idx >= 0 {
			return relPath[:idx+1] + dir
		}
	}
	for _, file := range generatedRootFiles {
		if relPath == file {
			return file
		}
	}
	for _, pattern := range generatedGithubPatterns {
		if relPath == strings.TrimSuffix(pattern, "/") || strings.HasPrefix(relPath, pattern) || relPath == pattern {
			return pattern
		}
		if idx := strings.Index(relPath, "/"+pattern); idx >= 0 {
			return relPath[:idx+1] + pattern
		}
	}
	if isDir {
		return strings.TrimSuffix(relPath, "/") + "/"
	}
	return relPath
}

var generatedRootFiles = [...]string{
	"AGENTS.md",
	"CLAUDE.md",
	"GEMINI.md",
	".mcp.json",
}

var generatedAssistantDirs = [...]string{
	".agents/",
	".claude/",
	".codex/",
	".cursor/",
	".gemini/",
	".continue/",
	".cline/",
	".clinerules/",
	".windsurf/",
	".junie/",
	".opencode/",
	".amp/",
}

var generatedGithubPatterns = [...]string{
	".github/copilot-instructions.md",
	".github/agents/",
	".github/commands/",
	".github/skills/",
}

// convertToRelativePath converts an absolute path to relative, or returns the original path
func (g *Generator) convertToRelativePath(path string) string {
	if !filepath.IsAbs(path) {
		return path
	}
	relPath, err := filepath.Rel(g.config.BaseDir, path)
	if err != nil {
		return filepath.Base(path)
	}
	return relPath
}

// shouldSkipPath checks if a path should be skipped for .gitignore
func (g *Generator) shouldSkipPath(relPath string) bool {
	configDir := g.configDirName()
	return relPath == configDir ||
		hasPrefix(relPath, configDir+"/") ||
		hasPrefix(relPath, configDir+"\\")
}

func (g *Generator) configDirName() string {
	if g.config.ConfigDirName != "" {
		return g.config.ConfigDirName
	}
	return ".ai-rulez"
}

// updateGitignore updates .gitignore with generated file paths using a fenced block
func (g *Generator) updateGitignore(outputs []config.OutputFile) error {
	gitignorePath := filepath.Join(g.config.BaseDir, ".gitignore")

	// Collect unique output paths
	paths := g.collectGitignorePaths(outputs)

	// Read existing .gitignore content
	existingData, err := os.ReadFile(gitignorePath)
	if err != nil && !os.IsNotExist(err) {
		return oops.
			With("path", gitignorePath).
			Wrapf(err, "read .gitignore")
	}
	existingContent := string(existingData)

	// Drop entries the user already has outside the managed fence — avoids
	// duplicating lines like ".cursor/" that pre-existed in the file.
	outsideFence := gitignore.PatternsOutsideFence(existingContent)

	// Sort remaining paths for deterministic output
	outsidePatterns := make([]string, 0, len(outsideFence))
	for pattern := range outsideFence {
		outsidePatterns = append(outsidePatterns, pattern)
	}
	var sortedPaths []string
	for p := range paths {
		if isIgnored(p, outsidePatterns) {
			continue
		}
		sortedPaths = append(sortedPaths, p)
	}
	sort.Strings(sortedPaths)

	if len(sortedPaths) == 0 {
		logger.Debug("No paths to add to .gitignore")
		// If a managed block already exists but every entry is now covered
		// outside the fence, replace the block with an empty one to keep the
		// file tidy.
		if !contains(existingContent, gitignore.BeginMarker) {
			return nil
		}
	}

	// Build the fenced block
	var fencedBlock strings.Builder
	fencedBlock.WriteString(gitignore.BeginMarker + "\n")
	for _, p := range sortedPaths {
		fencedBlock.WriteString(p + "\n")
	}
	fencedBlock.WriteString(gitignore.EndMarker + "\n")

	var newContent string

	switch {
	case contains(existingContent, gitignore.BeginMarker):
		newContent = gitignore.ReplaceFencedBlock(existingContent, fencedBlock.String())
	case contains(existingContent, gitignore.OldHeader):
		newContent = gitignore.ReplaceOldHeaderBlock(existingContent, fencedBlock.String())
	case len(existingData) == 0:
		newContent = fencedBlock.String()
	default:
		newContent = existingContent
		if !hasSuffix(newContent, "\n") {
			newContent += "\n"
		}
		newContent += "\n" + fencedBlock.String()
	}

	if err := os.WriteFile(gitignorePath, []byte(newContent), 0o644); err != nil { //nolint:gosec // path from config, not user input
		return oops.
			With("path", gitignorePath).
			Wrapf(err, "write .gitignore")
	}

	logger.Debug("Updated .gitignore", "entries", len(sortedPaths))
	return nil
}

func (g *Generator) ensureSecretOutputsIgnored(outputs []config.OutputFile) error {
	secretKeys := g.secretMCPEnvKeys()
	if len(secretKeys) == 0 {
		return nil
	}

	gitignorePath := filepath.Join(g.config.BaseDir, ".gitignore")
	existingData, err := os.ReadFile(gitignorePath)
	if err != nil && !os.IsNotExist(err) {
		return oops.With("path", gitignorePath).Wrapf(err, "read .gitignore")
	}

	patterns := gitignorePatterns(string(existingData))
	if g.config.ShouldUpdateGitignore() {
		for pattern := range g.collectGitignorePaths(outputs) {
			patterns = append(patterns, pattern)
		}
	}

	var unsafe []string
	for _, output := range outputs {
		if output.IsDir {
			continue
		}
		relPath := filepath.ToSlash(g.convertToRelativePath(g.absOutputPath(output.Path)))
		if !isMCPConfigOutput(relPath) {
			continue
		}
		if !isIgnored(relPath, patterns) {
			unsafe = append(unsafe, relPath)
		}
	}
	if len(unsafe) == 0 {
		return nil
	}
	sort.Strings(unsafe)
	return oops.
		With("paths", unsafe).
		With("env_keys", secretKeys).
		Hint("Enable gitignore generation with --gitignore or add these generated MCP config paths to .gitignore").
		Errorf("generated MCP config contains secrets but is not gitignored")
}

func (g *Generator) secretMCPEnvKeys() []string {
	keys := make(map[string]bool)
	for _, server := range g.config.MCPServers {
		for _, key := range server.SecretEnvKeys {
			keys[key] = true
		}
	}
	return sortedMapKeys(keys)
}

func isMCPConfigOutput(relPath string) bool {
	return relPath == ".mcp.json" ||
		relPath == ".claude/settings.json" ||
		relPath == ".gemini/settings.json" ||
		relPath == ".agents/settings.json" ||
		strings.HasSuffix(relPath, "/.mcp.json") ||
		strings.HasSuffix(relPath, "/.claude/settings.json") ||
		strings.HasSuffix(relPath, "/.gemini/settings.json") ||
		strings.HasSuffix(relPath, "/.agents/settings.json")
}

func gitignorePatterns(content string) []string {
	var patterns []string
	for _, line := range splitLines(content) {
		trimmed := trimSpace(line)
		if trimmed == "" || hasPrefix(trimmed, "#") {
			continue
		}
		patterns = append(patterns, trimmed)
	}
	return patterns
}

// isIgnored checks if a filename matches any gitignore pattern
func isIgnored(filename string, patterns []string) bool {
	for _, pattern := range patterns {
		if matchesPattern(filename, pattern) {
			return true
		}
	}
	return false
}

// matchesPattern checks if a filename matches a gitignore pattern
func matchesPattern(filename, pattern string) bool {
	// Exact match
	if pattern == filename {
		return true
	}

	// Directory pattern
	if hasSuffix(pattern, "/") {
		return matchesDirectory(filename, pattern)
	}

	// Glob pattern
	if contains(pattern, "*") || contains(pattern, "?") {
		if matched, _ := filepath.Match(pattern, filename); matched {
			return true
		}
		if matched, _ := filepath.Match(pattern, filepath.Base(filename)); matched {
			return true
		}
		return false
	}

	// Absolute pattern
	if hasPrefix(pattern, "/") {
		return filename == trimPrefix(pattern, "/")
	}

	// Substring match
	return filename == pattern ||
		hasSuffix(filename, "/"+pattern) ||
		contains(filename, "/"+pattern+"/") ||
		contains(filename, pattern)
}

// matchesDirectory checks if filename matches a directory pattern
func matchesDirectory(filename, pattern string) bool {
	dirPrefix := trimPrefix(trimSuffix(pattern, "/"), "/")

	if hasSuffix(filename, "/") {
		return pattern == filename || trimSuffix(filename, "/") == dirPrefix
	}

	return hasPrefix(filename, dirPrefix+"/") || filename == dirPrefix
}

// String helper functions to avoid importing strings package
func splitLines(s string) []string {
	var lines []string
	start := 0

	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}

	if start < len(s) {
		lines = append(lines, s[start:])
	}

	return lines
}

func trimSpace(s string) string {
	start := 0
	end := len(s)

	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}

	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}

	return s[start:end]
}

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

func hasSuffix(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}

func trimPrefix(s, prefix string) string {
	if hasPrefix(s, prefix) {
		return s[len(prefix):]
	}
	return s
}

func trimSuffix(s, suffix string) string {
	if hasSuffix(s, suffix) {
		return s[:len(s)-len(suffix)]
	}
	return s
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
