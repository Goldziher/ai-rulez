package generator

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/generator/presets" // Import to register presets and access MCPPresetGenerator
	"github.com/Goldziher/ai-rulez/internal/gitignore"
	"github.com/Goldziher/ai-rulez/internal/logger"
	"github.com/Goldziher/ai-rulez/internal/templates"
	"github.com/samber/oops"
)

const defaultProfileName = "default"

// Generator handles configuration generation
type Generator struct {
	config *config.Config
}

// NewGenerator creates a new generator
func NewGenerator(cfg *config.Config) *Generator {
	return &Generator{
		config: cfg,
	}
}

// Generate generates all outputs for the specified profile
func (g *Generator) Generate(profile string) error {
	// Determine the active profile
	activeProfile := g.resolveProfile(profile)

	logger.Info("Generating with configuration", "profile", activeProfile)

	// Get content for the profile
	contentTree, err := g.getContentForProfile(activeProfile)
	if err != nil {
		return err
	}

	logger.Debug("Content scanned",
		"rules", len(contentTree.Rules),
		"context", len(contentTree.Context),
		"skills", len(contentTree.Skills),
		"agents", len(contentTree.Agents),
		"domains", len(contentTree.Domains))

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
		return oops.
			Wrapf(err, "generate presets")
	}

	// Auto-generate MCP output if servers exist
	if len(mcpServers) > 0 {
		mcpGen := &presets.MCPPresetGenerator{}
		mcpOutputs, err := mcpGen.Generate(contentTree, g.config.BaseDir, &tempCfg)
		if err != nil {
			logger.Warn("Failed to generate MCP output", "error", err)
		} else if len(mcpOutputs) > 0 {
			allOutputs["mcp"] = mcpOutputs
			logger.Debug("Auto-generated MCP output", "count", len(mcpOutputs))
		}
	}

	// Flatten outputs for writing, detecting conflicts and deduplicating
	flatOutputs := flattenPresetOutputs(allOutputs)

	// Clean up stale files from managed directories before writing new output.
	// This ensures that renamed or removed skills/agents don't leave behind orphaned files.
	g.cleanManagedDirs(flatOutputs)

	// Write all output files
	if err := g.writeOutputs(flatOutputs); err != nil {
		return err
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

// writeOutputs writes all output files to disk
// flattenPresetOutputs merges outputs from all presets, deduplicating directories
// and detecting file conflicts (last write wins with a warning).
func flattenPresetOutputs(allOutputs map[string][]config.OutputFile) []config.OutputFile {
	var flatOutputs []config.OutputFile
	seenPaths := make(map[string]string) // path -> first preset that claimed it
	seenDirs := make(map[string]bool)
	for presetName, outputs := range allOutputs {
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
	absPath := output.Path
	if !filepath.IsAbs(absPath) {
		absPath = filepath.Join(g.config.BaseDir, output.Path)
	}

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
		serverJSON, err := json.Marshal(cfg.MCPServers[name])
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
		b.WriteByte('\n')
	}
}

// cleanManagedDirs removes stale files from directories that are fully managed by the generator.
// It collects all directory outputs from the new generation, then removes any existing files
// in those directories that are not part of the new output set.
func (g *Generator) cleanManagedDirs(outputs []config.OutputFile) {
	newFiles := g.collectOutputPaths(outputs, false)
	managedDirs := g.collectOutputPaths(outputs, true)

	for dir := range managedDirs {
		// Skip shared directories — they contain non-generated user content
		// (e.g., .github/ has workflows, CODEOWNERS, issue templates)
		relDir := g.convertToRelativePath(dir)
		topLevel := g.extractTopLevelDir(relDir)
		if sharedDirs[topLevel] {
			continue
		}

		entries, err := os.ReadDir(dir)
		if err != nil {
			continue // Directory may not exist yet
		}
		for _, entry := range entries {
			entryPath := filepath.Join(dir, entry.Name())
			if entry.IsDir() {
				g.removeStaleDir(entryPath, newFiles)
			} else if !newFiles[entryPath] {
				g.removeStaleFile(entryPath)
			}
		}
	}
}

// collectOutputPaths collects absolute paths from outputs, filtered by IsDir.
func (g *Generator) collectOutputPaths(outputs []config.OutputFile, dirsOnly bool) map[string]bool {
	result := make(map[string]bool)
	for _, o := range outputs {
		if o.IsDir != dirsOnly {
			continue
		}
		absPath := o.Path
		if !filepath.IsAbs(absPath) {
			absPath = filepath.Join(g.config.BaseDir, o.Path)
		}
		result[absPath] = true
	}
	return result
}

// removeStaleDir removes a directory if no new output file targets it.
func (g *Generator) removeStaleDir(dirPath string, newFiles map[string]bool) {
	prefix := dirPath + string(filepath.Separator)
	for newFile := range newFiles {
		if strings.HasPrefix(newFile, prefix) {
			return
		}
	}
	if err := os.RemoveAll(dirPath); err != nil {
		logger.Warn("Failed to remove stale directory", "path", dirPath, "error", err)
	} else {
		logger.Debug("Removed stale directory", "path", dirPath)
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

// sharedDirs are directories that contain both generated and non-generated content.
// These must not be added as directory-level gitignore patterns — only individual
// generated files inside them should be gitignored.
var sharedDirs = map[string]bool{
	".github": true,
}

// collectGitignorePaths collects unique paths to add to .gitignore.
// Top-level directories that are fully managed are added as directory patterns.
// For shared directories (e.g. .github), subdirectories are added as patterns
// and only top-level files are listed individually.
func (g *Generator) collectGitignorePaths(outputs []config.OutputFile) map[string]bool {
	topDirs, sharedSubDirs := g.classifyDirectories(outputs)
	deduped := deduplicateSubDirs(sharedSubDirs)

	paths := make(map[string]bool)
	for dir := range topDirs {
		paths[dir+"/"] = true
	}
	for _, dir := range deduped {
		paths[dir+"/"] = true
	}

	// Add files only if not covered by a tracked directory
	for _, output := range outputs {
		if output.IsDir {
			continue
		}
		relPath := g.convertToRelativePath(output.Path)
		if g.shouldSkipPath(relPath) {
			continue
		}
		topLevel := g.extractTopLevelDir(relPath)
		if topDirs[topLevel] || isPathCovered(relPath, deduped) {
			continue
		}
		paths[relPath] = true
	}

	return paths
}

// classifyDirectories separates output directories into fully-managed top-level dirs
// and subdirectories within shared dirs (e.g. .github/agents/).
func (g *Generator) classifyDirectories(outputs []config.OutputFile) (topDirs, sharedSubDirs map[string]bool) {
	topDirs = make(map[string]bool)
	sharedSubDirs = make(map[string]bool)
	for _, output := range outputs {
		if !output.IsDir {
			continue
		}
		relPath := g.convertToRelativePath(output.Path)
		if g.shouldSkipPath(relPath) {
			continue
		}
		topLevel := g.extractTopLevelDir(relPath)
		if topLevel == ".ai-rulez" {
			continue
		}
		if sharedDirs[topLevel] {
			if relPath != topLevel {
				sharedSubDirs[relPath] = true
			}
		} else {
			topDirs[topLevel] = true
		}
	}
	return
}

// deduplicateSubDirs returns only dirs that aren't children of another dir in the set.
func deduplicateSubDirs(dirs map[string]bool) []string {
	var result []string
	for dir := range dirs {
		covered := false
		for parent := range dirs {
			if parent != dir && strings.HasPrefix(dir, parent+"/") {
				covered = true
				break
			}
		}
		if !covered {
			result = append(result, dir)
		}
	}
	return result
}

// isPathCovered checks if a file path is inside any of the given directories.
func isPathCovered(path string, dirs []string) bool {
	for _, dir := range dirs {
		if strings.HasPrefix(path, dir+"/") {
			return true
		}
	}
	return false
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
	return relPath == ".ai-rulez" ||
		hasPrefix(relPath, ".ai-rulez/") ||
		hasPrefix(relPath, ".ai-rulez\\")
}

// extractTopLevelDir extracts the top-level directory from a path
func (g *Generator) extractTopLevelDir(relPath string) string {
	parts := strings.Split(filepath.Clean(relPath), string(filepath.Separator))
	if len(parts) == 0 {
		return relPath
	}
	return parts[0]
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
	var sortedPaths []string
	for p := range paths {
		if outsideFence[p] {
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
	dirPrefix := trimSuffix(pattern, "/")

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
