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

// TestAddRule tests adding rules
func TestAddRule(t *testing.T) {
	tests := []struct {
		name      string
		req       *crud.AddFileRequest
		shouldErr bool
		checkFn   func(t *testing.T, result *crud.FileResult, baseDir string)
	}{
		{
			name: "add rule to root",
			req: &crud.AddFileRequest{
				Name:     "coding-standards",
				Content:  "# Coding Standards\n\nFollow these standards...",
				Priority: "high",
				Targets:  []string{"claude", "cursor"},
			},
			shouldErr: false,
			checkFn: func(t *testing.T, result *crud.FileResult, baseDir string) {
				assert.Equal(t, "coding-standards", result.Name)
				assert.Equal(t, "rules", result.Type)
				assert.Empty(t, result.Domain)
				assert.FileExists(t, result.FullPath)

				// Verify content
				content, err := os.ReadFile(result.FullPath)
				require.NoError(t, err)
				assert.Contains(t, string(content), "# Coding Standards")
				assert.Contains(t, string(content), "priority: high")
				assert.Contains(t, string(content), "- claude")
				assert.Contains(t, string(content), "- cursor")
			},
		},
		{
			name: "add rule with default priority",
			req: &crud.AddFileRequest{
				Name:    "test-rule",
				Content: "# Test Rule",
			},
			shouldErr: false,
			checkFn: func(t *testing.T, result *crud.FileResult, baseDir string) {
				content, err := os.ReadFile(result.FullPath)
				require.NoError(t, err)
				assert.Contains(t, string(content), "priority: medium") // default
			},
		},
		{
			name: "add rule with template generation",
			req: &crud.AddFileRequest{
				Name:     "auto-generated-rule",
				Priority: "critical",
			},
			shouldErr: false,
			checkFn: func(t *testing.T, result *crud.FileResult, baseDir string) {
				assert.FileExists(t, result.FullPath)
				content, err := os.ReadFile(result.FullPath)
				require.NoError(t, err)
				assert.Contains(t, string(content), "priority: critical")
				// Should have auto-generated content
				assert.Contains(t, string(content), "#")
			},
		},
		{
			name: "add rule to domain",
			req: &crud.AddFileRequest{
				Name:     "backend-rule",
				Domain:   "backend",
				Priority: "high",
				Content:  "Backend-specific rule",
			},
			shouldErr: false,
			checkFn: func(t *testing.T, result *crud.FileResult, baseDir string) {
				assert.Equal(t, "backend", result.Domain)
				assert.Contains(t, filepath.ToSlash(result.FullPath), "domains/backend")
				assert.FileExists(t, result.FullPath)
			},
		},
		{
			name:      "nil request",
			req:       nil,
			shouldErr: true,
		},
		{
			name: "empty rule name",
			req: &crud.AddFileRequest{
				Name: "",
			},
			shouldErr: true,
		},
		{
			name: "invalid priority",
			req: &crud.AddFileRequest{
				Name:     "test",
				Priority: "urgent",
			},
			shouldErr: true,
		},
		// Duplicate rule test is covered in a separate test function
		{
			name: "domain not found",
			req: &crud.AddFileRequest{
				Name:   "test",
				Domain: "nonexistent",
			},
			shouldErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			baseDir := setupTestProject(t)

			// Create backend domain for domain-scoped tests
			op, err := crud.NewOperator(baseDir)
			require.NoError(t, err)
			op.AddDomain(context.Background(), &crud.AddDomainRequest{Name: "backend"})

			result, err := op.AddRule(context.Background(), tt.req)

			if tt.shouldErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				if tt.checkFn != nil {
					tt.checkFn(t, result, baseDir)
				}
			}
		})
	}
}

// TestAddRuleDuplicate tests that duplicate rules cannot be created
func TestAddRuleDuplicate(t *testing.T) {
	baseDir := setupTestProject(t)
	op, err := crud.NewOperator(baseDir)
	require.NoError(t, err)

	// Add rule first time - should succeed
	result, err := op.AddRule(context.Background(), &crud.AddFileRequest{Name: "my-rule"})
	require.NoError(t, err)
	assert.NotNil(t, result)

	// Try to add same rule again - should fail
	result, err = op.AddRule(context.Background(), &crud.AddFileRequest{Name: "my-rule"})
	assert.Error(t, err)
	assert.Nil(t, result)
}

// TestAddContext tests adding context files
func TestAddContext(t *testing.T) {
	tests := []struct {
		name      string
		req       *crud.AddFileRequest
		shouldErr bool
		checkFn   func(t *testing.T, result *crud.FileResult)
	}{
		{
			name: "add context to root",
			req: &crud.AddFileRequest{
				Name:     "project-structure",
				Content:  "# Project Structure\n\nOur project is organized...",
				Priority: "medium",
			},
			shouldErr: false,
			checkFn: func(t *testing.T, result *crud.FileResult) {
				assert.Equal(t, "project-structure", result.Name)
				assert.Equal(t, "context", result.Type)
				assert.FileExists(t, result.FullPath)
			},
		},
		{
			name: "add context to domain",
			req: &crud.AddFileRequest{
				Name:   "frontend-context",
				Domain: "frontend",
			},
			shouldErr: false,
			checkFn: func(t *testing.T, result *crud.FileResult) {
				assert.Equal(t, "frontend", result.Domain)
				assert.Contains(t, filepath.ToSlash(result.FullPath), "domains/frontend")
			},
		},
		{
			name:      "nil request",
			req:       nil,
			shouldErr: true,
		},
		{
			name: "invalid file name",
			req: &crud.AddFileRequest{
				Name: "context/invalid",
			},
			shouldErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			baseDir := setupTestProject(t)

			op, err := crud.NewOperator(baseDir)
			require.NoError(t, err)
			op.AddDomain(context.Background(), &crud.AddDomainRequest{Name: "frontend"})

			result, err := op.AddContext(context.Background(), tt.req)

			if tt.shouldErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				if tt.checkFn != nil {
					tt.checkFn(t, result)
				}
			}
		})
	}
}

// TestAddSkill tests adding skills
func TestAddSkill(t *testing.T) {
	tests := []struct {
		name      string
		req       *crud.AddFileRequest
		shouldErr bool
		checkFn   func(t *testing.T, result *crud.FileResult)
	}{
		{
			name: "add skill to root",
			req: &crud.AddFileRequest{
				Name:     "code-review",
				Priority: "high",
			},
			shouldErr: false,
			checkFn: func(t *testing.T, result *crud.FileResult) {
				assert.Equal(t, "code-review", result.Name)
				assert.Equal(t, "skills", result.Type)
				assert.Empty(t, result.Domain)

				// Skills have a different structure: directory with SKILL.md
				assert.FileExists(t, result.FullPath)
				assert.Contains(t, result.FullPath, "SKILL.md")

				// Verify skill directory structure
				skillDir := filepath.Dir(result.FullPath)
				assert.DirExists(t, skillDir)
			},
		},
		{
			name: "add skill to domain",
			req: &crud.AddFileRequest{
				Name:   "testing-skill",
				Domain: "qa",
			},
			shouldErr: false,
			checkFn: func(t *testing.T, result *crud.FileResult) {
				assert.Equal(t, "qa", result.Domain)
				assert.Contains(t, filepath.ToSlash(result.FullPath), "domains/qa")
				assert.Contains(t, result.FullPath, "SKILL.md")
			},
		},
		{
			name: "skill with custom content",
			req: &crud.AddFileRequest{
				Name:    "analysis-skill",
				Content: "# Analysis Skill\n\nCapabilities...",
			},
			shouldErr: false,
			checkFn: func(t *testing.T, result *crud.FileResult) {
				content, err := os.ReadFile(result.FullPath)
				require.NoError(t, err)
				assert.Contains(t, string(content), "# Analysis Skill")
			},
		},
		{
			name:      "skill with invalid name (must be alphanumeric/underscore/hyphen)",
			req:       &crud.AddFileRequest{Name: "skill/invalid"},
			shouldErr: true,
		},
		// Duplicate skill test is covered in a separate test function
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			baseDir := setupTestProject(t)

			op, err := crud.NewOperator(baseDir)
			require.NoError(t, err)
			op.AddDomain(context.Background(), &crud.AddDomainRequest{Name: "qa"})

			result, err := op.AddSkill(context.Background(), tt.req)

			if tt.shouldErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				if tt.checkFn != nil {
					tt.checkFn(t, result)
				}
			}
		})
	}
}

// TestAddSkillDuplicate tests that duplicate skills cannot be created
func TestAddSkillDuplicate(t *testing.T) {
	baseDir := setupTestProject(t)
	op, err := crud.NewOperator(baseDir)
	require.NoError(t, err)

	// Add skill first time - should succeed
	result, err := op.AddSkill(context.Background(), &crud.AddFileRequest{Name: "my-skill"})
	require.NoError(t, err)
	assert.NotNil(t, result)

	// Try to add same skill again - should fail
	result, err = op.AddSkill(context.Background(), &crud.AddFileRequest{Name: "my-skill"})
	assert.Error(t, err)
	assert.Nil(t, result)
}

// TestRemoveFile tests removing files and skills
func TestRemoveFile(t *testing.T) {
	tests := []struct {
		name          string
		setup         func(t *testing.T, op crud.Operator)
		domain        string
		fileType      string
		fileName      string
		shouldErr     bool
		verifyRemoved bool
	}{
		{
			name: "remove rule",
			setup: func(t *testing.T, op crud.Operator) {
				op.AddRule(context.Background(), &crud.AddFileRequest{Name: "test-rule"})
			},
			fileType:      "rules",
			fileName:      "test-rule",
			shouldErr:     false,
			verifyRemoved: true,
		},
		{
			name: "remove context",
			setup: func(t *testing.T, op crud.Operator) {
				op.AddContext(context.Background(), &crud.AddFileRequest{Name: "test-context"})
			},
			fileType:      "context",
			fileName:      "test-context",
			shouldErr:     false,
			verifyRemoved: true,
		},
		{
			name: "remove skill",
			setup: func(t *testing.T, op crud.Operator) {
				op.AddSkill(context.Background(), &crud.AddFileRequest{Name: "test-skill"})
			},
			fileType:      "skills",
			fileName:      "test-skill",
			shouldErr:     false,
			verifyRemoved: true,
		},
		{
			name: "remove from domain",
			setup: func(t *testing.T, op crud.Operator) {
				op.AddDomain(context.Background(), &crud.AddDomainRequest{Name: "backend"})
				op.AddRule(context.Background(), &crud.AddFileRequest{
					Name:   "domain-rule",
					Domain: "backend",
				})
			},
			domain:        "backend",
			fileType:      "rules",
			fileName:      "domain-rule",
			shouldErr:     false,
			verifyRemoved: true,
		},
		{
			name:      "remove non-existent file",
			setup:     func(t *testing.T, op crud.Operator) {},
			fileType:  "rules",
			fileName:  "nonexistent",
			shouldErr: true,
		},
		{
			name:      "invalid file type",
			setup:     func(t *testing.T, op crud.Operator) {},
			fileType:  "invalid",
			fileName:  "test",
			shouldErr: true,
		},
		{
			name:      "invalid file name",
			setup:     func(t *testing.T, op crud.Operator) {},
			fileType:  "rules",
			fileName:  "",
			shouldErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			baseDir := setupTestProject(t)
			op, err := crud.NewOperator(baseDir)
			require.NoError(t, err)

			tt.setup(t, op)

			err = op.RemoveFile(context.Background(), tt.domain, tt.fileType, tt.fileName)

			if tt.shouldErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)

				if tt.verifyRemoved {
					// Verify by listing files
					files, err := op.ListFiles(context.Background(), tt.domain, tt.fileType)
					require.NoError(t, err)

					for _, f := range files {
						assert.NotEqual(t, tt.fileName, f.Name)
					}
				}
			}
		})
	}
}

// TestListFiles tests listing files
func TestListFiles(t *testing.T) {
	tests := []struct {
		name          string
		setup         func(t *testing.T, op crud.Operator)
		domain        string
		fileType      string
		expectedCount int
		checkFn       func(t *testing.T, files []crud.FileInfo)
	}{
		{
			name:          "empty list",
			setup:         func(t *testing.T, op crud.Operator) {},
			fileType:      "rules",
			expectedCount: 0,
		},
		{
			name: "list rules",
			setup: func(t *testing.T, op crud.Operator) {
				op.AddRule(context.Background(), &crud.AddFileRequest{Name: "rule1"})
				op.AddRule(context.Background(), &crud.AddFileRequest{Name: "rule2"})
				op.AddRule(context.Background(), &crud.AddFileRequest{Name: "rule3"})
			},
			fileType:      "rules",
			expectedCount: 3,
			checkFn: func(t *testing.T, files []crud.FileInfo) {
				names := make([]string, len(files))
				for i, f := range files {
					names[i] = f.Name
				}
				assert.Contains(t, names, "rule1")
				assert.Contains(t, names, "rule2")
				assert.Contains(t, names, "rule3")
			},
		},
		{
			name: "list context",
			setup: func(t *testing.T, op crud.Operator) {
				op.AddContext(context.Background(), &crud.AddFileRequest{Name: "ctx1"})
				op.AddContext(context.Background(), &crud.AddFileRequest{Name: "ctx2"})
			},
			fileType:      "context",
			expectedCount: 2,
		},
		{
			name: "list skills",
			setup: func(t *testing.T, op crud.Operator) {
				op.AddSkill(context.Background(), &crud.AddFileRequest{Name: "skill1"})
				op.AddSkill(context.Background(), &crud.AddFileRequest{Name: "skill2"})
			},
			fileType:      "skills",
			expectedCount: 2,
		},
		{
			name: "list files in domain",
			setup: func(t *testing.T, op crud.Operator) {
				op.AddDomain(context.Background(), &crud.AddDomainRequest{Name: "backend"})
				op.AddRule(context.Background(), &crud.AddFileRequest{
					Name:   "backend-rule",
					Domain: "backend",
				})
				op.AddRule(context.Background(), &crud.AddFileRequest{Name: "root-rule"})
			},
			domain:        "backend",
			fileType:      "rules",
			expectedCount: 1,
			checkFn: func(t *testing.T, files []crud.FileInfo) {
				assert.Equal(t, "backend", files[0].Domain)
				assert.Equal(t, "backend-rule", files[0].Name)
			},
		},
		{
			name:          "invalid file type",
			setup:         func(t *testing.T, op crud.Operator) {},
			fileType:      "invalid",
			expectedCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			baseDir := setupTestProject(t)
			op, err := crud.NewOperator(baseDir)
			require.NoError(t, err)

			tt.setup(t, op)

			files, err := op.ListFiles(context.Background(), tt.domain, tt.fileType)

			if tt.fileType == "invalid" {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Len(t, files, tt.expectedCount)

				if tt.checkFn != nil {
					tt.checkFn(t, files)
				}
			}
		})
	}
}
