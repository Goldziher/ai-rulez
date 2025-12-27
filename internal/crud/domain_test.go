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

// setupTestProject creates a minimal .ai-rulez directory structure for testing
func setupTestProject(t *testing.T) string {
	tmpDir := t.TempDir()
	aiRulezDir := filepath.Join(tmpDir, ".ai-rulez")

	// Create basic structure
	require.NoError(t, os.MkdirAll(filepath.Join(aiRulezDir, "rules"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(aiRulezDir, "context"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(aiRulezDir, "skills"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(aiRulezDir, "domains"), 0o755))

	// Create minimal config.yaml
	configPath := filepath.Join(aiRulezDir, "config.yaml")
	configContent := `version: "3.0"
name: "test-project"
includes: []
profiles: {}
`
	require.NoError(t, os.WriteFile(configPath, []byte(configContent), 0o644))

	return tmpDir
}

// TestAddDomain tests adding a new domain
func TestAddDomain(t *testing.T) {
	tests := []struct {
		name      string
		req       *crud.AddDomainRequest
		shouldErr bool
		checkFn   func(t *testing.T, baseDir string, result *crud.DomainResult)
	}{
		{
			name: "valid domain creation",
			req: &crud.AddDomainRequest{
				Name:        "backend",
				Description: "Backend services and APIs",
			},
			shouldErr: false,
			checkFn: func(t *testing.T, baseDir string, result *crud.DomainResult) {
				assert.Equal(t, "backend", result.Name)
				assert.True(t, result.Created)
				assert.NotEmpty(t, result.Path)

				// Verify directory structure
				assert.DirExists(t, filepath.Join(result.Path, "rules"))
				assert.DirExists(t, filepath.Join(result.Path, "context"))
				assert.DirExists(t, filepath.Join(result.Path, "skills"))

				// Verify description file
				descPath := filepath.Join(result.Path, ".description")
				content, err := os.ReadFile(descPath)
				require.NoError(t, err)
				assert.Equal(t, "Backend services and APIs", string(content))
			},
		},
		{
			name: "domain without description",
			req: &crud.AddDomainRequest{
				Name: "frontend",
			},
			shouldErr: false,
			checkFn: func(t *testing.T, baseDir string, result *crud.DomainResult) {
				assert.Equal(t, "frontend", result.Name)
				assert.Empty(t, result.Description)

				// Verify directory structure exists
				assert.DirExists(t, filepath.Join(result.Path, "rules"))
			},
		},
		{
			name:      "nil request",
			req:       nil,
			shouldErr: true,
		},
		{
			name: "invalid domain name - empty",
			req: &crud.AddDomainRequest{
				Name: "",
			},
			shouldErr: true,
		},
		{
			name: "invalid domain name - reserved",
			req: &crud.AddDomainRequest{
				Name: "rules",
			},
			shouldErr: true,
		},
		{
			name: "invalid domain name - special chars",
			req: &crud.AddDomainRequest{
				Name: "backend@api",
			},
			shouldErr: true,
		},
		// Duplicate domain test is covered in a separate test function
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			baseDir := setupTestProject(t)
			op, err := crud.NewOperator(baseDir)
			require.NoError(t, err)

			result, err := op.AddDomain(context.Background(), tt.req)

			if tt.shouldErr {
				assert.Error(t, err, "expected error but got none")
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
			}

			if tt.checkFn != nil {
				tt.checkFn(t, baseDir, result)
			}
		})
	}
}

// TestAddDomainDuplicate tests that duplicate domains cannot be created
func TestAddDomainDuplicate(t *testing.T) {
	baseDir := setupTestProject(t)
	op, err := crud.NewOperator(baseDir)
	require.NoError(t, err)

	// Add domain first time - should succeed
	result, err := op.AddDomain(context.Background(), &crud.AddDomainRequest{Name: "my-domain"})
	require.NoError(t, err)
	assert.NotNil(t, result)

	// Try to add same domain again - should fail
	result, err = op.AddDomain(context.Background(), &crud.AddDomainRequest{Name: "my-domain"})
	assert.Error(t, err)
	assert.Nil(t, result)
}

// TestRemoveDomain tests removing a domain
func TestRemoveDomain(t *testing.T) {
	tests := []struct {
		name          string
		setup         func(t *testing.T, baseDir string)
		domainName    string
		shouldErr     bool
		verifyRemoved bool
	}{
		{
			name: "remove existing domain",
			setup: func(t *testing.T, baseDir string) {
				op, err := crud.NewOperator(baseDir)
				require.NoError(t, err)
				_, err = op.AddDomain(context.Background(), &crud.AddDomainRequest{Name: "to-remove"})
				require.NoError(t, err)
			},
			domainName:    "to-remove",
			shouldErr:     false,
			verifyRemoved: true,
		},
		{
			name:          "remove non-existent domain",
			setup:         func(t *testing.T, baseDir string) {},
			domainName:    "non-existent",
			shouldErr:     true,
			verifyRemoved: false,
		},
		{
			name:       "remove with invalid name",
			setup:      func(t *testing.T, baseDir string) {},
			domainName: "",
			shouldErr:  true,
		},
		{
			name:       "remove reserved name",
			setup:      func(t *testing.T, baseDir string) {},
			domainName: "rules",
			shouldErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			baseDir := setupTestProject(t)
			tt.setup(t, baseDir)

			op, err := crud.NewOperator(baseDir)
			require.NoError(t, err)

			err = op.RemoveDomain(context.Background(), tt.domainName)

			if tt.shouldErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)

				if tt.verifyRemoved {
					// Verify directory was removed
					domainPath := filepath.Join(baseDir, ".ai-rulez", "domains", tt.domainName)
					assert.NoDirExists(t, domainPath)
				}
			}
		})
	}
}

// TestListDomains tests listing domains
func TestListDomains(t *testing.T) {
	tests := []struct {
		name          string
		setup         func(t *testing.T, baseDir string)
		expectedCount int
		checkFn       func(t *testing.T, domains []crud.DomainInfo)
	}{
		{
			name:          "empty domains list",
			setup:         func(t *testing.T, baseDir string) {},
			expectedCount: 0,
			checkFn:       func(t *testing.T, domains []crud.DomainInfo) {},
		},
		{
			name: "single domain",
			setup: func(t *testing.T, baseDir string) {
				op, err := crud.NewOperator(baseDir)
				require.NoError(t, err)
				_, err = op.AddDomain(context.Background(), &crud.AddDomainRequest{
					Name:        "backend",
					Description: "Backend domain",
				})
				require.NoError(t, err)
			},
			expectedCount: 1,
			checkFn: func(t *testing.T, domains []crud.DomainInfo) {
				assert.Equal(t, "backend", domains[0].Name)
				assert.Equal(t, "Backend domain", domains[0].Description)
				assert.NotEmpty(t, domains[0].Path)
			},
		},
		{
			name: "multiple domains sorted",
			setup: func(t *testing.T, baseDir string) {
				op, err := crud.NewOperator(baseDir)
				require.NoError(t, err)

				// Add in non-alphabetical order
				names := []string{"zebra", "alpha", "beta"}
				for _, name := range names {
					_, err = op.AddDomain(context.Background(), &crud.AddDomainRequest{
						Name: name,
					})
					require.NoError(t, err)
				}
			},
			expectedCount: 3,
			checkFn: func(t *testing.T, domains []crud.DomainInfo) {
				// Should be sorted alphabetically
				assert.Equal(t, "alpha", domains[0].Name)
				assert.Equal(t, "beta", domains[1].Name)
				assert.Equal(t, "zebra", domains[2].Name)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			baseDir := setupTestProject(t)
			tt.setup(t, baseDir)

			op, err := crud.NewOperator(baseDir)
			require.NoError(t, err)

			domains, err := op.ListDomains(context.Background())

			assert.NoError(t, err)
			assert.Len(t, domains, tt.expectedCount)

			if tt.checkFn != nil {
				tt.checkFn(t, domains)
			}
		})
	}
}

// TestAddDomainEdgeCases tests edge cases for domain operations
func TestAddDomainEdgeCases(t *testing.T) {
	tests := []struct {
		name       string
		domainName string
		shouldErr  bool
	}{
		{"max length valid", "backend_api_v2_long_domain_name_fifty_char_max_1", false},
		{"max length exceeded", "a_very_long_domain_name_that_exceeds_the_fifty_character_limit_by_a_lot", true},
		{"single char", "a", false},
		{"single number", "1", false},
		{"underscore and hyphen mix", "api_v1-beta", false},
		{"starting with number", "2api", false},
		{"ending with number", "api2", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			baseDir := setupTestProject(t)
			op, err := crud.NewOperator(baseDir)
			require.NoError(t, err)

			_, err = op.AddDomain(context.Background(), &crud.AddDomainRequest{
				Name: tt.domainName,
			})

			if tt.shouldErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
