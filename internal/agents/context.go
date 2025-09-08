package agents

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/logger"
	"github.com/Goldziher/ai-rulez/internal/utils"
)

const (
	categoryRoot    = "root"
	categoryDocs    = "docs"
	categoryPackage = "package"
	categoryApp     = "app"
	categoryReadme  = "readme"

	// Repository types
	repoTypeMonorepo    = "monorepo"
	repoTypeSingle      = "single"
	repoTypeLibrary     = "library"
	repoTypeApplication = "application"

	// Common file names
	fileReadmeMD = "README.md"
)

type MarkdownFile struct {
	Path         string
	RelativePath string
	Title        string
	Category     string // "readme", "docs", "package", "app", "root"
	Content      string
	Size         int64
}

type DirectoryNode struct {
	Name     string
	Path     string
	IsDir    bool
	Children []*DirectoryNode
}

type ProjectContext struct {
	ProjectName      string
	RootPath         string
	RepoType         string // "monorepo", "single", "library", "application"
	Structure        *DirectoryNode
	MarkdownFiles    []MarkdownFile
	CodebaseInfo     *CodebaseInfo
	PackageLocations []string // For monorepos
	AppLocations     []string // For monorepos with apps
}

func GatherProjectContext(projectName string) *ProjectContext {
	rootPath, err := os.Getwd()
	if err != nil {
		rootPath = "."
	}

	logger.Info("📊 Gathering project context...")

	ctx := &ProjectContext{
		ProjectName:   projectName,
		RootPath:      rootPath,
		MarkdownFiles: []MarkdownFile{},
	}

	// Analyze codebase first
	codebaseInfo := AnalyzeCodebase(projectName)
	ctx.CodebaseInfo = &codebaseInfo

	// Detect repository type
	ctx.RepoType = detectRepoType(rootPath)
	logger.Info("  Repository type", "type", ctx.RepoType)

	// Scan for markdown files
	ctx.MarkdownFiles = scanMarkdownFiles(rootPath)
	logger.Info("  Found markdown files", "count", len(ctx.MarkdownFiles))

	// Map project structure
	ctx.Structure = mapProjectStructure(rootPath, 3)

	// Detect package and app locations for monorepos
	if ctx.RepoType == repoTypeMonorepo {
		ctx.PackageLocations = detectPackageLocations(rootPath)
		ctx.AppLocations = detectAppLocations(rootPath)
		logger.Info("  Monorepo structure", "packages", len(ctx.PackageLocations), "apps", len(ctx.AppLocations))
	}

	return ctx
}

func detectRepoType(rootPath string) string {
	// Check for monorepo indicators
	if utils.FileExists(filepath.Join(rootPath, "lerna.json")) ||
		utils.FileExists(filepath.Join(rootPath, "nx.json")) ||
		utils.FileExists(filepath.Join(rootPath, "pnpm-workspace.yaml")) ||
		utils.FileExists(filepath.Join(rootPath, "rush.json")) {
		return repoTypeMonorepo
	}

	// Check for packages directory with multiple packages
	packagesDir := filepath.Join(rootPath, "packages")
	if utils.DirExists(packagesDir) {
		entries, err := os.ReadDir(packagesDir)
		if err == nil && len(entries) > 1 {
			packageCount := 0
			for _, entry := range entries {
				if entry.IsDir() && utils.FileExists(filepath.Join(packagesDir, entry.Name(), "package.json")) {
					packageCount++
				}
			}
			if packageCount > 1 {
				return repoTypeMonorepo
			}
		}
	}

	// Check if it's a library
	if utils.FileExists(filepath.Join(rootPath, "setup.py")) ||
		utils.FileExists(filepath.Join(rootPath, "setup.cfg")) ||
		(utils.FileExists(filepath.Join(rootPath, "package.json")) && checkIfLibrary(rootPath)) ||
		utils.FileExists(filepath.Join(rootPath, "Cargo.toml")) && checkIfRustLibrary(rootPath) {
		return repoTypeLibrary
	}

	// Check if it's an application
	if utils.DirExists(filepath.Join(rootPath, "src")) ||
		utils.DirExists(filepath.Join(rootPath, "app")) ||
		utils.FileExists(filepath.Join(rootPath, "main.go")) ||
		utils.FileExists(filepath.Join(rootPath, "main.py")) {
		return repoTypeApplication
	}

	return repoTypeSingle
}

func checkIfLibrary(rootPath string) bool {
	data, err := os.ReadFile(filepath.Join(rootPath, "package.json"))
	if err != nil {
		return false
	}
	content := string(data)
	return strings.Contains(content, `"main":`) ||
		strings.Contains(content, `"module":`) ||
		strings.Contains(content, `"exports":`)
}

func checkIfRustLibrary(rootPath string) bool {
	data, err := os.ReadFile(filepath.Join(rootPath, "Cargo.toml"))
	if err != nil {
		return false
	}
	content := string(data)
	return strings.Contains(content, "[lib]")
}

func scanMarkdownFiles(rootPath string) []MarkdownFile {
	var files []MarkdownFile

	// Just scan from root - WalkDir will traverse subdirectories
	err := filepath.WalkDir(rootPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// Skip files/directories that can't be accessed
			return nil //nolint:nilerr
		}

		// Skip node_modules, vendor, and hidden directories (except .github)
		if d.IsDir() {
			name := d.Name()
			if name == "node_modules" || name == "vendor" || name == ".git" ||
				(strings.HasPrefix(name, ".") && name != ".github" && name != ".") {
				return filepath.SkipDir
			}
			// Skip deeply nested directories
			relPath, relErr := filepath.Rel(rootPath, path)
			if relErr != nil {
				return filepath.SkipDir
			}
			depth := len(strings.Split(relPath, string(filepath.Separator)))
			if depth > 4 {
				return filepath.SkipDir
			}
			return nil
		}

		// Check for markdown files
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".md" && ext != ".mdx" {
			return nil
		}

		// Read file info
		info, err := d.Info()
		if err != nil {
			// Skip files that can't be stat'd
			return nil //nolint:nilerr
		}

		// Skip very large files (> 1MB)
		if info.Size() > 1024*1024 {
			return nil
		}

		relPath, err := filepath.Rel(rootPath, path)
		if err != nil {
			// Skip files with path errors
			return nil //nolint:nilerr
		}
		name := strings.ToLower(d.Name())

		// Categorize the file
		category := categorizeMarkdownFile(relPath, name)

		// Read first 500 bytes to extract title
		content, err := os.ReadFile(path)
		if err != nil {
			content = []byte{}
		}
		title := extractMarkdownTitle(string(content), filepath.Base(path))

		files = append(files, MarkdownFile{
			Path:         path,
			RelativePath: relPath,
			Title:        title,
			Category:     category,
			Size:         info.Size(),
		})

		return nil
	})

	if err != nil {
		logger.Warn("Error scanning markdown files", "error", err)
	}

	// Sort by importance: root README first, then docs, then packages/apps
	sort.Slice(files, func(i, j int) bool {
		// Root README always first
		if files[i].RelativePath == fileReadmeMD {
			return true
		}
		if files[j].RelativePath == fileReadmeMD {
			return false
		}

		// Then by category priority
		categoryPriority := map[string]int{
			categoryRoot:    1,
			categoryDocs:    2,
			categoryPackage: 3,
			categoryApp:     4,
			categoryReadme:  5,
		}

		iPriority := categoryPriority[files[i].Category]
		jPriority := categoryPriority[files[j].Category]

		if iPriority != jPriority {
			return iPriority < jPriority
		}

		// Then alphabetically
		return files[i].RelativePath < files[j].RelativePath
	})

	return files
}

func categorizeMarkdownFile(relPath, name string) string {
	// Root level files
	if !strings.Contains(relPath, string(filepath.Separator)) {
		return categoryRoot
	}

	// Documentation directory
	if strings.HasPrefix(relPath, "docs"+string(filepath.Separator)) ||
		strings.HasPrefix(relPath, "documentation"+string(filepath.Separator)) {
		return categoryDocs
	}

	// Package documentation
	if strings.Contains(relPath, "packages"+string(filepath.Separator)) {
		return categoryPackage
	}

	// App documentation
	if strings.Contains(relPath, "apps"+string(filepath.Separator)) ||
		strings.Contains(relPath, "applications"+string(filepath.Separator)) {
		return categoryApp
	}

	// Any README file
	if name == "readme.md" {
		return categoryReadme
	}

	return categoryDocs
}

func extractMarkdownTitle(content, filename string) string {
	lines := strings.Split(content, "\n")

	// Look for first # heading
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# ") {
			return strings.TrimPrefix(trimmed, "# ")
		}
	}

	// Fallback to filename without extension
	name := strings.TrimSuffix(filename, filepath.Ext(filename))
	// Convert snake_case or kebab-case to Title Case
	name = strings.ReplaceAll(name, "_", " ")
	name = strings.ReplaceAll(name, "-", " ")
	// Simple title case implementation
	words := strings.Fields(strings.ToLower(name))
	for i, word := range words {
		if word != "" {
			words[i] = strings.ToUpper(string(word[0])) + word[1:]
		}
	}
	name = strings.Join(words, " ")

	return name
}

func mapProjectStructure(rootPath string, maxDepth int) *DirectoryNode {
	root := &DirectoryNode{
		Name:  filepath.Base(rootPath),
		Path:  rootPath,
		IsDir: true,
	}

	mapDirectory(root, rootPath, 0, maxDepth)
	return root
}

func mapDirectory(node *DirectoryNode, path string, currentDepth, maxDepth int) {
	if currentDepth >= maxDepth {
		return
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return
	}

	for _, entry := range entries {
		name := entry.Name()

		// Skip hidden files and common non-source directories
		if strings.HasPrefix(name, ".") && name != ".github" {
			continue
		}
		if name == "node_modules" || name == "vendor" || name == "dist" ||
			name == "build" || name == "coverage" || name == "__pycache__" {
			continue
		}

		childPath := filepath.Join(path, name)
		child := &DirectoryNode{
			Name:  name,
			Path:  childPath,
			IsDir: entry.IsDir(),
		}

		if entry.IsDir() {
			mapDirectory(child, childPath, currentDepth+1, maxDepth)
		}

		node.Children = append(node.Children, child)
	}

	// Sort children: directories first, then files
	sort.Slice(node.Children, func(i, j int) bool {
		if node.Children[i].IsDir != node.Children[j].IsDir {
			return node.Children[i].IsDir
		}
		return node.Children[i].Name < node.Children[j].Name
	})
}

func detectPackageLocations(rootPath string) []string {
	var locations []string

	// Common package directories
	packageDirs := []string{"packages", "libs", "modules"}

	for _, dir := range packageDirs {
		dirPath := filepath.Join(rootPath, dir)
		if !utils.DirExists(dirPath) {
			continue
		}

		entries, err := os.ReadDir(dirPath)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}

			packagePath := filepath.Join(dir, entry.Name())
			fullPath := filepath.Join(rootPath, packagePath)

			// Check if it's a valid package
			if utils.FileExists(filepath.Join(fullPath, "package.json")) ||
				utils.FileExists(filepath.Join(fullPath, "setup.py")) ||
				utils.FileExists(filepath.Join(fullPath, "Cargo.toml")) ||
				utils.FileExists(filepath.Join(fullPath, "go.mod")) {
				locations = append(locations, packagePath)
			}
		}
	}

	return locations
}

func detectAppLocations(rootPath string) []string {
	var locations []string

	// Common app directories
	appDirs := []string{"apps", "applications", "services", "frontend", "backend"}

	for _, dir := range appDirs {
		dirPath := filepath.Join(rootPath, dir)
		if !utils.DirExists(dirPath) {
			continue
		}

		// Check if the directory itself is an app
		if isAppDirectory(dirPath) {
			locations = append(locations, dir)
			continue
		}

		// Check subdirectories
		entries, err := os.ReadDir(dirPath)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}

			appPath := filepath.Join(dir, entry.Name())
			fullPath := filepath.Join(rootPath, appPath)

			if isAppDirectory(fullPath) {
				locations = append(locations, appPath)
			}
		}
	}

	return locations
}

func isAppDirectory(path string) bool {
	// Check for app indicators
	return utils.FileExists(filepath.Join(path, "package.json")) ||
		utils.FileExists(filepath.Join(path, "main.go")) ||
		utils.FileExists(filepath.Join(path, "main.py")) ||
		utils.FileExists(filepath.Join(path, "app.py")) ||
		utils.FileExists(filepath.Join(path, "server.js")) ||
		utils.FileExists(filepath.Join(path, "index.js")) ||
		utils.DirExists(filepath.Join(path, "src")) ||
		utils.DirExists(filepath.Join(path, "app"))
}

func (ctx *ProjectContext) GenerateStructureTree() string {
	var sb strings.Builder
	if ctx.Structure != nil && ctx.Structure.Children != nil {
		for i, child := range ctx.Structure.Children {
			generateTreeNode(&sb, child, "", i == len(ctx.Structure.Children)-1)
		}
	}
	return sb.String()
}

func generateTreeNode(sb *strings.Builder, node *DirectoryNode, prefix string, isLast bool) {
	if node == nil {
		return
	}

	// Draw tree lines
	if isLast {
		sb.WriteString(prefix + "└── ")
	} else {
		sb.WriteString(prefix + "├── ")
	}

	// Write node name
	sb.WriteString(node.Name)
	if node.IsDir {
		sb.WriteString("/")
		if len(node.Children) == 0 {
			sb.WriteString(" (empty)")
		}
	}
	sb.WriteString("\n")

	// Process children
	if node.IsDir && len(node.Children) > 0 {
		newPrefix := prefix
		if isLast {
			newPrefix += "    "
		} else {
			newPrefix += "│   "
		}

		for i, child := range node.Children {
			generateTreeNode(sb, child, newPrefix, i == len(node.Children)-1)
		}
	}
}

func (ctx *ProjectContext) GetDocumentationSummary() string {
	var sb strings.Builder

	if len(ctx.MarkdownFiles) == 0 {
		return "No documentation files found"
	}

	sb.WriteString(fmt.Sprintf("Found %d documentation files:\n", len(ctx.MarkdownFiles)))

	// Group by category
	byCategory := make(map[string][]MarkdownFile)
	for _, file := range ctx.MarkdownFiles {
		byCategory[file.Category] = append(byCategory[file.Category], file)
	}

	// Display by category
	categoryOrder := []string{categoryRoot, categoryDocs, categoryPackage, categoryApp, categoryReadme}
	for _, cat := range categoryOrder {
		files, exists := byCategory[cat]
		if !exists || len(files) == 0 {
			continue
		}

		// Capitalize category name
		catTitle := strings.ToUpper(string(cat[0])) + cat[1:]
		sb.WriteString(fmt.Sprintf("\n%s:\n", catTitle))
		for _, file := range files {
			sb.WriteString(fmt.Sprintf("  - %s: %s\n", file.RelativePath, file.Title))
		}
	}

	return sb.String()
}
