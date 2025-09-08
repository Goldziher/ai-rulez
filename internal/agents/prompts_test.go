package agents

import (
	"strings"
	"testing"
)

func TestBuildPhase1Prompt(t *testing.T) {
	context := &ProjectContext{
		ProjectName: "TestProject",
		RepoType:    "monorepo",
		CodebaseInfo: &CodebaseInfo{
			TechStack:    []string{"Go", "TypeScript"},
			MainLanguage: "Go",
			ProjectType:  "application",
			BuildCommand: "go build",
			TestCommand:  "go test",
			LintCommand:  "golangci-lint run",
		},
		PackageLocations: []string{"packages/core", "packages/utils"},
		AppLocations:     []string{"apps/web", "apps/api"},
		MarkdownFiles: []MarkdownFile{
			{RelativePath: "README.md", Title: "Main README", Category: categoryRoot},
		},
		Structure: &DirectoryNode{
			Name:  "test",
			IsDir: true,
		},
	}

	prompt := buildPhase1Prompt(context, []string{})

	// Check that prompt contains key information
	if !strings.Contains(prompt, "TestProject") {
		t.Error("Prompt should contain project name")
	}

	if !strings.Contains(prompt, "monorepo") {
		t.Error("Prompt should mention monorepo")
	}

	if !strings.Contains(prompt, "Go") {
		t.Error("Prompt should mention Go in tech stack")
	}

	if !strings.Contains(prompt, "TypeScript") {
		t.Error("Prompt should mention TypeScript in tech stack")
	}

	if !strings.Contains(prompt, "go build") {
		t.Error("Prompt should contain build command")
	}

	if !strings.Contains(prompt, "packages/core") {
		t.Error("Prompt should list packages")
	}

	if !strings.Contains(prompt, "apps/web") {
		t.Error("Prompt should list applications")
	}
}

func TestBuildPhase2Prompt(t *testing.T) {
	context := &ProjectContext{
		ProjectName: "TestProject",
		RepoType:    "monorepo",
		MarkdownFiles: []MarkdownFile{
			{RelativePath: "README.md", Title: "Main README", Category: categoryRoot},
			{RelativePath: "docs/api.md", Title: "API Docs", Category: categoryDocs},
			{RelativePath: "packages/core/README.md", Title: "Core Package", Category: categoryPackage},
			{RelativePath: "apps/web/README.md", Title: "Web App", Category: categoryApp},
		},
	}

	previousOutputs := []string{"Project analysis from phase 1"}

	prompt := buildPhase2Prompt(context, previousOutputs)

	// Check for documentation references
	if !strings.Contains(prompt, "@README.md") {
		t.Error("Prompt should reference root README")
	}

	if !strings.Contains(prompt, "@docs/api.md") {
		t.Error("Prompt should reference docs")
	}

	if !strings.Contains(prompt, "PACKAGE DOCUMENTATION") {
		t.Error("Prompt should have package documentation section for monorepo")
	}

	if !strings.Contains(prompt, "@packages/core/README.md") {
		t.Error("Prompt should reference package README")
	}

	if !strings.Contains(prompt, "APPLICATION DOCUMENTATION") {
		t.Error("Prompt should have app documentation section for monorepo")
	}

	if !strings.Contains(prompt, "@apps/web/README.md") {
		t.Error("Prompt should reference app README")
	}

	if !strings.Contains(prompt, "Project analysis from phase 1") {
		t.Error("Prompt should include previous phase output")
	}
}

func TestBuildPhase3Prompt(t *testing.T) {
	context := &ProjectContext{
		ProjectName: "TestProject",
		CodebaseInfo: &CodebaseInfo{
			MainLanguage: "Go",
			TechStack:    []string{"Go", "PostgreSQL"},
			BuildCommand: "make build",
			TestCommand:  "make test",
			LintCommand:  "make lint",
		},
	}

	previousOutputs := []string{"Previous analysis"}

	prompt := buildPhase3Prompt(context, previousOutputs)

	if !strings.Contains(prompt, "Go") {
		t.Error("Prompt should mention main language")
	}

	if !strings.Contains(prompt, "PostgreSQL") {
		t.Error("Prompt should mention tech stack")
	}

	if !strings.Contains(prompt, "make build") {
		t.Error("Prompt should include build command")
	}

	if !strings.Contains(prompt, "make test") {
		t.Error("Prompt should include test command")
	}

	if !strings.Contains(prompt, "make lint") {
		t.Error("Prompt should include lint command")
	}
}

func TestBuildPhase4Prompt(t *testing.T) {
	tests := []struct {
		name          string
		context       *ProjectContext
		shouldContain []string
	}{
		{
			name: "monorepo project",
			context: &ProjectContext{
				ProjectName: "TestMonorepo",
				RepoType:    "monorepo",
				CodebaseInfo: &CodebaseInfo{
					TechStack: []string{"TypeScript", "React"},
				},
			},
			shouldContain: []string{
				"monorepo-architect",
				"frontend-engineer",
				"React/Next.js specialist",
			},
		},
		{
			name: "go project with database",
			context: &ProjectContext{
				ProjectName: "GoAPI",
				RepoType:    "application",
				CodebaseInfo: &CodebaseInfo{
					TechStack:   []string{"Go"},
					HasDatabase: true,
				},
			},
			shouldContain: []string{
				"go-specialist",
				"database-specialist",
			},
		},
		{
			name: "python project with docker",
			context: &ProjectContext{
				ProjectName: "PythonService",
				RepoType:    "application",
				CodebaseInfo: &CodebaseInfo{
					TechStack: []string{"Python"},
					HasDocker: true,
				},
			},
			shouldContain: []string{
				"python-specialist",
				"devops-engineer",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prompt := buildPhase4Prompt(tt.context, []string{})

			for _, expected := range tt.shouldContain {
				if !strings.Contains(prompt, expected) {
					t.Errorf("Prompt should contain %q", expected)
				}
			}
		})
	}
}

func TestBuildPhase5Prompt(t *testing.T) {
	tests := []struct {
		name          string
		context       *ProjectContext
		shouldContain []string
	}{
		{
			name: "with commands",
			context: &ProjectContext{
				ProjectName: "TestProject",
				CodebaseInfo: &CodebaseInfo{
					BuildCommand: "npm run build",
					TestCommand:  "npm test",
					LintCommand:  "npm run lint",
				},
			},
			shouldContain: []string{
				"npm run build",
				"npm test",
				"npm run lint",
			},
		},
		{
			name: "with MCP capability",
			context: &ProjectContext{
				ProjectName: "TestProject",
				CodebaseInfo: &CodebaseInfo{
					HasMCP:     true,
					MCPCommand: "uvx",
				},
			},
			shouldContain: []string{
				"MCP CAPABILITY: uvx",
				"uvx ai-rulez",
				"AI-Rulez MCP server",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prompt := buildPhase5Prompt(tt.context, []string{})

			for _, expected := range tt.shouldContain {
				if !strings.Contains(prompt, expected) {
					t.Errorf("Prompt should contain %q", expected)
				}
			}
		})
	}
}

func TestTruncateForContext(t *testing.T) {
	tests := []struct {
		name      string
		text      string
		maxLength int
		expected  string
	}{
		{
			name:      "short text",
			text:      "Short",
			maxLength: 10,
			expected:  "Short",
		},
		{
			name:      "exact length",
			text:      "Exact",
			maxLength: 5,
			expected:  "Exact",
		},
		{
			name:      "needs truncation",
			text:      "This is a very long text that needs truncation",
			maxLength: 10,
			expected:  "This is a ...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncateForContext(tt.text, tt.maxLength)
			if result != tt.expected {
				t.Errorf("truncateForContext() = %q, want %q", result, tt.expected)
			}
		})
	}
}
