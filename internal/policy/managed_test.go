package policy

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
)

func TestManagedPathsPerOS(t *testing.T) {
	programDataEnv := func(v string) ambient.Env {
		return ambient.MapEnv{Vars: map[string]string{"ProgramData": v}, Home: "/h"}
	}
	tests := []struct {
		name string
		goos string
		env  ambient.Env
		want string
	}{
		{"macOS", "darwin", nil, "/Library/Application Support/ai-rulez/policy.toml"},
		{"Linux", "linux", nil, "/etc/ai-rulez/policy.toml"},
		{"another Unix", "freebsd", nil, "/etc/ai-rulez/policy.toml"},
		{"Windows without ProgramData", "windows", programDataEnv(""), `C:\ProgramData\ai-rulez\policy.toml`},
		{"Windows with ProgramData", "windows", programDataEnv(`D:\Data`), `D:\Data\ai-rulez\policy.toml`},
		{"Windows ProgramData with a trailing separator", "windows", programDataEnv(`D:\Data\`), `D:\Data\ai-rulez\policy.toml`},
		{"Windows ProgramData with forward slashes", "windows", programDataEnv(`D:/Data`), `D:/Data\ai-rulez\policy.toml`},
		{"Windows ProgramData on a UNC share", "windows", programDataEnv(`\\server\share`), `\\server\share\ai-rulez\policy.toml`},
		{"Windows relative ProgramData falls back to the default", "windows", programDataEnv(`data`), `C:\ProgramData\ai-rulez\policy.toml`},
		{"Windows drive-relative ProgramData falls back too", "windows", programDataEnv(`D:data`), `C:\ProgramData\ai-rulez\policy.toml`},
		{"Windows dot-relative ProgramData falls back too", "windows", programDataEnv(`.\..\x`), `C:\ProgramData\ai-rulez\policy.toml`},
		{"ProgramData is ignored off Windows", "linux", programDataEnv(`D:\Data`), "/etc/ai-rulez/policy.toml"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			got := managedPaths(DiscoverOptions{GOOS: tt.goos, Env: tt.env})
			// Assert
			assert.Equal(t, []string{tt.want}, got)
		})
	}
}

func TestManagedPathsAreAbsoluteOnEveryOS(t *testing.T) {
	// A managed anchor must never resolve against the working directory, which is
	// the repository being evaluated.
	for _, goos := range []string{"darwin", "linux", "windows", "freebsd", "plan9"} {
		for _, p := range managedPaths(DiscoverOptions{GOOS: goos, Env: ambient.MapEnv{Home: "/h"}}) {
			absolute := p[0] == '/' || (len(p) > 2 && p[1] == ':' && p[2] == '\\')
			assert.True(t, absolute, "%s: %q", goos, p)
		}
	}
}

func TestExplicitManagedPathsReplaceThePlatformOnes(t *testing.T) {
	// Act
	got := managedPaths(DiscoverOptions{GOOS: "linux", ManagedPaths: []string{"/x/policy.toml"}})
	// Assert
	assert.Equal(t, []string{"/x/policy.toml"}, got)
}

func TestLinkTimeManagedRootMovesTheHostPathOnly(t *testing.T) {
	// Arrange: what `go build -ldflags -X ...managedRoot=<dir>` does to a test binary.
	root := t.TempDir()
	old := managedRoot
	managedRoot = root
	t.Cleanup(func() { managedRoot = old })
	// Act
	host := managedPaths(DiscoverOptions{})
	named := managedPaths(DiscoverOptions{GOOS: "linux"})
	// Assert
	assert.Equal(t, []string{filepath.Join(root, "ai-rulez", "policy.toml")}, host)
	assert.Equal(t, []string{"/etc/ai-rulez/policy.toml"}, named, "naming a platform still names that platform's path")
}

func TestManagedLayerIsLoadedFromThePlatformLayout(t *testing.T) {
	// Arrange: the managed policy sits where the host layout puts it.
	root := t.TempDir()
	old := managedRoot
	managedRoot = root
	t.Cleanup(func() { managedRoot = old })
	require.NoError(t, os.MkdirAll(filepath.Join(root, "ai-rulez"), 0o755))
	writePolicy(t, filepath.Join(root, "ai-rulez"), "policy.toml", "policy_version = 1\nname = \"managed baseline\"\n[lock]\nenforce = true\n")
	// Act
	layers, err := Discover(DiscoverOptions{Env: ambient.MapEnv{Vars: map[string]string{}, Home: t.TempDir()}})
	// Assert
	require.NoError(t, err)
	require.Len(t, layers, 1)
	assert.Equal(t, OriginManaged, layers[0].Origin)
	assert.Equal(t, "managed baseline", layers[0].Name)
	assert.Equal(t, filepath.Join(root, "ai-rulez", "policy.toml"), layers[0].Path)
}
