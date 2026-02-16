package importer

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/logger"
	"github.com/Goldziher/ai-rulez/internal/parser"
	"github.com/Goldziher/ai-rulez/internal/utils"
	"github.com/samber/oops"
	"gopkg.in/yaml.v3"
)

// ContentType represents the type of imported content
type ContentType string

const (
	ContentTypeRule    ContentType = "rule"
	ContentTypeContext ContentType = "context"
	ContentTypeSkill   ContentType = "skill"
	claudePresetName   string      = "claude"
)

// ImportedContent represents a piece of content imported from a source
type ImportedContent struct {
	Name        string
	Type        ContentType
	Content     string
	Source      string
	Metadata    *config.MetadataV3
	Hash        string
	OriginalExt string // .md, .mdc, etc.
}

// Importer handles importing content from existing AI tool files
type Importer struct {
	sourceDir string
	outputDir string
}

// NewImporter creates a new importer
func NewImporter(sourceDir, outputDir string) *Importer {
	return &Importer{
		sourceDir: sourceDir,
		outputDir: outputDir,
	}
}

// Import detects and imports content from existing tool files
// sources: "auto" or comma-separated list like ".claude,.cursor,CLAUDE.md"
func (i *Importer) Import(sources string) error {
	var sourcesToImport []string
	var err error

	if sources == "auto" {
		sourcesToImport, err = i.detectSources()
		if err != nil {
			return oops.
				With("sources", sources).
				Wrapf(err, "detect sources")
		}

		if len(sourcesToImport) == 0 {
			return oops.
				Hint("No existing AI tool files found to import\nSupported: CLAUDE.md, .claude/, .cursor/, .windsurf/, .windsurf/rules/, .gemini/, .github/copilot-instructions.md, .continue/, .clinerules/").
				Errorf("no sources detected for import")
		}

		logger.Info("Auto-detected sources", "count", len(sourcesToImport), "sources", strings.Join(sourcesToImport, ", "))
	} else {
		// Parse comma-separated list
		parts := strings.Split(sources, ",")
		for _, part := range parts {
			source := strings.TrimSpace(part)
			if source != "" {
				sourcesToImport = append(sourcesToImport, source)
			}
		}
	}

	// Import from each source
	var allContent []ImportedContent
	detectedPresets := make(map[string]bool)

	for _, source := range sourcesToImport {
		content, preset, err := i.importFromSource(source)
		if err != nil {
			logger.Warn("Failed to import from source", "source", source, "error", err)
			continue
		}

		allContent = append(allContent, content...)
		if preset != "" {
			detectedPresets[preset] = true
		}

		logger.Info("Imported from source", "source", source, "items", len(content))
	}

	if len(allContent) == 0 {
		return oops.
			With("sources", sourcesToImport).
			Hint("No content could be imported from the specified sources\nCheck if the source files exist and contain valid content").
			Errorf("no content imported")
	}

	// Deduplicate content
	deduplicated := deduplicateContent(allContent)
	logger.Info("Deduplicated content", "before", len(allContent), "after", len(deduplicated))

	// Write to .ai-rulez/ structure
	if err := i.writeContent(deduplicated); err != nil {
		return oops.Wrapf(err, "write imported content")
	}

	// Generate config file with detected presets
	projectName := getProjectName(i.sourceDir)
	if err := i.writeConfig(projectName, detectedPresets); err != nil {
		return oops.Wrapf(err, "write config")
	}

	logger.Info("Import completed", "total_items", len(deduplicated), "presets", len(detectedPresets))
	return nil
}

// detectSources auto-detects available source files and directories
func (i *Importer) detectSources() ([]string, error) {
	var sources []string

	// Check for markdown files
	markdownFiles := []string{
		"CLAUDE.md",
		"GEMINI.md",
		".github/copilot-instructions.md",
	}

	for _, file := range markdownFiles {
		path := filepath.Join(i.sourceDir, file)
		if fileExists(path) {
			sources = append(sources, file)
		}
	}

	// Check for directories
	directories := []string{
		".claude/skills",
		".claude/agents",
		".cursor/rules",
		".windsurf",
		".windsurf/rules",
		".continue/rules",
		".continue/prompts",
		".clinerules",
	}

	for _, dir := range directories {
		path := filepath.Join(i.sourceDir, dir)
		if dirExists(path) {
			sources = append(sources, dir)
		}
	}

	return sources, nil
}

// importFromSource imports content from a specific source
// Returns: content items, detected preset name, error
func (i *Importer) importFromSource(source string) ([]ImportedContent, string, error) {
	sourcePath := filepath.Join(i.sourceDir, source)

	// Check if source is a file or directory
	info, err := os.Stat(sourcePath)
	if err != nil {
		return nil, "", oops.
			With("source", source).
			With("path", sourcePath).
			Wrapf(err, "stat source")
	}

	if info.IsDir() {
		return i.importFromDirectory(source, sourcePath)
	}

	return i.importFromFile(source, sourcePath)
}

// importFromFile imports content from a single file
func (i *Importer) importFromFile(source, path string) ([]ImportedContent, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", oops.
			With("source", source).
			With("path", path).
			Wrapf(err, "read file")
	}

	content := string(data)

	// Determine preset based on source
	preset := detectPresetFromSource(source)

	// Parse based on file type
	if strings.HasSuffix(source, ".md") {
		items := parseMarkdownFile(source, content)
		return items, preset, nil
	}

	// Default: treat entire file as context
	return []ImportedContent{
		{
			Name:    filenameWithoutExt(filepath.Base(path)),
			Type:    ContentTypeContext,
			Content: content,
			Source:  source,
			Hash:    hashContent(content),
		},
	}, preset, nil
}

// importFromDirectory imports content from a directory
func (i *Importer) importFromDirectory(source, path string) ([]ImportedContent, string, error) {
	// Handle different directory structures
	switch {
	case strings.Contains(source, ".claude/skills"):
		return i.importClaudeSkills(source, path)
	case strings.Contains(source, ".claude/agents"):
		return i.importClaudeAgents(source, path)
	case strings.Contains(source, ".cursor/rules"):
		return i.importCursorRules(source, path)
	case strings.Contains(source, ".windsurf"):
		return i.importWindsurfRules(source, path)
	case strings.Contains(source, ".continue/rules"):
		return i.importContinueRules(source, path)
	case strings.Contains(source, ".continue/prompts"):
		return i.importContinuePrompts(source, path)
	case strings.Contains(source, ".clinerules"):
		return i.importClineRules(source, path)
	default:
		// Generic directory scan for .md files
		return i.importGenericMarkdownDirectory(source, path)
	}
}

// importClaudeSkills imports from .claude/skills/ directory
func (i *Importer) importClaudeSkills(source, path string) ([]ImportedContent, string, error) {
	var items []ImportedContent

	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, "", oops.
			With("source", source).
			With("path", path).
			Wrapf(err, "read directory")
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		// Look for SKILL.md in each subdirectory
		skillFile := filepath.Join(path, entry.Name(), "SKILL.md")
		if !fileExists(skillFile) {
			continue
		}

		data, err := os.ReadFile(skillFile)
		if err != nil {
			logger.Warn("Failed to read skill file", "path", skillFile, "error", err)
			continue
		}

		content := string(data)
		metadata, actualContent := parseFrontmatterToMetadata(content)

		items = append(items, ImportedContent{
			Name:     entry.Name(),
			Type:     ContentTypeSkill,
			Content:  actualContent,
			Source:   source,
			Metadata: metadata,
			Hash:     hashContent(actualContent),
		})
	}

	return items, claudePresetName, nil
}

// importClaudeAgents imports from .claude/agents/ directory
func (i *Importer) importClaudeAgents(source, path string) ([]ImportedContent, string, error) {
	var items []ImportedContent

	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, "", oops.
			With("source", source).
			With("path", path).
			Wrapf(err, "read directory")
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		if !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		filePath := filepath.Join(path, entry.Name())
		data, err := os.ReadFile(filePath)
		if err != nil {
			logger.Warn("Failed to read agent file", "path", filePath, "error", err)
			continue
		}

		content := string(data)
		metadata, actualContent := parseFrontmatterToMetadata(content)

		items = append(items, ImportedContent{
			Name:     filenameWithoutExt(entry.Name()),
			Type:     ContentTypeSkill,
			Content:  actualContent,
			Source:   source,
			Metadata: metadata,
			Hash:     hashContent(actualContent),
		})
	}

	return items, claudePresetName, nil
}

// importCursorRules imports from .cursor/rules/ directory
func (i *Importer) importCursorRules(source, path string) ([]ImportedContent, string, error) {
	return i.importGenericMarkdownDirectory(source, path)
}

// importWindsurfRules imports from .windsurf/ or .windsurf/rules/ directory
func (i *Importer) importWindsurfRules(source, path string) ([]ImportedContent, string, error) {
	return i.importGenericMarkdownDirectory(source, path)
}

// importContinueRules imports from .continue/rules/ directory
func (i *Importer) importContinueRules(source, path string) ([]ImportedContent, string, error) {
	return i.importGenericMarkdownDirectory(source, path)
}

// importContinuePrompts imports from .continue/prompts/ directory (YAML files)
func (i *Importer) importContinuePrompts(source, path string) ([]ImportedContent, string, error) {
	var items []ImportedContent

	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, "", oops.
			With("source", source).
			With("path", path).
			Wrapf(err, "read directory")
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		if !strings.HasSuffix(entry.Name(), ".yaml") && !strings.HasSuffix(entry.Name(), ".yml") {
			continue
		}

		filePath := filepath.Join(path, entry.Name())
		data, err := os.ReadFile(filePath)
		if err != nil {
			logger.Warn("Failed to read prompt file", "path", filePath, "error", err)
			continue
		}

		// Parse YAML prompt format
		var prompt struct {
			Name        string `yaml:"name"`
			Description string `yaml:"description"`
			Prompt      string `yaml:"prompt"`
		}

		if err := yaml.Unmarshal(data, &prompt); err != nil {
			logger.Warn("Failed to parse prompt YAML", "path", filePath, "error", err)
			continue
		}

		// Use prompt content as context
		content := prompt.Prompt
		if prompt.Description != "" {
			content = fmt.Sprintf("# %s\n\n%s\n\n%s", prompt.Name, prompt.Description, content)
		}

		items = append(items, ImportedContent{
			Name:    filenameWithoutExt(entry.Name()),
			Type:    ContentTypeContext,
			Content: content,
			Source:  source,
			Hash:    hashContent(content),
		})
	}

	return items, "continue-dev", nil
}

// importClineRules imports from .clinerules/ directory
func (i *Importer) importClineRules(source, path string) ([]ImportedContent, string, error) {
	return i.importGenericMarkdownDirectory(source, path)
}

// importGenericMarkdownDirectory imports all .md files from a directory
func (i *Importer) importGenericMarkdownDirectory(source, path string) ([]ImportedContent, string, error) {
	var items []ImportedContent

	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, "", oops.
			With("source", source).
			With("path", path).
			Wrapf(err, "read directory")
	}

	preset := detectPresetFromSource(source)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		// Support both .md and .mdc files
		if !strings.HasSuffix(entry.Name(), ".md") && !strings.HasSuffix(entry.Name(), ".mdc") {
			continue
		}

		filePath := filepath.Join(path, entry.Name())
		data, err := os.ReadFile(filePath)
		if err != nil {
			logger.Warn("Failed to read file", "path", filePath, "error", err)
			continue
		}

		content := string(data)
		metadata, actualContent := parseFrontmatterToMetadata(content)

		// Classify based on filename and content
		contentType := classifyContent(entry.Name(), actualContent)

		items = append(items, ImportedContent{
			Name:        filenameWithoutExt(entry.Name()),
			Type:        contentType,
			Content:     actualContent,
			Source:      source,
			Metadata:    metadata,
			Hash:        hashContent(actualContent),
			OriginalExt: filepath.Ext(entry.Name()),
		})
	}

	return items, preset, nil
}

// parseMarkdownFile parses a markdown file like CLAUDE.md or GEMINI.md
func parseMarkdownFile(source, content string) []ImportedContent {
	var items []ImportedContent

	// Split by markdown headers (## or ###)
	sections := splitMarkdownSections(content)

	for _, section := range sections {
		if strings.TrimSpace(section.Content) == "" {
			continue
		}

		// Classify based on header and content
		contentType := classifyContent(section.Header, section.Content)

		items = append(items, ImportedContent{
			Name:    utils.SanitizeName(section.Header),
			Type:    contentType,
			Content: section.Content,
			Source:  source,
			Hash:    hashContent(section.Content),
		})
	}

	return items
}

// MarkdownSection represents a section of a markdown document
type MarkdownSection struct {
	Header  string
	Content string
}

// splitMarkdownSections splits markdown content into sections by headers
func splitMarkdownSections(content string) []MarkdownSection {
	var sections []MarkdownSection
	lines := strings.Split(content, "\n")

	var currentHeader string
	var currentContent strings.Builder

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Check for ## or ### headers
		if strings.HasPrefix(trimmed, "## ") || strings.HasPrefix(trimmed, "### ") {
			// Save previous section
			if currentHeader != "" {
				sections = append(sections, MarkdownSection{
					Header:  currentHeader,
					Content: strings.TrimSpace(currentContent.String()),
				})
				currentContent.Reset()
			}

			// Start new section
			currentHeader = strings.TrimPrefix(strings.TrimPrefix(trimmed, "### "), "## ")
		} else {
			// Add to current section
			currentContent.WriteString(line)
			currentContent.WriteString("\n")
		}
	}

	// Save last section
	if currentHeader != "" {
		sections = append(sections, MarkdownSection{
			Header:  currentHeader,
			Content: strings.TrimSpace(currentContent.String()),
		})
	}

	return sections
}

// classifyContent classifies content as rule, context, or skill using multiple signals
// It combines filename indicators, content structure, and keyword heuristics for better accuracy
//
//nolint:gocyclo // Complex logic, acceptable for this use case
func classifyContent(name, content string) ContentType {
	nameLower := strings.ToLower(name)
	contentLower := strings.ToLower(content)
	contentLen := len(strings.TrimSpace(content))
	wordCount := len(strings.Fields(content))

	// Calculate signals for each type
	var ruleScore, skillScore, contextScore int

	// --- RULE SIGNALS ---
	// Strong filename indicators
	if strings.Contains(nameLower, "rule") {
		ruleScore += 4
	}
	if strings.Contains(nameLower, "standard") || strings.Contains(nameLower, "guideline") {
		ruleScore += 3
	}
	if strings.Contains(nameLower, "requirement") || strings.Contains(nameLower, "constraint") {
		ruleScore += 3
	}

	// Content structure indicators
	if strings.Contains(contentLower, "## ") || strings.Contains(contentLower, "### ") {
		// Well-structured content (likely documentation)
		contextScore += 2
	}

	// Rule keywords with weighting
	ruleKeywords := map[string]int{
		"must":      2,
		"should":    2,
		"required":  2,
		"forbidden": 2,
		"always":    1,
		"never":     1,
		"must not":  2,
		"shall":     2,
	}
	for keyword, weight := range ruleKeywords {
		if strings.Contains(contentLower, keyword) {
			ruleScore += weight
		}
	}

	// --- SKILL SIGNALS ---
	// Strong filename indicators
	if strings.Contains(nameLower, "agent") {
		skillScore += 4
	}
	if strings.Contains(nameLower, "skill") {
		skillScore += 4
	}
	if strings.Contains(nameLower, "command") {
		skillScore += 3
	}
	if strings.Contains(nameLower, "tool") || strings.Contains(nameLower, "utility") {
		skillScore += 2
	}

	// Skill content indicators
	if strings.Contains(contentLower, "function") || strings.Contains(contentLower, "parameter") {
		skillScore += 2
	}
	if strings.Contains(contentLower, "usage:") || strings.Contains(contentLower, "example:") {
		skillScore += 2
	}

	// --- CONTEXT SIGNALS ---
	// Well-structured, longer content (documentation, background)
	if contentLen > 500 {
		contextScore += 2
	}
	if contentLen > 1000 {
		contextScore += 2
	}

	// Specific context keywords
	if strings.Contains(contentLower, "background") || strings.Contains(contentLower, "overview") {
		contextScore += 2
	}
	if strings.Contains(contentLower, "project") || strings.Contains(contentLower, "architecture") {
		contextScore += 2
	}
	if strings.Contains(contentLower, "## ") && wordCount > 100 {
		// Well-structured, substantial content
		contextScore += 2
	}

	// Short content without strong signals defaults to context
	if wordCount < 20 {
		if ruleScore == 0 && skillScore == 0 {
			contextScore++
		}
	}

	// Determine winner by score
	// Require higher threshold to avoid false positives from single keywords in descriptions
	if ruleScore > skillScore && ruleScore > contextScore && ruleScore >= 3 {
		return ContentTypeRule
	}

	if skillScore > ruleScore && skillScore > contextScore && skillScore >= 3 {
		return ContentTypeSkill
	}

	// Default to context (always safe default)
	return ContentTypeContext
}

// deduplicateContent removes duplicate content based on hash and merges metadata
// When duplicates are found, it keeps the item with highest priority and combines targets
func deduplicateContent(items []ImportedContent) []ImportedContent {
	// Map hash -> first occurrence index
	seenMap := make(map[string]int)
	var result []ImportedContent

	for _, item := range items {
		if idx, found := seenMap[item.Hash]; found {
			// Merge metadata from duplicate into the existing item
			mergeMetadata(&result[idx], &item)
		} else {
			// First time seeing this hash
			seenMap[item.Hash] = len(result)
			result = append(result, item)
		}
	}

	return result
}

// mergeMetadata combines metadata from duplicate items
// Keeps the highest priority and combines all targets
func mergeMetadata(existing, duplicate *ImportedContent) {
	// If duplicate has no metadata, nothing to merge
	if duplicate.Metadata == nil {
		return
	}

	// If existing has no metadata, copy from duplicate
	if existing.Metadata == nil {
		existing.Metadata = duplicate.Metadata
		return
	}

	// Merge priorities (keep highest)
	priorityOrder := map[string]int{
		"critical": 5,
		"high":     4,
		"medium":   3,
		"low":      2,
		"info":     1,
	}

	existingPrio := priorityOrder[existing.Metadata.Priority]
	duplicatePrio := priorityOrder[duplicate.Metadata.Priority]

	if duplicatePrio > existingPrio {
		existing.Metadata.Priority = duplicate.Metadata.Priority
	}

	// Merge targets (combine and deduplicate)
	if len(duplicate.Metadata.Targets) > 0 {
		targetSet := make(map[string]bool)

		// Add existing targets
		for _, t := range existing.Metadata.Targets {
			targetSet[t] = true
		}

		// Add duplicate targets
		for _, t := range duplicate.Metadata.Targets {
			targetSet[t] = true
		}

		// Convert back to slice
		var merged []string
		for t := range targetSet {
			merged = append(merged, t)
		}

		existing.Metadata.Targets = merged
	}
}

// hashContent generates a hash of normalized content
func hashContent(content string) string {
	normalized := strings.TrimSpace(strings.ToLower(content))
	hash := sha256.Sum256([]byte(normalized))
	return fmt.Sprintf("%x", hash)
}

// writeContent writes imported content to .ai-rulez/ structure
func (i *Importer) writeContent(items []ImportedContent) error {
	// Create base directories
	dirs := []string{
		i.outputDir,
		filepath.Join(i.outputDir, "rules"),
		filepath.Join(i.outputDir, "context"),
		filepath.Join(i.outputDir, "skills"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return oops.
				With("path", dir).
				Wrapf(err, "create directory")
		}
	}

	// Write content by type
	for idx := range items {
		if err := i.writeContentFile(&items[idx]); err != nil {
			return err
		}
	}

	return nil
}

// writeContentFile writes a single content file
func (i *Importer) writeContentFile(item *ImportedContent) error {
	var dir string

	switch item.Type {
	case ContentTypeRule:
		dir = filepath.Join(i.outputDir, "rules")
	case ContentTypeContext:
		dir = filepath.Join(i.outputDir, "context")
	case ContentTypeSkill:
		dir = filepath.Join(i.outputDir, "skills", item.Name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return oops.
				With("path", dir).
				Wrapf(err, "create skill directory")
		}
	}

	// Generate filename
	var filename string
	if item.Type == ContentTypeSkill {
		filename = filepath.Join(dir, "SKILL.md")
	} else {
		filename = filepath.Join(dir, item.Name+".md")
	}

	// Build content with optional frontmatter
	var output strings.Builder

	if item.Metadata != nil {
		output.WriteString("---\n")

		if item.Metadata.Priority != "" {
			output.WriteString(fmt.Sprintf("priority: %s\n", item.Metadata.Priority))
		}

		if len(item.Metadata.Targets) > 0 {
			output.WriteString("targets:\n")
			for _, target := range item.Metadata.Targets {
				output.WriteString(fmt.Sprintf("  - %s\n", target))
			}
		}

		output.WriteString("---\n\n")
	}

	output.WriteString(item.Content)

	// Write file
	if err := os.WriteFile(filename, []byte(output.String()), 0o644); err != nil {
		return oops.
			With("path", filename).
			Wrapf(err, "write file")
	}

	logger.Debug("Wrote content file", "path", filename, "type", item.Type)
	return nil
}

// writeConfig writes the config.yaml file with detected presets
func (i *Importer) writeConfig(projectName string, detectedPresets map[string]bool) error {
	// Convert presets map to slice
	var presets []config.PresetV3
	for preset := range detectedPresets {
		presets = append(presets, config.PresetV3{BuiltIn: preset})
	}

	// If no presets detected, default to claude
	if len(presets) == 0 {
		presets = []config.PresetV3{{BuiltIn: "claude"}}
	}

	cfg := &config.ConfigV3{
		Version: "3.0",
		Name:    projectName,
		Presets: presets,
	}

	// Marshal to YAML
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return oops.Wrapf(err, "marshal config")
	}

	// Add header comment
	var output strings.Builder
	output.WriteString("# AI-Rulez V3 Configuration\n")
	output.WriteString("# Imported from existing tool files\n")
	output.WriteString("# Documentation: https://github.com/Goldziher/ai-rulez\n\n")
	output.WriteString(string(data))

	configPath := filepath.Join(i.outputDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(output.String()), 0o644); err != nil {
		return oops.
			With("path", configPath).
			Wrapf(err, "write config file")
	}

	logger.Debug("Wrote config file", "path", configPath, "presets", len(presets))
	return nil
}

// detectPresetFromSource detects the preset name from a source path
func detectPresetFromSource(source string) string {
	sourceLower := strings.ToLower(source)

	switch {
	case strings.Contains(sourceLower, "claude"):
		return "claude"
	case strings.Contains(sourceLower, "cursor"):
		return "cursor"
	case strings.Contains(sourceLower, "gemini"):
		return "gemini"
	case strings.Contains(sourceLower, "copilot"):
		return "copilot"
	case strings.Contains(sourceLower, "continue"):
		return "continue-dev"
	case strings.Contains(sourceLower, "windsurf"):
		return "windsurf"
	case strings.Contains(sourceLower, "cline"):
		return "cline"
	default:
		return ""
	}
}

// parseFrontmatterToMetadata parses frontmatter and returns metadata + content
// Uses the canonical parser from internal/parser package
// This function maintains backward compatibility with existing importer code
func parseFrontmatterToMetadata(content string) (metadata *config.MetadataV3, body string) {
	// Use canonical parser, non-fatal version for importer compatibility
	parserMetadata, actualContent := parser.ParseFrontmatterNonFatal(content)

	// Convert parser.MetadataV3 to config.MetadataV3
	if parserMetadata != nil {
		metadata = &config.MetadataV3{
			Priority: parserMetadata.Priority,
			Targets:  parserMetadata.Targets,
			Extra:    parserMetadata.Extra,
		}
	}

	return metadata, actualContent
}

// Helper functions

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func filenameWithoutExt(filename string) string {
	return strings.TrimSuffix(filename, filepath.Ext(filename))
}

func getProjectName(dir string) string {
	// Get directory name as project name
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return "imported-project"
	}

	name := filepath.Base(absDir)
	if name == "." || name == "/" {
		return "imported-project"
	}

	return name
}
