package includes

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
)

// Test helper to create temporary directories with .ai-rulez structure
func createTestAIRulezDir(t *testing.T, base string) string {
	t.Helper()

	// Create main .ai-rulez directory
	aiRulezDir := filepath.Join(base, ".ai-rulez")
	if err := os.MkdirAll(aiRulezDir, 0755); err != nil {
		t.Fatalf("failed to create .ai-rulez directory: %v", err)
	}

	// Create rules directory with sample content
	rulesDir := filepath.Join(aiRulezDir, "rules")
	if err := os.MkdirAll(rulesDir, 0755); err != nil {
		t.Fatalf("failed to create rules directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(rulesDir, "test_rule.md"), []byte("# Test Rule\n\nContent"), 0644); err != nil {
		t.Fatalf("failed to write rule file: %v", err)
	}

	// Create context directory with sample content
	contextDir := filepath.Join(aiRulezDir, "context")
	if err := os.MkdirAll(contextDir, 0755); err != nil {
		t.Fatalf("failed to create context directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(contextDir, "test_context.md"), []byte("# Test Context\n\nContent"), 0644); err != nil {
		t.Fatalf("failed to write context file: %v", err)
	}

	// Create skills directory with sample skill
	skillsDir := filepath.Join(aiRulezDir, "skills", "test_skill")
	if err := os.MkdirAll(skillsDir, 0755); err != nil {
		t.Fatalf("failed to create skills directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillsDir, "SKILL.md"), []byte("# Test Skill\n\nContent"), 0644); err != nil {
		t.Fatalf("failed to write skill file: %v", err)
	}

	return aiRulezDir
}

func TestNewLocalSource(t *testing.T) {
	// Arrange
	name := "test-source"
	path := "/test/path"
	baseDir := "/test/base"
	include := []string{"rules", "context"}

	// Act
	source := NewLocalSource(name, path, baseDir, include)

	// Assert
	if source.name != name {
		t.Errorf("expected name %q, got %q", name, source.name)
	}
	if source.path != path {
		t.Errorf("expected path %q, got %q", path, source.path)
	}
	if source.baseDir != baseDir {
		t.Errorf("expected baseDir %q, got %q", baseDir, source.baseDir)
	}
	if len(source.include) != 2 {
		t.Errorf("expected 2 include items, got %d", len(source.include))
	}
}

func TestLocalSourceGetType(t *testing.T) {
	// Arrange
	source := NewLocalSource("test", "/path", "/base", nil)

	// Act
	sourceType := source.GetType()

	// Assert
	if sourceType != SourceTypeLocal {
		t.Errorf("expected SourceTypeLocal, got %v", sourceType)
	}
}

func TestLocalSourceGetName(t *testing.T) {
	// Arrange
	name := "my-source"
	source := NewLocalSource(name, "/path", "/base", nil)

	// Act
	result := source.GetName()

	// Assert
	if result != name {
		t.Errorf("expected %q, got %q", name, result)
	}
}

func TestResolvePathAbsolute(t *testing.T) {
	// Arrange
	// Use a real temporary directory for an actual absolute path
	tmpDir := t.TempDir()
	source := NewLocalSource("test", tmpDir, "/base", nil)

	// Act
	resolved, err := source.resolvePath()

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved != tmpDir {
		t.Errorf("expected %q, got %q", tmpDir, resolved)
	}
}

func TestResolvePathRelative(t *testing.T) {
	// Arrange
	baseDir := "/test/base"
	relativePath := "relative/path"
	source := NewLocalSource("test", relativePath, baseDir, nil)
	expected := filepath.Join(baseDir, relativePath)

	// Act
	resolved, err := source.resolvePath()

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved != expected {
		t.Errorf("expected %q, got %q", expected, resolved)
	}
}

func TestResolvePathCleans(t *testing.T) {
	// Arrange
	baseDir := "/test/base"
	relativePath := "./relative/../actual"
	source := NewLocalSource("test", relativePath, baseDir, nil)
	expected := filepath.Join(baseDir, "actual")

	// Act
	resolved, err := source.resolvePath()

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved != expected {
		t.Errorf("expected %q, got %q", expected, resolved)
	}
}

func TestValidatePathNonExistent(t *testing.T) {
	// Arrange
	source := NewLocalSource("test", "/nonexistent/path", "/base", nil)

	// Act
	err := source.validatePath("/nonexistent/path")

	// Assert
	if err == nil {
		t.Error("expected error for non-existent path")
	}
}

func TestValidatePathNotDirectory(t *testing.T) {
	// Arrange
	tmpFile, err := os.CreateTemp("", "test-file")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	source := NewLocalSource("test", tmpFile.Name(), "/base", nil)

	// Act
	err = source.validatePath(tmpFile.Name())

	// Assert
	if err == nil {
		t.Error("expected error for non-directory path")
	}
}

func TestValidatePathDirectory(t *testing.T) {
	// Arrange
	tmpDir := t.TempDir()
	source := NewLocalSource("test", tmpDir, "/base", nil)

	// Act
	err := source.validatePath(tmpDir)

	// Assert
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestFindAIRulesDirInPath(t *testing.T) {
	// Arrange
	tmpDir := t.TempDir()
	createTestAIRulezDir(t, tmpDir)
	source := NewLocalSource("test", tmpDir, "/base", nil)

	// Act
	found := source.findAIRulezDir(tmpDir)

	// Assert
	expected := filepath.Join(tmpDir, ".ai-rulez")
	if found != expected {
		t.Errorf("expected %q, got %q", expected, found)
	}
}

func TestFindAIRulesDirWhenIsAIRulez(t *testing.T) {
	// Arrange
	tmpDir := t.TempDir()
	aiRulezPath := createTestAIRulezDir(t, tmpDir)
	source := NewLocalSource("test", aiRulezPath, "/base", nil)

	// Act
	found := source.findAIRulezDir(aiRulezPath)

	// Assert
	if found != aiRulezPath {
		t.Errorf("expected %q, got %q", aiRulezPath, found)
	}
}

func TestFindAIRulesDirNotFound(t *testing.T) {
	// Arrange
	tmpDir := t.TempDir()
	// Don't create .ai-rulez directory
	source := NewLocalSource("test", tmpDir, "/base", nil)

	// Act
	found := source.findAIRulezDir(tmpDir)

	// Assert
	if found != "" {
		t.Errorf("expected empty string, got %q", found)
	}
}

func TestLocalSourceFilterContentIncludesRules(t *testing.T) {
	// Arrange
	tree := &config.ContentTreeV3{
		Rules: []config.ContentFile{
			{Name: "rule1", Content: "rule content"},
		},
		Context: []config.ContentFile{
			{Name: "context1", Content: "context content"},
		},
		Skills: []config.ContentFile{
			{Name: "skill1", Content: "skill content"},
		},
		Domains: make(map[string]*config.DomainV3),
	}
	source := NewLocalSource("test", "/path", "/base", []string{"rules"})

	// Act
	filtered := source.filterContent(tree)

	// Assert
	if len(filtered.Rules) != 1 {
		t.Errorf("expected 1 rule, got %d", len(filtered.Rules))
	}
	if len(filtered.Context) != 0 {
		t.Errorf("expected 0 context, got %d", len(filtered.Context))
	}
	if len(filtered.Skills) != 0 {
		t.Errorf("expected 0 skills, got %d", len(filtered.Skills))
	}
}

func TestLocalSourceFilterContentIncludesMultiple(t *testing.T) {
	// Arrange
	tree := &config.ContentTreeV3{
		Rules: []config.ContentFile{
			{Name: "rule1", Content: "rule content"},
		},
		Context: []config.ContentFile{
			{Name: "context1", Content: "context content"},
		},
		Skills: []config.ContentFile{
			{Name: "skill1", Content: "skill content"},
		},
		Domains: make(map[string]*config.DomainV3),
	}
	source := NewLocalSource("test", "/path", "/base", []string{"rules", "context"})

	// Act
	filtered := source.filterContent(tree)

	// Assert
	if len(filtered.Rules) != 1 {
		t.Errorf("expected 1 rule, got %d", len(filtered.Rules))
	}
	if len(filtered.Context) != 1 {
		t.Errorf("expected 1 context, got %d", len(filtered.Context))
	}
	if len(filtered.Skills) != 0 {
		t.Errorf("expected 0 skills, got %d", len(filtered.Skills))
	}
}

func TestLocalSourceFilterContentEmptyInclude(t *testing.T) {
	// Arrange
	tree := &config.ContentTreeV3{
		Rules: []config.ContentFile{
			{Name: "rule1", Content: "rule content"},
		},
		Context: []config.ContentFile{
			{Name: "context1", Content: "context content"},
		},
		Skills: []config.ContentFile{
			{Name: "skill1", Content: "skill content"},
		},
		Domains: make(map[string]*config.DomainV3),
	}
	source := NewLocalSource("test", "/path", "/base", []string{})

	// Act
	filtered := source.filterContent(tree)

	// Assert
	if len(filtered.Rules) != 0 {
		t.Errorf("expected 0 rules, got %d", len(filtered.Rules))
	}
	if len(filtered.Context) != 0 {
		t.Errorf("expected 0 context, got %d", len(filtered.Context))
	}
	if len(filtered.Skills) != 0 {
		t.Errorf("expected 0 skills, got %d", len(filtered.Skills))
	}
}

func TestLocalSourceFilterContentPreserveDomains(t *testing.T) {
	// Arrange
	domain := &config.DomainV3{
		Name:  "backend",
		Rules: []config.ContentFile{{Name: "domain-rule"}},
	}
	tree := &config.ContentTreeV3{
		Rules:   []config.ContentFile{{Name: "rule1"}},
		Context: []config.ContentFile{{Name: "context1"}},
		Domains: map[string]*config.DomainV3{
			"backend": domain,
		},
	}
	source := NewLocalSource("test", "/path", "/base", []string{"rules"})

	// Act
	filtered := source.filterContent(tree)

	// Assert
	if len(filtered.Domains) != 1 {
		t.Errorf("expected 1 domain, got %d", len(filtered.Domains))
	}
	if filtered.Domains["backend"] != domain {
		t.Error("expected domain to be preserved")
	}
}

func TestFetchInvalidPath(t *testing.T) {
	// Arrange
	source := NewLocalSource("test", "/nonexistent/path", "/base", nil)
	ctx := context.Background()

	// Act
	_, err := source.Fetch(ctx)

	// Assert
	if err == nil {
		t.Error("expected error for invalid path")
	}
}

func TestFetchNoAIRulezDir(t *testing.T) {
	// Arrange - bare structure support means empty dir is now valid
	tmpDir := t.TempDir()
	// Don't create .ai-rulez directory or any content - empty bare structure
	source := NewLocalSource("test", tmpDir, "/base", nil)
	ctx := context.Background()

	// Act
	contentTree, err := source.Fetch(ctx)

	// Assert - should succeed with empty content (bare structure with no content)
	if err != nil {
		t.Errorf("unexpected error with bare structure: %v", err)
	}
	if contentTree == nil {
		t.Error("expected non-nil ContentTreeV3 for bare structure")
	}
	// Empty bare structure is valid, just has no content
}

func TestFetchSuccess(t *testing.T) {
	// Arrange
	tmpDir := t.TempDir()
	createTestAIRulezDir(t, tmpDir)
	source := NewLocalSource("test", tmpDir, tmpDir, nil)
	ctx := context.Background()

	// Act
	contentTree, err := source.Fetch(ctx)

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if contentTree == nil {
		t.Fatalf("expected non-nil ContentTreeV3")
	}
	if len(contentTree.Rules) == 0 {
		t.Error("expected rules to be loaded")
	}
	if len(contentTree.Context) == 0 {
		t.Error("expected context to be loaded")
	}
	if len(contentTree.Skills) == 0 {
		t.Error("expected skills to be loaded")
	}
}

func TestFetchWithRelativePath(t *testing.T) {
	// Arrange
	tmpDir := t.TempDir()
	subDir := filepath.Join(tmpDir, "sub", "dir")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("failed to create subdirectory: %v", err)
	}
	createTestAIRulezDir(t, subDir)

	// Create source with relative path from tmpDir
	source := NewLocalSource("test", "sub/dir", tmpDir, nil)
	ctx := context.Background()

	// Act
	contentTree, err := source.Fetch(ctx)

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if contentTree == nil {
		t.Error("expected non-nil ContentTreeV3")
	}
}

func TestFetchWithFiltering(t *testing.T) {
	// Arrange
	tmpDir := t.TempDir()
	createTestAIRulezDir(t, tmpDir)
	source := NewLocalSource("test", tmpDir, tmpDir, []string{"rules"})
	ctx := context.Background()

	// Act
	contentTree, err := source.Fetch(ctx)

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if contentTree == nil {
		t.Fatalf("expected non-nil ContentTreeV3")
	}
	if len(contentTree.Rules) == 0 {
		t.Error("expected rules to be loaded")
	}
	if len(contentTree.Context) != 0 {
		t.Error("expected context to be filtered out")
	}
	if len(contentTree.Skills) != 0 {
		t.Error("expected skills to be filtered out")
	}
}

// Test that domains are discovered and scanned from includes
func TestLocalSourceFetchWithDomains(t *testing.T) {
	// Arrange - create a temp directory with .ai-rulez structure including domains
	tmpDir := t.TempDir()
	aiRulezDir := createTestAIRulezDir(t, tmpDir)

	// Create domains in the .ai-rulez directory
	backendDomainDir := filepath.Join(aiRulezDir, "domains", "backend", "rules")
	if err := os.MkdirAll(backendDomainDir, 0755); err != nil {
		t.Fatalf("failed to create backend domain: %v", err)
	}
	if err := os.WriteFile(filepath.Join(backendDomainDir, "api.md"), []byte("# API Rules"), 0644); err != nil {
		t.Fatalf("failed to write backend rule: %v", err)
	}

	frontendDomainDir := filepath.Join(aiRulezDir, "domains", "frontend", "rules")
	if err := os.MkdirAll(frontendDomainDir, 0755); err != nil {
		t.Fatalf("failed to create frontend domain: %v", err)
	}
	if err := os.WriteFile(filepath.Join(frontendDomainDir, "components.md"), []byte("# Component Rules"), 0644); err != nil {
		t.Fatalf("failed to write frontend rule: %v", err)
	}

	source := NewLocalSource("test", tmpDir, tmpDir, nil)
	ctx := context.Background()

	// Act
	contentTree, err := source.Fetch(ctx)

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if contentTree == nil {
		t.Fatalf("expected non-nil ContentTreeV3")
	}

	// Verify domains were discovered and scanned
	if contentTree.Domains == nil {
		t.Fatal("expected Domains map to be non-nil")
	}
	if len(contentTree.Domains) != 2 {
		t.Errorf("expected 2 domains, got %d", len(contentTree.Domains))
	}

	backend, hasBackend := contentTree.Domains["backend"]
	if !hasBackend {
		t.Error("expected backend domain to be discovered")
	} else if len(backend.Rules) == 0 {
		t.Error("expected backend domain to have rules")
	}

	frontend, hasFrontend := contentTree.Domains["frontend"]
	if !hasFrontend {
		t.Error("expected frontend domain to be discovered")
	} else if len(frontend.Rules) == 0 {
		t.Error("expected frontend domain to have rules")
	}
}

// Test discoverDomains helper function
func TestDiscoverDomains(t *testing.T) {
	tests := []struct {
		name            string
		setupFunc       func(t *testing.T, dir string)
		expectedDomains []string
	}{
		{
			name: "no domains directory",
			setupFunc: func(t *testing.T, dir string) {
				// Don't create domains directory
			},
			expectedDomains: nil,
		},
		{
			name: "empty domains directory",
			setupFunc: func(t *testing.T, dir string) {
				if err := os.MkdirAll(filepath.Join(dir, "domains"), 0755); err != nil {
					t.Fatalf("failed to create domains dir: %v", err)
				}
			},
			expectedDomains: nil,
		},
		{
			name: "single domain",
			setupFunc: func(t *testing.T, dir string) {
				if err := os.MkdirAll(filepath.Join(dir, "domains", "backend"), 0755); err != nil {
					t.Fatalf("failed to create domain: %v", err)
				}
			},
			expectedDomains: []string{"backend"},
		},
		{
			name: "multiple domains",
			setupFunc: func(t *testing.T, dir string) {
				for _, domain := range []string{"backend", "frontend", "qa"} {
					if err := os.MkdirAll(filepath.Join(dir, "domains", domain), 0755); err != nil {
						t.Fatalf("failed to create domain %s: %v", domain, err)
					}
				}
			},
			expectedDomains: []string{"backend", "frontend", "qa"},
		},
		{
			name: "ignores files in domains directory",
			setupFunc: func(t *testing.T, dir string) {
				domainsDir := filepath.Join(dir, "domains")
				if err := os.MkdirAll(domainsDir, 0755); err != nil {
					t.Fatalf("failed to create domains dir: %v", err)
				}
				// Create a directory (domain)
				if err := os.MkdirAll(filepath.Join(domainsDir, "backend"), 0755); err != nil {
					t.Fatalf("failed to create domain: %v", err)
				}
				// Create a file (should be ignored)
				if err := os.WriteFile(filepath.Join(domainsDir, "README.md"), []byte("readme"), 0644); err != nil {
					t.Fatalf("failed to create file: %v", err)
				}
			},
			expectedDomains: []string{"backend"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			aiRulezDir := filepath.Join(tmpDir, ".ai-rulez")
			if err := os.MkdirAll(aiRulezDir, 0755); err != nil {
				t.Fatalf("failed to create .ai-rulez: %v", err)
			}

			tt.setupFunc(t, aiRulezDir)

			domains := discoverDomains(aiRulezDir)

			if tt.expectedDomains == nil {
				if len(domains) != 0 {
					t.Errorf("expected no domains, got %v", domains)
				}
				return
			}

			if len(domains) != len(tt.expectedDomains) {
				t.Errorf("expected %d domains, got %d: %v", len(tt.expectedDomains), len(domains), domains)
				return
			}

			// Check all expected domains are present (order may vary)
			domainSet := make(map[string]bool)
			for _, d := range domains {
				domainSet[d] = true
			}
			for _, expected := range tt.expectedDomains {
				if !domainSet[expected] {
					t.Errorf("expected domain %q not found in %v", expected, domains)
				}
			}
		})
	}
}
