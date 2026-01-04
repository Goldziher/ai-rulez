package generator

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/generator/presets" // Import to register presets and access MCPPresetGenerator
	"github.com/Goldziher/ai-rulez/internal/logger"
	"github.com/samber/oops"
)

const defaultProfileName = "default"

// GeneratorV3 handles V3 configuration generation
type GeneratorV3 struct {
	config *config.ConfigV3
}

// NewGeneratorV3 creates a new V3 generator
func NewGeneratorV3(cfg *config.ConfigV3) *GeneratorV3 {
	return &GeneratorV3{
		config: cfg,
	}
}

// Generate generates all outputs for the specified profile
func (g *GeneratorV3) Generate(profile string) error {
	// Determine the active profile
	activeProfile := g.resolveProfile(profile)

	logger.Info("Generating with V3 configuration", "profile", activeProfile)

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

	// Collect MCP servers for this profile
	mcpServers := g.collectMCPServersForProfile(activeProfile)

	// Create a temporary config with the filtered content and MCP servers
	tempCfg := *g.config
	tempCfg.Content = contentTree
	tempCfg.MCPServers = mcpServers

	// Generate outputs for all presets using the existing infrastructure
	allOutputs, err := config.GeneratePresetsV3(&tempCfg)
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

	// Flatten outputs for writing
	var flatOutputs []config.OutputFileV3
	for presetName, outputs := range allOutputs {
		logger.Debug("Generated outputs for preset", "preset", presetName, "count", len(outputs))
		flatOutputs = append(flatOutputs, outputs...)
	}

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
func (g *GeneratorV3) resolveProfile(profile string) string {
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
func (g *GeneratorV3) getContentForProfile(profile string) (*config.ContentTreeV3, error) {
	// Special case: "default" profile means root content only (no domains)
	if profile == defaultProfileName {
		return &config.ContentTreeV3{
			Rules:    g.config.Content.Rules,
			Context:  g.config.Content.Context,
			Skills:   g.config.Content.Skills,
			Agents:   g.config.Content.Agents,
			Commands: g.config.Content.Commands,
			Domains:  make(map[string]*config.DomainV3), // Empty - no domains for default profile
		}, nil
	}

	// Check if profile exists
	if !g.config.HasProfile(profile) {
		availableProfiles := make([]string, 0, len(g.config.Profiles))
		for name := range g.config.Profiles {
			availableProfiles = append(availableProfiles, name)
		}

		return nil, oops.
			With("profile", profile).
			With("available_profiles", availableProfiles).
			Hint(fmt.Sprintf("Available profiles: %v\nOr use 'default' for root content only", availableProfiles)).
			Errorf("profile not found: %s", profile)
	}

	// Get content for the profile (includes root + specified domains)
	return g.config.GetContentForProfile(profile)
}

// collectMCPServersForProfile collects MCP servers for the active profile
// Logic: collect root servers + domain servers from active profile
// Domain servers override root by name
// Only include enabled servers
func (g *GeneratorV3) collectMCPServersForProfile(profile string) map[string]*config.MCPServerV3 {
	collected := make(map[string]*config.MCPServerV3)

	// Always include root servers (if enabled)
	for name, server := range g.config.MCPServers {
		if server.IsEnabled() {
			collected[name] = server
		}
	}

	// Include servers from domains in the active profile
	domains := g.config.GetProfileDomains(profile)
	for _, domainName := range domains {
		if domain, ok := g.config.Content.Domains[domainName]; ok {
			for name, server := range domain.MCPServers {
				if server.IsEnabled() {
					// Domain servers override root servers by name
					collected[name] = server
				}
			}
		}
	}

	return collected
}

// writeOutputs writes all output files to disk
func (g *GeneratorV3) writeOutputs(outputs []config.OutputFileV3) error {
	for _, output := range outputs {
		if err := g.writeOutput(output); err != nil {
			return oops.
				With("path", output.Path).
				Wrapf(err, "write output file")
		}
	}
	return nil
}

// writeOutput writes a single output file or creates a directory
func (g *GeneratorV3) writeOutput(output config.OutputFileV3) error {
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
	if err := os.WriteFile(absPath, []byte(output.Content), 0o644); err != nil {
		return oops.
			With("path", absPath).
			Hint(fmt.Sprintf("Check write permissions for: %s", absPath)).
			Wrapf(err, "write file")
	}

	logger.Debug("Wrote file", "path", output.Path, "size", len(output.Content))
	return nil
}

// collectGitignorePaths collects unique paths to add to .gitignore
func (g *GeneratorV3) collectGitignorePaths(outputs []config.OutputFileV3) map[string]bool {
	paths := make(map[string]bool)
	seen := make(map[string]bool)

	for _, output := range outputs {
		// Convert to relative path if absolute
		relPath := g.convertToRelativePath(output.Path)

		// Skip .ai-rulez directory and anything inside it
		if g.shouldSkipPath(relPath) {
			continue
		}

		// For files, add them directly
		if !output.IsDir {
			paths[relPath] = true
			continue
		}

		// For directories, add top-level directory
		topLevel := g.extractTopLevelDir(relPath)
		if topLevel == ".ai-rulez" {
			continue
		}

		if !seen[topLevel] {
			seen[topLevel] = true
			paths[topLevel+"/"] = true
		}
	}

	return paths
}

// convertToRelativePath converts an absolute path to relative, or returns the original path
func (g *GeneratorV3) convertToRelativePath(path string) string {
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
func (g *GeneratorV3) shouldSkipPath(relPath string) bool {
	return relPath == ".ai-rulez" ||
		hasPrefix(relPath, ".ai-rulez/") ||
		hasPrefix(relPath, ".ai-rulez\\")
}

// extractTopLevelDir extracts the top-level directory from a path
func (g *GeneratorV3) extractTopLevelDir(relPath string) string {
	parts := strings.Split(filepath.Clean(relPath), string(filepath.Separator))
	if len(parts) == 0 {
		return relPath
	}
	return parts[0]
}

// updateGitignore updates .gitignore with generated file paths
func (g *GeneratorV3) updateGitignore(outputs []config.OutputFileV3) error {
	gitignorePath := filepath.Join(g.config.BaseDir, ".gitignore")

	// Collect unique output paths
	paths := g.collectGitignorePaths(outputs)

	// Read existing gitignore entries
	existingEntries, err := readGitignoreEntries(gitignorePath)
	if err != nil && !os.IsNotExist(err) {
		return oops.
			With("path", gitignorePath).
			Wrapf(err, "read .gitignore")
	}

	// Determine which paths need to be added
	var toAdd []string
	for path := range paths {
		if !isIgnored(path, existingEntries) {
			toAdd = append(toAdd, path)
		}
	}

	if len(toAdd) == 0 {
		logger.Debug("All generated files already in .gitignore")
		return nil
	}

	// Append to gitignore
	if err := appendToGitignore(gitignorePath, toAdd, len(existingEntries) == 0); err != nil {
		return oops.
			With("path", gitignorePath).
			With("entries", toAdd).
			Wrapf(err, "update .gitignore")
	}

	logger.Debug("Updated .gitignore", "added", len(toAdd))
	return nil
}

// readGitignoreEntries reads non-comment, non-empty lines from .gitignore
func readGitignoreEntries(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var entries []string
	lines := splitLines(string(data))

	for _, line := range lines {
		line = trimSpace(line)
		if line != "" && !hasPrefix(line, "#") {
			entries = append(entries, line)
		}
	}

	return entries, nil
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

// appendToGitignore appends entries to .gitignore
func appendToGitignore(path string, entries []string, isNewFile bool) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()

	var content string
	if isNewFile {
		content = "# AI Rules generated files\n"
	} else {
		content = "\n# AI Rules generated files\n"
	}

	for _, entry := range entries {
		content += entry + "\n"
	}

	_, err = file.WriteString(content)
	return err
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
