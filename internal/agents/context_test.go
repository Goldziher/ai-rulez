package agents

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectRepoType(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(dir string) error
		expected string
	}{
		{
			name: "monorepo with lerna.json",
			setup: func(dir string) error {
				return os.WriteFile(filepath.Join(dir, "lerna.json"), []byte("{}"), 0o644)
			},
			expected: "monorepo",
		},
		{
			name: "monorepo with pnpm-workspace.yaml",
			setup: func(dir string) error {
				return os.WriteFile(filepath.Join(dir, "pnpm-workspace.yaml"), []byte("packages:\n  - packages/*"), 0o644)
			},
			expected: "monorepo",
		},
		{
			name: "monorepo with multiple packages",
			setup: func(dir string) error {
				packagesDir := filepath.Join(dir, "packages")
				if err := os.MkdirAll(filepath.Join(packagesDir, "pkg1"), 0o755); err != nil {
					return err
				}
				if err := os.MkdirAll(filepath.Join(packagesDir, "pkg2"), 0o755); err != nil {
					return err
				}
				if err := os.WriteFile(filepath.Join(packagesDir, "pkg1", "package.json"), []byte("{}"), 0o644); err != nil {
					return err
				}
				return os.WriteFile(filepath.Join(packagesDir, "pkg2", "package.json"), []byte("{}"), 0o644)
			},
			expected: "monorepo",
		},
		{
			name: "library with setup.py",
			setup: func(dir string) error {
				return os.WriteFile(filepath.Join(dir, "setup.py"), []byte("from setuptools import setup"), 0o644)
			},
			expected: "library",
		},
		{
			name: "library with package.json main field",
			setup: func(dir string) error {
				return os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"main": "index.js"}`), 0o644)
			},
			expected: "library",
		},
		{
			name: "application with src directory",
			setup: func(dir string) error {
				return os.MkdirAll(filepath.Join(dir, "src"), 0o755)
			},
			expected: "application",
		},
		{
			name: "application with main.go",
			setup: func(dir string) error {
				return os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0o644)
			},
			expected: "application",
		},
		{
			name: "single project",
			setup: func(dir string) error {
				return os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Project"), 0o644)
			},
			expected: "single",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()

			if err := tt.setup(tempDir); err != nil {
				t.Fatalf("Failed to setup test: %v", err)
			}

			result := detectRepoType(tempDir)
			if result != tt.expected {
				t.Errorf("detectRepoType() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestCategorizeMarkdownFile(t *testing.T) {
	tests := []struct {
		relPath  string
		name     string
		expected string
	}{
		{"README.md", "readme.md", categoryRoot},
		{"CONTRIBUTING.md", "contributing.md", categoryRoot},
		{filepath.Join("docs", "api.md"), "api.md", categoryDocs},
		{filepath.Join("documentation", "guide.md"), "guide.md", categoryDocs},
		{filepath.Join("packages", "core", "README.md"), "readme.md", categoryPackage},
		{filepath.Join("apps", "web", "README.md"), "readme.md", categoryApp},
		{filepath.Join("src", "readme.md"), "readme.md", categoryReadme},
		{filepath.Join("lib", "something.md"), "something.md", categoryDocs},
	}

	for _, tt := range tests {
		t.Run(tt.relPath, func(t *testing.T) {
			result := categorizeMarkdownFile(tt.relPath, tt.name)
			if result != tt.expected {
				t.Errorf("categorizeMarkdownFile(%q, %q) = %v, want %v",
					tt.relPath, tt.name, result, tt.expected)
			}
		})
	}
}

func TestExtractMarkdownTitle(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		filename string
		expected string
	}{
		{
			name:     "heading with #",
			content:  "# My Project\n\nThis is a description",
			filename: "README.md",
			expected: "My Project",
		},
		{
			name:     "heading with spaces",
			content:  "  # Title with Spaces  \n\nContent",
			filename: "README.md",
			expected: "Title with Spaces",
		},
		{
			name:     "no heading uses filename",
			content:  "This is just text",
			filename: "my-awesome-project.md",
			expected: "My Awesome Project",
		},
		{
			name:     "empty content uses filename",
			content:  "",
			filename: "user_guide.md",
			expected: "User Guide",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractMarkdownTitle(tt.content, tt.filename)
			if result != tt.expected {
				t.Errorf("extractMarkdownTitle() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestScanMarkdownFiles(t *testing.T) {
	tempDir := t.TempDir()

	files := map[string]string{
		"README.md":                          "# Main README",
		"CONTRIBUTING.md":                    "# Contributing",
		"docs/api.md":                        "# API Documentation",
		"packages/core/README.md":            "# Core Package",
		"apps/web/README.md":                 "# Web App",
		".github/ISSUE_TEMPLATE.md":          "# Issue Template",
		"node_modules/pkg/README.md":         "# Should be ignored",
		"some/very/deep/nested/path/file.md": "# Should be ignored (too deep)",
	}

	for path, content := range files {
		fullPath := filepath.Join(tempDir, path)
		dir := filepath.Dir(fullPath)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("Failed to create directory %s: %v", dir, err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
			t.Fatalf("Failed to write file %s: %v", fullPath, err)
		}
	}

	result := scanMarkdownFiles(tempDir)

	expectedCount := 6
	if len(result) != expectedCount {
		t.Errorf("scanMarkdownFiles() found %d files, want %d", len(result), expectedCount)
		for i, file := range result {
			t.Logf("  [%d] %s (category: %s)", i, file.RelativePath, file.Category)
		}
	}

	if len(result) > 0 && result[0].RelativePath != "README.md" {
		t.Errorf("First file should be README.md, got %s", result[0].RelativePath)
	}

	for _, file := range result {
		if strings.Contains(file.RelativePath, "node_modules") {
			t.Errorf("node_modules should be excluded, but found: %s", file.RelativePath)
		}
		if strings.Count(file.RelativePath, string(filepath.Separator)) > 3 {
			t.Errorf("File too deeply nested (>4 separators) should be excluded: %s (has %d)", file.RelativePath, strings.Count(file.RelativePath, string(filepath.Separator)))
		}
	}
}

func TestDetectPackageLocations(t *testing.T) {
	tempDir := t.TempDir()

	packages := []string{
		"packages/core",
		"packages/utils",
		"libs/shared",
		"modules/auth",
	}

	for _, pkg := range packages {
		pkgDir := filepath.Join(tempDir, pkg)
		if err := os.MkdirAll(pkgDir, 0o755); err != nil {
			t.Fatalf("Failed to create directory %s: %v", pkgDir, err)
		}
		if err := os.WriteFile(filepath.Join(pkgDir, "package.json"), []byte("{}"), 0o644); err != nil {
			t.Fatalf("Failed to write package.json: %v", err)
		}
	}

	if err := os.MkdirAll(filepath.Join(tempDir, "packages", "not-a-package"), 0o755); err != nil {
		t.Fatal(err)
	}

	result := detectPackageLocations(tempDir)

	expectedCount := 4
	if len(result) != expectedCount {
		t.Errorf("detectPackageLocations() found %d packages, want %d", len(result), expectedCount)
		t.Errorf("Packages found: %v", result)
	}

	expectedPackages := map[string]bool{
		filepath.Join("packages", "core"):  false,
		filepath.Join("packages", "utils"): false,
		filepath.Join("libs", "shared"):    false,
		filepath.Join("modules", "auth"):   false,
	}

	for _, pkg := range result {
		if _, ok := expectedPackages[pkg]; ok {
			expectedPackages[pkg] = true
		}
	}

	for pkg, found := range expectedPackages {
		if !found {
			t.Errorf("Expected package %s was not detected", pkg)
		}
	}
}

func TestDetectAppLocations(t *testing.T) {
	tempDir := t.TempDir()

	apps := map[string]string{
		"apps/web":     "package.json",
		"apps/mobile":  "main.go",
		"services/api": "app.py",
		"frontend":     "package.json",
		"backend":      "server.js",
	}

	for appPath, mainFile := range apps {
		appDir := filepath.Join(tempDir, appPath)
		if err := os.MkdirAll(appDir, 0o755); err != nil {
			t.Fatalf("Failed to create directory %s: %v", appDir, err)
		}
		if err := os.WriteFile(filepath.Join(appDir, mainFile), []byte("content"), 0o644); err != nil {
			t.Fatalf("Failed to write %s: %v", mainFile, err)
		}
	}

	if err := os.MkdirAll(filepath.Join(tempDir, "apps", "not-an-app"), 0o755); err != nil {
		t.Fatal(err)
	}

	result := detectAppLocations(tempDir)

	if len(result) != 5 {
		t.Errorf("detectAppLocations() found %d apps, want 5. Got: %v", len(result), result)
	}
}

func TestGatherProjectContext(t *testing.T) {
	originalWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(originalWd)

	tempDir := t.TempDir()

	if err := os.MkdirAll(filepath.Join(tempDir, "packages", "core"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(tempDir, "packages", "utils"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(tempDir, "apps", "web"), 0o755); err != nil {
		t.Fatal(err)
	}

	files := map[string]string{
		"README.md":                   "# Test Project",
		"package.json":                `{"name": "test"}`,
		"packages/core/package.json":  `{"name": "core"}`,
		"packages/utils/package.json": `{"name": "utils"}`,
		"apps/web/package.json":       `{"name": "web"}`,
	}

	for path, content := range files {
		fullPath := filepath.Join(tempDir, path)
		dir := filepath.Dir(fullPath)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if err := os.Chdir(tempDir); err != nil {
		t.Fatal(err)
	}

	ctx := GatherProjectContext("TestProject")

	if ctx.ProjectName != "TestProject" {
		t.Errorf("ProjectName = %v, want TestProject", ctx.ProjectName)
	}

	if ctx.RepoType != "monorepo" {
		t.Errorf("RepoType = %v, want monorepo", ctx.RepoType)
	}

	if len(ctx.PackageLocations) != 2 {
		t.Errorf("PackageLocations count = %d, want 2", len(ctx.PackageLocations))
	}

	if len(ctx.AppLocations) != 1 {
		t.Errorf("AppLocations count = %d, want 1", len(ctx.AppLocations))
	}

	if len(ctx.MarkdownFiles) == 0 {
		t.Error("No markdown files found")
	}

	if ctx.Structure == nil {
		t.Error("Structure is nil")
	}
}

func TestGenerateStructureTree(t *testing.T) {
	ctx := &ProjectContext{
		Structure: &DirectoryNode{
			Name:  "project",
			Path:  "/project",
			IsDir: true,
			Children: []*DirectoryNode{
				{
					Name:  "src",
					Path:  "/project/src",
					IsDir: true,
					Children: []*DirectoryNode{
						{
							Name:  "main.go",
							Path:  "/project/src/main.go",
							IsDir: false,
						},
					},
				},
				{
					Name:  "README.md",
					Path:  "/project/README.md",
					IsDir: false,
				},
			},
		},
	}

	tree := ctx.GenerateStructureTree()

	if tree == "" {
		t.Fatal("Tree is empty")
	}

	t.Logf("Generated tree:\n%s", tree)

	expectedLines := []string{
		"├── src/",
		"│   └── main.go",
		"└── README.md",
	}

	for _, expected := range expectedLines {
		if !strings.Contains(tree, expected) {
			t.Errorf("Tree should contain %q", expected)
		}
	}
}

func TestGetDocumentationSummary(t *testing.T) {
	ctx := &ProjectContext{
		MarkdownFiles: []MarkdownFile{
			{RelativePath: "README.md", Title: "Main README", Category: categoryRoot},
			{RelativePath: "docs/api.md", Title: "API Docs", Category: categoryDocs},
			{RelativePath: "packages/core/README.md", Title: "Core Package", Category: categoryPackage},
		},
	}

	summary := ctx.GetDocumentationSummary()

	if !strings.Contains(summary, "Found 3 documentation files") {
		t.Error("Summary should mention file count")
	}

	if !strings.Contains(summary, "Root:") {
		t.Error("Summary should have Root section")
	}

	if !strings.Contains(summary, "Docs:") {
		t.Error("Summary should have Docs section")
	}

	if !strings.Contains(summary, "Package:") {
		t.Error("Summary should have Package section")
	}
}
