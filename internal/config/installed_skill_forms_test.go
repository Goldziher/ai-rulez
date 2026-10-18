package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/samber/oops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
)

func TestValidateInstalledSkillFieldsRefusesUnsafeForms(t *testing.T) {
	tests := []struct {
		name    string
		skill   InstalledSkillConfig
		wantErr string
	}{
		{"source with a newline", InstalledSkillConfig{Name: "a", Source: "https://h/r\nfoo"}, "must not contain control characters"},
		{"source with a NUL", InstalledSkillConfig{Name: "a", Source: "https://h/r\x00"}, "must not contain control characters"},
		{"source with DEL", InstalledSkillConfig{Name: "a", Source: "https://h/r\x7f"}, "must not contain control characters"},
		{"uppercase plain http", InstalledSkillConfig{Name: "a", Source: "HTTP://h/r"}, "plain http://"},
		{"source option behind whitespace", InstalledSkillConfig{Name: "a", Source: "  --upload-pack=x"}, "must not start with '-'"},
		{"uppercase transport helper", InstalledSkillConfig{Name: "a", Source: "EXT::sh -c x"}, "transport helper"},
		{"empty github prefix", InstalledSkillConfig{Name: "a", Source: "github:"}, "must be github:owner/repo"},
		{"github prefix with extra segment", InstalledSkillConfig{Name: "a", Source: "github:owner/repo/extra"}, "must be github:owner/repo"},
		{"github prefix with trailing slash", InstalledSkillConfig{Name: "a", Source: "github:owner/repo/"}, "must be github:owner/repo"},
		{"tilde user", InstalledSkillConfig{Name: "a", Source: "~root"}, "~user"},
		{"tilde user with path", InstalledSkillConfig{Name: "a", Source: "~bob/skills"}, "~user"},
		{"path with a drive letter", InstalledSkillConfig{Name: "a", Source: "https://h/r", Path: `C:\x`}, "must be relative"},
		{"path with a drive letter and slash", InstalledSkillConfig{Name: "a", Source: "https://h/r", Path: "C:/x"}, "must be relative"},
		{"path with a UNC prefix", InstalledSkillConfig{Name: "a", Source: "https://h/r", Path: `\\srv\share`}, "must be relative"},
		{"path with a colon in the second position", InstalledSkillConfig{Name: "a", Source: "https://h/r", Path: "a:b"}, "must be relative"},
		{"path with backslash traversal", InstalledSkillConfig{Name: "a", Source: "https://h/r", Path: `skills\..\..\x`}, "must not contain '..'"},
		{"path with a control character", InstalledSkillConfig{Name: "a", Source: "https://h/r", Path: "skills/a\tb"}, "must not contain control characters"},
		{"resource with a drive letter", InstalledSkillConfig{Name: "a", Source: "https://h/r", Resources: []string{`C:\x`}}, "must be relative"},
		{"resource with a UNC prefix", InstalledSkillConfig{Name: "a", Source: "https://h/r", Resources: []string{`\\srv\share`}}, "must be relative"},
		{"resource with a control character", InstalledSkillConfig{Name: "a", Source: "https://h/r", Resources: []string{"docs/\x01"}}, "must not contain control characters"},
		{"resource with dotdot", InstalledSkillConfig{Name: "a", Source: "https://h/r", Resources: []string{"!../secrets"}}, "must not contain '..'"},
		{"empty resource", InstalledSkillConfig{Name: "a", Source: "https://h/r", Resources: []string{"!"}}, "is empty"},
		{"resource that is only slashes", InstalledSkillConfig{Name: "a", Source: "https://h/r", Resources: []string{"//"}}, "is empty"},
		{"unterminated resource class", InstalledSkillConfig{Name: "a", Source: "https://h/r", Resources: []string{"[bad"}}, "is not a valid glob"},
		{"backslash in a resource glob matches nothing", InstalledSkillConfig{Name: "a", Source: "https://h/r", Resources: []string{`docs\*.md`}}, "backslash"},
		{"backslash in an exclusion matches nothing", InstalledSkillConfig{Name: "a", Source: "https://h/r", Resources: []string{`!secrets\*`}}, "backslash"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			err := ValidateInstalledSkillFields(&tt.skill)
			// Assert
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Contains(t, err.Error(), `installed skill "a"`)
		})
	}
}

func TestValidateInstalledSkillFieldsAcceptsFormsResolvedLater(t *testing.T) {
	tests := []struct {
		name   string
		source string
	}{
		{"windows drive path is left for the resolver", `C:\skills`},
		{"UNC path is left for the resolver", `\\srv\share\skills`},
		{"tilde with a backslash", `~\skills`},
		{"owner repo extra is a relative directory", "owner/repo/extra"},
		{"trailing slash is a relative directory", "owner/repo/"},
		{"file url with localhost", "file://localhost/srv/skills"},
		{"uppercase file url", "FILE:///srv/skills"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			err := ValidateInstalledSkillFields(&InstalledSkillConfig{Name: "a", Source: tt.source})
			// Assert
			require.NoError(t, err)
		})
	}
}

func TestValidateInstalledSkillVersionChecksTheConstraintKeys(t *testing.T) {
	tests := []struct {
		name    string
		skill   InstalledSkillConfig
		wantErr string
	}{
		{"no version keys", InstalledSkillConfig{Name: "a", Source: "https://h/r"}, ""},
		{"a caret range", InstalledSkillConfig{Name: "a", Source: "https://h/r", Version: "^1.2"}, ""},
		{"version and ref exclude each other", InstalledSkillConfig{Name: "a", Source: "https://h/r", Version: "^1.2", Ref: "v1.0.0"}, "ref"},
		{"unparsable constraint", InstalledSkillConfig{Name: "a", Source: "https://h/r", Version: "not-a-range"}, "version"},
		{"unparsable release age", InstalledSkillConfig{Name: "a", Source: "https://h/r", Version: "^1", MinReleaseAge: "soon"}, "min_release_age"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			err := ValidateInstalledSkillVersion(&tt.skill)
			// Assert
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestNormalizeInstalledSkillSourcesLeavesAmbiguousFormsAlone(t *testing.T) {
	base := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(base, "owner", "repo"), 0o755))
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{"owner repo that is an existing directory stays local", "owner/repo", "owner/repo"},
		{"owner repo extra is not shorthand", "other/repo/extra", "other/repo/extra"},
		{"trailing slash is not shorthand", "other/repo/", "other/repo/"},
		{"case is preserved in the url", "Other/Repo", "https://github.com/Other/Repo"},
		{"github prefix is case sensitive", "GitHub:o/r", "GitHub:o/r"},
		{"windows drive path", `C:\skills`, `C:\skills`},
		{"windows drive path with slash", "C:/skills", "C:/skills"},
		{"UNC path", `\\srv\share`, `\\srv\share`},
		{"surrounding space is trimmed for github prefix", " github:o/r ", "https://github.com/o/r"},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			skills := []InstalledSkillConfig{{Name: "s", Source: tt.source}}
			// Act
			NormalizeInstalledSkillSources(base, skills)
			// Assert
			assert.Equal(t, tt.want, skills[0].Source)
		})
	}
}

func TestIsHomeSource(t *testing.T) {
	tests := []struct {
		source string
		want   bool
	}{
		{"~", true},
		{"~/skills", true},
		{`~\skills`, true},
		{"~root", false},
		{"~root/skills", false},
		{"./~", false},
		{"", false},
		{"/home/u/skills", false},
	}
	for _, tt := range tests {
		t.Run(tt.source, func(t *testing.T) {
			// Act and Assert
			assert.Equal(t, tt.want, IsHomeSource(tt.source))
		})
	}
}

func TestLocalSkillSourceForms(t *testing.T) {
	root := t.TempDir()
	bare := filepath.Join(root, "bare.git")
	headOnly := filepath.Join(root, "headonly")
	worktree := filepath.Join(root, "worktree")
	require.NoError(t, os.MkdirAll(filepath.Join(bare, "objects"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(bare, "HEAD"), []byte("ref: refs/heads/main\n"), 0o644))
	require.NoError(t, os.MkdirAll(headOnly, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(headOnly, "HEAD"), []byte("not a repo"), 0o644))
	require.NoError(t, os.MkdirAll(worktree, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(worktree, ".git"), []byte("gitdir: ../elsewhere\n"), 0o644))
	env := ambient.MapEnv{Home: filepath.Join(root, "home")}
	slash := filepath.ToSlash

	tests := []struct {
		name      string
		source    string
		env       ambient.Env
		wantDir   string
		wantLocal bool
		wantErr   string
	}{
		{"bare repository is a git source", "file://" + slash(bare), env, "file://" + slash(bare), false, ""},
		{"HEAD without objects is a plain directory", "file://" + slash(headOnly), env, headOnly, true, ""},
		{"worktree with a .git file is a git source", "file://" + slash(worktree), env, "file://" + slash(worktree), false, ""},
		{"file url of a missing directory is local", "file://" + slash(filepath.Join(root, "missing")), env, filepath.Join(root, "missing"), true, ""},
		{"uppercase scheme", "FILE://" + slash(headOnly), env, headOnly, true, ""},
		{"localhost authority", "file://localhost/" + strings.TrimPrefix(slash(headOnly), "/"), env, headOnly, true, ""},
		{"tilde backslash", `~\skills`, env, filepath.Join(env.Home, "skills"), true, ""},
		{"tilde without a home names the fix", "~", ambient.MapEnv{}, "", false, "no home directory"},
		{"tilde path without a home", "~/x", ambient.MapEnv{}, "", false, "no home directory"},
		{"windows drive path stays a plain source", `C:\skills`, env, `C:\skills`, false, ""},
		{"scp address stays a plain source", "git@h:o/r.git", env, "git@h:o/r.git", false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			dir, local, err := LocalSkillSource(tt.env, tt.source)
			// Assert
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				oe, ok := oops.AsOops(err)
				require.True(t, ok)
				assert.Contains(t, oe.Hint(), "Set HOME")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantDir, dir)
			assert.Equal(t, tt.wantLocal, local)
		})
	}
}
