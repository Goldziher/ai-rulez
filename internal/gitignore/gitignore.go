package gitignore

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
)

// Fenced block markers for managed gitignore sections
const (
	BeginMarker = "# BEGIN ai-rulez (DO NOT EDIT - managed by ai-rulez)"
	EndMarker   = "# END ai-rulez"
	OldHeader   = "# AI Rules generated files"
)

func UpdateGitignoreFiles(configFile string, cfg *config.Config) error {
	configDir := filepath.Dir(configFile)
	gitignorePath := filepath.Join(configDir, ".gitignore")

	var outputPaths []string
	seen := make(map[string]bool)

	for _, output := range cfg.Outputs {
		path := output.Path
		if !seen[path] {
			outputPaths = append(outputPaths, path)
			seen[path] = true
		}
	}

	if len(outputPaths) == 0 {
		return nil
	}

	return updateGitignoreFile(gitignorePath, outputPaths)
}

func updateGitignoreFile(gitignorePath string, outputFiles []string) error {
	// Sort for deterministic output
	sort.Strings(outputFiles)

	// Build the fenced block
	var fencedBlock strings.Builder
	fencedBlock.WriteString(BeginMarker + "\n")
	for _, entry := range outputFiles {
		fencedBlock.WriteString(entry + "\n")
	}
	fencedBlock.WriteString(EndMarker + "\n")

	// Read existing .gitignore content
	existingData, err := os.ReadFile(gitignorePath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to read .gitignore: %w", err)
	}

	var newContent string
	existingContent := string(existingData)

	switch {
	case strings.Contains(existingContent, BeginMarker):
		newContent = ReplaceFencedBlock(existingContent, fencedBlock.String())
	case strings.Contains(existingContent, OldHeader):
		newContent = ReplaceOldHeaderBlock(existingContent, fencedBlock.String())
	case len(existingData) == 0:
		newContent = fencedBlock.String()
	default:
		newContent = existingContent
		if !strings.HasSuffix(newContent, "\n") {
			newContent += "\n"
		}
		newContent += "\n" + fencedBlock.String()
	}

	if err := os.WriteFile(gitignorePath, []byte(newContent), 0o644); err != nil {
		return fmt.Errorf("failed to write .gitignore: %w", err)
	}

	return nil
}

// ReplaceFencedBlock replaces the content between BEGIN and END markers
func ReplaceFencedBlock(content, newBlock string) string {
	lines := strings.Split(content, "\n")
	var result strings.Builder
	inBlock := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == BeginMarker {
			inBlock = true
			continue
		}
		if trimmed == EndMarker {
			inBlock = false
			continue
		}
		if !inBlock {
			result.WriteString(line + "\n")
		}
	}

	// Remove trailing newlines to avoid accumulating blank lines
	out := strings.TrimRight(result.String(), "\n")
	if out != "" {
		out += "\n\n"
	}
	out += newBlock
	return out
}

// ReplaceOldHeaderBlock replaces from the old-style header to end of file
func ReplaceOldHeaderBlock(content, newBlock string) string {
	lines := strings.Split(content, "\n")
	var result strings.Builder

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == OldHeader {
			break
		}
		result.WriteString(line + "\n")
	}

	out := strings.TrimRight(result.String(), "\n")
	if out != "" {
		out += "\n\n"
	}
	out += newBlock
	return out
}

func readGitignoreEntries(gitignorePath string) ([]string, error) {
	data, err := os.ReadFile(gitignorePath)
	if err != nil {
		return nil, err
	}

	var entries []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			entries = append(entries, line)
		}
	}

	return entries, nil
}

func isIgnored(filename string, patterns []string) bool {
	for _, pattern := range patterns {
		if matchesPattern(filename, pattern) {
			return true
		}
	}
	return false
}

func matchesPattern(filename, pattern string) bool {
	if pattern == filename {
		return true
	}

	if strings.HasSuffix(pattern, "/") {
		return matchesDirectory(filename, pattern)
	}

	if strings.Contains(pattern, "*") || strings.Contains(pattern, "?") {
		if matched, _ := filepath.Match(pattern, filename); matched {
			return true
		}
		if matched, _ := filepath.Match(pattern, filepath.Base(filename)); matched {
			return true
		}
		return false
	}

	if strings.HasPrefix(pattern, "/") {
		return filename == strings.TrimPrefix(pattern, "/")
	}

	return filename == pattern ||
		strings.HasSuffix(filename, "/"+pattern) ||
		strings.Contains(filename, "/"+pattern+"/") ||
		strings.Contains(filename, pattern)
}

func matchesDirectory(filename, pattern string) bool {
	dirPrefix := strings.TrimSuffix(pattern, "/")

	if strings.HasSuffix(filename, "/") {
		return pattern == filename || strings.TrimSuffix(filename, "/") == dirPrefix
	}

	return strings.HasPrefix(filename, dirPrefix+"/") || filename == dirPrefix
}
