package scanner

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/parser"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestScanner_BasicScanning tests basic scanning without domains
func TestScanner_BasicScanning(t *testing.T) {
	// Arrange
	tmpDir := t.TempDir()
	setupBasicStructure(t, tmpDir)

	cfg := &config.ConfigV3{
		Version: "3.0",
		Name:    "test-project",
		Profiles: map[string][]string{
			"default": {},
		},
		Default: "default",
	}

	scanner := NewScanner(tmpDir, cfg)

	// Act
	tree, err := scanner.ScanProfile("default")

	// Assert
	require.NoError(t, err)
	require.NotNil(t, tree)
	assert.Len(t, tree.Rules, 2)
	assert.Len(t, tree.Context, 1)
	assert.Len(t, tree.Skills, 1)
	assert.Len(t, tree.Domains, 0)

	// Check file names
	assert.Equal(t, "coding-standards", tree.Rules[0].Name)
	assert.Equal(t, "testing", tree.Rules[1].Name)
	assert.Equal(t, "project-overview", tree.Context[0].Name)
	assert.Equal(t, "python-expert", tree.Skills[0].Name)
}

// TestScanner_ProfileBasedScanning tests scanning with multiple domains
func TestScanner_ProfileBasedScanning(t *testing.T) {
	// Arrange
	tmpDir := t.TempDir()
	setupMultiDomainStructure(t, tmpDir)

	cfg := &config.ConfigV3{
		Version: "3.0",
		Name:    "test-project",
		Profiles: map[string][]string{
			"backend":   {"backend", "database"},
			"frontend":  {"frontend"},
			"fullstack": {"backend", "frontend", "database"},
		},
		Default: "fullstack",
	}

	scanner := NewScanner(tmpDir, cfg)

	// Act - Backend profile
	backendTree, err := scanner.ScanProfile("backend")

	// Assert
	require.NoError(t, err)
	require.NotNil(t, backendTree)

	// Should have root content + backend domain + database domain
	assert.GreaterOrEqual(t, len(backendTree.Rules), 2)
	assert.Len(t, backendTree.Domains, 2)
	assert.NotNil(t, backendTree.Domains["backend"])
	assert.NotNil(t, backendTree.Domains["database"])

	// Act - Frontend profile
	frontendTree, err := scanner.ScanProfile("frontend")

	// Assert
	require.NoError(t, err)
	require.NotNil(t, frontendTree)
	assert.Len(t, frontendTree.Domains, 1)
	assert.NotNil(t, frontendTree.Domains["frontend"])

	// Act - Fullstack profile
	fullstackTree, err := scanner.ScanProfile("fullstack")

	// Assert
	require.NoError(t, err)
	require.NotNil(t, fullstackTree)
	assert.Len(t, fullstackTree.Domains, 3)
}

// TestScanner_Namespacing tests that domain content is properly namespaced
func TestScanner_Namespacing(t *testing.T) {
	// Arrange
	tmpDir := t.TempDir()
	setupMultiDomainStructure(t, tmpDir)

	cfg := &config.ConfigV3{
		Version: "3.0",
		Name:    "test-project",
		Profiles: map[string][]string{
			"backend": {"backend"},
		},
	}

	scanner := NewScanner(tmpDir, cfg)

	// Act
	tree, err := scanner.ScanProfile("backend")

	// Assert
	require.NoError(t, err)
	require.NotNil(t, tree)

	// Find domain content in merged results
	foundNamespaced := false
	for _, rule := range tree.Rules {
		if rule.Name == "backend: api-standards" {
			foundNamespaced = true
			break
		}
	}
	assert.True(t, foundNamespaced, "Domain content should be namespaced")

	// Check domain map has non-namespaced content
	backendDomain := tree.Domains["backend"]
	require.NotNil(t, backendDomain)
	// Domain content in the Domains map is namespaced
	foundInDomain := false
	for _, rule := range backendDomain.Rules {
		if rule.Name == "backend: api-standards" {
			foundInDomain = true
			break
		}
	}
	assert.True(t, foundInDomain)
}

// TestScanner_CollisionHandling tests that domain files override root files
func TestScanner_CollisionHandling(t *testing.T) {
	// Arrange
	tmpDir := t.TempDir()
	setupCollisionStructure(t, tmpDir)

	cfg := &config.ConfigV3{
		Version: "3.0",
		Name:    "test-project",
		Profiles: map[string][]string{
			"default": {"backend"},
		},
	}

	scanner := NewScanner(tmpDir, cfg)

	// Act
	tree, err := scanner.ScanProfile("default")

	// Assert
	require.NoError(t, err)
	require.NotNil(t, tree)

	// Should only have 2 rules total (root non-collision + domain override)
	// Root has: python.md (overridden), testing.md (kept)
	// Domain has: python.md (overrides root)
	assert.Len(t, tree.Rules, 2)

	// Check that we have the domain version (namespaced)
	foundDomainPython := false
	foundRootPython := false
	for _, rule := range tree.Rules {
		if rule.Name == "backend: python" {
			foundDomainPython = true
			assert.Contains(t, rule.Content, "domain-specific Python rules")
		}
		if rule.Name == "python" && !foundDomainPython {
			foundRootPython = true
		}
	}
	assert.True(t, foundDomainPython, "Should have domain version of python.md")
	assert.False(t, foundRootPython, "Should not have root version of python.md")
}

// TestScanner_PrioritySorting tests that files are sorted by priority
func TestScanner_PrioritySorting(t *testing.T) {
	// Arrange
	tmpDir := t.TempDir()
	setupPriorityStructure(t, tmpDir)

	cfg := &config.ConfigV3{
		Version: "3.0",
		Name:    "test-project",
		Profiles: map[string][]string{
			"default": {},
		},
	}

	scanner := NewScanner(tmpDir, cfg)

	// Act
	tree, err := scanner.ScanProfile("default")

	// Assert
	require.NoError(t, err)
	require.NotNil(t, tree)
	assert.Len(t, tree.Rules, 4)

	// Check order (critical > high > medium > low)
	assert.Equal(t, "critical", tree.Rules[0].Name)
	assert.Equal(t, "high", tree.Rules[1].Name)
	assert.Equal(t, "medium", tree.Rules[2].Name)
	assert.Equal(t, "low", tree.Rules[3].Name)
}

// TestScanner_MissingDirectories tests graceful handling of missing directories
func TestScanner_MissingDirectories(t *testing.T) {
	// Arrange
	tmpDir := t.TempDir()
	// Only create .ai-rulez/config.yaml, no content directories
	aiRulezDir := filepath.Join(tmpDir, ".ai-rulez")
	require.NoError(t, os.MkdirAll(aiRulezDir, 0o755))

	cfg := &config.ConfigV3{
		Version: "3.0",
		Name:    "test-project",
		Profiles: map[string][]string{
			"default": {},
		},
	}

	scanner := NewScanner(tmpDir, cfg)

	// Act
	tree, err := scanner.ScanProfile("default")

	// Assert
	require.NoError(t, err)
	require.NotNil(t, tree)
	assert.Len(t, tree.Rules, 0)
	assert.Len(t, tree.Context, 0)
	assert.Len(t, tree.Skills, 0)
}

// TestScanner_InvalidProfile tests error handling for invalid profiles
func TestScanner_InvalidProfile(t *testing.T) {
	// Arrange
	tmpDir := t.TempDir()
	setupBasicStructure(t, tmpDir)

	cfg := &config.ConfigV3{
		Version: "3.0",
		Name:    "test-project",
		Profiles: map[string][]string{
			"default": {},
		},
	}

	scanner := NewScanner(tmpDir, cfg)

	// Act
	tree, err := scanner.ScanProfile("nonexistent")

	// Assert
	require.Error(t, err)
	assert.Nil(t, tree)
	assert.Contains(t, err.Error(), "profile not found")
}

// TestScanner_DomainNotFound tests error handling when domain directory doesn't exist
func TestScanner_DomainNotFound(t *testing.T) {
	// Arrange
	tmpDir := t.TempDir()
	setupBasicStructure(t, tmpDir)

	cfg := &config.ConfigV3{
		Version: "3.0",
		Name:    "test-project",
		Profiles: map[string][]string{
			"default": {"nonexistent"},
		},
	}

	scanner := NewScanner(tmpDir, cfg)

	// Act
	tree, err := scanner.ScanProfile("default")

	// Assert
	require.Error(t, err)
	assert.Nil(t, tree)
	assert.Contains(t, err.Error(), "domain directory not found")
}

// TestScanner_SkillNaming tests that skills are named correctly
func TestScanner_SkillNaming(t *testing.T) {
	// Arrange
	tmpDir := t.TempDir()
	setupSkillsStructure(t, tmpDir)

	cfg := &config.ConfigV3{
		Version: "3.0",
		Name:    "test-project",
		Profiles: map[string][]string{
			"default": {"backend"},
		},
	}

	scanner := NewScanner(tmpDir, cfg)

	// Act
	tree, err := scanner.ScanProfile("default")

	// Assert
	require.NoError(t, err)
	require.NotNil(t, tree)

	// Should have 3 skills: 2 root + 1 domain
	assert.Len(t, tree.Skills, 3)

	// Root skills should be named after directory
	foundAPIDesigner := false
	foundCodeReviewer := false
	for _, skill := range tree.Skills {
		if skill.Name == "api-designer" {
			foundAPIDesigner = true
		}
		if skill.Name == "code-reviewer" {
			foundCodeReviewer = true
		}
	}
	assert.True(t, foundAPIDesigner, "Root skill 'api-designer' should exist")
	assert.True(t, foundCodeReviewer, "Root skill 'code-reviewer' should exist")

	// Domain skill should be prefixed with domain name
	foundDomainSkill := false
	for _, skill := range tree.Skills {
		if skill.Name == "backend-debugger" {
			foundDomainSkill = true
		}
	}
	assert.True(t, foundDomainSkill, "Domain skill should be prefixed with domain name")
}

// TestScanner_EmptyProfile tests scanning with empty profile (no domains)
func TestScanner_EmptyProfile(t *testing.T) {
	// Arrange
	tmpDir := t.TempDir()
	setupBasicStructure(t, tmpDir)

	cfg := &config.ConfigV3{
		Version: "3.0",
		Name:    "test-project",
		Profiles: map[string][]string{
			"empty": {},
		},
	}

	scanner := NewScanner(tmpDir, cfg)

	// Act
	tree, err := scanner.ScanProfile("empty")

	// Assert
	require.NoError(t, err)
	require.NotNil(t, tree)
	// Should only have root content
	assert.Len(t, tree.Rules, 2)
	assert.Len(t, tree.Domains, 0)
}

// Helper functions to set up test structures

func setupBasicStructure(t *testing.T, baseDir string) {
	t.Helper()

	aiRulezDir := filepath.Join(baseDir, ".ai-rulez")

	// Create rules
	rulesDir := filepath.Join(aiRulezDir, "rules")
	require.NoError(t, os.MkdirAll(rulesDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(rulesDir, "coding-standards.md"),
		[]byte("# Coding Standards\n\nFollow these standards."),
		0o644,
	))
	require.NoError(t, os.WriteFile(
		filepath.Join(rulesDir, "testing.md"),
		[]byte("# Testing\n\nWrite tests."),
		0o644,
	))

	// Create context
	contextDir := filepath.Join(aiRulezDir, "context")
	require.NoError(t, os.MkdirAll(contextDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(contextDir, "project-overview.md"),
		[]byte("# Project Overview\n\nThis is the project."),
		0o644,
	))

	// Create skills
	skillsDir := filepath.Join(aiRulezDir, "skills", "python-expert")
	require.NoError(t, os.MkdirAll(skillsDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(skillsDir, "SKILL.md"),
		[]byte("# Python Expert\n\nI am a Python expert."),
		0o644,
	))
}

func setupMultiDomainStructure(t *testing.T, baseDir string) {
	t.Helper()

	setupBasicStructure(t, baseDir)

	aiRulezDir := filepath.Join(baseDir, ".ai-rulez")

	// Backend domain
	backendRulesDir := filepath.Join(aiRulezDir, "domains", "backend", "rules")
	require.NoError(t, os.MkdirAll(backendRulesDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(backendRulesDir, "api-standards.md"),
		[]byte("# API Standards\n\nRESTful API guidelines."),
		0o644,
	))

	// Frontend domain
	frontendContextDir := filepath.Join(aiRulezDir, "domains", "frontend", "context")
	require.NoError(t, os.MkdirAll(frontendContextDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(frontendContextDir, "ui-components.md"),
		[]byte("# UI Components\n\nReact components."),
		0o644,
	))

	// Database domain
	databaseRulesDir := filepath.Join(aiRulezDir, "domains", "database", "rules")
	require.NoError(t, os.MkdirAll(databaseRulesDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(databaseRulesDir, "migrations.md"),
		[]byte("# Migrations\n\nDatabase migration strategy."),
		0o644,
	))
}

func setupCollisionStructure(t *testing.T, baseDir string) {
	t.Helper()

	aiRulezDir := filepath.Join(baseDir, ".ai-rulez")

	// Root rules
	rulesDir := filepath.Join(aiRulezDir, "rules")
	require.NoError(t, os.MkdirAll(rulesDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(rulesDir, "python.md"),
		[]byte("# Python\n\nGeneral Python rules."),
		0o644,
	))
	require.NoError(t, os.WriteFile(
		filepath.Join(rulesDir, "testing.md"),
		[]byte("# Testing\n\nGeneral testing rules."),
		0o644,
	))

	// Backend domain with collision
	backendRulesDir := filepath.Join(aiRulezDir, "domains", "backend", "rules")
	require.NoError(t, os.MkdirAll(backendRulesDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(backendRulesDir, "python.md"),
		[]byte("# Python\n\nBackend domain-specific Python rules."),
		0o644,
	))
}

func setupPriorityStructure(t *testing.T, baseDir string) {
	t.Helper()

	aiRulezDir := filepath.Join(baseDir, ".ai-rulez")
	rulesDir := filepath.Join(aiRulezDir, "rules")
	require.NoError(t, os.MkdirAll(rulesDir, 0o755))

	// Critical priority
	require.NoError(t, os.WriteFile(
		filepath.Join(rulesDir, "critical.md"),
		[]byte("---\npriority: critical\n---\n# Critical\n\nCritical rule."),
		0o644,
	))

	// High priority
	require.NoError(t, os.WriteFile(
		filepath.Join(rulesDir, "high.md"),
		[]byte("---\npriority: high\n---\n# High\n\nHigh priority rule."),
		0o644,
	))

	// Medium priority (no frontmatter, defaults to medium)
	require.NoError(t, os.WriteFile(
		filepath.Join(rulesDir, "medium.md"),
		[]byte("# Medium\n\nMedium priority rule."),
		0o644,
	))

	// Low priority
	require.NoError(t, os.WriteFile(
		filepath.Join(rulesDir, "low.md"),
		[]byte("---\npriority: low\n---\n# Low\n\nLow priority rule."),
		0o644,
	))
}

func setupSkillsStructure(t *testing.T, baseDir string) {
	t.Helper()

	aiRulezDir := filepath.Join(baseDir, ".ai-rulez")

	// Root skill
	rootSkillDir := filepath.Join(aiRulezDir, "skills", "api-designer")
	require.NoError(t, os.MkdirAll(rootSkillDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(rootSkillDir, "SKILL.md"),
		[]byte("# API Designer\n\nDesign APIs."),
		0o644,
	))

	// Another root skill (to avoid collision with domain skill)
	rootSkillDir2 := filepath.Join(aiRulezDir, "skills", "code-reviewer")
	require.NoError(t, os.MkdirAll(rootSkillDir2, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(rootSkillDir2, "SKILL.md"),
		[]byte("# Code Reviewer\n\nReview code."),
		0o644,
	))

	// Domain skill
	domainSkillDir := filepath.Join(aiRulezDir, "domains", "backend", "skills", "debugger")
	require.NoError(t, os.MkdirAll(domainSkillDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(domainSkillDir, "SKILL.md"),
		[]byte("# Debugger\n\nDebug backend code."),
		0o644,
	))
}

// TestParseFrontmatter tests the frontmatter parsing function
func TestParseFrontmatter(t *testing.T) {
	tests := []struct {
		name            string
		content         string
		expectedMeta    *config.MetadataV3
		expectedContent string
	}{
		{
			name:            "no frontmatter",
			content:         "# Title\n\nContent here.",
			expectedMeta:    nil,
			expectedContent: "# Title\n\nContent here.",
		},
		{
			name: "with priority",
			content: `---
priority: high
---
# Title

Content here.`,
			expectedMeta: &config.MetadataV3{
				Priority: "high",
				Extra:    map[string]string{},
			},
			expectedContent: "# Title\n\nContent here.",
		},
		{
			name: "with targets",
			content: `---
priority: critical
targets: [claude, cursor]
---
# Title

Content here.`,
			expectedMeta: &config.MetadataV3{
				Priority: "critical",
				Targets:  []string{"claude", "cursor"},
				Extra:    map[string]string{},
			},
			expectedContent: "# Title\n\nContent here.",
		},
		{
			name: "with extra fields",
			content: `---
priority: medium
author: John Doe
version: 1.0
---
# Title`,
			expectedMeta: &config.MetadataV3{
				Priority: "medium",
				Extra: map[string]string{
					"author":  "John Doe",
					"version": "1.0",
				},
			},
			expectedContent: "# Title",
		},
		{
			name: "incomplete frontmatter",
			content: `---
priority: high
# Title`,
			expectedMeta:    nil,
			expectedContent: "---\npriority: high\n# Title",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use the canonical parser from parser package
			parserMeta, content, _ := parser.ParseFrontmatter(tt.content)

			// Convert parser.MetadataV3 to config.MetadataV3 for comparison
			var meta *config.MetadataV3
			if parserMeta != nil {
				meta = &config.MetadataV3{
					Priority: parserMeta.Priority,
					Targets:  parserMeta.Targets,
					Extra:    parserMeta.Extra,
				}
			}

			// Assert
			if tt.expectedMeta == nil {
				assert.Nil(t, meta)
			} else {
				require.NotNil(t, meta)
				assert.Equal(t, tt.expectedMeta.Priority, meta.Priority)
				assert.Equal(t, tt.expectedMeta.Targets, meta.Targets)
			}
			assert.Equal(t, tt.expectedContent, content)
		})
	}
}
