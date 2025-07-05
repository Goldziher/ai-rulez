// Package gitignore provides functionality to update .gitignore files with generated output files.
package gitignore

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
)

// UpdateGitignoreFiles updates .gitignore files in the directories containing config files
// to include the generated output files if they're not already ignored.
func UpdateGitignoreFiles(configFile string, cfg *config.Config) error {
	configDir := filepath.Dir(configFile)
	gitignorePath := filepath.Join(configDir, ".gitignore")

	// Get the list of output file names
	var outputFiles []string
	for _, output := range cfg.Outputs {
		outputFiles = append(outputFiles, output.File)
	}

	if len(outputFiles) == 0 {
		return nil
	}

	return updateGitignoreFile(gitignorePath, outputFiles)
}

// UpdateGitignoreFilesRecursive updates .gitignore files for all provided config files
func UpdateGitignoreFilesRecursive(configFiles []string) error {
	for _, configFile := range configFiles {
		// Load configuration to get output files
		cfg, err := config.LoadConfig(configFile)
		if err != nil {
			return fmt.Errorf("failed to load config %s: %w", configFile, err)
		}

		if err := UpdateGitignoreFiles(configFile, cfg); err != nil {
			return fmt.Errorf("failed to update gitignore for %s: %w", configFile, err)
		}
	}
	return nil
}

// updateGitignoreFile adds the specified files to the .gitignore file if they're not already present
func updateGitignoreFile(gitignorePath string, outputFiles []string) error {
	// Read existing gitignore content
	existingEntries, err := readGitignoreEntries(gitignorePath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to read .gitignore: %w", err)
	}

	// Find which output files need to be added
	var toAdd []string
	for _, outputFile := range outputFiles {
		if !isIgnored(outputFile, existingEntries) {
			toAdd = append(toAdd, outputFile)
		}
	}

	// If nothing to add, we're done
	if len(toAdd) == 0 {
		return nil
	}

	// Append new entries to .gitignore
	return appendToGitignore(gitignorePath, toAdd, len(existingEntries) == 0)
}

// readGitignoreEntries reads all non-empty, non-comment lines from .gitignore
func readGitignoreEntries(gitignorePath string) ([]string, error) {
	file, err := os.Open(gitignorePath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()

	var entries []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && !strings.HasPrefix(line, "#") {
			entries = append(entries, line)
		}
	}

	return entries, scanner.Err()
}

// isIgnored checks if a file would be ignored by any of the existing gitignore patterns
func isIgnored(filename string, patterns []string) bool {
	for _, pattern := range patterns {
		if matchesPattern(filename, pattern) {
			return true
		}
	}
	return false
}

// matchesPattern checks if a filename matches a gitignore pattern
// This is a simplified implementation that handles basic patterns
func matchesPattern(filename, pattern string) bool {
	// Exact match
	if pattern == filename {
		return true
	}

	// Pattern ends with / - directory only
	if strings.HasSuffix(pattern, "/") {
		return false // We're dealing with files, not directories
	}

	// Pattern with wildcards
	if strings.Contains(pattern, "*") {
		return matchesWildcard(filename, pattern)
	}

	// Pattern starting with / - absolute path from repo root
	if strings.HasPrefix(pattern, "/") {
		return filename == strings.TrimPrefix(pattern, "/")
	}

	// Simple name or substring match for patterns without special chars
	return filename == pattern || strings.HasSuffix(filename, "/"+pattern) || strings.Contains(filename, pattern)
}

// matchesWildcard performs basic wildcard matching
func matchesWildcard(filename, pattern string) bool {
	// Very basic wildcard implementation - handles *.extension patterns
	if pattern == "*" {
		return true
	}

	if strings.HasPrefix(pattern, "*.") {
		extension := strings.TrimPrefix(pattern, "*")
		return strings.HasSuffix(filename, extension)
	}

	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(filename, prefix)
	}

	// For more complex patterns, do a simple contains check
	return strings.Contains(filename, strings.ReplaceAll(pattern, "*", ""))
}

// appendToGitignore appends new entries to the .gitignore file
func appendToGitignore(gitignorePath string, entries []string, isNewFile bool) error {
	file, err := os.OpenFile(gitignorePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("failed to open .gitignore for writing: %w", err)
	}
	defer func() { _ = file.Close() }()

	// Add a comment section for ai-rulez generated files
	if isNewFile {
		if _, err := file.WriteString("# AI Rules generated files\n"); err != nil {
			return err
		}
	} else {
		if _, err := file.WriteString("\n# AI Rules generated files\n"); err != nil {
			return err
		}
	}

	// Add each entry
	for _, entry := range entries {
		if _, err := file.WriteString(entry + "\n"); err != nil {
			return err
		}
	}

	return nil
}
