package agents

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/logger"
	"github.com/Goldziher/ai-rulez/internal/utils"
	"gopkg.in/yaml.v3"
)

const (
	categoryRoot    = "root"
	categoryDocs    = "docs"
	categoryPackage = "package"
	categoryApp     = "app"
	categoryReadme  = "readme"

	repoTypeMonorepo    = "monorepo"
	repoTypeSingle      = "single"
	repoTypeLibrary     = "library"
	repoTypeApplication = "application"

	fileReadmeMD = "README.md"
)

type MarkdownFile struct {
	Path         string
	RelativePath string
	Title        string
	Category     string
	Content      string
	Size         int64
}

type DirectoryNode struct {
	Name     string
	Path     string
	IsDir    bool
	Children []*DirectoryNode
}

type GitHistory struct {
	HasGit            bool
	CommitCount       int
	RecentCommits     []string
	CommonPatterns    []string
	CodingConventions []string
}

type ProjectContext struct {
	ProjectName        string
	RootPath           string
	RepoType           string
	Structure          *DirectoryNode
	MarkdownFiles      []MarkdownFile
	CodebaseInfo       *CodebaseInfo
	PackageLocations   []string
	AppLocations       []string
	ExistingConfigs    map[string]string
	GitHistory         *GitHistory
	DirectoryStructure map[string][]string
	AIRulezCommand     string
	ExistingRules      []RuleResponse
	ExistingSections   []SectionResponse
	ExistingAgents     []AgentDefinition
	GenerationPhase    int
	TasksCompleted     []string
}

func GatherProjectContext(projectName string) *ProjectContext {
	rootPath, err := os.Getwd()
	if err != nil {
		rootPath = "."
	}

	logger.Info("📊 Gathering project context...")

	ctx := &ProjectContext{
		ProjectName:     projectName,
		RootPath:        rootPath,
		MarkdownFiles:   []MarkdownFile{},
		ExistingConfigs: make(map[string]string),
		AIRulezCommand:  detectAIRulezCommand(),
	}

	codebaseInfo := AnalyzeCodebase(projectName)
	ctx.CodebaseInfo = &codebaseInfo

	ctx.RepoType = detectRepoType(rootPath)
	logger.Info("  Repository type", "type", ctx.RepoType)

	ctx.MarkdownFiles = scanMarkdownFiles(rootPath)
	logger.Info("  Found markdown files", "count", len(ctx.MarkdownFiles))

	ctx.readExistingAIConfigs(rootPath)
	if len(ctx.ExistingConfigs) > 0 {
		logger.Info("  Found existing documentation/configs (will verify against code)", "count", len(ctx.ExistingConfigs))
	}

	ctx.Structure = mapProjectStructure(rootPath, 3)

	if ctx.RepoType == repoTypeMonorepo {
		ctx.PackageLocations = detectPackageLocations(rootPath)
		ctx.AppLocations = detectAppLocations(rootPath)
		logger.Info("  Monorepo structure", "packages", len(ctx.PackageLocations), "apps", len(ctx.AppLocations))
	}

	ctx.GitHistory = analyzeGitHistory(rootPath)
	if ctx.GitHistory != nil && ctx.GitHistory.HasGit {
		logger.Info("  Git history analyzed", "commits", ctx.GitHistory.CommitCount)
	}

	ctx.DirectoryStructure = buildDirectoryStructureMap(rootPath)

	ctx.GenerationPhase = 1

	return ctx
}

func (ctx *ProjectContext) extractRulesFromConfig(config map[string]interface{}) {
	rulesData, ok := config["rules"].([]interface{})
	if !ok {
		return
	}

	ctx.ExistingRules = make([]RuleResponse, 0, len(rulesData))
	for _, r := range rulesData {
		ruleMap, ok := r.(map[string]interface{})
		if !ok {
			continue
		}

		rule := RuleResponse{}
		if name, ok := ruleMap["name"].(string); ok {
			rule.Name = name
		}
		if priority, ok := ruleMap["priority"].(string); ok {
			rule.Priority = priority
		}
		if content, ok := ruleMap["content"].(string); ok {
			rule.Content = content
		}

		if rule.Name != "" && rule.Content != "" {
			ctx.ExistingRules = append(ctx.ExistingRules, rule)
		}
	}
}

func (ctx *ProjectContext) extractSectionsFromConfig(config map[string]interface{}) {
	sectionsData, ok := config["sections"].([]interface{})
	if !ok {
		return
	}

	ctx.ExistingSections = make([]SectionResponse, 0, len(sectionsData))
	for _, s := range sectionsData {
		sectionMap, ok := s.(map[string]interface{})
		if !ok {
			continue
		}

		section := SectionResponse{}
		if name, ok := sectionMap["name"].(string); ok {
			section.Name = name
		}
		if priority, ok := sectionMap["priority"].(string); ok {
			section.Priority = priority
		}
		if content, ok := sectionMap["content"].(string); ok {
			section.Content = content
		}

		if section.Name != "" && section.Content != "" {
			ctx.ExistingSections = append(ctx.ExistingSections, section)
		}
	}
}

func (ctx *ProjectContext) extractAgentsFromConfig(config map[string]interface{}) {
	agentsData, ok := config["agents"].([]interface{})
	if !ok {
		return
	}

	ctx.ExistingAgents = make([]AgentDefinition, 0, len(agentsData))
	for _, a := range agentsData {
		agentMap, ok := a.(map[string]interface{})
		if !ok {
			continue
		}

		agent := AgentDefinition{}
		if name, ok := agentMap["name"].(string); ok {
			agent.Name = name
		}
		if description, ok := agentMap["description"].(string); ok {
			agent.Description = description
		}

		if agent.Name != "" && agent.Description != "" {
			ctx.ExistingAgents = append(ctx.ExistingAgents, agent)
		}
	}
}

func (ctx *ProjectContext) UpdateContextFromConfig() error {
	configPath := "ai-rulez.yaml"
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to read config file: %w", err)
	}

	var config map[string]interface{}
	if err := yaml.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}

	ctx.extractRulesFromConfig(config)
	ctx.extractSectionsFromConfig(config)
	ctx.extractAgentsFromConfig(config)

	return nil
}

func (ctx *ProjectContext) AddCompletedTask(taskName string) {
	ctx.TasksCompleted = append(ctx.TasksCompleted, taskName)
	if len(ctx.TasksCompleted) > 0 {
		ctx.GenerationPhase = min(4, (len(ctx.TasksCompleted)/2)+1)
	}
}

func detectRepoType(rootPath string) string {
	if isMonorepo(rootPath) {
		return repoTypeMonorepo
	}
	if isLibrary(rootPath) {
		return repoTypeLibrary
	}
	if isApplication(rootPath) {
		return repoTypeApplication
	}
	return repoTypeSingle
}

func isMonorepo(rootPath string) bool {
	monorepoFiles := []string{"lerna.json", "nx.json", "pnpm-workspace.yaml", "rush.json"}
	for _, file := range monorepoFiles {
		if utils.FileExists(filepath.Join(rootPath, file)) {
			return true
		}
	}

	return hasMultiplePackages(rootPath)
}

func hasMultiplePackages(rootPath string) bool {
	packagesDir := filepath.Join(rootPath, "packages")
	if !utils.DirExists(packagesDir) {
		return false
	}

	entries, err := os.ReadDir(packagesDir)
	if err != nil || len(entries) <= 1 {
		return false
	}

	packageCount := 0
	for _, entry := range entries {
		if entry.IsDir() && utils.FileExists(filepath.Join(packagesDir, entry.Name(), "package.json")) {
			packageCount++
		}
	}
	return packageCount > 1
}

func isLibrary(rootPath string) bool {
	if utils.FileExists(filepath.Join(rootPath, "setup.py")) || utils.FileExists(filepath.Join(rootPath, "setup.cfg")) {
		return true
	}

	if utils.FileExists(filepath.Join(rootPath, "package.json")) && checkIfLibrary(rootPath) {
		return true
	}

	if utils.FileExists(filepath.Join(rootPath, "Cargo.toml")) && checkIfRustLibrary(rootPath) {
		return true
	}

	return false
}

func isApplication(rootPath string) bool {
	appIndicators := []string{"src", "app"}
	for _, dir := range appIndicators {
		if utils.DirExists(filepath.Join(rootPath, dir)) {
			return true
		}
	}

	appFiles := []string{"main.go", "main.py"}
	for _, file := range appFiles {
		if utils.FileExists(filepath.Join(rootPath, file)) {
			return true
		}
	}

	return false
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

type markdownScanner struct {
	rootPath string
	files    *[]MarkdownFile
}

func (s *markdownScanner) walkFunc(path string, d fs.DirEntry, err error) error {
	if err != nil {
		return err
	}

	if d.IsDir() {
		return s.handleDirectory(path, d)
	}

	return s.handleFile(path, d)
}

func (s *markdownScanner) handleDirectory(path string, d fs.DirEntry) error {
	if s.shouldSkipDirectory(d.Name()) {
		return filepath.SkipDir
	}

	if s.isTooDeep(path) {
		return filepath.SkipDir
	}

	return nil
}

func (s *markdownScanner) shouldSkipDirectory(name string) bool {
	skipDirs := []string{"node_modules", "vendor", ".git"}
	for _, skip := range skipDirs {
		if name == skip {
			return true
		}
	}
	return strings.HasPrefix(name, ".") && name != ".github" && name != "."
}

func (s *markdownScanner) isTooDeep(path string) bool {
	relPath, err := filepath.Rel(s.rootPath, path)
	if err != nil {
		return true
	}
	depth := len(strings.Split(relPath, string(filepath.Separator)))
	return depth > 4
}

func (s *markdownScanner) handleFile(path string, d fs.DirEntry) error {
	if !s.isMarkdownFile(path) {
		return nil
	}

	info, err := d.Info()
	if err != nil {
		return err
	}

	if info.Size() > 1024*1024 {
		return nil
	}

	s.processMarkdownFile(path, d, info)
	return nil
}

func (s *markdownScanner) isMarkdownFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".md" || ext == ".mdx"
}

func (s *markdownScanner) processMarkdownFile(path string, d fs.DirEntry, info fs.FileInfo) {
	relPath, err := filepath.Rel(s.rootPath, path)
	if err != nil {
		return
	}

	category := categorizeMarkdownFile(relPath, strings.ToLower(d.Name()))

	content, err := os.ReadFile(path)
	if err != nil {
		content = []byte{}
	}
	title := extractMarkdownTitle(string(content), filepath.Base(path))

	*s.files = append(*s.files, MarkdownFile{
		Path:         path,
		RelativePath: relPath,
		Title:        title,
		Category:     category,
		Size:         info.Size(),
	})
}

func scanMarkdownFiles(rootPath string) []MarkdownFile {
	var files []MarkdownFile

	scanner := &markdownScanner{
		rootPath: rootPath,
		files:    &files,
	}

	err := filepath.WalkDir(rootPath, scanner.walkFunc)

	if err != nil {
		logger.Warn("Error scanning markdown files", "error", err)
	}

	sort.Slice(files, func(i, j int) bool {
		if files[i].RelativePath == fileReadmeMD {
			return true
		}
		if files[j].RelativePath == fileReadmeMD {
			return false
		}

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

		return files[i].RelativePath < files[j].RelativePath
	})

	return files
}

func categorizeMarkdownFile(relPath, name string) string {
	if !strings.Contains(relPath, string(filepath.Separator)) {
		return categoryRoot
	}

	if strings.HasPrefix(relPath, "docs"+string(filepath.Separator)) ||
		strings.HasPrefix(relPath, "documentation"+string(filepath.Separator)) {
		return categoryDocs
	}

	if strings.Contains(relPath, "packages"+string(filepath.Separator)) {
		return categoryPackage
	}

	if strings.Contains(relPath, "apps"+string(filepath.Separator)) ||
		strings.Contains(relPath, "applications"+string(filepath.Separator)) {
		return categoryApp
	}

	if name == "readme.md" {
		return categoryReadme
	}

	return categoryDocs
}

func extractMarkdownTitle(content, filename string) string {
	lines := strings.Split(content, "\n")

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# ") {
			return strings.TrimPrefix(trimmed, "# ")
		}
	}

	name := strings.TrimSuffix(filename, filepath.Ext(filename))
	name = strings.ReplaceAll(name, "_", " ")
	name = strings.ReplaceAll(name, "-", " ")
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

	sort.Slice(node.Children, func(i, j int) bool {
		if node.Children[i].IsDir != node.Children[j].IsDir {
			return node.Children[i].IsDir
		}
		return node.Children[i].Name < node.Children[j].Name
	})
}

func detectPackageLocations(rootPath string) []string {
	var locations []string

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

	appDirs := []string{"apps", "applications", "services", "frontend", "backend"}

	for _, dir := range appDirs {
		dirPath := filepath.Join(rootPath, dir)
		if !utils.DirExists(dirPath) {
			continue
		}

		if isAppDirectory(dirPath) {
			locations = append(locations, dir)
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

	if isLast {
		sb.WriteString(prefix + "└── ")
	} else {
		sb.WriteString(prefix + "├── ")
	}

	sb.WriteString(node.Name)
	if node.IsDir {
		sb.WriteString("/")
		if len(node.Children) == 0 {
			sb.WriteString(" (empty)")
		}
	}
	sb.WriteString("\n")

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

	byCategory := make(map[string][]MarkdownFile)
	for _, file := range ctx.MarkdownFiles {
		byCategory[file.Category] = append(byCategory[file.Category], file)
	}

	categoryOrder := []string{categoryRoot, categoryDocs, categoryPackage, categoryApp, categoryReadme}
	for _, cat := range categoryOrder {
		files, exists := byCategory[cat]
		if !exists || len(files) == 0 {
			continue
		}

		catTitle := strings.ToUpper(string(cat[0])) + cat[1:]
		sb.WriteString(fmt.Sprintf("\n%s:\n", catTitle))
		for _, file := range files {
			sb.WriteString(fmt.Sprintf("  - %s: %s\n", file.RelativePath, file.Title))
		}
	}

	return sb.String()
}

func (ctx *ProjectContext) readExistingAIConfigs(rootPath string) {
	configFiles := []struct {
		name string
		path string
	}{
		{"README", "README.md"},
		{"CLAUDE", "CLAUDE.md"},
		{"CLAUDE_CLAUDE", ".claude/CLAUDE.md"},
		{"GEMINI", "GEMINI.md"},
		{"GEMINI_GEMINI", ".gemini/GEMINI.md"},
		{"AMP", "AMP.md"},
		{"CONTINUE", "CONTINUE.md"},
		{"CURSOR", ".cursorrules"},
		{"CURSOR_MDC", ".cursor/rules/.mdc"},
		{"AI_RULEZ", "ai-rulez.yaml"},
		{"AI_RULEZ_OLD", "ai-rulez.yaml"},
	}

	for _, cf := range configFiles {
		filePath := filepath.Join(rootPath, cf.path)
		if utils.FileExists(filePath) {
			content, err := os.ReadFile(filePath)
			if err == nil && len(content) > 0 {
				if len(content) > 10240 {
					content = content[:10240]
				}
				ctx.ExistingConfigs[cf.name] = string(content)
			}
		}
	}
}

func detectAIRulezCommand() string {
	if os.Getenv("UV_PROJECT_ENVIRONMENT") != "" || os.Getenv("VIRTUAL_ENV") != "" {
		return "uvx ai-rulez"
	}

	if os.Getenv("npm_lifecycle_event") != "" || os.Getenv("npm_execpath") != "" {
		return "npx ai-rulez"
	}

	executable := os.Args[0]

	if filepath.IsAbs(executable) {
		if _, err := exec.LookPath("ai-rulez"); err == nil {
			return "ai-rulez"
		}
		return executable
	}

	if strings.Contains(executable, string(filepath.Separator)) {
		if absPath, err := filepath.Abs(executable); err == nil {
			return absPath
		}
		return executable
	}

	return "ai-rulez"
}
