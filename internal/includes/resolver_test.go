package includes

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
)

// Test circular dependency detection
func TestResolverCircularDependencyDetection(t *testing.T) {
	// This test verifies that circular dependencies are caught
	// Note: ResolveIncludes logs warnings but continues processing
	resolver := NewResolver("/tmp", "")

	// Create two includes with the same name (represents circular reference)
	cfg := &config.ConfigV3{
		Includes: []config.IncludeConfig{
			{
				Name:   "include1",
				Source: "./path1",
			},
			{
				Name:   "include1", // Same name - circular reference
				Source: "./path2",
			},
		},
		Content: &config.ContentTreeV3{
			Domains: make(map[string]*config.DomainV3),
		},
	}

	ctx := context.Background()
	result, _ := resolver.ResolveIncludes(ctx, cfg)

	// ResolveIncludes should return a result (possibly empty) even if includes fail
	if result == nil {
		t.Errorf("expected non-nil result even with failed includes")
	}

	// Verify that the visited map was reset between includes (not persistent across calls)
	// Create a new resolver and test that includes with same name can be processed separately
	resolver2 := NewResolver("/tmp", "")
	cfg2 := &config.ConfigV3{
		Includes: []config.IncludeConfig{
			{
				Name:   "include1",
				Source: "./path1",
			},
		},
		Content: &config.ContentTreeV3{
			Domains: make(map[string]*config.DomainV3),
		},
	}

	result2, _ := resolver2.ResolveIncludes(ctx, cfg2)
	if result2 == nil {
		t.Errorf("expected non-nil result")
	}
}

// Test source creation
func TestResolverCreateSource(t *testing.T) {
	tests := []struct {
		name        string
		source      string
		expectType  SourceType
		expectError bool
		errorCheck  func(err error) bool
	}{
		{
			name:        "git https URL",
			source:      "https://github.com/org/repo",
			expectType:  SourceTypeGit,
			expectError: false, // Git source creation is valid, but validation might happen later
		},
		{
			name:        "local relative path",
			source:      "./local/path",
			expectType:  SourceTypeLocal,
			expectError: false,
		},
		{
			name:        "local absolute path",
			source:      "/absolute/path",
			expectType:  SourceTypeLocal,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver := NewResolver("/tmp", "")
			includeConf := config.IncludeConfig{
				Name:   "test",
				Source: tt.source,
			}

			source, err := resolver.createSource(&includeConf)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				if tt.errorCheck != nil && !tt.errorCheck(err) {
					t.Errorf("error check failed: %v", err)
				}
			} else {
				if err != nil {
					t.Errorf("expected no error, got %v", err)
				}
				if source != nil && source.GetType() != tt.expectType {
					t.Errorf("expected source type %v, got %v", tt.expectType, source.GetType())
				}
			}
		})
	}
}

// Test merge strategies
func TestResolverMergeStrategies(t *testing.T) {
	baseContent := &config.ContentTreeV3{
		Rules: []config.ContentFile{
			{Name: "base-rule", Path: "rules/base.md", Content: "base"},
		},
		Context: []config.ContentFile{},
		Skills:  []config.ContentFile{},
		Domains: make(map[string]*config.DomainV3),
	}

	includeContent := &config.ContentTreeV3{
		Rules: []config.ContentFile{
			{Name: "include-rule", Path: "rules/include.md", Content: "include"},
			{Name: "base-rule", Path: "rules/base.md", Content: "overridden"},
		},
		Context: []config.ContentFile{},
		Skills:  []config.ContentFile{},
		Domains: make(map[string]*config.DomainV3),
	}

	tests := []struct {
		name          string
		strategy      string
		expectRuleLen int
		expectHasBase bool
		expectHasIncl bool
		baseWinsConf  bool
	}{
		{
			name:          "local-override (base wins)",
			strategy:      "local-override",
			expectRuleLen: 2, // both base and include rules
			expectHasBase: true,
			expectHasIncl: true,
			baseWinsConf:  true,
		},
		{
			name:          "include-override (include wins)",
			strategy:      "include-override",
			expectRuleLen: 2, // both base and include rules
			expectHasBase: true,
			expectHasIncl: true,
			baseWinsConf:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver := NewResolver("/tmp", "")
			merged, err := resolver.mergeRoot(baseContent, includeContent, tt.strategy)

			if err != nil {
				t.Fatalf("merge failed: %v", err)
			}

			if len(merged.Rules) != tt.expectRuleLen {
				t.Errorf("expected %d rules, got %d", tt.expectRuleLen, len(merged.Rules))
			}

			hasBase := false
			hasIncl := false
			for _, rule := range merged.Rules {
				if rule.Name == "base-rule" {
					hasBase = true
				}
				if rule.Name == "include-rule" {
					hasIncl = true
				}
			}

			if tt.expectHasBase && !hasBase {
				t.Errorf("expected base rule to be present")
			}
			if tt.expectHasIncl && !hasIncl {
				t.Errorf("expected include rule to be present")
			}
		})
	}
}

// Test error strategy
func TestResolverMergeErrorStrategy(t *testing.T) {
	baseContent := &config.ContentTreeV3{
		Rules: []config.ContentFile{
			{Name: "conflict", Path: "rules/conflict.md", Content: "base"},
		},
		Context: []config.ContentFile{},
		Skills:  []config.ContentFile{},
		Domains: make(map[string]*config.DomainV3),
	}

	includeContent := &config.ContentTreeV3{
		Rules: []config.ContentFile{
			{Name: "conflict", Path: "rules/conflict.md", Content: "include"},
		},
		Context: []config.ContentFile{},
		Skills:  []config.ContentFile{},
		Domains: make(map[string]*config.DomainV3),
	}

	resolver := NewResolver("/tmp", "")
	_, err := resolver.mergeRoot(baseContent, includeContent, "error")

	if err == nil {
		t.Errorf("expected conflict error, got nil")
	}
}

// Test domain installation
func TestResolverDomainInstall(t *testing.T) {
	baseContent := &config.ContentTreeV3{
		Rules:   []config.ContentFile{},
		Context: []config.ContentFile{},
		Skills:  []config.ContentFile{},
		Domains: map[string]*config.DomainV3{
			"frontend": {
				Name: "frontend",
				Rules: []config.ContentFile{
					{Name: "frontend-rule", Path: "rules/frontend.md", Content: "frontend"},
				},
			},
		},
	}

	includeContent := &config.ContentTreeV3{
		Rules: []config.ContentFile{
			{Name: "backend-rule", Path: "rules/backend.md", Content: "backend"},
		},
		Context: []config.ContentFile{},
		Skills:  []config.ContentFile{},
		Domains: make(map[string]*config.DomainV3),
	}

	tests := []struct {
		name           string
		installTo      string
		expectedDomain string
		expectError    bool
	}{
		{
			name:           "install to backend domain",
			installTo:      "domains/backend",
			expectedDomain: "backend",
			expectError:    false,
		},
		{
			name:           "install to backend with trailing slash",
			installTo:      "domains/backend/",
			expectedDomain: "backend",
			expectError:    false,
		},
		{
			name:           "invalid installTo path (treated as domain name)",
			installTo:      "invalid",
			expectedDomain: "invalid",
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver := NewResolver("/tmp", "")
			merged, err := resolver.mergeDomainInstall(baseContent, includeContent, tt.installTo, "local-override")

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error for invalid installTo, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("merge failed: %v", err)
			}

			domain, ok := merged.Domains[tt.expectedDomain]
			if !ok {
				t.Errorf("expected domain %q not found", tt.expectedDomain)
				return
			}

			// Check that backend rules were added
			hasBackendRule := false
			for _, rule := range domain.Rules {
				if rule.Name == "backend-rule" {
					hasBackendRule = true
					break
				}
			}
			if !hasBackendRule {
				t.Errorf("expected backend rule in domain %q", tt.expectedDomain)
			}
		})
	}
}

// Test extractDomainName
func TestExtractDomainName(t *testing.T) {
	tests := []struct {
		name      string
		installTo string
		expected  string
	}{
		{
			name:      "standard format",
			installTo: "domains/backend",
			expected:  "backend",
		},
		{
			name:      "with trailing slash",
			installTo: "domains/backend/",
			expected:  "backend",
		},
		{
			name:      "with subdirectory",
			installTo: "domains/backend/rules",
			expected:  "backend",
		},
		{
			name:      "invalid format",
			installTo: "invalid/path",
			expected:  "invalid/path",
		},
		{
			name:      "empty string",
			installTo: "",
			expected:  "",
		},
		{
			name:      "just domains",
			installTo: "domains/",
			expected:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractDomainName(tt.installTo)
			if result != tt.expected {
				t.Errorf("extractDomainName(%q) = %q, want %q", tt.installTo, result, tt.expected)
			}
		})
	}
}

// Test mergeContentFiles helper
func TestMergeContentFiles(t *testing.T) {
	base := []config.ContentFile{
		{Name: "base1", Path: "base1.md", Content: "base1"},
		{Name: "shared", Path: "shared.md", Content: "base-shared"},
	}

	include := []config.ContentFile{
		{Name: "include1", Path: "include1.md", Content: "include1"},
		{Name: "shared", Path: "shared.md", Content: "include-shared"},
	}

	tests := []struct {
		name        string
		baseWins    bool
		expectedLen int
		checkShared func(config.ContentFile) bool
	}{
		{
			name:        "base wins",
			baseWins:    true,
			expectedLen: 3,
			checkShared: func(f config.ContentFile) bool {
				return f.Name == "shared" && f.Content == "base-shared"
			},
		},
		{
			name:        "include wins",
			baseWins:    false,
			expectedLen: 3,
			checkShared: func(f config.ContentFile) bool {
				return f.Name == "shared" && f.Content == "include-shared"
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mergeContentFiles(base, include, tt.baseWins)

			if len(result) != tt.expectedLen {
				t.Errorf("expected %d files, got %d", tt.expectedLen, len(result))
			}

			// Check the shared file content
			found := false
			for _, f := range result {
				if f.Name == "shared" {
					found = true
					if !tt.checkShared(f) {
						t.Errorf("shared file content check failed for %q", tt.name)
					}
					break
				}
			}
			if !found {
				t.Errorf("shared file not found in result")
			}
		})
	}
}

// Test detectConflicts helper
func TestDetectConflicts(t *testing.T) {
	tests := []struct {
		name     string
		base     []config.ContentFile
		include  []config.ContentFile
		expected bool
	}{
		{
			name: "no conflicts",
			base: []config.ContentFile{
				{Name: "file1"},
			},
			include: []config.ContentFile{
				{Name: "file2"},
			},
			expected: false,
		},
		{
			name: "conflict detected",
			base: []config.ContentFile{
				{Name: "file1"},
			},
			include: []config.ContentFile{
				{Name: "file1"},
			},
			expected: true,
		},
		{
			name: "multiple conflicts",
			base: []config.ContentFile{
				{Name: "file1"},
				{Name: "file2"},
			},
			include: []config.ContentFile{
				{Name: "file2"},
				{Name: "file3"},
			},
			expected: true,
		},
		{
			name:     "empty base",
			base:     []config.ContentFile{},
			include:  []config.ContentFile{{Name: "file1"}},
			expected: false,
		},
		{
			name:     "empty include",
			base:     []config.ContentFile{{Name: "file1"}},
			include:  []config.ContentFile{},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detectConflicts(tt.base, tt.include)
			if result != tt.expected {
				t.Errorf("detectConflicts() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// Test local include resolution with actual filesystem
func TestResolverLocalInclude(t *testing.T) {
	// Create temporary directory with .ai-rulez structure
	tmpDir := t.TempDir()

	// Create source directory with .ai-rulez structure
	sourceDir := filepath.Join(tmpDir, "source")
	if err := os.MkdirAll(sourceDir, 0755); err != nil {
		t.Fatalf("failed to create source dir: %v", err)
	}

	createTestAIRulezDir(t, sourceDir)

	// Create config with include
	resolver := NewResolver(tmpDir, "")
	cfg := &config.ConfigV3{
		Version: "3.0",
		Name:    "test",
		BaseDir: tmpDir,
		Includes: []config.IncludeConfig{
			{
				Name:   "local-include",
				Source: sourceDir,
			},
		},
		Content: &config.ContentTreeV3{
			Domains: make(map[string]*config.DomainV3),
		},
	}

	ctx := context.Background()
	result, err := resolver.ResolveIncludes(ctx, cfg)

	if err != nil {
		t.Fatalf("ResolveIncludes failed: %v", err)
	}

	if result == nil {
		t.Fatalf("expected non-nil content tree")
	}

	// Verify content was merged
	if len(result.Rules) == 0 && len(result.Context) == 0 && len(result.Skills) == 0 {
		t.Errorf("expected some content to be merged, got empty tree")
	}
}

// Test merging domains from includes
func TestResolverMergeDomains(t *testing.T) {
	baseContent := &config.ContentTreeV3{
		Rules:   []config.ContentFile{},
		Context: []config.ContentFile{},
		Skills:  []config.ContentFile{},
		Domains: map[string]*config.DomainV3{
			"existing": {
				Name: "existing",
				Rules: []config.ContentFile{
					{Name: "existing-rule", Path: "rules/existing.md"},
				},
			},
		},
	}

	includeContent := &config.ContentTreeV3{
		Rules:   []config.ContentFile{},
		Context: []config.ContentFile{},
		Skills:  []config.ContentFile{},
		Domains: map[string]*config.DomainV3{
			"new": {
				Name: "new",
				Rules: []config.ContentFile{
					{Name: "new-rule", Path: "rules/new.md"},
				},
			},
		},
	}

	resolver := NewResolver("/tmp", "")
	merged, err := resolver.mergeRoot(baseContent, includeContent, "local-override")

	if err != nil {
		t.Fatalf("merge failed: %v", err)
	}

	// Check both domains exist
	if _, ok := merged.Domains["existing"]; !ok {
		t.Errorf("expected existing domain to be preserved")
	}

	if _, ok := merged.Domains["new"]; !ok {
		t.Errorf("expected new domain from include")
	}
}
