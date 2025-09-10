package agents

import (
	"encoding/json"
	"os"
	"os/exec"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/utils"
)

const (
	langTypeScript = "TypeScript"
	langPython     = "Python"
	langRust       = "Rust"
	langJavaScript = "JavaScript"

	repoTypeService = "service"
)

const (
	cmdNpmBuild = "npm run build"
	cmdNpmTest  = "npm test"
	cmdNpmLint  = "npm run lint"
)

type CodebaseInfo struct {
	ProjectName  string   `json:"project_name"`
	TechStack    []string `json:"tech_stack"`
	ProjectType  string   `json:"project_type"`
	BuildCommand string   `json:"build_command"`
	TestCommand  string   `json:"test_command"`
	LintCommand  string   `json:"lint_command"`
	HasDatabase  bool     `json:"has_database"`
	HasDocker    bool     `json:"has_docker"`
	ConfigFiles  []string `json:"config_files"`
	MainLanguage string   `json:"main_language"`
	HasMCP       bool     `json:"has_mcp"`
	MCPCommand   string   `json:"mcp_command"`
}

func AnalyzeCodebase(projectName string) CodebaseInfo {
	info := CodebaseInfo{
		ProjectName: projectName,
		TechStack:   []string{},
		ConfigFiles: []string{},
	}

	detectGoStack(&info)
	detectJSStack(&info)
	detectPythonStack(&info)
	detectRustStack(&info)
	detectCommands(&info)
	detectProjectType(&info)
	detectDatabase(&info)
	detectDocker(&info)
	detectMCPCapability(&info)

	return info
}

func detectGoStack(info *CodebaseInfo) {
	if utils.FileExists("go.mod") {
		info.TechStack = append(info.TechStack, "Go")
		info.ConfigFiles = append(info.ConfigFiles, "go.mod")
		info.MainLanguage = "Go"
	}
}

func detectJSStack(info *CodebaseInfo) {
	if !utils.FileExists("package.json") {
		return
	}

	info.ConfigFiles = append(info.ConfigFiles, "package.json")

	data, err := os.ReadFile("package.json")
	if err != nil {
		return
	}

	var pkg map[string]interface{}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return
	}

	processJSDependencies(info, pkg)
}

func processJSDependencies(info *CodebaseInfo, pkg map[string]interface{}) {
	if devDeps, exists := pkg["devDependencies"]; exists {
		if deps, ok := devDeps.(map[string]interface{}); ok {
			if _, hasTS := deps["typescript"]; hasTS {
				info.TechStack = append(info.TechStack, langTypeScript)
				if info.MainLanguage == "" {
					info.MainLanguage = langTypeScript
				}
			}
		}
	}

	if dependencies, exists := pkg["dependencies"]; exists {
		if deps, ok := dependencies.(map[string]interface{}); ok {
			if _, hasReact := deps["react"]; hasReact {
				info.TechStack = append(info.TechStack, "React")
			}
			if _, hasVue := deps["vue"]; hasVue {
				info.TechStack = append(info.TechStack, "Vue")
			}
			if _, hasAngular := deps["@angular/core"]; hasAngular {
				info.TechStack = append(info.TechStack, "Angular")
			}
		}
	}

	if info.MainLanguage == "" {
		info.TechStack = append(info.TechStack, langJavaScript)
		info.MainLanguage = langJavaScript
	}
}

func detectPythonStack(info *CodebaseInfo) {
	if utils.FileExists("pyproject.toml") || utils.FileExists("requirements.txt") || utils.FileExists("setup.py") {
		info.TechStack = append(info.TechStack, langPython)
		if info.MainLanguage == "" {
			info.MainLanguage = langPython
		}
		if utils.FileExists("pyproject.toml") {
			info.ConfigFiles = append(info.ConfigFiles, "pyproject.toml")
		}
		if utils.FileExists("requirements.txt") {
			info.ConfigFiles = append(info.ConfigFiles, "requirements.txt")
		}
	}
}

func detectRustStack(info *CodebaseInfo) {
	if utils.FileExists("Cargo.toml") {
		info.TechStack = append(info.TechStack, langRust)
		info.ConfigFiles = append(info.ConfigFiles, "Cargo.toml")
		if info.MainLanguage == "" {
			info.MainLanguage = langRust
		}
	}
}

func detectDatabase(info *CodebaseInfo) {
	info.HasDatabase = utils.FileExists("docker-compose.yml") ||
		utils.FileExists("docker-compose.yaml") ||
		utils.DirExists("migrations") ||
		utils.FileExists("prisma/schema.prisma") ||
		utils.FileExists("schema.sql") ||
		utils.DirExists("db")
}

func detectDocker(info *CodebaseInfo) {
	info.HasDocker = utils.FileExists("Dockerfile") ||
		utils.FileExists("docker-compose.yml") ||
		utils.FileExists("docker-compose.yaml")
}

func detectCommands(info *CodebaseInfo) {
	if detectTaskfileCommands(info) {
		return
	}

	if detectPackageJSONCommands(info) {
		return
	}

	if detectMakefileCommands(info) {
		return
	}

	detectLanguageSpecificCommands(info)
}

func detectTaskfileCommands(info *CodebaseInfo) bool {
	if utils.FileExists("Taskfile.yml") || utils.FileExists("Taskfile.yaml") {
		info.BuildCommand = "task build"
		info.TestCommand = "task test"
		info.LintCommand = "task lint"
		info.ConfigFiles = append(info.ConfigFiles, "Taskfile.yml")
		return true
	}
	return false
}

func detectPackageJSONCommands(info *CodebaseInfo) bool {
	if !utils.FileExists("package.json") {
		return false
	}

	data, err := os.ReadFile("package.json")
	if err != nil {
		return false
	}

	var pkg map[string]interface{}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return false
	}

	scriptsRaw, exists := pkg["scripts"]
	if !exists {
		return false
	}

	scripts, ok := scriptsRaw.(map[string]interface{})
	if !ok {
		return false
	}

	if _, hasBuild := scripts["build"]; hasBuild {
		info.BuildCommand = cmdNpmBuild
	}

	if _, hasTest := scripts["test"]; hasTest {
		info.TestCommand = cmdNpmTest
	}

	if _, hasLint := scripts["lint"]; hasLint {
		info.LintCommand = cmdNpmLint
	} else if _, hasESLint := scripts["eslint"]; hasESLint {
		info.LintCommand = "npm run eslint"
	}

	return true
}

func detectMakefileCommands(info *CodebaseInfo) bool {
	if utils.FileExists("Makefile") {
		info.BuildCommand = "make build"
		info.TestCommand = "make test"
		info.LintCommand = "make lint"
		info.ConfigFiles = append(info.ConfigFiles, "Makefile")
		return true
	}
	return false
}

func detectLanguageSpecificCommands(info *CodebaseInfo) {
	switch info.MainLanguage {
	case "Go":
		info.BuildCommand = "go build"
		info.TestCommand = "go test ./..."
		info.LintCommand = "golangci-lint run"
	case langPython:
		detectPythonCommands(info)
	case langRust:
		info.BuildCommand = "cargo build"
		info.TestCommand = "cargo test"
		info.LintCommand = "cargo clippy"
	case langJavaScript, langTypeScript:
		setDefaultNPMCommands(info)
	}
}

func detectPythonCommands(info *CodebaseInfo) {
	if utils.FileExists("pyproject.toml") {
		info.BuildCommand = "poetry build"
		info.TestCommand = "poetry run pytest"
		info.LintCommand = "poetry run flake8"
	} else {
		info.BuildCommand = "python setup.py build"
		info.TestCommand = "pytest"
		info.LintCommand = "flake8"
	}
}

func setDefaultNPMCommands(info *CodebaseInfo) {
	if info.BuildCommand == "" {
		info.BuildCommand = cmdNpmBuild
	}
	if info.TestCommand == "" {
		info.TestCommand = cmdNpmTest
	}
	if info.LintCommand == "" {
		info.LintCommand = cmdNpmLint
	}
}

func detectProjectType(info *CodebaseInfo) {
	if utils.FileExists("lerna.json") || utils.FileExists("nx.json") || utils.FileExists("pnpm-workspace.yaml") {
		info.ProjectType = repoTypeMonorepo
		return
	}

	if utils.FileExists("setup.py") || utils.FileExists("Cargo.toml") ||
		(utils.FileExists("package.json") && strings.Contains(info.ProjectName, "lib")) {
		info.ProjectType = repoTypeLibrary
		return
	}

	if utils.DirExists("api") || utils.DirExists("src/api") || utils.DirExists("routes") {
		info.ProjectType = repoTypeService
		return
	}

	info.ProjectType = repoTypeApplication
}

func detectMCPCapability(info *CodebaseInfo) {
	if _, err := exec.LookPath("uvx"); err == nil {
		info.HasMCP = true
		info.MCPCommand = "uvx"
		return
	}

	if _, err := exec.LookPath("npx"); err == nil {
		info.HasMCP = true
		info.MCPCommand = "npx"
		return
	}

	if _, err := exec.LookPath("go"); err == nil {
		info.HasMCP = true
		info.MCPCommand = "go run"
		return
	}
}
