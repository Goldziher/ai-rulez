package generator

import (
	"bufio"
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
// It computes a content hash of the body (everything after the header),
// injects the hash into the header, and skips writing if the existing
// file already contains the same hash — avoiding noisy git diffs when
// content has not changed.
func (g *Generator) writeOutput(output config.OutputFile) error {
	// Resolve absolute path
	absPath := output.Path
	if !filepath.IsAbs(absPath) {
		absPath = filepath.Join(g.config.BaseDir, output.Path)
	}

	// If this is a directory, just create it
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

	// Compute content hash from the body (content after header).
	body := stripHeader(output.Content, output.Path)
	contentHash := templates.HashContent(body)

	// Check if existing file already has the same content hash.
	existingHash := extractContentHash(absPath)
	if existingHash != "" && existingHash == contentHash {
		logger.Debug("Skipped unchanged file", "path", output.Path, "hash", contentHash)
		return nil
	}

	// Inject the content hash into the header of the generated content.
	finalContent := injectContentHash(output.Content, output.Path, contentHash)

	// Create parent directories for files
	dir := filepath.Dir(absPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return oops.
			With("dir", dir).
			With("path", absPath).
			Hint(fmt.Sprintf("Check directory permissions for: %s", dir)).
			Wrapf(err, "create parent directory")
	}

	// Write file
	if err := os.WriteFile(absPath, []byte(finalContent), 0o644); err != nil {
		return oops.
			With("path", absPath).
			Hint(fmt.Sprintf("Check write permissions for: %s", absPath)).
			Wrapf(err, "write file")
	}

	logger.Debug("Wrote file", "path", output.Path, "size", len(finalContent))
	return nil
}

// extractContentHash scans the header of an existing file and returns
// the Content-Hash value if found, or an empty string otherwise.
// It reads up to 60 lines to accommodate detailed headers.
func extractContentHash(filePath string) string {
	file, err := os.Open(filePath)
	if err != nil {
		return ""
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for i := 0; i < 60 && scanner.Scan(); i++ {
		line := strings.TrimSpace(scanner.Text())
		// Strip common comment prefixes so we can find the hash line
		// regardless of comment style (HTML, hash, slash, semicolon).
		stripped := line
		for _, prefix := range []string{"<!--", "-->", "//", "#", ";", "/*", "*/"} {
			stripped = strings.TrimPrefix(stripped, prefix)
		}
		stripped = strings.TrimSpace(stripped)

		if strings.HasPrefix(stripped, "Content-Hash: ") {
			return strings.TrimPrefix(stripped, "Content-Hash: ")
		}
	}
	return ""
}

// stripHeader removes the header comment from generated content, returning
// only the body. The header format depends on the output file extension.
func stripHeader(content, outputPath string) string {
	ext := strings.ToLower(filepath.Ext(outputPath))

	switch ext {
	case ".md", ".markdown", ".mdx", ".html":
		// HTML comment: everything before and including "-->\n\n"
		if idx := strings.Index(content, "-->\n\n"); idx >= 0 {
			return content[idx+len("-->\n\n"):]
		}
		if idx := strings.Index(content, "-->\n"); idx >= 0 {
			return content[idx+len("-->\n"):]
		}
		return content
	default:
		// Line-prefix comments (# , // , ; ): skip leading comment lines
		// and the blank line that follows them.
		lines := strings.Split(content, "\n")
		i := 0
		for i < len(lines) {
			trimmed := strings.TrimSpace(lines[i])
			if trimmed == "" {
				// A blank line after comment lines marks end of header
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
// generated content. It modifies the content in place by finding the
// header closing marker and inserting the hash line before it.
func injectContentHash(content, outputPath, hash string) string {
	ext := strings.ToLower(filepath.Ext(outputPath))

	switch ext {
	case ".md", ".markdown", ".mdx", ".html":
		// HTML comment: insert before "-->"
		marker := "\n-->\n"
		if idx := strings.Index(content, marker); idx >= 0 {
			return content[:idx] + "\nContent-Hash: " + hash + marker + content[idx+len(marker):]
		}
		return content
	default:
		// Line-prefix comments: find the first blank line after comments
		// and insert the hash line before it.
		prefix := "# "
		switch ext {
		case ".json", ".jsonc", ".go", ".js", ".ts", ".tsx", ".jsx", ".java", ".c", ".cc", ".cpp", ".cs":
			prefix = "// "
		case ".ini":
			prefix = "; "
		}

		lines := strings.Split(content, "\n")
		// Find the blank line that ends the header block
		for i, line := range lines {
			if strings.TrimSpace(line) == "" && i > 0 {
				// Check that the previous line was a comment
				prevTrimmed := strings.TrimSpace(lines[i-1])
				isComment := strings.HasPrefix(prevTrimmed, "#") ||
					strings.HasPrefix(prevTrimmed, "//") ||
					strings.HasPrefix(prevTrimmed, ";")
				if isComment {
					// Insert hash line before this blank line
					hashLine := prefix + "Content-Hash: " + hash
					result := make([]string, 0, len(lines)+1)
					result = append(result, lines[:i]...)
					result = append(result, hashLine)
					result = append(result, lines[i:]...)
					return strings.Join(result, "\n")
				}
			}
		}
		return content
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
