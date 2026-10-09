package crud

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// Content type constants
const (
	ContentTypeRules   = "rules"
	ContentTypeContext = "context"
	ContentTypeSkills  = "skills"
	// ContentTypeAgents and ContentTypeCommands are flat markdown files, like
	// rules and context.
	ContentTypeAgents   = "agents"
	ContentTypeCommands = "commands"
	// ContentTypeChecks is a flat markdown file of code-review guidance.
	ContentTypeChecks = "checks"
)

// FileManager handles file I/O operations for CRUD
type FileManager struct {
	aiRulezDir string
	// guard runs before anything is written; a failure aborts the write. The
	// local operator uses it to make sure the tree is gitignored first.
	guard func() error
	// private makes new files owner-only (0600) and new directories 0700.
	private bool
	// root is the configuration directory the confinement check starts from; ""
	// means aiRulezDir. The machine-local tree sets it to the directory above
	// its own, so a symlinked local/ is caught too.
	root string
}

// NewFileManager creates a new FileManager for the given .ai-rulez directory
func NewFileManager(aiRulezDir string) *FileManager {
	return &FileManager{
		aiRulezDir: aiRulezDir,
	}
}

// Confined reports an error when path is outside the configuration directory or
// any component of it below the configuration directory is a symlink. Content is
// read, written and deleted by path, and a link planted inside the tree would
// otherwise carry the operation to wherever it points. Components that do not
// exist yet are fine. The configuration directory itself may be reached through
// a link: only what lies below it is checked.
func (fm *FileManager) Confined(path string) error {
	root := fm.root
	if root == "" {
		root = fm.aiRulezDir
	}
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return oops.With("path", path).Hint("Only files inside the configuration directory can be changed.").
			Errorf("%s is outside the configuration directory", path)
	}
	cur := root
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		if part == "." || part == "" {
			continue
		}
		cur = filepath.Join(cur, part)
		info, err := os.Lstat(cur)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return oops.With("path", cur).Wrapf(err, "inspect path")
		}
		if info.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 {
			return oops.With("path", cur).
				Hint("Replace the symlink with a real directory or file, or remove it; ai-rulez does not follow links inside the configuration directory.").
				Errorf("%s is a symlink: refusing to follow it", cur)
		}
	}
	return nil
}

// isGeneratedListing reports whether path is the index.md or log.md that
// "migrate okf" generates: a listing, not content. A file of that name that holds
// prose is still content.
func (fm *FileManager) isGeneratedListing(path string) bool {
	switch strings.ToLower(filepath.Base(path)) {
	case "index.md", "log.md":
	default:
		return false
	}
	if fm.Confined(path) != nil {
		return false
	}
	data, err := os.ReadFile(path) //nolint:gosec // G304: confined to the configuration directory above
	return err == nil && config.IsOKFListing(data)
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
	if err := fm.Confined(path); err != nil {
		return err
	}
	if fm.guard != nil {
		if err := fm.guard(); err != nil {
			return err
		}
	}
	mode := os.FileMode(0o755)
	if fm.private {
		mode = 0o700
	}
	if err := os.MkdirAll(path, mode); err != nil {
		return oops.
			With("path", path).
			Hint("Check filesystem permissions and available disk space.").
			Wrapf(err, "create directory")
	}
	return nil
}

// DeleteDirectory recursively deletes a directory
func (fm *FileManager) DeleteDirectory(path string) error {
	if err := fm.Confined(path); err != nil {
		return err
	}
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

// WriteFile writes content to a new file atomically (temp file → rename),
// refusing to overwrite an existing file. Use WriteFileOverwrite for updates.
func (fm *FileManager) WriteFile(path string, content string) error {
	// Check if file already exists
	if fm.PathExists(path) {
		return &FileExistsError{
			Path: path,
			Type: "file",
		}
	}

	return fm.writeFileAtomic(path, content)
}

// WriteFileOverwrite writes content to a file atomically (temp file → rename),
// overwriting any existing file. Used by update operations where the target is
// expected to already exist.
func (fm *FileManager) WriteFileOverwrite(path string, content string) error {
	return fm.writeFileAtomic(path, content)
}

// writeFileAtomic writes content via a temp file and atomic rename. The rename
// overwrites the destination if it exists, so callers that must not clobber an
// existing file guard with PathExists before calling.
func (fm *FileManager) writeFileAtomic(path string, content string) error {
	if err := fm.Confined(path); err != nil {
		return err
	}
	// Ensure parent directory exists
	dir := filepath.Dir(path)
	if err := fm.CreateDirectory(dir); err != nil {
		return err
	}

	perm := os.FileMode(0o644)
	if fm.private {
		perm = 0o600
	}
	// Exclusively created temp file beside the target, then renamed over it: a
	// pre-planted path (or symlink) can never be written through.
	if err := config.WriteFileAtomic(path, []byte(content), perm); err != nil {
		return oops.
			With("path", path).
			Hint("Check filesystem permissions and available disk space.").
			Wrapf(err, "write file")
	}

	return nil
}

// ReadFile reads file contents
func (fm *FileManager) ReadFile(path string) (string, error) {
	if err := fm.Confined(path); err != nil {
		return "", err
	}
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
	if err := fm.Confined(path); err != nil {
		return err
	}
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
		if entry.Type()&(os.ModeSymlink|os.ModeIrregular) != 0 {
			continue // a link is never followed, so it is not listed
		}
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

// GetAgentsPath returns the path for the agents directory
func (fm *FileManager) GetAgentsPath(domainName string) string {
	return fm.contentDir(domainName, ContentTypeAgents)
}

// GetCommandsPath returns the path for the commands directory
func (fm *FileManager) GetCommandsPath(domainName string) string {
	return fm.contentDir(domainName, ContentTypeCommands)
}

// GetChecksPath returns the path for the checks directory
func (fm *FileManager) GetChecksPath(domainName string) string {
	return fm.contentDir(domainName, ContentTypeChecks)
}

func (fm *FileManager) contentDir(domainName, sub string) string {
	if domainName != "" {
		return filepath.Join(fm.GetDomainPath(domainName), sub)
	}
	return filepath.Join(fm.aiRulezDir, sub)
}

// GetFilePath returns the full path for a content file
func (fm *FileManager) GetFilePath(domain, ftype, name string) string {
	var dirPath string

	switch ftype {
	case ContentTypeRules:
		dirPath = fm.GetRulesPath(domain)
	case ContentTypeContext:
		dirPath = fm.GetContextPath(domain)
	case ContentTypeAgents:
		dirPath = fm.GetAgentsPath(domain)
	case ContentTypeCommands:
		dirPath = fm.GetCommandsPath(domain)
	case ContentTypeChecks:
		dirPath = fm.GetChecksPath(domain)
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
	subdirs := []string{ContentTypeRules, ContentTypeContext, ContentTypeSkills, ContentTypeAgents, ContentTypeCommands, ContentTypeChecks}
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
	return fm.PathExists(filePath) && !fm.isGeneratedListing(filePath)
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

// EnsureTrailingNewline ensures the content ends with a single newline
func EnsureTrailingNewline(content string) string {
	content = strings.TrimRight(content, "\r\n")
	if content != "" {
		content += "\n"
	}
	return content
}
