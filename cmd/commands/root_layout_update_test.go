package commands

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/tagresolve/tagtest"
	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

// rootUpdateFixture is a remote with v1.0.0, v1.1.0 and v1.2.0 and a root-layout
// project (ai-rulez.toml alone) that includes it by version range.
type rootUpdateFixture struct {
	*ageFixture
	configPath string
}

func newRootUpdateFixture(t *testing.T, versionLine string) *rootUpdateFixture {
	t.Helper()
	testutil.IsolateHome(t)
	resetUpdateFlags(t)
	resetLockViewFlags(t)
	resetAgeFlags(t)
	f := &ageFixture{t: t, repo: tagtest.New(t)}
	f.repo.Date = longAgo
	f.release("one", "v1.0.0", false)
	f.release("two", "v1.1.0", true)
	f.repo.Date = ""
	f.release("three", "v1.2.0", false)
	f.root = rootLayoutProject(t, rootLayoutConfig+"\n[[includes]]\nname = \"shared\"\nsource = \""+tomlEscape(f.repo.URL)+"\"\n"+versionLine+"\n")
	return &rootUpdateFixture{ageFixture: f, configPath: filepath.Join(f.root, "ai-rulez.toml")}
}

func (f *rootUpdateFixture) rootLock() *lockfile.File {
	f.t.Helper()
	lock, err := lockfile.Load(f.root)
	require.NoError(f.t, err)
	return lock
}

func (f *rootUpdateFixture) rootConfig() string {
	f.t.Helper()
	return readFileString(f.t, f.configPath)
}

func TestRootLayoutLock_PinsARangeIncludeInTheProjectRoot(t *testing.T) {
	// Arrange
	f := newRootUpdateFixture(t, `version = "^1"`)

	// Act
	code := writeLockAt("", "", nil)

	// Assert
	require.Equal(t, 0, code)
	assert.FileExists(t, filepath.Join(f.root, "ai-rulez.lock"))
	assert.NoDirExists(t, filepath.Join(f.root, ".ai-rulez"))
	entry := f.rootLock().Find(lockfile.KindInclude, "shared")
	require.NotNil(t, entry)
	assert.Equal(t, "v1.2.0", entry.Tag)
	assert.Equal(t, 0, runCLI(t, "lock", "--check").code, "lock --check agrees with the pins")
}

func TestRootLayoutUpdate_MovesThePinAndLeavesTheConfigAlone(t *testing.T) {
	// Arrange: pinned at v1.2.0, then the remote gets v1.3.0
	f := newRootUpdateFixture(t, `version = "^1"`)
	require.Equal(t, 0, writeLockAt("", "", nil))
	f.release("four", "v1.3.0", false)
	before := f.rootConfig()

	// Act
	rep, code := runUpdateJSON(t)

	// Assert
	require.Equal(t, 0, code)
	require.Len(t, rep.Updates, 1)
	assert.Equal(t, "v1.3.0", rep.Updates[0].To.Tag)
	assert.Equal(t, "v1.3.0", f.rootLock().Find(lockfile.KindInclude, "shared").Tag)
	assert.Equal(t, before, f.rootConfig(), "update edits only the lock")
}

func TestRootLayoutUpdateMajor_WriteConfigPatchesAiRulezToml(t *testing.T) {
	// Arrange
	f := newRootUpdateFixture(t, `version = "^1"  # stay on 1.x`)
	f.release("five", "v2.0.0", false)
	require.Equal(t, 0, writeLockAt("", "", nil))
	before := f.rootConfig()
	updateMajor, updateWriteConfig = true, true

	// Act
	rep, code := runUpdateJSON(t)

	// Assert
	require.Equal(t, 0, code)
	require.Len(t, rep.Major, 1)
	assert.True(t, rep.Major[0].Written)
	assert.Equal(t, strings.Replace(before, `version = "^1"  # stay on 1.x`, `version = "^2.0"  # stay on 1.x`, 1), f.rootConfig())
	assert.Equal(t, "v2.0.0", f.rootLock().Find(lockfile.KindInclude, "shared").Tag)
	assert.NoFileExists(t, filepath.Join(f.root, "config.toml"))
}

func TestRootLayoutUpdateMajor_ARefusedScanRestoresAiRulezToml(t *testing.T) {
	// Arrange: v2.0.0 carries a secret-shaped string, which the scan refuses.
	f := newRootUpdateFixture(t, `version = "^1"  # keep`)
	f.repo.Write(".ai-rulez/rules/leak.md", "# Leak\naws_secret_access_key = \"wJalrXUtnFEMI/K7MDENG/bPxRfiCYQ9d3kGh7Lz\"\nAKIAQYLPMN5HHHFPZAM2\n")
	f.release("five", "v2.0.0", false)
	require.Equal(t, 0, writeLockAt("", "", nil))
	before, lockBefore := f.rootConfig(), f.rootLock()
	updateMajor, updateWriteConfig = true, true

	// Act
	rep, code := runUpdateJSON(t)

	// Assert
	assert.Equal(t, exitDrift, code)
	assert.Equal(t, before, f.rootConfig(), "ai-rulez.toml is restored byte for byte")
	assert.Equal(t, lockBefore, f.rootLock())
	require.Len(t, rep.Major, 1)
	assert.False(t, rep.Major[0].Written)
}

func TestPatchMajor_RewritesTheRootConfigAndReportsTheOriginal(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		items   []majorItem
		want    string
		wantErr string
	}{
		{
			"one include",
			"[[includes]]\nname = \"shared\"\nsource = \"https://example.com/a\"\nversion = \"^1\"  # keep\n",
			[]majorItem{{Kind: lockfile.KindInclude, Name: "shared", To: "^2.0"}},
			"[[includes]]\nname = \"shared\"\nsource = \"https://example.com/a\"\nversion = \"^2.0\"  # keep\n",
			"",
		},
		{
			"an installed skill and a skill source",
			"[[installed_skills]]\nname = \"s\"\nsource = \"https://example.com/s\"\nversion = \"^1\"\n\n[[skill_sources]]\nname = \"src\"\nurl = \"https://example.com/x\"\nversion = \"^3\"\n",
			[]majorItem{{Kind: lockfile.KindSkill, Name: "s", To: "^2.0"}, {Kind: lockfile.KindSource, Name: "src", To: "^4.0"}},
			"[[installed_skills]]\nname = \"s\"\nsource = \"https://example.com/s\"\nversion = \"^2.0\"\n\n[[skill_sources]]\nname = \"src\"\nurl = \"https://example.com/x\"\nversion = \"^4.0\"\n",
			"",
		},
		{
			"an inline array has no line to rewrite",
			"includes = [{ name = \"shared\", source = \"https://example.com/a\", version = \"^1\" }]\n",
			[]majorItem{{Kind: lockfile.KindInclude, Name: "shared", To: "^2.0"}},
			"includes = [{ name = \"shared\", source = \"https://example.com/a\", version = \"^1\" }]\n",
			"cannot update the constraint of include \"shared\"",
		},
		{
			"an unknown name",
			"[[includes]]\nname = \"shared\"\nsource = \"https://example.com/a\"\nversion = \"^1\"\n",
			[]majorItem{{Kind: lockfile.KindInclude, Name: "other", To: "^2.0"}},
			"[[includes]]\nname = \"shared\"\nsource = \"https://example.com/a\"\nversion = \"^1\"\n",
			"cannot update the constraint of include \"other\"",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := t.TempDir()
			path := filepath.Join(root, "ai-rulez.toml")
			body := rootLayoutConfig + "\n" + tt.body
			writeFile(t, path, body)
			require.NoError(t, os.Chmod(path, 0o640))
			wantMode := os.FileMode(0o640)
			if runtime.GOOS == "windows" { // Windows has no group bits: the file keeps whatever mode it reports
				info, statErr := os.Stat(path)
				require.NoError(t, statErr)
				wantMode = info.Mode().Perm()
			}
			cfg, err := config.LoadConfig(context.Background(), root, config.WithoutRemote(), config.WithoutLocal())
			require.NoError(t, err)

			// Act
			original, mode, err := patchMajor(cfg, tt.items)

			// Assert
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			} else {
				require.NoError(t, err)
				assert.Equal(t, body, string(original))
				assert.Equal(t, wantMode, mode, "the permission bits are kept")
				for i := range tt.items {
					assert.True(t, tt.items[i].Written)
				}
			}
			assert.Equal(t, rootLayoutConfig+"\n"+tt.want, readFileString(t, path))
		})
	}
}
