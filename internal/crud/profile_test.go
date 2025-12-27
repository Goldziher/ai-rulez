package crud_test

import (
	"context"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/crud"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAddProfile tests adding profiles
func TestAddProfile(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(t *testing.T, op crud.Operator)
		profName  string
		domains   []string
		shouldErr bool
		checkFn   func(t *testing.T, op crud.Operator)
	}{
		{
			name: "add profile with single domain",
			setup: func(t *testing.T, op crud.Operator) {
				op.AddDomain(context.Background(), &crud.AddDomainRequest{Name: "backend"})
			},
			profName:  "backend-only",
			domains:   []string{"backend"},
			shouldErr: false,
			checkFn: func(t *testing.T, op crud.Operator) {
				profiles, err := op.ListProfiles(context.Background())
				require.NoError(t, err)
				assert.Len(t, profiles, 1)
				assert.Equal(t, "backend-only", profiles[0].Name)
				assert.Equal(t, []string{"backend"}, profiles[0].Domains)
			},
		},
		{
			name: "add profile with multiple domains",
			setup: func(t *testing.T, op crud.Operator) {
				op.AddDomain(context.Background(), &crud.AddDomainRequest{Name: "backend"})
				op.AddDomain(context.Background(), &crud.AddDomainRequest{Name: "frontend"})
				op.AddDomain(context.Background(), &crud.AddDomainRequest{Name: "db"})
			},
			profName:  "full-stack",
			domains:   []string{"backend", "frontend", "db"},
			shouldErr: false,
			checkFn: func(t *testing.T, op crud.Operator) {
				profiles, err := op.ListProfiles(context.Background())
				require.NoError(t, err)
				assert.Len(t, profiles, 1)
				assert.Equal(t, 3, len(profiles[0].Domains))
			},
		},
		{
			name:      "empty profile name",
			setup:     func(t *testing.T, op crud.Operator) {},
			profName:  "",
			domains:   []string{"backend"},
			shouldErr: true,
		},
		{
			name:      "no domains provided",
			setup:     func(t *testing.T, op crud.Operator) {},
			profName:  "empty-profile",
			domains:   []string{},
			shouldErr: true,
		},
		{
			name: "reserved profile name - default",
			setup: func(t *testing.T, op crud.Operator) {
				op.AddDomain(context.Background(), &crud.AddDomainRequest{Name: "backend"})
			},
			profName:  "default",
			domains:   []string{"backend"},
			shouldErr: true,
		},
		{
			name: "reserved profile name - root",
			setup: func(t *testing.T, op crud.Operator) {
				op.AddDomain(context.Background(), &crud.AddDomainRequest{Name: "backend"})
			},
			profName:  "root",
			domains:   []string{"backend"},
			shouldErr: true,
		},
		{
			name: "reserved profile name - all",
			setup: func(t *testing.T, op crud.Operator) {
				op.AddDomain(context.Background(), &crud.AddDomainRequest{Name: "backend"})
			},
			profName:  "all",
			domains:   []string{"backend"},
			shouldErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			baseDir := setupTestProject(t)
			op, err := crud.NewOperator(baseDir)
			require.NoError(t, err)

			tt.setup(t, op)

			err = op.AddProfile(context.Background(), tt.profName, tt.domains)

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

// TestRemoveProfile tests removing profiles
func TestRemoveProfile(t *testing.T) {
	tests := []struct {
		name          string
		setup         func(t *testing.T, op crud.Operator)
		profileName   string
		shouldErr     bool
		verifyRemoved bool
	}{
		{
			name: "remove existing profile",
			setup: func(t *testing.T, op crud.Operator) {
				op.AddDomain(context.Background(), &crud.AddDomainRequest{Name: "backend"})
				op.AddProfile(context.Background(), "to-remove", []string{"backend"})
			},
			profileName:   "to-remove",
			shouldErr:     false,
			verifyRemoved: true,
		},
		{
			name:        "remove non-existent profile",
			setup:       func(t *testing.T, op crud.Operator) {},
			profileName: "nonexistent",
			shouldErr:   true,
		},
		{
			name:        "empty profile name",
			setup:       func(t *testing.T, op crud.Operator) {},
			profileName: "",
			shouldErr:   true,
		},
		{
			name: "cannot remove default profile",
			setup: func(t *testing.T, op crud.Operator) {
				op.AddDomain(context.Background(), &crud.AddDomainRequest{Name: "backend"})
				op.AddProfile(context.Background(), "default-prof", []string{"backend"})
				op.SetDefaultProfile(context.Background(), "default-prof")
			},
			profileName: "default-prof",
			shouldErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			baseDir := setupTestProject(t)
			op, err := crud.NewOperator(baseDir)
			require.NoError(t, err)

			tt.setup(t, op)

			err = op.RemoveProfile(context.Background(), tt.profileName)

			if tt.shouldErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)

				if tt.verifyRemoved {
					profiles, err := op.ListProfiles(context.Background())
					require.NoError(t, err)

					for _, p := range profiles {
						assert.NotEqual(t, tt.profileName, p.Name)
					}
				}
			}
		})
	}
}

// TestSetDefaultProfile tests setting a default profile
func TestSetDefaultProfile(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(t *testing.T, op crud.Operator)
		profName  string
		shouldErr bool
		checkFn   func(t *testing.T, op crud.Operator)
	}{
		{
			name: "set default profile",
			setup: func(t *testing.T, op crud.Operator) {
				op.AddDomain(context.Background(), &crud.AddDomainRequest{Name: "backend"})
				op.AddProfile(context.Background(), "my-profile", []string{"backend"})
			},
			profName:  "my-profile",
			shouldErr: false,
			checkFn: func(t *testing.T, op crud.Operator) {
				profiles, err := op.ListProfiles(context.Background())
				require.NoError(t, err)

				for _, p := range profiles {
					if p.Name == "my-profile" {
						assert.True(t, p.IsDefault)
					}
				}
			},
		},
		{
			name:      "set non-existent profile as default",
			setup:     func(t *testing.T, op crud.Operator) {},
			profName:  "nonexistent",
			shouldErr: true,
		},
		{
			name:      "empty profile name",
			setup:     func(t *testing.T, op crud.Operator) {},
			profName:  "",
			shouldErr: true,
		},
		{
			name: "switch default profile",
			setup: func(t *testing.T, op crud.Operator) {
				op.AddDomain(context.Background(), &crud.AddDomainRequest{Name: "backend"})
				op.AddDomain(context.Background(), &crud.AddDomainRequest{Name: "frontend"})
				op.AddProfile(context.Background(), "prof1", []string{"backend"})
				op.AddProfile(context.Background(), "prof2", []string{"frontend"})
				op.SetDefaultProfile(context.Background(), "prof1")
			},
			profName:  "prof2",
			shouldErr: false,
			checkFn: func(t *testing.T, op crud.Operator) {
				profiles, err := op.ListProfiles(context.Background())
				require.NoError(t, err)

				prof1Found := false
				prof2Found := false

				for _, p := range profiles {
					if p.Name == "prof1" {
						prof1Found = true
						assert.False(t, p.IsDefault, "prof1 should no longer be default")
					}
					if p.Name == "prof2" {
						prof2Found = true
						assert.True(t, p.IsDefault, "prof2 should be default")
					}
				}

				assert.True(t, prof1Found && prof2Found)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			baseDir := setupTestProject(t)
			op, err := crud.NewOperator(baseDir)
			require.NoError(t, err)

			tt.setup(t, op)

			err = op.SetDefaultProfile(context.Background(), tt.profName)

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

// TestListProfiles tests listing profiles
func TestListProfiles(t *testing.T) {
	tests := []struct {
		name          string
		setup         func(t *testing.T, op crud.Operator)
		expectedCount int
		checkFn       func(t *testing.T, profiles []crud.ProfileInfo)
	}{
		{
			name:          "empty profiles list",
			setup:         func(t *testing.T, op crud.Operator) {},
			expectedCount: 0,
		},
		{
			name: "single profile",
			setup: func(t *testing.T, op crud.Operator) {
				op.AddDomain(context.Background(), &crud.AddDomainRequest{Name: "backend"})
				op.AddProfile(context.Background(), "backend-only", []string{"backend"})
			},
			expectedCount: 1,
			checkFn: func(t *testing.T, profiles []crud.ProfileInfo) {
				assert.Equal(t, "backend-only", profiles[0].Name)
				assert.Equal(t, []string{"backend"}, profiles[0].Domains)
			},
		},
		{
			name: "multiple profiles",
			setup: func(t *testing.T, op crud.Operator) {
				op.AddDomain(context.Background(), &crud.AddDomainRequest{Name: "backend"})
				op.AddDomain(context.Background(), &crud.AddDomainRequest{Name: "frontend"})

				op.AddProfile(context.Background(), "prof1", []string{"backend"})
				op.AddProfile(context.Background(), "prof2", []string{"frontend"})
				op.AddProfile(context.Background(), "prof3", []string{"backend", "frontend"})
			},
			expectedCount: 3,
			checkFn: func(t *testing.T, profiles []crud.ProfileInfo) {
				names := make([]string, len(profiles))
				for i, p := range profiles {
					names[i] = p.Name
				}
				assert.Contains(t, names, "prof1")
				assert.Contains(t, names, "prof2")
				assert.Contains(t, names, "prof3")
			},
		},
		{
			name: "with default profile",
			setup: func(t *testing.T, op crud.Operator) {
				op.AddDomain(context.Background(), &crud.AddDomainRequest{Name: "backend"})
				op.AddProfile(context.Background(), "main", []string{"backend"})
				op.AddProfile(context.Background(), "secondary", []string{"backend"})
				op.SetDefaultProfile(context.Background(), "main")
			},
			expectedCount: 2,
			checkFn: func(t *testing.T, profiles []crud.ProfileInfo) {
				// Find the main profile and verify it's marked as default
				mainFound := false
				for _, p := range profiles {
					switch p.Name {
					case "main":
						mainFound = true
						assert.True(t, p.IsDefault)
					case "secondary":
						assert.False(t, p.IsDefault)
					}
				}
				assert.True(t, mainFound)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			baseDir := setupTestProject(t)
			op, err := crud.NewOperator(baseDir)
			require.NoError(t, err)

			tt.setup(t, op)

			profiles, err := op.ListProfiles(context.Background())

			assert.NoError(t, err)
			assert.Len(t, profiles, tt.expectedCount)

			if tt.checkFn != nil {
				tt.checkFn(t, profiles)
			}
		})
	}
}
