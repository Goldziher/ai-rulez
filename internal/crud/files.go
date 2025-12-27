package crud

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/samber/oops"
)

// Content type constants
const (
	ContentTypeRules   = "rules"
	ContentTypeContext = "context"
	ContentTypeSkills  = "skills"
)

// FileManager handles file I/O operations for CRUD
type FileManager struct {
	aiRulezDir string
}

// NewFileManager creates a new FileManager for the given .ai-rulez directory
func NewFileManager(aiRulezDir string) *FileManager {
	return &FileManager{
		aiRulezDir: aiRulezDir,
	}
}

// PathExists checks if a path exists
func (fm *FileManager) PathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// IsDirectory checks if a path is a directory
func (fm *FileManager) IsDirectory(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// CreateDirectory creates a directory with all parent directories
func (fm *FileManager) CreateDirectory(path string) error {
	if err := os.MkdirAll(path, 0o755); err != nil {
		return oops.
			With("path", path).
			Hint("Check filesystem permissions and available disk space.").
			Wrapf(err, "create directory")
	}
	return nil
}

// DeleteDirectory recursively deletes a directory
func (fm *FileManager) DeleteDirectory(path string) error {
	if !fm.PathExists(path) {
		return oops.
			With("path", path).
			Hint("The directory to delete does not exist.").
			Errorf("directory not found")
	}

	if err := os.RemoveAll(path); err != nil {
		return oops.
			With("path", path).
			Hint("Check filesystem permissions and whether the directory is locked by another process.").
			Wrapf(err, "delete directory")
	}

	return nil
}

// WriteFile writes content to a file atomically (temp file → rename)
func (fm *FileManager) WriteFile(path string, content string) error {
	// Check if file already exists
	if fm.PathExists(path) {
		return &FileExistsError{
			Path: path,
			Type: "file",
		}
	}

	// Ensure parent directory exists
	dir := filepath.Dir(path)
	if err := fm.CreateDirectory(dir); err != nil {
		return err
	}

	// Write to temp file first
	tempFile := path + ".tmp"
	if err := os.WriteFile(tempFile, []byte(content), 0o644); err != nil {
		return oops.
			With("path", tempFile).
			Hint("Check filesystem permissions and available disk space.").
			Wrapf(err, "write temporary file")
	}

	// Atomic rename
	if err := os.Rename(tempFile, path); err != nil {
		//nolint:gosec,errcheck // best effort cleanup on rename failure
		_ = os.Remove(tempFile)
		return oops.
			With("path", path).
			With("temp_path", tempFile).
			Hint("Check filesystem permissions and available disk space.").
			Wrapf(err, "atomic rename failed")
	}

	return nil
}

// ReadFile reads file contents
func (fm *FileManager) ReadFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", oops.
				With("path", path).
				Hint("The file does not exist.").
				Wrapf(err, "file not found")
		}
		return "", oops.
			With("path", path).
			Wrapf(err, "read file")
	}
	return string(data), nil
}

// DeleteFile deletes a file
func (fm *FileManager) DeleteFile(path string) error {
	if !fm.PathExists(path) {
		return oops.
			With("path", path).
			Hint("The file to delete does not exist.").
			Errorf("file not found")
	}

	if err := os.Remove(path); err != nil {
		return oops.
			With("path", path).
			Hint("Check filesystem permissions and whether the file is locked by another process.").
			Wrapf(err, "delete file")
	}

	return nil
}

// ListDirectory lists files in a directory (non-recursive)
func (fm *FileManager) ListDirectory(path string) ([]os.DirEntry, error) {
	if !fm.PathExists(path) {
		return []os.DirEntry{}, nil
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, oops.
			With("path", path).
			Wrapf(err, "read directory")
	}

	return entries, nil
}

// ListMarkdownFiles lists .md files in a directory
func (fm *FileManager) ListMarkdownFiles(path string) ([]string, error) {
	entries, err := fm.ListDirectory(path)
	if err != nil {
		return nil, err
	}

	var files []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".md") {
			files = append(files, entry.Name())
		}
	}

	return files, nil
}

// ListSubdirectories lists subdirectories in a directory
func (fm *FileManager) ListSubdirectories(path string) ([]string, error) {
	entries, err := fm.ListDirectory(path)
	if err != nil {
		return nil, err
	}

	var dirs []string
	for _, entry := range entries {
		if entry.IsDir() {
			dirs = append(dirs, entry.Name())
		}
	}

	return dirs, nil
}

// GetDomainPath returns the path for a domain directory
func (fm *FileManager) GetDomainPath(domainName string) string {
	return filepath.Join(fm.aiRulezDir, "domains", domainName)
}

// GetRulesPath returns the path for the rules directory
func (fm *FileManager) GetRulesPath(domainName string) string {
	if domainName != "" {
		return filepath.Join(fm.GetDomainPath(domainName), "rules")
	}
	return filepath.Join(fm.aiRulezDir, "rules")
}

// GetContextPath returns the path for the context directory
func (fm *FileManager) GetContextPath(domainName string) string {
	if domainName != "" {
		return filepath.Join(fm.GetDomainPath(domainName), "context")
	}
	return filepath.Join(fm.aiRulezDir, "context")
}

// GetSkillsPath returns the path for the skills directory
func (fm *FileManager) GetSkillsPath(domainName string) string {
	if domainName != "" {
		return filepath.Join(fm.GetDomainPath(domainName), "skills")
	}
	return filepath.Join(fm.aiRulezDir, "skills")
}

// GetFilePath returns the full path for a content file
func (fm *FileManager) GetFilePath(domain, ftype, name string) string {
	var dirPath string

	switch ftype {
	case ContentTypeRules:
		dirPath = fm.GetRulesPath(domain)
	case ContentTypeContext:
		dirPath = fm.GetContextPath(domain)
	case ContentTypeSkills:
		// For skills, return the SKILL.md file within the skill directory
		dirPath = fm.GetSkillsPath(domain)
		return filepath.Join(dirPath, name, "SKILL.md")
	default:
		dirPath = fm.GetRulesPath(domain)
	}

	return filepath.Join(dirPath, name+".md")
}

// CreateDomainStructure creates the domain directory structure
func (fm *FileManager) CreateDomainStructure(domainName string) error {
	domainPath := fm.GetDomainPath(domainName)

	// Create main domain directory
	if err := fm.CreateDirectory(domainPath); err != nil {
		return err
	}

	// Create subdirectories
	subdirs := []string{"rules", "context", "skills"}
	for _, subdir := range subdirs {
		path := filepath.Join(domainPath, subdir)
		if err := fm.CreateDirectory(path); err != nil {
			return err
		}
	}

	return nil
}

// DomainExists checks if a domain directory exists
func (fm *FileManager) DomainExists(domainName string) bool {
	path := fm.GetDomainPath(domainName)
	return fm.IsDirectory(path)
}

// FileOrSkillExists checks if a file exists
// For skills, checks if the skill directory exists
func (fm *FileManager) FileOrSkillExists(domain, ftype, name string) bool {
	if ftype == "skills" {
		// For skills, check if the skill directory exists
		skillDir := filepath.Join(fm.GetSkillsPath(domain), name)
		return fm.IsDirectory(skillDir)
	}

	filePath := fm.GetFilePath(domain, ftype, name)
	return fm.PathExists(filePath)
}

// MakeFileName converts a name to a valid filename (removes extension if present)
func MakeFileName(name string) string {
	// Remove .md extension if present
	if strings.HasSuffix(name, ".md") {
		return strings.TrimSuffix(name, ".md")
	}
	return name
}

// NormalizeTargets normalizes targets array (removes duplicates, sorts)
func NormalizeTargets(targets []string) []string {
	if len(targets) == 0 {
		return targets
	}

	seen := make(map[string]bool)
	var normalized []string

	for _, target := range targets {
		target = strings.TrimSpace(target)
		if target != "" && !seen[target] {
			normalized = append(normalized, target)
			seen[target] = true
		}
	}

	return normalized
}

// FormatContent ensures content is properly formatted
func FormatContent(content string) string {
	// Trim leading/trailing whitespace
	content = strings.TrimSpace(content)

	// Ensure single trailing newline
	if content != "" {
		content += "\n"
	}

	return content
}

// PathRelativeToWorkdir returns a path relative to the working directory
func PathRelativeToWorkdir(path string) (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return path, err
	}

	rel, err := filepath.Rel(wd, path)
	if err != nil {
		return path, err
	}

	return rel, nil
}

// EnsureTrailingNewline ensures the content ends with a single newline
func EnsureTrailingNewline(content string) string {
	content = strings.TrimRight(content, "\r\n")
	if content != "" {
		content += "\n"
	}
	return content
}
