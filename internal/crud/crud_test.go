package crud_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/crud"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestOperatorCreation tests creating a new operator
func TestOperatorCreation(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(t *testing.T) string // Returns base directory
		shouldErr bool
	}{
		{
			name: "valid operator creation",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				aiRulezDir := filepath.Join(tmpDir, ".ai-rulez")
				require.NoError(t, os.MkdirAll(aiRulezDir, 0o755))
				return tmpDir
			},
			shouldErr: false,
		},
		{
			name: "no .ai-rulez directory",
			setup: func(t *testing.T) string {
				return t.TempDir()
			},
			shouldErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			baseDir := tt.setup(t)

			op, err := crud.NewOperator(baseDir)

			if tt.shouldErr {
				assert.Error(t, err)
				assert.Nil(t, op)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, op)
			}
		})
	}
}

// TestFullWorkflow_DomainCreationAndPopulation tests a complete workflow:
// 1. Create a domain
// 2. Add rules to it
// 3. List the rules
// 4. Verify they exist
func TestFullWorkflow_DomainCreationAndPopulation(t *testing.T) {
	baseDir := setupTestProject(t)
	op, err := crud.NewOperator(baseDir)
	require.NoError(t, err)

	// Step 1: Create a domain
	domainResult, err := op.AddDomain(context.Background(), &crud.AddDomainRequest{
		Name:        "backend",
		Description: "Backend services",
	})
	require.NoError(t, err)
	assert.Equal(t, "backend", domainResult.Name)

	// Step 2: Add rules to the domain
	rule1, err := op.AddRule(context.Background(), &crud.AddFileRequest{
		Name:     "api-standards",
		Domain:   "backend",
		Priority: "high",
		Content:  "# API Standards\n\nFollow REST conventions",
	})
	require.NoError(t, err)

	rule2, err := op.AddRule(context.Background(), &crud.AddFileRequest{
		Name:     "database-rules",
		Domain:   "backend",
		Priority: "critical",
	})
	require.NoError(t, err)

	// Step 3: List the rules
	rules, err := op.ListFiles(context.Background(), "backend", "rules")
	require.NoError(t, err)

	// Step 4: Verify they exist
	assert.Len(t, rules, 2)
	names := []string{rules[0].Name, rules[1].Name}
	assert.Contains(t, names, "api-standards")
	assert.Contains(t, names, "database-rules")

	// Verify files exist on disk
	assert.FileExists(t, rule1.FullPath)
	assert.FileExists(t, rule2.FullPath)
}

// TestFullWorkflow_MultiDomainStructure tests creating a multi-domain structure
func TestFullWorkflow_MultiDomainStructure(t *testing.T) {
	baseDir := setupTestProject(t)
	op, err := crud.NewOperator(baseDir)
	require.NoError(t, err)

	// Create multiple domains
	domains := []string{"backend", "frontend", "devops"}
	for _, domainName := range domains {
		_, err := op.AddDomain(context.Background(), &crud.AddDomainRequest{
			Name: domainName,
		})
		require.NoError(t, err)
	}

	// Add root-level rules
	_, err = op.AddRule(context.Background(), &crud.AddFileRequest{
		Name: "global-standards",
	})
	require.NoError(t, err)

	// Add domain-specific rules
	_, err = op.AddRule(context.Background(), &crud.AddFileRequest{
		Name:   "backend-api-rules",
		Domain: "backend",
	})
	require.NoError(t, err)

	_, err = op.AddRule(context.Background(), &crud.AddFileRequest{
		Name:   "frontend-ui-rules",
		Domain: "frontend",
	})
	require.NoError(t, err)

	// Verify domain count
	domainList, err := op.ListDomains(context.Background())
	require.NoError(t, err)
	assert.Len(t, domainList, 3)

	// Verify root rules
	rootRules, err := op.ListFiles(context.Background(), "", "rules")
	require.NoError(t, err)
	assert.Len(t, rootRules, 1)

	// Verify backend rules
	backendRules, err := op.ListFiles(context.Background(), "backend", "rules")
	require.NoError(t, err)
	assert.Len(t, backendRules, 1)

	// Verify frontend rules
	frontendRules, err := op.ListFiles(context.Background(), "frontend", "rules")
	require.NoError(t, err)
	assert.Len(t, frontendRules, 1)
}

// TestFullWorkflow_SkillsAndContext tests working with skills and context
func TestFullWorkflow_SkillsAndContext(t *testing.T) {
	baseDir := setupTestProject(t)
	op, err := crud.NewOperator(baseDir)
	require.NoError(t, err)

	// Create domain
	op.AddDomain(context.Background(), &crud.AddDomainRequest{Name: "qa"})

	// Add context files
	ctx1, err := op.AddContext(context.Background(), &crud.AddFileRequest{
		Name:    "test-strategy",
		Content: "# Testing Strategy\n\nWe use...",
	})
	require.NoError(t, err)
	assert.FileExists(t, ctx1.FullPath)

	ctx2, err := op.AddContext(context.Background(), &crud.AddFileRequest{
		Name:   "qa-domain-context",
		Domain: "qa",
	})
	require.NoError(t, err)
	assert.FileExists(t, ctx2.FullPath)

	// Add skills
	skill1, err := op.AddSkill(context.Background(), &crud.AddFileRequest{
		Name: "test-generation",
	})
	require.NoError(t, err)
	assert.FileExists(t, skill1.FullPath)

	skill2, err := op.AddSkill(context.Background(), &crud.AddFileRequest{
		Name:   "bug-analysis",
		Domain: "qa",
	})
	require.NoError(t, err)
	assert.FileExists(t, skill2.FullPath)

	// List all
	contexts, err := op.ListFiles(context.Background(), "", "context")
	require.NoError(t, err)
	assert.Len(t, contexts, 1)

	skills, err := op.ListFiles(context.Background(), "", "skills")
	require.NoError(t, err)
	assert.Len(t, skills, 1)

	qaContexts, err := op.ListFiles(context.Background(), "qa", "context")
	require.NoError(t, err)
	assert.Len(t, qaContexts, 1)

	qaSkills, err := op.ListFiles(context.Background(), "qa", "skills")
	require.NoError(t, err)
	assert.Len(t, qaSkills, 1)
}

// TestFullWorkflow_ProfileManagement tests profile creation and management
func TestFullWorkflow_ProfileManagement(t *testing.T) {
	baseDir := setupTestProject(t)
	op, err := crud.NewOperator(baseDir)
	require.NoError(t, err)

	// Create domains
	op.AddDomain(context.Background(), &crud.AddDomainRequest{Name: "backend"})
	op.AddDomain(context.Background(), &crud.AddDomainRequest{Name: "frontend"})

	// Create profiles
	err = op.AddProfile(context.Background(), "backend-only", []string{"backend"})
	require.NoError(t, err)

	err = op.AddProfile(context.Background(), "full-stack", []string{"backend", "frontend"})
	require.NoError(t, err)

	// Set default
	err = op.SetDefaultProfile(context.Background(), "full-stack")
	require.NoError(t, err)

	// List and verify
	profiles, err := op.ListProfiles(context.Background())
	require.NoError(t, err)

	assert.Len(t, profiles, 2)

	// Find default profile
	defaultFound := false
	for _, p := range profiles {
		if p.Name == "full-stack" {
			assert.True(t, p.IsDefault)
			defaultFound = true
		}
	}
	assert.True(t, defaultFound)
}

// TestFullWorkflow_IncludeManagement tests include source management
func TestFullWorkflow_IncludeManagement(t *testing.T) {
	baseDir := setupTestProject(t)
	op, err := crud.NewOperator(baseDir)
	require.NoError(t, err)

	// Add multiple includes
	includes := []struct {
		name   string
		source string
	}{
		{"shared-rules", "https://github.com/org/shared-rules.git"},
		{"company-standards", "https://github.com/org/company-standards.git"},
		{"security-rules", "git@github.com:org/security-rules.git"},
	}

	for _, inc := range includes {
		err := op.AddInclude(context.Background(), &crud.AddIncludeRequest{
			Name:    inc.name,
			Source:  inc.source,
			Include: []string{"rules", "context"},
		})
		require.NoError(t, err)
	}

	// List and verify
	incList, err := op.ListIncludes(context.Background())
	require.NoError(t, err)

	assert.Len(t, incList, 3)

	// Verify sources
	names := []string{incList[0].Name, incList[1].Name, incList[2].Name}
	assert.Contains(t, names, "shared-rules")
	assert.Contains(t, names, "company-standards")
	assert.Contains(t, names, "security-rules")

	// Remove one
	err = op.RemoveInclude(context.Background(), "company-standards")
	require.NoError(t, err)

	// Verify removal
	incList, err = op.ListIncludes(context.Background())
	require.NoError(t, err)
	assert.Len(t, incList, 2)
}

// TestCompleteWorkflow tests a realistic complete workflow
func TestCompleteWorkflow(t *testing.T) {
	baseDir := setupTestProject(t)
	op, err := crud.NewOperator(baseDir)
	require.NoError(t, err)

	// 1. Set up project structure with multiple domains
	_, err = op.AddDomain(context.Background(), &crud.AddDomainRequest{
		Name:        "backend",
		Description: "Backend services",
	})
	require.NoError(t, err)

	_, err = op.AddDomain(context.Background(), &crud.AddDomainRequest{
		Name:        "frontend",
		Description: "Frontend applications",
	})
	require.NoError(t, err)

	// 2. Add rules at root level
	_, err = op.AddRule(context.Background(), &crud.AddFileRequest{
		Name:     "general-standards",
		Priority: "high",
		Targets:  []string{"claude", "cursor"},
		Content:  "# General Standards\n\nApply to all work",
	})
	require.NoError(t, err)

	// 3. Add domain-specific rules
	_, err = op.AddRule(context.Background(), &crud.AddFileRequest{
		Name:     "api-design",
		Domain:   "backend",
		Priority: "critical",
		Content:  "# API Design Standards\n\nREST conventions...",
	})
	require.NoError(t, err)

	_, err = op.AddRule(context.Background(), &crud.AddFileRequest{
		Name:   "react-patterns",
		Domain: "frontend",
	})
	require.NoError(t, err)

	// 4. Add context
	_, err = op.AddContext(context.Background(), &crud.AddFileRequest{
		Name:    "project-overview",
		Content: "# Project Overview\n\nOur project structure...",
	})
	require.NoError(t, err)

	_, err = op.AddContext(context.Background(), &crud.AddFileRequest{
		Name:   "backend-tech-stack",
		Domain: "backend",
	})
	require.NoError(t, err)

	// 5. Add skills
	_, err = op.AddSkill(context.Background(), &crud.AddFileRequest{
		Name: "code-review",
	})
	require.NoError(t, err)

	_, err = op.AddSkill(context.Background(), &crud.AddFileRequest{
		Name:   "performance-optimization",
		Domain: "backend",
	})
	require.NoError(t, err)

	// 6. Add includes
	err = op.AddInclude(context.Background(), &crud.AddIncludeRequest{
		Name:    "company-standards",
		Source:  "https://github.com/company/standards.git",
		Include: []string{"rules", "context"},
	})
	require.NoError(t, err)

	// 7. Create profiles
	err = op.AddProfile(context.Background(), "backend-focused", []string{"backend"})
	require.NoError(t, err)

	err = op.AddProfile(context.Background(), "fullstack", []string{"backend", "frontend"})
	require.NoError(t, err)

	err = op.SetDefaultProfile(context.Background(), "fullstack")
	require.NoError(t, err)

	// 8. Verify complete structure
	domains, err := op.ListDomains(context.Background())
	require.NoError(t, err)
	assert.Len(t, domains, 2)

	rootRules, err := op.ListFiles(context.Background(), "", "rules")
	require.NoError(t, err)
	assert.Len(t, rootRules, 1)

	backendRules, err := op.ListFiles(context.Background(), "backend", "rules")
	require.NoError(t, err)
	assert.Len(t, backendRules, 1)

	frontendRules, err := op.ListFiles(context.Background(), "frontend", "rules")
	require.NoError(t, err)
	assert.Len(t, frontendRules, 1)

	rootContexts, err := op.ListFiles(context.Background(), "", "context")
	require.NoError(t, err)
	assert.Len(t, rootContexts, 1)

	backendContexts, err := op.ListFiles(context.Background(), "backend", "context")
	require.NoError(t, err)
	assert.Len(t, backendContexts, 1)

	rootSkills, err := op.ListFiles(context.Background(), "", "skills")
	require.NoError(t, err)
	assert.Len(t, rootSkills, 1)

	backendSkills, err := op.ListFiles(context.Background(), "backend", "skills")
	require.NoError(t, err)
	assert.Len(t, backendSkills, 1)

	includes, err := op.ListIncludes(context.Background())
	require.NoError(t, err)
	assert.Len(t, includes, 1)

	profiles, err := op.ListProfiles(context.Background())
	require.NoError(t, err)
	assert.Len(t, profiles, 2)

	// Verify default profile
	defaultFound := false
	for _, p := range profiles {
		if p.Name == "fullstack" && p.IsDefault {
			defaultFound = true
			assert.Len(t, p.Domains, 2)
		}
	}
	assert.True(t, defaultFound)
}
