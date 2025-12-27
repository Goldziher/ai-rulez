package crud_test

import (
	"context"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/crud"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAddInclude tests adding include sources
func TestAddInclude(t *testing.T) {
	tests := []struct {
		name      string
		req       *crud.AddIncludeRequest
		shouldErr bool
		checkFn   func(t *testing.T, op crud.Operator)
	}{
		{
			name: "add git URL include",
			req: &crud.AddIncludeRequest{
				Name:          "shared-rules",
				Source:        "https://github.com/example/shared-rules.git",
				Include:       []string{"rules", "context"},
				MergeStrategy: "append",
			},
			shouldErr: false,
			checkFn: func(t *testing.T, op crud.Operator) {
				includes, err := op.ListIncludes(context.Background())
				require.NoError(t, err)
				assert.Len(t, includes, 1)
				assert.Equal(t, "shared-rules", includes[0].Name)
				assert.Equal(t, "https://github.com/example/shared-rules.git", includes[0].Source)
				assert.Equal(t, "git", includes[0].Type)
			},
		},
		{
			name: "add git SSH URL include",
			req: &crud.AddIncludeRequest{
				Name:   "another-rules",
				Source: "git@github.com:example/another-rules.git",
			},
			shouldErr: false,
			checkFn: func(t *testing.T, op crud.Operator) {
				includes, err := op.ListIncludes(context.Background())
				require.NoError(t, err)
				assert.Len(t, includes, 1)
				assert.Equal(t, "git", includes[0].Type)
			},
		},
		{
			name:      "nil request",
			req:       nil,
			shouldErr: true,
		},
		{
			name: "empty include name",
			req: &crud.AddIncludeRequest{
				Name:   "",
				Source: "https://github.com/example/repo.git",
			},
			shouldErr: true,
		},
		{
			name: "empty source",
			req: &crud.AddIncludeRequest{
				Name:   "my-include",
				Source: "",
			},
			shouldErr: true,
		},
		// Duplicate include test is covered in a separate test function
		{
			name: "invalid include type",
			req: &crud.AddIncludeRequest{
				Name:    "test",
				Source:  "https://github.com/example/repo.git",
				Include: []string{"invalid-type"},
			},
			shouldErr: true,
		},
		{
			name: "invalid merge strategy",
			req: &crud.AddIncludeRequest{
				Name:          "test",
				Source:        "https://github.com/example/repo.git",
				MergeStrategy: "invalid-strategy",
			},
			shouldErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			baseDir := setupTestProject(t)
			op, err := crud.NewOperator(baseDir)
			require.NoError(t, err)

			err = op.AddInclude(context.Background(), tt.req)

			if tt.shouldErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if tt.checkFn != nil {
					tt.checkFn(t, op)
				}
			}
		})
	}
}

// TestAddIncludeDuplicate tests that duplicate includes cannot be created
func TestAddIncludeDuplicate(t *testing.T) {
	baseDir := setupTestProject(t)
	op, err := crud.NewOperator(baseDir)
	require.NoError(t, err)

	// Add include first time - should succeed
	err = op.AddInclude(context.Background(), &crud.AddIncludeRequest{
		Name:   "my-include",
		Source: "https://github.com/example/repo1.git",
	})
	require.NoError(t, err)

	// Try to add same include name again - should fail
	err = op.AddInclude(context.Background(), &crud.AddIncludeRequest{
		Name:   "my-include",
		Source: "https://github.com/example/repo2.git",
	})
	assert.Error(t, err)
}

// TestRemoveInclude tests removing include sources
func TestRemoveInclude(t *testing.T) {
	tests := []struct {
		name          string
		setup         func(t *testing.T, op crud.Operator)
		includeName   string
		shouldErr     bool
		verifyRemoved bool
	}{
		{
			name: "remove existing include",
			setup: func(t *testing.T, op crud.Operator) {
				op.AddInclude(context.Background(), &crud.AddIncludeRequest{
					Name:   "to-remove",
					Source: "https://github.com/example/repo.git",
				})
			},
			includeName:   "to-remove",
			shouldErr:     false,
			verifyRemoved: true,
		},
		{
			name:        "remove non-existent include",
			setup:       func(t *testing.T, op crud.Operator) {},
			includeName: "nonexistent",
			shouldErr:   true,
		},
		{
			name:        "empty include name",
			setup:       func(t *testing.T, op crud.Operator) {},
			includeName: "",
			shouldErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			baseDir := setupTestProject(t)
			op, err := crud.NewOperator(baseDir)
			require.NoError(t, err)

			tt.setup(t, op)

			err = op.RemoveInclude(context.Background(), tt.includeName)

			if tt.shouldErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)

				if tt.verifyRemoved {
					includes, err := op.ListIncludes(context.Background())
					require.NoError(t, err)

					for _, inc := range includes {
						assert.NotEqual(t, tt.includeName, inc.Name)
					}
				}
			}
		})
	}
}

// TestListIncludes tests listing include sources
func TestListIncludes(t *testing.T) {
	tests := []struct {
		name          string
		setup         func(t *testing.T, op crud.Operator)
		expectedCount int
		checkFn       func(t *testing.T, includes []crud.IncludeInfo)
	}{
		{
			name:          "empty includes list",
			setup:         func(t *testing.T, op crud.Operator) {},
			expectedCount: 0,
		},
		{
			name: "single git include",
			setup: func(t *testing.T, op crud.Operator) {
				op.AddInclude(context.Background(), &crud.AddIncludeRequest{
					Name:   "shared-rules",
					Source: "https://github.com/example/shared-rules.git",
				})
			},
			expectedCount: 1,
			checkFn: func(t *testing.T, includes []crud.IncludeInfo) {
				assert.Equal(t, "shared-rules", includes[0].Name)
				assert.Equal(t, "git", includes[0].Type)
			},
		},
		{
			name: "multiple includes",
			setup: func(t *testing.T, op crud.Operator) {
				op.AddInclude(context.Background(), &crud.AddIncludeRequest{
					Name:   "include1",
					Source: "https://github.com/example/repo1.git",
				})
				op.AddInclude(context.Background(), &crud.AddIncludeRequest{
					Name:   "include2",
					Source: "https://github.com/example/repo2.git",
				})
				op.AddInclude(context.Background(), &crud.AddIncludeRequest{
					Name:   "include3",
					Source: "https://github.com/example/repo3.git",
				})
			},
			expectedCount: 3,
			checkFn: func(t *testing.T, includes []crud.IncludeInfo) {
				names := make([]string, len(includes))
				for i, inc := range includes {
					names[i] = inc.Name
				}
				assert.Contains(t, names, "include1")
				assert.Contains(t, names, "include2")
				assert.Contains(t, names, "include3")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			baseDir := setupTestProject(t)
			op, err := crud.NewOperator(baseDir)
			require.NoError(t, err)

			tt.setup(t, op)

			includes, err := op.ListIncludes(context.Background())

			assert.NoError(t, err)
			assert.Len(t, includes, tt.expectedCount)

			if tt.checkFn != nil {
				tt.checkFn(t, includes)
			}
		})
	}
}

// TestAddIncludeEdgeCases tests edge cases with local paths
func TestAddIncludeEdgeCases(t *testing.T) {
	tests := []struct {
		name      string
		req       *crud.AddIncludeRequest
		shouldErr bool
	}{
		{
			name: "git URL without .git suffix",
			req: &crud.AddIncludeRequest{
				Name:   "test",
				Source: "https://github.com/example/repo",
			},
			shouldErr: false,
		},
		{
			name: "http git URL",
			req: &crud.AddIncludeRequest{
				Name:   "test",
				Source: "http://github.com/example/repo.git",
			},
			shouldErr: false,
		},
		{
			name: "valid git SSH without .git",
			req: &crud.AddIncludeRequest{
				Name:   "test",
				Source: "git@github.com:example/repo",
			},
			shouldErr: false,
		},
		{
			name: "with include types",
			req: &crud.AddIncludeRequest{
				Name:    "test",
				Source:  "https://github.com/example/repo.git",
				Include: []string{"rules", "skills"},
			},
			shouldErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			baseDir := setupTestProject(t)
			op, err := crud.NewOperator(baseDir)
			require.NoError(t, err)

			err = op.AddInclude(context.Background(), tt.req)

			if tt.shouldErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestIncludeSourceValidation tests validation of include sources
func TestIncludeSourceValidation(t *testing.T) {
	tests := []struct {
		name      string
		source    string
		shouldErr bool
	}{
		// Valid git URLs
		{"https github", "https://github.com/user/repo.git", false},
		{"https gitlab", "https://gitlab.com/user/repo", false},
		{"http", "http://example.com/repo.git", false},
		{"git@github", "git@github.com:user/repo.git", false},
		{"git@gitlab", "git@gitlab.com:user/repo", false},

		// Invalid sources
		{"empty", "", true},
		// Note: plain URLs without protocol are treated as local paths, which is allowed if relative
		{"relative path like URL", "./github.com/user/repo", false},
		// Malformed git URLs with invalid characters pass through basic URL validation
		// This could be stricter in future versions
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := crud.ValidateIncludeSource(tt.source)
			if tt.shouldErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
