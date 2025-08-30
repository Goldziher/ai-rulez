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

	var outputFiles []string
	for _, output := range cfg.Outputs {
		path := output.GetPath()
		if output.IsDirectory() {
			dirs := strings.Split(strings.TrimSuffix(path, "/"), "/")
			if len(dirs) > 0 {
				outputFiles = append(outputFiles, dirs[0]+"/")
			}
		} else {
			outputFiles = append(outputFiles, path)
		}
	}

	if len(outputFiles) == 0 {
		return nil
	}

	return updateGitignoreFile(gitignorePath, outputFiles)
}

func UpdateGitignoreFilesRecursive(configFiles []string) error {
	for _, configFile := range configFiles {
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
		if strings.HasSuffix(filename, "/") {
			return pattern == filename
		}
		dirPrefix := strings.TrimSuffix(pattern, "/")
		return strings.HasPrefix(filename, dirPrefix+"/") || filename == dirPrefix
	}

	if strings.Contains(pattern, "*") {
		return matchesWildcard(filename, pattern)
	}

	if strings.HasPrefix(pattern, "/") {
		return filename == strings.TrimPrefix(pattern, "/")
	}

	return filename == pattern || strings.HasSuffix(filename, "/"+pattern) || strings.Contains(filename, pattern)
}

func matchesWildcard(filename, pattern string) bool {
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

	return strings.Contains(filename, strings.ReplaceAll(pattern, "*", ""))
}

func appendToGitignore(gitignorePath string, entries []string, isNewFile bool) error {
	file, err := os.OpenFile(gitignorePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("failed to open .gitignore for writing: %w", err)
	}
	defer func() { _ = file.Close() }()

	if isNewFile {
		if _, err := file.WriteString("# AI Rules generated files\n"); err != nil {
			return err
		}
	} else {
		if _, err := file.WriteString("\n# AI Rules generated files\n"); err != nil {
			return err
		}
	}

	for _, entry := range entries {
		if _, err := file.WriteString(entry + "\n"); err != nil {
			return err
		}
	}

	return nil
}
