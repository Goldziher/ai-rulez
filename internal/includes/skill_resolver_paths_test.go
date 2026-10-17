package includes

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/samber/oops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

// skillRepo is a git repository with one skill, built outside any project.
func skillRepo(t *testing.T, skill string) string {
	t.Helper()
	repo := initLocalRepo(t)
	commitFile(t, repo, "skills/"+skill+"/SKILL.md", "---\nname: "+skill+"\n---\n# "+skill+"\n")
	return repo
}

// fileURLEnv is an environment with its own home (so the fetch cache stays in
// the test) and, unlike the test process, no file:// opt-in unless allow is set.
func fileURLEnv(t *testing.T, allow string) ambient.Env {
	t.Helper()
	env := ambient.MapEnv{Home: t.TempDir(), Vars: map[string]string{}}
	if allow != "" {
		env.Vars[lockfile.EnvAllowFileURLs] = allow
	}
	return env
}

func TestResolveInstalledSkills_FileURLSources(t *testing.T) {
	tests := []struct {
		name string
		// setup returns the project directory and the file:// source
		setup   func(t *testing.T) (project, source string)
		allow   string
		want    []string
		wantErr string
	}{
		{
			name: "a git repository outside the project is refused",
			setup: func(t *testing.T) (string, string) {
				return t.TempDir(), "file://" + skillRepo(t, "alpha")
			},
			wantErr: "outside the project",
		},
		{
			name: "a git repository outside the project is allowed by the opt-in",
			setup: func(t *testing.T) (string, string) {
				return t.TempDir(), "file://" + skillRepo(t, "alpha")
			},
			allow: "1",
			want:  []string{"alpha"},
		},
		{
			name: "the opt-in must be exactly 1",
			setup: func(t *testing.T) (string, string) {
				return t.TempDir(), "file://" + skillRepo(t, "alpha")
			},
			allow:   "true",
			wantErr: "outside the project",
		},
		{
			name: "a git repository inside the project resolves",
			setup: func(t *testing.T) (string, string) {
				repo := skillRepo(t, "alpha")
				return filepath.Dir(repo), "file://" + repo
			},
			want: []string{"alpha"},
		},
		{
			name: "a plain directory outside the project is refused",
			setup: func(t *testing.T) (string, string) {
				dir := t.TempDir()
				writeSkill(t, filepath.Join(dir, "skills", "alpha"), "alpha")
				return t.TempDir(), "file://" + dir
			},
			wantErr: "outside the project",
		},
		{
			name: "a plain directory outside the project is allowed by the opt-in",
			setup: func(t *testing.T) (string, string) {
				dir := t.TempDir()
				writeSkill(t, filepath.Join(dir, "skills", "alpha"), "alpha")
				return t.TempDir(), "file://" + dir
			},
			allow: "1",
			want:  []string{"alpha"},
		},
		{
			name: "a plain directory inside the project resolves",
			setup: func(t *testing.T) (string, string) {
				project := t.TempDir()
				writeSkill(t, filepath.Join(project, "vendor", "skills", "alpha"), "alpha")
				return project, "file://" + filepath.Join(project, "vendor")
			},
			want: []string{"alpha"},
		},
		{
			name: "an uppercase scheme is held to the same rule",
			setup: func(t *testing.T) (string, string) {
				dir := t.TempDir()
				writeSkill(t, filepath.Join(dir, "skills", "alpha"), "alpha")
				return t.TempDir(), "FILE://" + dir
			},
			wantErr: "outside the project",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			testutil.IsolateHome(t)
			project, source := tt.setup(t)
			cfg := &config.Config{BaseDir: project, InstalledSkills: []config.InstalledSkillConfig{{Name: "alpha", Source: source}}}
			cfg.Host.Env = fileURLEnv(t, tt.allow)
			// Act
			skills, err := ResolveInstalledSkills(t.Context(), cfg, "")
			// Assert
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.ErrorIs(t, err, config.ErrIncludeOutsideProject)
				assert.ErrorIs(t, err, config.ErrSkillUnresolved)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, resolvedNames(skills))
		})
	}
}

func TestResolveInstalledSkills_LocalSourceMustStayInsideTheProject(t *testing.T) {
	tests := []struct {
		name    string
		build   func(t *testing.T, root string) config.InstalledSkillConfig
		wantErr string
	}{
		{
			name: "a dotdot source that leaves the project",
			build: func(t *testing.T, root string) config.InstalledSkillConfig {
				writeSkill(t, filepath.Join(root, "outside", "skills", "x"), "x")
				return config.InstalledSkillConfig{Name: "x", Source: "../outside"}
			},
			wantErr: "outside the project",
		},
		{
			name: "an absolute source outside the project",
			build: func(t *testing.T, root string) config.InstalledSkillConfig {
				writeSkill(t, filepath.Join(root, "outside", "skills", "x"), "x")
				return config.InstalledSkillConfig{Name: "x", Source: filepath.Join(root, "outside")}
			},
			wantErr: "outside the project",
		},
		{
			name: "a dotdot that comes back inside the project is fine",
			build: func(t *testing.T, root string) config.InstalledSkillConfig {
				writeSkill(t, filepath.Join(root, "project", "vendor", "skills", "x"), "x")
				return config.InstalledSkillConfig{Name: "x", Source: "vendor/../vendor"}
			},
		},
		{
			name: "a symlinked source directory that points outside",
			build: func(t *testing.T, root string) config.InstalledSkillConfig {
				writeSkill(t, filepath.Join(root, "outside", "skills", "x"), "x")
				testutil.SymlinkOrSkip(t, filepath.Join(root, "outside"), filepath.Join(root, "project", "vendor"))
				return config.InstalledSkillConfig{Name: "x", Source: "vendor"}
			},
			wantErr: "outside the project",
		},
		{
			name: "a symlinked parent component of the source that points outside",
			build: func(t *testing.T, root string) config.InstalledSkillConfig {
				writeSkill(t, filepath.Join(root, "outside", "deep", "skills", "x"), "x")
				testutil.SymlinkOrSkip(t, filepath.Join(root, "outside"), filepath.Join(root, "project", "link"))
				return config.InstalledSkillConfig{Name: "x", Source: "link/deep"}
			},
			wantErr: "outside the project",
		},
		{
			name: "a symlinked source that stays inside the project",
			build: func(t *testing.T, root string) config.InstalledSkillConfig {
				writeSkill(t, filepath.Join(root, "project", "real", "skills", "x"), "x")
				testutil.SymlinkOrSkip(t, filepath.Join(root, "project", "real"), filepath.Join(root, "project", "alias"))
				return config.InstalledSkillConfig{Name: "x", Source: "alias"}
			},
		},
		{
			name: "a tilde source in the committed config",
			build: func(t *testing.T, root string) config.InstalledSkillConfig {
				return config.InstalledSkillConfig{Name: "x", Source: "~/skills"}
			},
			wantErr: "outside the project",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := t.TempDir()
			project := filepath.Join(root, "project")
			require.NoError(t, os.MkdirAll(project, 0o755))
			entry := tt.build(t, root)
			home := filepath.Join(root, "home")
			require.NoError(t, os.MkdirAll(filepath.Join(home, "skills", "x"), 0o755))
			cfg := &config.Config{BaseDir: project, InstalledSkills: []config.InstalledSkillConfig{entry}}
			cfg.Host.Env = ambient.MapEnv{Home: home}
			// Act
			skills, err := ResolveInstalledSkills(t.Context(), cfg, "")
			// Assert
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.ErrorIs(t, err, config.ErrIncludeOutsideProject)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, []string{"x"}, resolvedNames(skills))
		})
	}
}

func TestResolveInstalledSkills_TildeHintNamesTheMachineLocalOverlay(t *testing.T) {
	// Arrange
	root := t.TempDir()
	project := filepath.Join(root, "project")
	require.NoError(t, os.MkdirAll(project, 0o755))
	cfg := &config.Config{BaseDir: project, InstalledSkills: []config.InstalledSkillConfig{{Name: "x", Source: "~/skills"}}}
	cfg.Host.Env = ambient.MapEnv{Home: filepath.Join(root, "home")}
	// Act
	_, err := ResolveInstalledSkills(t.Context(), cfg, "")
	// Assert
	require.Error(t, err)
	var oe oops.OopsError
	require.True(t, errors.As(err, &oe))
	assert.Contains(t, oe.Hint(), "config.local.toml")
}

func TestResolveInstalledSkills_TildeSourceWithoutAHomeDirectory(t *testing.T) {
	// Arrange
	cfg := &config.Config{BaseDir: t.TempDir(), UserScope: true, InstalledSkills: []config.InstalledSkillConfig{{Name: "x", Source: "~/skills"}}}
	cfg.Host.Env = ambient.MapEnv{}
	// Act
	_, err := ResolveInstalledSkills(t.Context(), cfg, "")
	// Assert
	require.Error(t, err)
	assert.ErrorIs(t, err, config.ErrSkillUnresolved)
	assert.Contains(t, err.Error(), "no home directory")
}

func TestResolveInstalledSkills_UserScopeMayLeaveTheProject(t *testing.T) {
	// Arrange
	root := t.TempDir()
	home := filepath.Join(root, "home")
	writeSkill(t, filepath.Join(home, "skills", "x"), "x")
	cfg := &config.Config{BaseDir: filepath.Join(root, "project"), UserScope: true, InstalledSkills: []config.InstalledSkillConfig{{Name: "x", Source: "~"}}}
	cfg.Host.Env = ambient.MapEnv{Home: home}
	// Act
	skills, err := ResolveInstalledSkills(t.Context(), cfg, "")
	// Assert
	require.NoError(t, err)
	assert.Equal(t, []string{"x"}, resolvedNames(skills))
}

func TestResolveInstalledSkills_LocalOverrideRules(t *testing.T) {
	tests := []struct {
		name     string
		mode     config.LockMode
		overlay  bool
		tracked  bool
		missing  bool
		want     []string
		wantErrs []error
	}{
		{name: "honored without lock enforcement", mode: config.LockAuto, want: []string{"x"}},
		{name: "refused when committed under --locked", mode: config.LockRequire, wantErrs: []error{config.ErrLockViolation}},
		{name: "refused when committed under --frozen", mode: config.LockFrozen, wantErrs: []error{config.ErrLockViolation}},
		{name: "honored when the machine-local overlay sets it", mode: config.LockRequire, overlay: true, want: []string{"x"}},
		{name: "a git-tracked overlay does not count", mode: config.LockRequire, overlay: true, tracked: true, wantErrs: []error{config.ErrLockViolation}},
		{name: "a missing override path skips the skill", mode: config.LockAuto, missing: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := t.TempDir()
			project := filepath.Join(root, "project")
			override := filepath.Join(root, "override")
			writeSkill(t, filepath.Join(override, "skills", "x"), "x")
			if tt.missing {
				override = filepath.Join(root, "nowhere")
			}
			cfg := &config.Config{
				BaseDir:         project,
				LockPolicy:      config.LockPolicy{Mode: tt.mode},
				InstalledSkills: []config.InstalledSkillConfig{{Name: "x", Source: "https://example.invalid/o/r.git", LocalOverride: override}},
			}
			if tt.overlay {
				cfg.LocalOverlay = &config.LocalOverlay{Tracked: tt.tracked, Doc: map[string]any{
					"installed_skills": []any{map[string]any{"name": "x", "local_override": override}},
				}}
			}
			// Act
			skills, err := ResolveInstalledSkills(t.Context(), cfg, "")
			// Assert
			if len(tt.wantErrs) > 0 {
				require.Error(t, err)
				for _, want := range tt.wantErrs {
					assert.ErrorIs(t, err, want)
				}
				return
			}
			require.NoError(t, err)
			assert.ElementsMatch(t, tt.want, resolvedNames(skills))
		})
	}
}
