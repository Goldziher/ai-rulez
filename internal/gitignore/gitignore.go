package gitignore

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
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
	existingEntries, err := readGitignoreEntries(gitignorePath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to read .gitignore: %w", err)
	}

	var toAdd []string
	for _, outputFile := range outputFiles {
		if !isIgnored(outputFile, existingEntries) {
			toAdd = append(toAdd, outputFile)
		}
	}

	if len(toAdd) == 0 {
		return nil
	}

	return appendToGitignore(gitignorePath, toAdd, len(existingEntries) == 0)
}

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

func appendToGitignore(gitignorePath string, entries []string, isNewFile bool) error {
	file, err := os.OpenFile(gitignorePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("failed to open .gitignore for writing: %w", err)
	}
	defer func() { _ = file.Close() }()

	writer := bufio.NewWriter(file)
	defer func() { _ = writer.Flush() }()

	if isNewFile {
		if _, err := writer.WriteString("# AI Rules generated files\n"); err != nil {
			return err
		}
	} else {
		if _, err := writer.WriteString("\n# AI Rules generated files\n"); err != nil {
			return err
		}
	}

	for _, entry := range entries {
		if _, err := writer.WriteString(entry + "\n"); err != nil {
			return err
		}
	}

	return writer.Flush()
}
